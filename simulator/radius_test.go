package simulator_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/internal/realdata"
	"github.com/xaroth/pbt-engine/simulator"
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
	sim, err := simulator.New(realdata.Open(t, realdata.Current))
	if err != nil {
		t.Fatal(err)
	}
	large := simulator.Radius{ID: "Large"}
	for socket, want := range map[uint32]int{agnosticSocket: 79, ironWillSocket: 100, conduitSocket: 96} {
		nodes, err := sim.NodesInRadius(build.DefaultVariant, socket, large)
		if err != nil {
			t.Fatal(err)
		}
		if len(nodes) != want {
			t.Errorf("socket %d: %d nodes in Large radius, want %d", socket, len(nodes), want)
		}
	}

	// The Large ring starts at 1400, outside the Medium disc of 1200.
	ring, err := sim.NodesInRadius(build.DefaultVariant, agnosticSocket, simulator.Radius{ID: "Large", Ring: true})
	if err != nil {
		t.Fatal(err)
	}
	inner, err := sim.NodesInRadius(build.DefaultVariant, agnosticSocket, simulator.Radius{ID: "Medium"})
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

	if _, err := sim.NodesInRadius(build.DefaultVariant, theAgnostic, large); !errors.Is(err, simulator.ErrNotSocket) {
		t.Errorf("The Agnostic as socket: %v, want ErrNotSocket", err)
	}
	if _, err := sim.NodesInRadius(build.DefaultVariant, agnosticSocket, simulator.Radius{ID: "Huge"}); !errors.Is(err, dataset.ErrNotInDataset) {
		t.Errorf("unknown radius: %v, want ErrNotInDataset", err)
	}
}
