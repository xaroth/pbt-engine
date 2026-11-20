package timeless_test

import (
	"cmp"
	"context"
	"slices"
	"testing"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/internal/realdata"
	"github.com/xaroth/pbt-engine/poe1/passives"
	"github.com/xaroth/pbt-engine/poe1/timeless"
)

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
	versions, err := timeless.New(store).Versions(build.DefaultVariant)
	if err != nil {
		b.Fatal(err)
	}
	for _, jv := range versions {
		v, seeds := &tree.TimelessVersions[jv.Index], jv.Seeds
		conquer, err := timeless.NewTree(tables(v))
		if err != nil {
			b.Fatal(err)
		}
		b.Run(v.ID+"/forward", func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				conquerAll(conquer, timeless.Jewel{Seed: seeds.First}, nodes)
			}
			b.ReportMetric(float64(len(nodes)), "passives")
		})
		b.Run(v.ID+"/reverse", func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				for s := seeds.First; s <= seeds.Last; s += seeds.Step {
					conquerAll(conquer, timeless.Jewel{Seed: s}, nodes)
				}
			}
			b.ReportMetric(float64((seeds.Last-seeds.First)/seeds.Step+1), "seeds")
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
	_, ids := fullestSocket(tb, store, tree)
	for _, id := range ids {
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

// fullestSocket returns the jewel socket with the most nodes in its Large radius, and those nodes.
func fullestSocket(tb testing.TB, store *dataset.Store, tree *dataset.Tree) (uint32, []uint32) {
	tb.Helper()
	reader := passives.New(store)
	var socket uint32
	var fullest []uint32
	for _, s := range tree.JewelSlots {
		ids, err := reader.NodesInRadius(build.DefaultVariant, s, passives.Radius{ID: "Large"})
		if err == nil && len(ids) > len(fullest) {
			socket, fullest = s, ids
		}
	}
	return socket, fullest
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

// BenchmarkSearch searches every seed of each version in the socket with the most passives in
// its Large radius, on the pinned current dataset, for a named notable and a stat of every kind
// of passive, so every passive in the radius is conquered, and returns the best 50 with what each
// gives (target: < 500 ms WASM).
func BenchmarkSearch(b *testing.B) {
	store := realdata.Open(b, realdata.Current)
	tree, err := store.Tree(build.DefaultVariant)
	if err != nil {
		b.Fatal(err)
	}
	stats, err := store.Stats()
	if err != nil {
		b.Fatal(err)
	}
	names, err := store.Names("en")
	if err != nil {
		b.Fatal(err)
	}
	socket, _ := fullestSocket(b, store, tree)
	reader := timeless.New(store)
	versions, err := reader.Versions(build.DefaultVariant)
	if err != nil {
		b.Fatal(err)
	}
	for _, jv := range versions {
		q := timeless.Query{Socket: socket, Version: jv.Index, Language: "en", Limit: 50, Wants: benchWants(&tree.TimelessVersions[jv.Index], stats, names)}
		b.Run(jv.ID, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if _, err := reader.Search(context.Background(), build.DefaultVariant, q, nil); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// benchWants wants the first named notable replacement of v, and weighs the first stat of the
// first row of each kind of passive.
func benchWants(v *dataset.TimelessVersion, stats []dataset.Stat, names *dataset.Names) []timeless.Want {
	var out []timeless.Want
	for _, r := range v.Replacements {
		i := slices.IndexFunc(names.TimelessNames, func(n dataset.Name) bool { return n.ID == r.ID })
		if slices.Contains(r.Kinds, dataset.TimelessNotable) && i >= 0 {
			out = append(out, timeless.Want{Name: names.TimelessNames[i].Name})
			break
		}
	}
	for _, k := range []dataset.TimelessKind{dataset.TimelessSmallAttribute, dataset.TimelessSmallNormal, dataset.TimelessNotable} {
		if stat, ok := firstStat(v, k); ok {
			w := timeless.Want{Stat: stats[stat].ID, Weight: 1}
			if !slices.Contains(out, w) {
				out = append(out, w)
			}
		}
	}
	return out
}

// firstStat gives the first stat of v's first replacement or addition with stats for kind k.
func firstStat(v *dataset.TimelessVersion, k dataset.TimelessKind) (uint32, bool) {
	var rows [][]dataset.StatRange
	for _, r := range v.Replacements {
		if slices.Contains(r.Kinds, k) {
			rows = append(rows, r.Stats)
		}
	}
	for _, a := range v.Additions {
		if slices.Contains(a.Kinds, k) {
			rows = append(rows, a.Stats)
		}
	}
	for _, r := range rows {
		if len(r) > 0 {
			return r[0].Stat, true
		}
	}
	return 0, false
}
