package dataset_test

import (
	"errors"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"testing/fstest"

	"github.com/google/go-cmp/cmp"

	"github.com/xaroth/pbt-engine/dataset"
)

// writeVersion writes sample() as game and version into root/dir.
func writeVersion(t *testing.T, root, dir, game, version string) {
	t.Helper()
	d := sample()
	d.Manifest.Game, d.Manifest.Version = game, version
	path := filepath.Join(root, dir)
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := dataset.Write(path, d); err != nil {
		t.Fatal(err)
	}
}

func TestResolve(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	for _, v := range []string{"3.28.0.16", "3.29.1.0", "3.29.3.3", "3.29.10.1"} {
		writeVersion(t, root, v, "poe1", v)
	}
	writeVersion(t, root, "poe2", "poe2", "0.3.0")
	if err := os.Mkdir(filepath.Join(root, "not-a-dataset"), 0o755); err != nil {
		t.Fatal(err)
	}
	r, err := dataset.OpenRoot(os.DirFS(root))
	if err != nil {
		t.Fatal(err)
	}

	testCases := []struct {
		name, game, version string
		want                string // empty: ErrNoVersion
	}{
		{"empty is newest", "poe1", "", "3.29.10.1"},
		{"latest", "poe1", "latest", "3.29.10.1"},
		{"exact", "poe1", "3.29.3.3", "3.29.3.3"},
		{"league prefix, numeric order", "poe1", "3.29", "3.29.10.1"},
		{"older league prefix", "poe1", "3.28", "3.28.0.16"},
		{"patch prefix", "poe1", "3.29.3", "3.29.3.3"},
		{"later patch falls back within league", "poe1", "3.29.3.4", "3.29.3.3"},
		{"partial component does not match", "poe1", "3.2", ""},
		{"no older patch in league", "poe1", "3.29.0.1", ""},
		{"no fallback across leagues", "poe1", "3.30.0.1", ""},
		{"other game", "poe2", "", "0.3.0"},
		{"unknown game", "poe3", "", ""},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			v, err := r.Resolve(testCase.game, testCase.version)
			if testCase.want == "" {
				if !errors.Is(err, dataset.ErrNoVersion) {
					t.Errorf("got %v, %v, want ErrNoVersion", v, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if v.Name() != testCase.want || v.Game() != testCase.game {
				t.Errorf("got %s %s, want %s %s", v.Game(), v.Name(), testCase.game, testCase.want)
			}
		})
	}

	var names []string
	for _, v := range r.Versions("poe1") {
		names = append(names, v.Name())
	}
	if diff := cmp.Diff([]string{"3.28.0.16", "3.29.1.0", "3.29.3.3", "3.29.10.1"}, names); diff != "" {
		t.Errorf("Versions (-want +got):\n%s", diff)
	}
}

func TestRegistryAddErrors(t *testing.T) {
	t.Parallel()
	good := written(t, sample())
	var r dataset.Registry
	if _, err := r.Add(good); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Add(good); err == nil {
		t.Error("same version twice: got nil error, want error")
	}
	d := sample()
	d.Manifest.Version = ""
	if _, err := r.Add(written(t, d)); err == nil {
		t.Error("no version: got nil error, want error")
	}
}

// TestTwoLeagues loads two versions in one registry and reads them concurrently.
func TestTwoLeagues(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeVersion(t, root, "a", "poe1", "3.28.0.16")
	writeVersion(t, root, "b", "poe1", "3.29.3.3")
	r, err := dataset.OpenRoot(os.DirFS(root))
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for _, version := range []string{"3.28", "3.29", "3.28", "3.29"} {
		wg.Go(func() {
			v, err := r.Resolve("poe1", version)
			if err != nil {
				t.Error(err)
				return
			}
			tree, err := v.Tree("standard")
			if err != nil {
				t.Error(err)
				return
			}
			if len(tree.Nodes) != 2 {
				t.Errorf("%s: got %d nodes, want 2", version, len(tree.Nodes))
			}
		})
	}
	wg.Wait()
}

// countFS counts reads per file.
type countFS struct {
	fs.FS
	mu    sync.Mutex
	reads map[string]int
}

func (c *countFS) ReadFile(name string) ([]byte, error) {
	c.mu.Lock()
	c.reads[name]++
	c.mu.Unlock()
	return fs.ReadFile(c.FS, name)
}

func (c *countFS) snapshot() map[string]int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return maps.Clone(c.reads)
}

func TestVersionLazy(t *testing.T) {
	t.Parallel()
	fsys := &countFS{FS: written(t, sample()), reads: map[string]int{}}
	v, err := dataset.Open(fsys)
	if err != nil {
		t.Fatal(err)
	}
	steps := []struct {
		name string
		do   func() error
		want map[string]int
	}{
		{"open", func() error { return nil }, map[string]int{"manifest.json": 1}},
		{"tree", func() error { _, err := v.Tree("standard"); return err },
			map[string]int{"manifest.json": 1, "stats.fb": 1, "characters.fb": 1, "tree-standard.fb": 1}},
		{"tree again", func() error { _, err := v.Tree("standard"); return err },
			map[string]int{"manifest.json": 1, "stats.fb": 1, "characters.fb": 1, "tree-standard.fb": 1}},
		{"text reads every tree", func() error { _, err := v.Text("en"); return err },
			map[string]int{"manifest.json": 1, "stats.fb": 1, "characters.fb": 1, "tree-standard.fb": 1, "tree-ruthless.fb": 1, "text-en.fb": 1}},
		{"constants", func() error { _, err := v.Constants(); return err },
			map[string]int{"manifest.json": 1, "stats.fb": 1, "characters.fb": 1, "tree-standard.fb": 1, "tree-ruthless.fb": 1, "text-en.fb": 1, "constants.fb": 1}},
	}
	for _, step := range steps {
		if err := step.do(); err != nil {
			t.Fatalf("%s: %v", step.name, err)
		}
		if diff := cmp.Diff(step.want, fsys.snapshot()); diff != "" {
			t.Errorf("%s: reads (-want +got):\n%s", step.name, diff)
		}
	}
	if got := v.Variants(); !slices.Equal(got, []string{"standard", "ruthless"}) {
		t.Errorf("Variants: got %v, want [standard ruthless]", got)
	}
}

func TestVersionErrors(t *testing.T) {
	t.Parallel()
	good := written(t, sample())
	broken := maps.Clone(good)
	broken["tree-standard.fb"] = &fstest.MapFile{Data: []byte("x")}
	testCases := []struct {
		name string
		fsys fs.FS
		do   func(v *dataset.Version) error
	}{
		{"unknown variant", good, func(v *dataset.Version) error { _, err := v.Tree("alternate"); return err }},
		{"unknown language", good, func(v *dataset.Version) error { _, err := v.Text("fr"); return err }},
		{"damaged tree", broken, func(v *dataset.Version) error { _, err := v.Tree("standard"); return err }},
		{"damaged tree read by text", broken, func(v *dataset.Version) error { _, err := v.Text("en"); return err }},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			v, err := dataset.Open(testCase.fsys)
			if err != nil {
				t.Fatal(err)
			}
			if err := testCase.do(v); err == nil {
				t.Error("got nil error, want error")
			}
		})
	}
	// A damaged tree does not stop reading the other groups.
	v, err := dataset.Open(broken)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := v.Characters(); err != nil {
		t.Errorf("Characters with a damaged tree: got %v, want nil", err)
	}
}
