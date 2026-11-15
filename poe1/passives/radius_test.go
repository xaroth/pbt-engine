package passives_test

import (
	"errors"
	"math"
	"slices"
	"testing"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/internal/realdata"
	"github.com/xaroth/pbt-engine/poe1/passives"
)

// Sockets of the 3.29 tree, named by a keystone in their Large radius.
const (
	agnosticSocket = 26196 // between Templar and Witch, by The Agnostic
	ironWillSocket = 6230  // by Scion, Iron Will and Necromantic Aegis
	conduitSocket  = 48768 // by Scion, Conduit and Solipsism
	theAgnostic    = 19732
)

func TestNodesInRadius(t *testing.T) {
	t.Parallel()
	reader := passives.New(realdata.Open(t, realdata.Current))
	large := passives.Radius{ID: "Large"}
	for socket, want := range map[uint32]int{agnosticSocket: 79, ironWillSocket: 100, conduitSocket: 96} {
		nodes, err := reader.NodesInRadius(build.DefaultVariant, socket, large)
		if err != nil {
			t.Fatal(err)
		}
		if len(nodes) != want {
			t.Errorf("socket %d: %d nodes in Large radius, want %d", socket, len(nodes), want)
		}
	}

	// The Large ring starts at 1400, outside the Medium disc of 1200.
	ring, err := reader.NodesInRadius(build.DefaultVariant, agnosticSocket, passives.Radius{ID: "Large", Ring: true})
	if err != nil {
		t.Fatal(err)
	}
	inner, err := reader.NodesInRadius(build.DefaultVariant, agnosticSocket, passives.Radius{ID: "Medium"})
	if err != nil {
		t.Fatal(err)
	}
	if len(ring) != 33 || len(inner) == 0 {
		t.Errorf("Large ring: %d nodes, want 33; Medium disc: %d nodes", len(ring), len(inner))
	}
	for _, n := range ring {
		if slices.Contains(inner, n) {
			t.Errorf("node %d is in the Large ring and the Medium disc", n)
		}
	}

	if _, err := reader.NodesInRadius(build.DefaultVariant, theAgnostic, large); !errors.Is(err, passives.ErrNotSocket) {
		t.Errorf("The Agnostic as socket: %v, want ErrNotSocket", err)
	}
	if _, err := reader.NodesInRadius(build.DefaultVariant, agnosticSocket, passives.Radius{ID: "Huge"}); !errors.Is(err, dataset.ErrNotInDataset) {
		t.Errorf("unknown radius: %v, want ErrNotInDataset", err)
	}
}

// TestNodesInRadiusLeavesOutClusters checks that no radius reaches the nodes drawn in a cluster
// jewel socket's group, though the disc of some covers them. The passives a cluster jewel adds
// are not nodes of the tree, so no radius can reach them.
func TestNodesInRadiusLeavesOutClusters(t *testing.T) {
	t.Parallel()
	store := realdata.Open(t, realdata.Current)
	tree, err := store.Tree(build.DefaultVariant)
	if err != nil {
		t.Fatal(err)
	}
	layout, err := store.Layout(build.DefaultVariant)
	if err != nil {
		t.Fatal(err)
	}
	proxy := map[uint32]bool{}
	for _, g := range layout.Groups {
		if g.Proxy {
			proxy[g.ID] = true
		}
	}
	var inCluster []uint32
	for _, n := range layout.Nodes {
		if proxy[n.Group] {
			inCluster = append(inCluster, n.ID)
		}
	}
	// The tree export draws a Large radius at 1.2 times its value.
	const large = 1.2 * 1500
	reader := passives.New(store)
	covered := 0
	for _, socket := range tree.JewelSlots {
		nodes, err := reader.NodesInRadius(build.DefaultVariant, socket, passives.Radius{ID: "Large"})
		if err != nil {
			t.Fatal(err)
		}
		sx, sy, _ := layout.Position(socket)
		for _, id := range inCluster {
			if slices.Contains(nodes, id) {
				t.Errorf("socket %d reaches node %d of a cluster socket's group", socket, id)
			}
			if x, y, ok := layout.Position(id); ok && id != socket && math.Hypot(x-sx, y-sy) <= large {
				covered++
			}
		}
	}
	if covered == 0 {
		t.Error("no Large disc covers a node of a cluster socket's group; the test checks nothing")
	}
}
