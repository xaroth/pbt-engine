package cluster_test

import (
	"testing"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/internal/poe1/cluster"
	"github.com/xaroth/pbt-engine/internal/realdata"
)

// BenchmarkGenerator is the generator's setup, once per tree variant, and one large jewel's
// generation in an outer large socket, on the pinned current dataset.
func BenchmarkGenerator(b *testing.B) {
	store := realdata.Open(b, realdata.Current)
	b.Run("setup", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if _, err := cluster.NewGenerator(store, build.DefaultVariant); err != nil {
				b.Fatal(err)
			}
		}
	})
	g, err := cluster.NewGenerator(store, build.DefaultVariant)
	if err != nil {
		b.Fatal(err)
	}
	socket, jewel := outerLargeSocket(b, store), benchJewel(b, g, store)
	b.Run("generate", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if _, err := g.Generate(socket, jewel); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func outerLargeSocket(tb testing.TB, store *dataset.Store) uint32 {
	tb.Helper()
	tree, err := store.Tree(build.DefaultVariant)
	if err != nil {
		tb.Fatal(err)
	}
	for _, s := range tree.Clusters.Sockets {
		if s.Size == dataset.LargeCluster && s.Parent == 0 {
			return s.Node
		}
	}
	tb.Fatal("no outer large cluster socket")
	return 0
}

// benchJewel is a large cluster jewel with two notables and two sockets. It fails tb unless g
// reads every line of it.
func benchJewel(tb testing.TB, g *cluster.Generator, store *dataset.Store) cluster.Jewel {
	tb.Helper()
	j := cluster.Jewel{Base: largeBase, Lines: []string{
		"Adds 8 Passive Skills",
		"2 Added Passive Skills are Jewel Sockets",
		"Added Small Passive Skills grant: 12% increased Fire Damage",
		"1 Added Passive Skill is Burning Bright",
		"1 Added Passive Skill is Prismatic Heart",
	}}
	sg, err := g.Generate(outerLargeSocket(tb, store), j)
	if err != nil {
		tb.Fatal(err)
	}
	notables := 0
	for _, n := range sg.Nodes {
		if n.Kind == dataset.Notable {
			notables++
		}
	}
	if len(sg.Unread) != 0 || notables != 2 {
		tb.Fatalf("jewel read with %d notables, lines not read: %v", notables, sg.Unread)
	}
	return j
}
