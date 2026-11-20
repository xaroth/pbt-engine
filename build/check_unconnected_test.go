package build_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/dataset"
)

// unconnectedStore is a Marauder tree on a line, with each node's x: start 1 (0), small node 2
// (1000), socket 3 (2000), joined in that order; small node 4 (2900), notable 5 (3100) and small
// node 6 (4000), joined in that order and not to the socket; keystone 7 (10000), small node 8
// (10500) and small node 9 (11500), joined in that order. A Small radius reaches 960 layout
// units, its ring 960 to 1320: from the socket, 4 is in the disc and 5 in the ring, and from the
// keystone, 8 is in the disc.
func unconnectedStore(t *testing.T) *dataset.Store {
	t.Helper()
	xs := []float64{0, 1000, 2000, 2900, 3100, 4000, 10000, 10500, 11500}
	layout := make([]dataset.LayoutNode, 0, len(xs))
	groups := make([]dataset.LayoutGroup, 0, len(xs))
	for i, x := range xs {
		id := uint32(i + 1)
		groups = append(groups, dataset.LayoutGroup{ID: id, X: x})
		layout = append(layout, dataset.LayoutNode{ID: id, Group: id, Icon: "n.png"})
	}
	return openStore(t, &dataset.Dataset{
		Manifest:   dataset.Manifest{Game: "poe1", Version: "3.29.3.3"},
		Characters: []dataset.Character{{ID: marauder}},
		Trees: []dataset.Tree{{
			Variant: "standard",
			Classes: []dataset.Class{{Character: marauder, StartNode: 1}},
			Nodes: []dataset.Node{
				{ID: 1, Kind: dataset.ClassStart, Neighbours: []uint32{2}},
				{ID: 2, Neighbours: []uint32{1, 3}},
				{ID: 3, Kind: dataset.JewelSocket, Neighbours: []uint32{2}},
				{ID: 4, Neighbours: []uint32{5}},
				{ID: 5, Kind: dataset.Notable, Neighbours: []uint32{4, 6}},
				{ID: 6, Neighbours: []uint32{5}},
				{ID: 7, Kind: dataset.Keystone, Neighbours: []uint32{8}},
				{ID: 8, Neighbours: []uint32{7, 9}},
				{ID: 9, Neighbours: []uint32{8}},
			},
			JewelRadii: []dataset.JewelRadius{{ID: "Small", Radius: 800, RingInner: 800, RingOuter: 1100}},
		}},
		Layouts: []dataset.Layout{{Variant: "standard", Orbits: []dataset.Orbit{{Slots: 1}}, Groups: groups, Nodes: layout}},
	})
}

// unconnectedJewel is a jewel with stats, as Thread of Hope, Intuitive Leap and Impossible
// Escape hold them.
func unconnectedJewel(stats ...build.StatValue) build.Item {
	return build.Item{
		Rarity: build.Unique, Base: "Metadata/Items/Jewels/JewelStr",
		Mods: []build.ModLine{{Kind: build.Explicit, Text: "unconnected", Stats: stats}},
	}
}

const nearbyUnconnected = "local_unique_jewel_nearby_disconnected_passives_can_be_allocated"

func intuitiveLeap() build.Item {
	return unconnectedJewel(build.StatValue{Stat: nearbyUnconnected, Value: 1})
}

// threadOfHope reaches the Small ring.
func threadOfHope() build.Item {
	return unconnectedJewel(build.StatValue{Stat: "local_jewel_variable_ring_radius_value", Value: 1}, build.StatValue{Stat: nearbyUnconnected, Value: 1})
}

// impossibleEscape reaches around keystone 7.
func impossibleEscape() build.Item {
	return unconnectedJewel(build.StatValue{Stat: "local_unique_jewel_disconnected_passives_can_be_allocated_around_keystone_hash", Value: 7})
}

// A jewel in a socket joined to the start connects the passives it reaches, and paths from them;
// any other allocated node warns, and so do the nodes a jewel held once it is gone.
func TestCheckUnconnectedPassives(t *testing.T) {
	t.Parallel()
	s := unconnectedStore(t)
	disconnected := func(subject string, nodes ...uint32) []build.Warning {
		return []build.Warning{{
			Code: build.NodesDisconnected, Subject: subject, Nodes: nodes,
			Message: "not connected to the class, ascendancy or bloodline start; kept",
		}}
	}
	testCases := []struct {
		name  string
		nodes []uint32
		jewel func() build.Item
		want  []build.Warning
	}{
		{name: "Intuitive Leap's radius and a path from it", nodes: []uint32{1, 2, 3, 4, 5, 6}, jewel: intuitiveLeap},
		{name: "outside Intuitive Leap's radius", nodes: []uint32{1, 2, 3, 5, 6}, jewel: intuitiveLeap, want: disconnected("nodes 5, 6", 5, 6)},
		{name: "Thread of Hope's ring", nodes: []uint32{1, 2, 3, 5, 6}, jewel: threadOfHope},
		{name: "inside Thread of Hope's ring", nodes: []uint32{1, 2, 3, 4}, jewel: threadOfHope, want: disconnected("nodes 4", 4)},
		{name: "Impossible Escape's keystone and its radius", nodes: []uint32{1, 2, 3, 7, 8, 9}, jewel: impossibleEscape},
		{name: "outside Impossible Escape's radius", nodes: []uint32{1, 2, 3, 9}, jewel: impossibleEscape, want: disconnected("nodes 9", 9)},
		{name: "socket not allocated", nodes: []uint32{1, 2, 4, 5}, jewel: intuitiveLeap, want: disconnected("nodes 4, 5", 4, 5)},
		{name: "socket not joined to the start", nodes: []uint32{1, 3, 4}, jewel: intuitiveLeap, want: disconnected("nodes 3, 4", 3, 4)},
		{name: "jewel removed", nodes: []uint32{1, 2, 3, 4, 5, 6}, want: disconnected("nodes 4, 5, 6", 4, 5, 6)},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			b := &build.Build{Format: build.Format, Game: "poe1", Variant: "standard", Class: marauder, Level: 90, Nodes: testCase.nodes}
			if testCase.jewel != nil {
				b.Jewels = build.Jewels{3: testCase.jewel()}
			}
			_, warnings := check(t, b, s, build.NodesDisconnected)
			if diff := cmp.Diff(testCase.want, warnings, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("warnings (-want +got):\n%s", diff)
			}
		})
	}
}

// Deallocating the path to a jewel's socket takes the passives the jewel held; deallocating
// the passive a jewel connects takes the path from it.
func TestDeallocateBranchUnconnected(t *testing.T) {
	t.Parallel()
	s := unconnectedStore(t)
	b := &build.Build{
		Format: build.Format, Game: "poe1", Variant: "standard", Class: marauder, Level: 90,
		Nodes: []uint32{1, 2, 3, 4, 5, 6}, Jewels: build.Jewels{3: intuitiveLeap()},
	}
	c, _ := check(t, b, s)
	for node, want := range map[uint32][]uint32{2: {2, 3, 4, 5, 6}, 4: {4, 5, 6}, 5: {5, 6}} {
		edits, ok := c.DeallocateBranch(node)
		if !ok {
			t.Fatalf("DeallocateBranch(%d) not ok", node)
		}
		if diff := cmp.Diff([]build.Edit{build.Deallocate(want...)}, edits); diff != "" {
			t.Errorf("DeallocateBranch(%d) (-want +got):\n%s", node, diff)
		}
	}
}
