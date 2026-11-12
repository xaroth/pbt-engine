// Package realdata names the real datasets the tests read from $PBT_DATASETS and opens them.
package realdata

import (
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/xaroth/pbt-engine/dataset"

	_ "embed"
)

// The pinned poe1 datasets: Previous is the league before Current.
const (
	Previous = "3.28.0.16"
	Current  = "3.29.3.6"
)

// commit is the pbt-data commit that holds the pinned datasets. CI and the README read the same
// file.
//
//go:embed pbt-data-commit.txt
var commit string

// Commit returns the pbt-data commit that holds the pinned datasets.
func Commit() string { return strings.TrimSpace(commit) }

// ids are the dataset ids of the pinned versions.
//
//nolint:gochecknoglobals // a constant table
var ids = map[string]string{
	Previous: "ab036e6f7a2a49481a99d34d1b79a2bdf1e09329bc748f36038e0f6cec41dcb9",
	Current:  "f9bfe4de4f58bfaa7848ff5ff26852480c8af50ebd7d40b6fbc62e8f7ebc33b9",
}

// Versions are the pinned versions, oldest first.
func Versions() []string {
	return []string{Previous, Current}
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
// per directory: pulled from pbt-data, or one per version). It skips without $PBT_DATASETS and
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
		tb.Fatalf("PBT_DATASETS has poe1 %s (id %s), want %s (id %s): check out pbt-data %s", m.Version, m.ID, version, want, Commit())
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
			tb.Fatalf("PBT_DATASETS has poe1 %s with id %s, want id %s: check out pbt-data %s", version, m.ID, want, Commit())
		}
		return &Recorder{fsys: fsys, opened: map[string]bool{}}
	}
	tb.Fatalf("PBT_DATASETS has no poe1 %s: check out pbt-data %s", version, Commit())
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
