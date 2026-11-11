package dataset_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/xaroth/pbt-engine/dataset"
)

// TestRewriteRealDatasets reads every group of the 3.28.0.16 and 3.29.3.3 datasets under
// $PBT_DATASETS and writes them again: each file and the id come out unchanged.
func TestRewriteRealDatasets(t *testing.T) {
	t.Parallel()
	root := os.Getenv("PBT_DATASETS")
	if root == "" {
		t.Skip("PBT_DATASETS not set")
	}
	ents, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	found := 0
	for _, e := range ents {
		dir := filepath.Join(root, e.Name())
		m, err := dataset.ReadManifest(os.DirFS(dir))
		if err != nil || (m.Version != "3.28.0.16" && m.Version != "3.29.3.3") {
			continue
		}
		found++
		t.Run(m.Version, func(t *testing.T) {
			t.Parallel()
			d, err := readAll(os.DirFS(dir))
			if err != nil {
				t.Fatal(err)
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
	if found == 0 {
		t.Skip("PBT_DATASETS has neither 3.28.0.16 nor 3.29.3.3")
	}
}
