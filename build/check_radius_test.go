package build_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/dataset"
)

// A tree without jewel radii, as a dataset older than them, still checks: each jewel that needs
// a radius gets one warning and its radius work is left out. With the radii the same build
// checks without those warnings and the jewels work.
func TestCheckWithoutJewelRadii(t *testing.T) {
	t.Parallel()
	const socket = 8
	// The second socket joins socket 3's neighbour, outside the radius of socket 3.
	storeWith := func(radii []dataset.JewelRadius) *dataset.Store {
		d := timelessDataset()
		tree, layout := &d.Trees[0], &d.Layouts[0]
		tree.Nodes = append(tree.Nodes, dataset.Node{ID: socket, Kind: dataset.JewelSocket, Neighbours: []uint32{2}})
		tree.JewelRadii = radii
		layout.Groups = append(layout.Groups, dataset.LayoutGroup{ID: socket, X: 9000})
		layout.Nodes = append(layout.Nodes, dataset.LayoutNode{ID: socket, Group: socket, Icon: "n.png"})
		return openStore(t, d)
	}
	b := func() *build.Build {
		thread := threadOfHope()
		thread.Name = "Thread of Hope"
		eternal := timelessJewel(tlEternal, 0)
		eternal.Name = "Elegant Hubris"
		return &build.Build{
			Format: build.Format, Game: "poe1", Variant: "standard", Class: marauder, Level: 90,
			Nodes: []uint32{1, 2, 3, socket}, Jewels: build.Jewels{3: eternal, socket: thread},
		}
	}

	c, warnings := check(t, b(), storeWith(nil), build.RadiusMissing)
	want := []build.Warning{
		{
			Code: build.RadiusMissing, Subject: "jewel 3", Nodes: []uint32{3},
			Message: "Elegant Hubris: the standard tree has no Large jewel radius; what it does in its radius is left out",
		},
		{
			Code: build.RadiusMissing, Subject: "jewel 8", Nodes: []uint32{8},
			Message: "Thread of Hope: the standard tree has no Small jewel radius; what it does in its radius is left out",
		},
	}
	if diff := cmp.Diff(want, warnings); diff != "" {
		t.Errorf("warnings without radii (-want +got):\n%s", diff)
	}
	if got := c.Conquests(); len(got) != 0 {
		t.Errorf("Conquests without radii = %d, want none", len(got))
	}

	radii := []dataset.JewelRadius{{ID: "Small", Radius: 800, RingInner: 800, RingOuter: 1100}, {ID: "Large", Radius: 1500}}
	c, warnings = check(t, b(), storeWith(radii), build.RadiusMissing)
	if len(warnings) != 0 {
		t.Errorf("warnings with radii = %v, want none", warnings)
	}
	if got := c.Conquests(); len(got) == 0 {
		t.Error("Conquests with radii = none, want the Elegant Hubris's")
	}
}
