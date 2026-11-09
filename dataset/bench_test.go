package dataset_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/xaroth/pbt-engine/dataset"
)

// BenchmarkOpen opens the 3.29.3.3 semantics fixture (the real standard tree's nodes with the
// stats the table knows, characters, English text) and loads every group (01-goals, load one
// dataset: < 200 ms native for the real one).
func BenchmarkOpen(b *testing.B) {
	fsys := os.DirFS(filepath.Join("..", "semantics", "poe1", "testdata", "fixture", "3.29.3.3"))
	b.ReportAllocs()
	for b.Loop() {
		s, err := dataset.Open(fsys)
		if err != nil {
			b.Fatal(err)
		}
		if _, err := s.Dataset(dataset.Options{Trees: s.Manifest().Trees(), Languages: s.Manifest().Languages()}); err != nil {
			b.Fatal(err)
		}
	}
}
