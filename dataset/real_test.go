package dataset_test

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/internal/realdata"
)

// TestRewriteRealDatasets reads every group of the pinned datasets under $PBT_DATASETS, which
// must include items, skills, buffs and monsters, and writes them again: each file comes out
// unchanged, and the id too apart from the names files a dataset written before them gains.
func TestRewriteRealDatasets(t *testing.T) {
	t.Parallel()
	for _, version := range realdata.Versions() {
		t.Run(version, func(t *testing.T) {
			t.Parallel()
			m := realdata.Open(t, version).Manifest()
			dir := realDir(t, version)
			d, err := readAll(os.DirFS(dir))
			if err != nil {
				t.Fatal(err)
			}
			if d.Items == nil || len(d.Mods) == 0 || d.Skills == nil || d.Buffs == nil || d.Monsters == nil {
				t.Errorf("groups: items %t, mods %d, skills %t, buffs %t, monsters %t; want all",
					d.Items != nil, len(d.Mods), d.Skills != nil, d.Buffs != nil, d.Monsters != nil)
			}
			out := t.TempDir()
			got, err := dataset.Write(out, d)
			if err != nil {
				t.Fatalf("Write: %v", err)
			}
			if want := withNamesFiles(m, got); got.ID != want.ID {
				t.Errorf("id: got %s, want %s", got.ID, want.ID)
			}
			for _, f := range m.Files {
				a, errA := os.ReadFile(filepath.Join(dir, f.Name))
				b, errB := os.ReadFile(filepath.Join(out, f.Name))
				if errA != nil || errB != nil || !bytes.Equal(a, b) {
					t.Errorf("%s: bytes differ after a rewrite (%v, %v)", f.Name, errA, errB)
				}
			}
		})
	}
}

// withNamesFiles returns pinned with the names files of written added when pinned has none, as
// a dataset written before the names group gains them on a rewrite, and its id updated.
func withNamesFiles(pinned, written dataset.Manifest) dataset.Manifest {
	isNames := func(f dataset.File) bool { return strings.HasPrefix(f.Name, "names-") }
	if slices.ContainsFunc(pinned.Files, isNames) {
		return pinned
	}
	out := pinned
	out.Files = slices.Clone(pinned.Files)
	for _, f := range written.Files {
		if isNames(f) {
			out.Files = append(out.Files, f)
		}
	}
	out.ID = dataset.ContentID(out)
	return out
}

// realDir returns the directory under $PBT_DATASETS that holds the pinned dataset of version.
func realDir(tb testing.TB, version string) string {
	tb.Helper()
	root := realdata.Root(tb)
	ents, err := os.ReadDir(root)
	if err != nil {
		tb.Fatal(err)
	}
	for _, e := range ents {
		dir := filepath.Join(root, e.Name())
		if m, err := dataset.ReadManifest(os.DirFS(dir)); err == nil && m.Version == version && m.ID == realdata.ID(version) {
			return dir
		}
	}
	tb.Fatalf("no pinned dataset %s under %s", version, root)
	return ""
}
