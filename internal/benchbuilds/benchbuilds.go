// Package benchbuilds holds the builds the benchmarks run on: Path of Building files made for
// benchmarking, with a full tree, skill groups with supports and auras, and items that add gem
// levels, read on the pinned current dataset under $PBT_DATASETS.
package benchbuilds

import (
	"embed"
	"os"
	"strings"
	"testing"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/internal/realdata"
	"github.com/xaroth/pbt-engine/pob"
)

//go:embed *.xml
var files embed.FS

// Case is one build: its name, its Path of Building file and the build imported from it.
type Case struct {
	Name  string
	XML   []byte
	Build *build.Build
}

// Data is the pinned current dataset and a registry holding it, as a host has them.
type Data struct {
	Registry *dataset.Registry
	Store    *dataset.Store
}

// Open opens the pinned current dataset. It skips without $PBT_DATASETS and fails when the
// dataset is missing or not the pinned one.
func Open(tb testing.TB) Data {
	tb.Helper()
	store := realdata.Open(tb, realdata.Current)
	reg, err := dataset.OpenRoot(os.DirFS(realdata.Root(tb)))
	if reg == nil {
		tb.Fatal(err)
	}
	return Data{Registry: reg, Store: store}
}

// Builds imports every build on d. Each must import without warnings; its version is set to
// the store's, as a host does.
func Builds(tb testing.TB, d Data) []Case {
	tb.Helper()
	ents, err := files.ReadDir(".")
	if err != nil {
		tb.Fatal(err)
	}
	out := make([]Case, 0, len(ents))
	for _, e := range ents {
		xml, err := files.ReadFile(e.Name())
		if err != nil {
			tb.Fatal(err)
		}
		b, warnings, err := pob.FromXML(xml, d.Registry)
		if err != nil {
			tb.Fatalf("%s: %v", e.Name(), err)
		}
		if len(warnings) != 0 {
			tb.Fatalf("%s imports with warnings: %v", e.Name(), warnings)
		}
		b.Version = d.Store.Manifest().Version
		name, _, _ := strings.Cut(e.Name(), "-")
		out = append(out, Case{Name: name, XML: xml, Build: b})
	}
	return out
}
