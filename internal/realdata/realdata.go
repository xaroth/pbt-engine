// Package realdata names the real datasets and art manifests the tests read from $PBT_DATASETS
// and opens them.
package realdata

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"

	"github.com/xaroth/pbt-engine/dataset"
)

// The pinned poe1 datasets: Previous is the league before Current. InGame is the version the
// cases confirmed in the game client were recorded on.
const (
	Previous = "3.28.0.16"
	Current  = "3.29.3.6"
	InGame   = "3.30.0.1"
)

// ids are the dataset ids of the pinned versions.
//
//nolint:gochecknoglobals // a constant table
var ids = map[string]string{
	Previous: "5f14cda3263e1d93da9969242fe91c41b0fbb5145ca4864b405947ddc1b13427",
	Current:  "a7ac693b1a4da19aecd8110379fa8c8f2b7bfdac1c48387573bb4d249ced86db",
	InGame:   "fe5fd41f270eadc01b1a059798eec7887e22754428b122821559a63613e5be4e",
}

// artIDs are the art manifest ids, each the manifest's SHA-256, published with the pinned
// versions.
//
//nolint:gochecknoglobals // a constant table
var artIDs = map[string]string{
	Previous: "34692fb8eb2093c41ec79dc904017be3725ca89234c0f5d74692a9e8c8c6d689",
	Current:  "1bfa96b1c78bd161b9b624e62540150cef061afc2433ff589c7fe8c5faef7ee6",
	InGame:   "fc11aebc338c297d9157fd811d44f437f3e9340a766959a2ffd006a4f29e3299",
}

// fetchHint tells how to put the pinned datasets under $PBT_DATASETS.
const fetchHint = "fetch them with go run ./internal/realdata/fetch \"$PBT_DATASETS\""

// ID returns the dataset id pinned for version, or "" when version is not pinned.
func ID(version string) string { return ids[version] }

// ArtID returns the art manifest id pinned for version, or "" when version is not pinned.
func ArtID(version string) string { return artIDs[version] }

// ArtPath is where fetch puts the art manifest with id under the datasets root.
func ArtPath(root, id string) string { return filepath.Join(root, "art", id+".json") }

// ArtManifest returns the JSON of the art manifest pinned for version, from $PBT_DATASETS. It
// skips without $PBT_DATASETS and fails when the manifest is missing or not the pinned one.
func ArtManifest(tb testing.TB, version string) []byte {
	tb.Helper()
	id, ok := artIDs[version]
	if !ok {
		tb.Fatalf("realdata: %s is not a pinned version", version)
	}
	b, err := os.ReadFile(ArtPath(Root(tb), id))
	if err != nil {
		tb.Fatalf("art manifest of %s: %v: %s", version, err, fetchHint)
	}
	if sum := sha256.Sum256(b); hex.EncodeToString(sum[:]) != id {
		tb.Fatalf("art manifest of %s does not match its id %s: %s", version, id, fetchHint)
	}
	return b
}

// Versions are the pinned versions the sheet is tested on, oldest first.
func Versions() []string {
	return []string{Previous, Current}
}

// Pinned are all pinned versions, oldest first.
func Pinned() []string {
	return []string{Previous, Current, InGame}
}

// Root is $PBT_DATASETS, and skips tb without it.
func Root(tb testing.TB) string {
	tb.Helper()
	root := os.Getenv("PBT_DATASETS")
	if root == "" {
		tb.Skip("PBT_DATASETS not set")
	}
	return root
}

// Open opens the pinned dataset of exactly version among the datasets under $PBT_DATASETS (one
// per directory, such as fetch writes them). It skips without $PBT_DATASETS and
// fails when the dataset is missing or is not the pinned one, so a stale checkout cannot pass
// by skipping.
func Open(tb testing.TB, version string) *dataset.Store {
	tb.Helper()
	want, ok := ids[version]
	if !ok {
		tb.Fatalf("realdata: %s is not a pinned version", version)
	}
	reg, err := dataset.OpenRoot(os.DirFS(Root(tb)))
	if reg == nil {
		tb.Fatal(err)
	}
	res, err := reg.Resolve("poe1", version)
	if err != nil {
		tb.Fatalf("dataset %s: %v", version, err)
	}
	m := res.Store.Manifest()
	if m.Version != version || m.ID != want {
		tb.Fatalf("PBT_DATASETS has poe1 %s (id %s), want %s (id %s): %s", m.Version, m.ID, version, want, fetchHint)
	}
	return res.Store
}

// Recorder is an fs.FS over a pinned dataset that records the files opened through it.
type Recorder struct {
	fsys   fs.FS
	mu     sync.Mutex
	opened map[string]bool
}

// Record returns a Recorder over the pinned dataset of exactly version, found and checked as
// Open finds and checks it.
func Record(tb testing.TB, version string) *Recorder {
	tb.Helper()
	want, ok := ids[version]
	if !ok {
		tb.Fatalf("realdata: %s is not a pinned version", version)
	}
	root := Root(tb)
	ents, err := os.ReadDir(root)
	if err != nil {
		tb.Fatal(err)
	}
	for _, e := range ents {
		fsys := os.DirFS(filepath.Join(root, e.Name()))
		m, err := dataset.ReadManifest(fsys)
		if !e.IsDir() || err != nil || m.Version != version {
			continue
		}
		if m.ID != want {
			tb.Fatalf("PBT_DATASETS has poe1 %s with id %s, want id %s: %s", version, m.ID, want, fetchHint)
		}
		return &Recorder{fsys: fsys, opened: map[string]bool{}}
	}
	tb.Fatalf("PBT_DATASETS has no poe1 %s: %s", version, fetchHint)
	return nil
}

// Open opens name and records it, unless it is the manifest.
func (r *Recorder) Open(name string) (fs.File, error) {
	if name != dataset.ManifestName {
		r.mu.Lock()
		r.opened[name] = true
		r.mu.Unlock()
	}
	//nolint:wrapcheck // an fs.FS returns the *fs.PathError of the FS it wraps as it is
	return r.fsys.Open(name)
}

// Opened returns the names of the files opened so far, sorted.
func (r *Recorder) Opened() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Sorted(maps.Keys(r.opened))
}
