package simulator

import (
	"fmt"
	"sync"

	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/internal/poe1/radius"
)

// ErrNotSocket is a node that is not a jewel socket on the tree.
var ErrNotSocket = radius.ErrNotSocket

// Radius is the part of the tree a jewel reaches around its socket: the disc of the
// dataset.JewelRadius with ID, such as "Large", or with Ring the ring of it that Thread of Hope
// reaches, which leaves out the disc inside.
type Radius struct {
	ID   string
	Ring bool
}

// radii index the nodes around jewel sockets per tree variant, each made on first use.
type radii struct {
	mu        sync.Mutex
	byVariant map[string]func() (*radius.Index, error)
}

func (r *radii) get(store *dataset.Store, variant string) (*radius.Index, error) {
	r.mu.Lock()
	f, ok := r.byVariant[variant]
	if !ok {
		if r.byVariant == nil {
			r.byVariant = map[string]func() (*radius.Index, error){}
		}
		f = sync.OnceValues(func() (*radius.Index, error) {
			tree, err := store.Tree(variant)
			if err != nil {
				return nil, err //nolint:wrapcheck // wrapped by the caller
			}
			layout, err := store.Layout(variant)
			if err != nil {
				return nil, err //nolint:wrapcheck // wrapped by the caller
			}
			return radius.New(tree, layout), nil
		})
		r.byVariant[variant] = f
	}
	r.mu.Unlock()
	return f()
}

// NodesInRadius returns the passives of the tree variant, such as build.DefaultVariant, that a
// jewel in socket reaches with r, sorted by id. They are the nodes whose drawn position lies
// within r of the socket's, bounds included, among the normal passives, notables, keystones and
// jewel sockets outside ascendancies and cluster jewels; the passives a cluster jewel adds are
// not covered. An unknown variant or radius, or a dataset without the layout, returns an error
// matching dataset.ErrNotInDataset, and a node that is not a jewel socket on the tree
// ErrNotSocket.
func (s *Sim) NodesInRadius(variant string, socket uint32, r Radius) ([]uint32, error) {
	ix, err := s.radii.get(s.store, variant)
	if err != nil {
		return nil, fmt.Errorf("simulator: %w", err)
	}
	tree, err := s.store.Tree(variant)
	if err != nil {
		return nil, fmt.Errorf("simulator: %w", err)
	}
	var area radius.Area
	found := false
	for _, jr := range tree.JewelRadii {
		if jr.ID == r.ID {
			area, found = radius.Disc(jr), true
			if r.Ring {
				area = radius.Ring(jr)
			}
		}
	}
	if !found {
		return nil, fmt.Errorf("simulator: jewel radius %q in the %s tree: %w", r.ID, variant, dataset.ErrNotInDataset)
	}
	nodes, err := ix.Nodes(socket, area)
	if err != nil {
		return nil, fmt.Errorf("simulator: %w", err)
	}
	return nodes, nil
}
