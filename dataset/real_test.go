package dataset_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/internal/realdata"
)

// TestRewriteRealDatasets reads every group of the pinned datasets under $PBT_DATASETS, which
// must include items, skills, buffs and monsters, and writes them again: each file and the id
// come out unchanged.
func TestRewriteRealDatasets(t *testing.T) {
	t.Parallel()
	root := realdata.Root(t)
	ents, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	dirs := map[string]string{}
	for _, e := range ents {
		dir := filepath.Join(root, e.Name())
		if m, err := dataset.ReadManifest(os.DirFS(dir)); err == nil {
			dirs[m.Version] = dir
		}
	}
	for _, version := range realdata.Versions() {
		t.Run(version, func(t *testing.T) {
			t.Parallel()
			m := realdata.Open(t, version).Manifest()
			dir := dirs[version]
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
			if got.ID != m.ID {
				t.Errorf("id: got %s, want %s", got.ID, m.ID)
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
