package build_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/dataset"
)

const (
	jewelBase   = "Metadata/Items/Jewels/JewelStr"
	abyssBase   = "Metadata/Items/Jewels/JewelAbyssMelee"
	clusterBase = "Metadata/Items/Jewels/JewelPassiveTreeExpansionLarge"
)

// jewelStore is itemsStore's dataset with jewel socket 70 joined to node 3, jewel socket 71
// joining nothing, and a jewel, abyss jewel and large cluster jewel base.
func jewelStore(t *testing.T) *dataset.Store {
	t.Helper()
	d := testDataset()
	tree := &d.Trees[0]
	tree.Nodes[2].Neighbours = append(tree.Nodes[2].Neighbours, 70)
	tree.Nodes = append(tree.Nodes,
		dataset.Node{ID: 70, Kind: dataset.JewelSocket, Neighbours: []uint32{3}},
		dataset.Node{ID: 71, Kind: dataset.JewelSocket},
	)
	tree.JewelSlots = []uint32{70, 71}
	d.Items = &dataset.Items{
		Classes: []dataset.ItemClass{{ID: "Ring"}, {ID: "Jewel"}, {ID: "AbyssJewel"}},
		Bases: []dataset.ItemBase{
			{ID: ringBase, Class: "Ring"},
			{ID: jewelBase, Class: "Jewel"},
			{ID: abyssBase, Class: "AbyssJewel"},
			{ID: clusterBase, Class: "Jewel"},
		},
	}
	return openStore(t, d)
}

func jewel(base string) build.Item {
	return build.Item{Rarity: build.Rare, Name: "Gloom Spark", Base: base, Mods: []build.ModLine{
		{Kind: build.Explicit, Text: "+7% to maximum Life", Stats: []build.StatValue{{Stat: "base_maximum_life", Value: 7}}},
	}}
}

// TestCheckJewels: a jewel in an allocated socket checks clean; one in an unallocated socket
// is kept with a warning; one in a node that is not a socket or that is not a jewel is left
// out with a warning.
func TestCheckJewels(t *testing.T) {
	t.Parallel()
	s := jewelStore(t)
	unknown := jewel(jewelBase)
	unknown.Base = "Metadata/Items/Nothing"
	testCases := []struct {
		name   string
		jewels build.Jewels
		want   []build.Warning
		kept   build.Jewels
	}{
		{
			name:   "jewel in an allocated socket",
			jewels: build.Jewels{70: jewel(jewelBase)},
			kept:   build.Jewels{70: jewel(jewelBase)},
		},
		{
			name:   "abyss jewel in an allocated socket",
			jewels: build.Jewels{70: jewel(abyssBase)},
			kept:   build.Jewels{70: jewel(abyssBase)},
		},
		{
			name:   "cluster jewel in an allocated socket",
			jewels: build.Jewels{70: jewel(clusterBase)},
			kept:   build.Jewels{70: jewel(clusterBase)},
		},
		{
			name:   "unallocated socket",
			jewels: build.Jewels{71: jewel(jewelBase)},
			want:   []build.Warning{{Code: build.JewelNotAllocated, Subject: "jewel 71", Message: "socket not allocated; kept, it counts once allocated"}},
			kept:   build.Jewels{71: jewel(jewelBase)},
		},
		{
			name:   "not a socket",
			jewels: build.Jewels{3: jewel(jewelBase), 99: jewel(jewelBase)},
			want: []build.Warning{
				{Code: build.JewelNotSocket, Subject: "jewel 3", Message: "node 3 is not a jewel socket of the standard tree; jewel left out"},
				{Code: build.JewelNotSocket, Subject: "jewel 99", Message: "node 99 is not a jewel socket of the standard tree; jewel left out"},
			},
		},
		{
			name:   "not a jewel",
			jewels: build.Jewels{70: jewel(ringBase)},
			want:   []build.Warning{{Code: build.JewelKindMismatch, Subject: "jewel 70", Message: "a Ring does not fit a jewel socket; left out"}},
		},
		{
			name:   "unknown base",
			jewels: build.Jewels{70: unknown},
			want:   []build.Warning{{Code: build.ItemBaseUnknown, Subject: "jewel 70", Message: "base Metadata/Items/Nothing not in 3.29.3.3; item left out"}},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			b := juggernaut()
			b.Nodes = append(b.Nodes, 70)
			b.Jewels = testCase.jewels
			c, warnings, err := build.Check(b, s)
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(testCase.want, warnings); diff != "" {
				t.Errorf("warnings (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(testCase.kept, c.Build().Jewels); diff != "" {
				t.Errorf("jewels (-want +got):\n%s", diff)
			}
		})
	}
}

// Without an items group, jewel bases go unchecked but a jewel must still be in a socket.
func TestCheckJewelsNoItemsGroup(t *testing.T) {
	t.Parallel()
	d := testDataset()
	d.Trees[0].Nodes = append(d.Trees[0].Nodes, dataset.Node{ID: 70, Kind: dataset.JewelSocket})
	b := juggernaut()
	b.Nodes = append(b.Nodes, 70)
	b.Jewels = build.Jewels{70: jewel(ringBase), 3: jewel(jewelBase)}
	c, warnings, err := build.Check(b, openStore(t, d))
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff([]build.Code{build.NodesDisconnected, build.ItemsUnchecked, build.JewelNotSocket}, codesOf(warnings)); diff != "" {
		t.Errorf("codes (-want +got):\n%s\n%v", diff, warnings)
	}
	if diff := cmp.Diff(build.Jewels{70: jewel(ringBase)}, c.Build().Jewels); diff != "" {
		t.Errorf("jewels (-want +got):\n%s", diff)
	}
}
