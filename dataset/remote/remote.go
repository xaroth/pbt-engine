// Package remote keeps local copies of published datasets. A source is addressed by content:
//
//	index.json            {"<game>": {"<league>": {"version": "...", "dataset": "<id>", "art": "<sha256>", "format": 1}}}
//	manifests/<id>.json   a dataset's manifest, by its dataset id
//	blobs/<sha256>        every file a manifest lists, by its SHA-256
//
// Pull keeps the latest dataset of each league in dataset.FormatVersion; PullID fetches one dataset by its id. Each
// fetches only what the local copy lacks and checks every byte: a manifest against the id it
// was fetched by, a file against its SHA-256.
//
// A host that reads the source without a local copy finds a league's latest dataset with Latest
// and reads it through FS, which fetches each file when it is read and checks it the same way.
package remote

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/xaroth/pbt-engine/dataset"
)

// IndexName is the index's name at the source.
const IndexName = "index.json"

// DefaultSource is pbt-data's bucket.
const DefaultSource = "https://pbt-data.xaroth.nl/"

// Problems Pull and PullID report.
var (
	ErrUnknownLeague = errors.New("league not in the index")
	ErrMismatch      = errors.New("content does not match its hash")
	ErrStatus        = errors.New("unexpected HTTP status")
)

// Permissions of what Pull writes: published data, readable by all.
const (
	dirPerm  = 0o755
	filePerm = 0o644
)

// Fetcher reads a file from a source by its slash-separated path, such as blobs/<sha256>.
type Fetcher interface {
	Fetch(ctx context.Context, name string) ([]byte, error)
}

// HTTP fetches from Base, a URL ending in a slash, with Client, or without one a client that
// gives up on a request after fetchTimeout.
type HTTP struct {
	Base   string
	Client *http.Client
}

// fetchTimeout bounds one file's request: the largest is a few MB.
const fetchTimeout = 2 * time.Minute

// Fetch gets Base+name and fails on any status but 200.
func (h HTTP) Fetch(ctx context.Context, name string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.Base+name, nil)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", name, err)
	}
	c := h.Client
	if c == nil {
		c = &http.Client{Timeout: fetchTimeout}
	}
	resp, err := c.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", name, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch %s: %w %s", h.Base+name, ErrStatus, resp.Status)
	}
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", name, err)
	}
	return b, nil
}

// Result is what Pull or PullID did for one dataset.
type Result struct {
	// League is empty for PullID.
	League, Version string
	// Updated is false when the local copy was current.
	Updated bool
	// Fetched counts the files downloaded, the manifest included; the others were reused.
	Fetched int
}

// Pull brings the latest datasets of game's leagues in dir up to date with the source, dir
// holding one directory per league. Without leagues it pulls every league of game in the index.
// Each league is pulled as PullID pulls the dataset the index names, which must be of game and
// the version the index gives. A league whose entry is in another format than
// dataset.FormatVersion fails with dataset.ErrFormat before anything is fetched for it. A league that fails keeps its old copy and does not stop the
// others: Pull returns the results of the leagues pulled together with the joined errors of
// those that failed.
func Pull(ctx context.Context, f Fetcher, dir, game string, leagues ...string) ([]Result, error) {
	idx, err := readIndex(ctx, f)
	if err != nil {
		return nil, err
	}
	if len(leagues) == 0 {
		leagues = slices.Sorted(maps.Keys(idx[game]))
	}
	out := make([]Result, 0, len(leagues))
	var errs []error
	for _, league := range leagues {
		e, err := idx.latest(game, league)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		r, err := pull(ctx, f, filepath.Join(dir, league), e.Dataset, func(m dataset.Manifest) error {
			if m.Game != game || m.Version != e.Version {
				return fmt.Errorf("manifest is %s %s, index says %s %s: %w", m.Game, m.Version, game, e.Version, ErrMismatch)
			}
			return nil
		})
		r.League = league
		if err != nil {
			errs = append(errs, fmt.Errorf("pull %s %s: %w", game, league, err))
			continue
		}
		out = append(out, r)
	}
	return out, errors.Join(errs...)
}

// PullID makes dir a copy of the dataset with id. A dir that already holds it is left alone;
// otherwise the manifest is fetched and checked against id, each file it lists is reused from
// dir when its SHA-256 matches or fetched and checked otherwise, and the complete dataset
// replaces dir. On failure dir keeps what it held.
func PullID(ctx context.Context, f Fetcher, dir, id string) (Result, error) {
	r, err := pull(ctx, f, dir, id, nil)
	if err != nil {
		return r, fmt.Errorf("pull %s: %w", id, err)
	}
	return r, nil
}

