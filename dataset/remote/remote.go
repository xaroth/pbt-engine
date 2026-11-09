// Package remote keeps local copies of published datasets current. A source holds an index
// and one dataset per game and league:
//
//	index.json                  {"<game>": {"<league>": {"version": "...", "manifest": "<sha256>"}}}
//	<game>/<league>/            manifest.json and the files it lists
//
// Pull mirrors that layout into a local directory, fetching only what changed: the index names
// each manifest's SHA-256 and each manifest its files', so every byte fetched is checked.
package remote

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/xaroth/pbt-engine/dataset"
)

// IndexName is the index's name at the source.
const IndexName = "index.json"

// DefaultSource is the latest published data. A commit in place of main gives a fixed snapshot.
const DefaultSource = "https://raw.githubusercontent.com/xaroth/pbt-data/main/"

// Problems Pull reports.
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

// Entry is a league's dataset in the index: its game version and the SHA-256 of its manifest.
type Entry struct {
	Version  string `json:"version"`
	Manifest string `json:"manifest"`
}

// Index is index.json: per game, per league.
type Index map[string]map[string]Entry

// Fetcher reads a file from a source by its slash-separated path, such as poe1/3.29/stats.fb.
type Fetcher interface {
	Fetch(ctx context.Context, name string) ([]byte, error)
}

// HTTP fetches from Base, a URL ending in a slash.
type HTTP struct {
	Base   string
	Client *http.Client
}

// Fetch gets Base+name and fails on any status but 200.
func (h HTTP) Fetch(ctx context.Context, name string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.Base+name, nil)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", name, err)
	}
	c := h.Client
	if c == nil {
		c = http.DefaultClient
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

// Result is what Pull did for one league.
type Result struct {
	League, Version string
	// Updated is false when the local copy was current.
	Updated bool
	// Fetched counts the files downloaded, the manifest included; the others were reused.
	Fetched int
}

// Pull brings the datasets of game's leagues in dir up to date with the source, dir holding
// one directory per league as the source does under <game>/. Without leagues it pulls every
// league of game in the index. A league whose manifest matches the index is left alone;
// otherwise the new manifest is fetched and checked against the index, each file it lists is
// reused from the local copy when its SHA-256 matches or fetched and checked otherwise, and
// the complete dataset replaces the league's directory. A failed league keeps its old copy.
func Pull(ctx context.Context, f Fetcher, dir, game string, leagues ...string) ([]Result, error) {
	ib, err := f.Fetch(ctx, IndexName)
	if err != nil {
		return nil, fmt.Errorf("index: %w", err)
	}
	var idx Index
	if err := json.Unmarshal(ib, &idx); err != nil {
		return nil, fmt.Errorf("%s: %w", IndexName, err)
	}
	entries := idx[game]
	if len(leagues) == 0 {
		leagues = slices.Sorted(maps.Keys(entries))
	}
	if err := os.MkdirAll(dir, dirPerm); err != nil {
		return nil, fmt.Errorf("pull: %w", err)
	}
	out := make([]Result, 0, len(leagues))
	for _, league := range leagues {
		e, ok := entries[league]
		if !ok || !validName(league) {
			return out, fmt.Errorf("%w: %s %s", ErrUnknownLeague, game, league)
		}
		r, err := pullLeague(ctx, f, dir, game, league, e)
		if err != nil {
			return out, fmt.Errorf("pull %s %s: %w", game, league, err)
		}
		out = append(out, r)
	}
	return out, nil
}

func pullLeague(ctx context.Context, f Fetcher, dir, game, league string, e Entry) (Result, error) {
	r := Result{League: league, Version: e.Version}
	local := filepath.Join(dir, league)
	if b, err := os.ReadFile(filepath.Join(local, dataset.ManifestName)); err == nil && sum(b) == e.Manifest {
		return r, nil
	}
	stage, err := os.MkdirTemp(dir, "."+league+"-")
	if err != nil {
		return r, fmt.Errorf("stage: %w", err)
	}
	defer func() { _ = os.RemoveAll(stage) }()
	src := path.Join(game, league)
	m, err := stageManifest(ctx, f, src, stage, game, e)
	if err != nil {
		return r, err
	}
	r.Fetched = 1
	for _, file := range m.Files {
		fetched, err := stageFile(ctx, f, src, local, stage, file)
		if err != nil {
			return r, err
		}
		if fetched {
			r.Fetched++
		}
	}
	if err := replace(local, stage); err != nil {
		return r, err
	}
	r.Updated = true
	return r, nil
}

// stageManifest fetches the manifest of src into stage and checks it against e.
func stageManifest(ctx context.Context, f Fetcher, src, stage, game string, e Entry) (dataset.Manifest, error) {
	mb, err := f.Fetch(ctx, path.Join(src, dataset.ManifestName))
	if err != nil {
		return dataset.Manifest{}, fmt.Errorf("manifest: %w", err)
	}
	if sum(mb) != e.Manifest {
		return dataset.Manifest{}, fmt.Errorf("%s: %w", dataset.ManifestName, ErrMismatch)
	}
	if err := os.WriteFile(filepath.Join(stage, dataset.ManifestName), mb, filePerm); err != nil {
		return dataset.Manifest{}, fmt.Errorf("stage manifest: %w", err)
	}
	m, err := dataset.ReadManifest(os.DirFS(stage))
	if err != nil {
		return m, fmt.Errorf("manifest: %w", err)
	}
	if m.Game != game || m.Version != e.Version {
		return m, fmt.Errorf("manifest is %s %s, index says %s %s: %w", m.Game, m.Version, game, e.Version, ErrMismatch)
	}
	return m, nil
}

// stageFile puts file into stage from the local copy when it matches, else from the source,
// and reports whether it was fetched.
func stageFile(ctx context.Context, f Fetcher, src, local, stage string, file dataset.File) (bool, error) {
	if !validName(file.Name) {
		return false, fmt.Errorf("file name %q: %w", file.Name, dataset.ErrInvalid)
	}
	b, err := os.ReadFile(filepath.Join(local, file.Name))
	fetched := err != nil || !matches(b, file)
	if fetched {
		if b, err = f.Fetch(ctx, path.Join(src, file.Name)); err != nil {
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

func sum(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}
