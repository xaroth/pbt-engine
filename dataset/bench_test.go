package dataset_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/xaroth/pbt-engine/internal/realdata"
)

// BenchmarkOpen opens the current semantics fixture (the real standard tree's nodes with the
// stats the table knows, characters, English text) and loads every group (target for the real
// dataset: < 200 ms native).
func BenchmarkOpen(b *testing.B) {
	fsys := os.DirFS(filepath.Join("..", "internal", "poe1", "testdata", "fixture", realdata.Current))
	b.ReportAllocs()
	for b.Loop() {
		if _, err := readAll(fsys); err != nil {
			b.Fatal(err)
		}
	}
}