// pull is PullID, with check, when not nil, run on the fetched manifest.
func pull(ctx context.Context, f Fetcher, local, id string, check func(dataset.Manifest) error) (Result, error) {
	var r Result
	if !validHash(id) {
		return r, fmt.Errorf("dataset id %q: %w", id, dataset.ErrInvalid)
	}
	parent, name := filepath.Dir(local), filepath.Base(local)
	if err := os.MkdirAll(parent, dirPerm); err != nil {
		return r, fmt.Errorf("pull: %w", err)
	}
	if err := removeStages(parent, name); err != nil {
		return r, err
	}
	if m, err := dataset.ReadManifest(os.DirFS(local)); err == nil && m.ID == id {
		r.Version = m.Version
		return r, nil
	}
	stage, err := os.MkdirTemp(parent, stagePrefix(name))
	if err != nil {
		return r, fmt.Errorf("stage: %w", err)
	}
	defer func() { _ = os.RemoveAll(stage) }()
	r, err = stageDataset(ctx, f, local, stage, id, check)
	if err != nil {
		return r, err
	}
	if err := replace(local, stage); err != nil {
		return r, err
	}
	r.Updated = true
	return r, nil
}

// stageDataset puts the complete dataset with id into stage, reusing files from local.
func stageDataset(ctx context.Context, f Fetcher, local, stage, id string, check func(dataset.Manifest) error) (Result, error) {
	var r Result
	m, err := stageManifest(ctx, f, stage, id)
	if err != nil {
		return r, err
	}
	if check != nil {
		if err := check(m); err != nil {
			return r, err
		}
	}
	r.Version = m.Version
	r.Fetched = 1
	for _, file := range m.Files {
		fetched, err := stageFile(ctx, f, local, stage, file)
		if err != nil {
			return r, err
		}
		if fetched {
			r.Fetched++
		}
	}
	return r, nil
}

// stageManifest fetches the manifest of the dataset with id into stage and checks it.
func stageManifest(ctx context.Context, f Fetcher, stage, id string) (dataset.Manifest, error) {
	mb, err := f.Fetch(ctx, "manifests/"+id+".json")
	if err != nil {
		return dataset.Manifest{}, fmt.Errorf("manifest: %w", err)
	}
	if err := os.WriteFile(filepath.Join(stage, dataset.ManifestName), mb, filePerm); err != nil {
		return dataset.Manifest{}, fmt.Errorf("stage manifest: %w", err)
	}
	// ReadManifest checks the id against the content, so a manifest with the right id is the right one.
	m, err := dataset.ReadManifest(os.DirFS(stage))
	if err != nil {
		return m, fmt.Errorf("manifest %s: %w", id, err)
	}
	if m.ID != id {
		return m, fmt.Errorf("manifest %s has id %s: %w", id, m.ID, ErrMismatch)
	}
	return m, nil
}

// stageFile puts file into stage from the local copy when it matches, else from the source,
// and reports whether it was fetched.
func stageFile(ctx context.Context, f Fetcher, local, stage string, file dataset.File) (bool, error) {
	if !validName(file.Name) || !validHash(file.SHA256) {
		return false, fmt.Errorf("file %q, SHA-256 %q: %w", file.Name, file.SHA256, dataset.ErrInvalid)
	}
	b, err := os.ReadFile(filepath.Join(local, file.Name))
	fetched := err != nil || !matches(b, file)
	if fetched {
		if b, err = f.Fetch(ctx, "blobs/"+file.SHA256); err != nil {
			return false, fmt.Errorf("%s: %w", file.Name, err)
		}
		if !matches(b, file) {
			return false, fmt.Errorf("%s: %w", file.Name, ErrMismatch)
		}
	}
	if err := os.WriteFile(filepath.Join(stage, file.Name), b, filePerm); err != nil { //nolint:gosec // published data
		return false, fmt.Errorf("stage %s: %w", file.Name, err)
	}
	return fetched, nil
}

func matches(b []byte, file dataset.File) bool { return len(b) == file.Size && sum(b) == file.SHA256 }

func stagePrefix(name string) string { return "." + name + "-" }

// removeStages removes what an interrupted pull into dir/name left in dir.
func removeStages(dir, name string) error {
	left, err := filepath.Glob(filepath.Join(dir, stagePrefix(name)+"*"))
	if err != nil {
		return fmt.Errorf("stage: %w", err)
	}
	for _, l := range left {
		if err := os.RemoveAll(l); err != nil {
			return fmt.Errorf("stage: %w", err)
		}
	}
	return nil
}

// replace swaps the complete stage in for local, restoring local when that fails.
func replace(local, stage string) error {
	old := stage + ".old"
	if err := os.Rename(local, old); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("replace: %w", err)
	}
	if err := os.Rename(stage, local); err != nil {
		_ = os.Rename(old, local)
		return fmt.Errorf("replace: %w", err)
	}
	if err := os.RemoveAll(old); err != nil {
		return fmt.Errorf("replace: %w", err)
	}
	return nil
}

// validName rejects names that would leave their directory.
func validName(name string) bool {
	return name != "" && !strings.HasPrefix(name, ".") && !strings.ContainsAny(name, `/\`)
}

// validHash accepts a lowercase hex SHA-256, the form ids and file hashes take.
func validHash(h string) bool {
	if len(h) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(h)
	return err == nil && strings.ToLower(h) == h
}

func sum(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}
