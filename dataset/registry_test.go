package dataset_test

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	gocmp "github.com/google/go-cmp/cmp"

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
	for _, v := range []string{"3.28.0.16", "3.29.1.0", "3.29.3", "3.29.3.3", "3.29.10.1"} {
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
		match               dataset.Match
	}{
		{"empty is newest", "poe1", "", "3.29.10.1", dataset.MatchNewest},
		{"latest", "poe1", "latest", "3.29.10.1", dataset.MatchNewest},
		{"exact", "poe1", "3.29.3.3", "3.29.3.3", dataset.MatchExact},
		{"exact wins over longer versions under it", "poe1", "3.29.3", "3.29.3", dataset.MatchExact},
		{"league prefix, numeric order", "poe1", "3.29", "3.29.10.1", dataset.MatchNewest},
		{"older league prefix", "poe1", "3.28", "3.28.0.16", dataset.MatchNewest},
		{"later patch falls back within league", "poe1", "3.29.3.4", "3.29.3.3", dataset.MatchEarlierPatch},
		{"partial component does not match", "poe1", "3.2", "", 0},
		{"no earlier patch falls forward to newest of league", "poe1", "3.29.0.1", "3.29.10.1", dataset.MatchLaterPatch},
		{"earlier patch falls forward within league only", "poe1", "3.28.0.1", "3.28.0.16", dataset.MatchLaterPatch},
		{"no fallback across leagues", "poe1", "3.30.0.1", "", 0},
		{"other game", "poe2", "", "0.3.0", dataset.MatchNewest},
		{"unknown game", "poe3", "", "", 0},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			res, err := r.Resolve(testCase.game, testCase.version)
			if testCase.want == "" {
				if !errors.Is(err, dataset.ErrNoVersion) {
					t.Errorf("got %v, want ErrNoVersion", err)
				}
				if res.Match != 0 || res.Store != nil {
					t.Errorf("got match %v, store %v with an error, want neither", res.Match, res.Store)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			m := res.Store.Manifest()
			got := []any{m.Game, m.Version, res.Requested, res.Match}
			want := []any{testCase.game, testCase.want, testCase.version, testCase.match}
			if diff := gocmp.Diff(want, got); diff != "" {
				t.Errorf("game, version, requested, match (-want +got):\n%s", diff)
			}
		})
	}

	stores := r.Stores("poe1")
	names := make([]string, 0, len(stores))
	for _, s := range stores {
		names = append(names, s.Manifest().Version)
	}
	if diff := gocmp.Diff([]string{"3.28.0.16", "3.29.1.0", "3.29.3", "3.29.3.3", "3.29.10.1"}, names); diff != "" {
		t.Errorf("Stores (-want +got):\n%s", diff)
	}
}

func TestOpenRootSkipsBadDatasets(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeVersion(t, root, "good", "poe1", "3.29.3.3")
	writeVersion(t, root, "twice", "poe1", "3.29.3.3")
	writeVersion(t, root, "old", "poe1", "3.28.0.16")
	path := filepath.Join(root, "old", dataset.ManifestName)
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	b = bytes.Replace(b, []byte(`"format": 1`), []byte(`"format": 2`), 1)
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}

	r, err := dataset.OpenRoot(os.DirFS(root))
	if r == nil {
		t.Fatalf("OpenRoot: got a nil registry with %v, want the good dataset", err)
	}
	for _, want := range []string{"old: dataset format 2, want 1", "twice: dataset poe1 3.29.3.3 added twice"} {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("OpenRoot error: got %v, want it to contain %q", err, want)
		}
	}
	res, err := r.Resolve("poe1", "3.29")
	if err != nil || res.Store.Manifest().Version != "3.29.3.3" {
		t.Errorf("Resolve(3.29) after skipping: got %v, want 3.29.3.3", err)
	}
	if _, err := r.Resolve("poe1", "3.28"); !errors.Is(err, dataset.ErrNoVersion) {
		t.Errorf("Resolve(3.28): got %v, want ErrNoVersion for the skipped dataset", err)
	}
}

func TestResolveMalformedVersion(t *testing.T) {
	t.Parallel()
	var r dataset.Registry
	if _, err := r.Add(written(t, sample())); err != nil {
		t.Fatal(err)
	}
	for _, version := range []string{"3.29.x", "3..29", "3.029"} {
		_, err := r.Resolve("poe1", version)
		if err == nil || errors.Is(err, dataset.ErrNoVersion) || !strings.Contains(err.Error(), "want numbers separated by dots") {
			t.Errorf("Resolve(%q): got %v, want a malformed version error", version, err)
		}
	}
}

func TestRegistryAddErrors(t *testing.T) {
	t.Parallel()
	var r dataset.Registry
	if _, err := r.Add(written(t, sample())); err != nil {
		t.Fatal(err)
	}
	testCases := []struct {
		name, want string
		edit       func(m *dataset.Manifest)
	}{
		{name: "same version twice", want: "dataset poe1 3.29.3.3 added twice", edit: func(*dataset.Manifest) {}},
		{name: "no game", want: "game is required", edit: func(m *dataset.Manifest) { m.Game = "" }},
		{name: "no version", want: `version "": want numbers`, edit: func(m *dataset.Manifest) { m.Version = "" }},
		{name: "version not numbers", want: `version "3.29-beta": want numbers`, edit: func(m *dataset.Manifest) { m.Version = "3.29-beta" }},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			d := sample()
			testCase.edit(&d.Manifest)
			_, err := r.Add(written(t, d))
			checkError(t, err, nil, testCase.want)
		})
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
			res, err := r.Resolve("poe1", version)
			if err != nil {
				t.Error(err)
				return
			}
			tree, err := res.Store.Tree("standard")
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
