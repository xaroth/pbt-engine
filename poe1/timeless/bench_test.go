package timeless_test

import (
	"cmp"
	"slices"
	"testing"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/internal/realdata"
	"github.com/xaroth/pbt-engine/poe1/timeless"
	"github.com/xaroth/pbt-engine/simulator"
)

// seedRanges are the seeds each version's jewel can show: first, last and step.
//
//nolint:gochecknoglobals // a constant table
var seedRanges = map[string][3]uint32{
	"Vaal":      {100, 8000, 1},
	"Karui":     {10000, 18000, 1},
	"Maraketh":  {500, 8000, 1},
	"Templar":   {2000, 10000, 1},
	"Eternal":   {2000, 160000, 20},
	"Kalguuran": {100, 4000, 1},
}

// BenchmarkConquer conquers the passives in the Large radius of the socket with the most of
// them, on the pinned current dataset, per version: one seed (forward), and every seed the
// version's jewel can show (reverse, what a search for a seed scans; target: < 500 ms WASM).
func BenchmarkConquer(b *testing.B) {
	store := realdata.Open(b, realdata.Current)
	nodes := fullestRadius(b, store)
	tree, err := store.Tree(build.DefaultVariant)
	if err != nil {
		b.Fatal(err)
	}
	for i := range tree.TimelessVersions {
		v := &tree.TimelessVersions[i]
		seeds, ok := seedRanges[v.ID]
		if !ok {
			continue
		}
		conquer, err := timeless.NewTree(tables(v))
		if err != nil {
			b.Fatal(err)
		}
		b.Run(v.ID+"/forward", func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				conquerAll(conquer, timeless.Jewel{Seed: seeds[0]}, nodes)
			}
			b.ReportMetric(float64(len(nodes)), "passives")
		})
		b.Run(v.ID+"/reverse", func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				for s := seeds[0]; s <= seeds[1]; s += seeds[2] {
					conquerAll(conquer, timeless.Jewel{Seed: s}, nodes)
				}
			}
			b.ReportMetric(float64((seeds[1]-seeds[0])/seeds[2]+1), "seeds")
		})
	}
}

// conquerAll conquers nodes and reads every result, as matching a seed does.
func conquerAll(t *timeless.Tree, j timeless.Jewel, nodes []timeless.Node) {
	for _, n := range nodes {
		r := t.Conquer(j, n)
		_ = r.Additions()
	}
}

// fullestRadius returns the passives outside ascendancies in the Large radius of the jewel
// socket with the most of them.
func fullestRadius(tb testing.TB, store *dataset.Store) []timeless.Node {
	tb.Helper()
	tree, err := store.Tree(build.DefaultVariant)
	if err != nil {
		tb.Fatal(err)
	}
	stats, err := store.Stats()
	if err != nil {
		tb.Fatal(err)
	}
	var out []timeless.Node
	for _, id := range fullestSocket(tb, store, tree) {
		i, ok := slices.BinarySearchFunc(tree.Nodes, id, func(n dataset.Node, id uint32) int { return cmp.Compare(n.ID, id) })
		if !ok || tree.Nodes[i].Ascendancy != "" {
			continue
		}
		if k, ok := kindOf(&tree.Nodes[i], stats); ok {
			out = append(out, timeless.Node{ID: id, Kind: k})
		}
	}
	return out
}

// fullestSocket returns the nodes in the Large radius of the jewel socket with the most of them.
func fullestSocket(tb testing.TB, store *dataset.Store, tree *dataset.Tree) []uint32 {
	tb.Helper()
	sim, err := simulator.New(store)
	if err != nil {
		tb.Fatal(err)
	}
	var fullest []uint32
	for _, s := range tree.JewelSlots {
		ids, err := sim.NodesInRadius(build.DefaultVariant, s, simulator.Radius{ID: "Large"})
		if err == nil && len(ids) > len(fullest) {
			fullest = ids
		}
	}
	return fullest
}

// kindOf is n's kind as the alternate tree tables number it; false for a node no jewel changes.
func kindOf(n *dataset.Node, stats []dataset.Stat) (timeless.Kind, bool) {
	switch n.Kind {
	case dataset.Notable:
		return timeless.Notable, true
	case dataset.Keystone:
		return timeless.Keystone, true
	case dataset.JewelSocket:
		return timeless.JewelSocket, true
	case dataset.Normal:
		ids := make([]string, 0, len(n.Stats))
		for _, sv := range n.Stats {
			ids = append(ids, stats[sv.Stat].ID)
		}
		return timeless.SmallKind(ids), true
	default:
		return 0, false
	}
}
