package simulator

import (
	"fmt"
	"math"
	"slices"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/output"
)

// percent turns a ratio into a percentage.
const percent = 100

// Change is one output before and after. Pct is Abs relative to Before, NaN when Before is 0.
type Change struct {
	Before, After, Abs, Pct float64
	// Has says the output was computed on both sides.
	Has bool
}

// NodeChange is an allocated node whose data differs between two datasets: the fields that
// differ and its stats on each side.
type NodeChange struct {
	Node          uint32
	Fields        []string
	Before, After map[string]int32
}

// Diff compares two simulations.
type Diff struct {
	Before, After *Stats
	// Edits turn the before build into the after build, empty for equal builds; nil when they
	// cannot (another game or tree variant).
	Edits []build.Edit
	// DataChanges are the nodes either build allocates whose data differs between the two
	// datasets; empty when both ran on one dataset or on different tree variants. A node
	// listed may change no output.
	DataChanges []NodeChange
}

// Get returns output k's change.
func (d *Diff) Get(k output.Key) Change {
	a, okA := d.Before.Get(k)
	b, okB := d.After.Get(k)
	c := Change{Before: a, After: b, Abs: b - a, Has: okA && okB, Pct: math.NaN()}
	if a != 0 {
		c.Pct = (b - a) / math.Abs(a) * percent
	}
	return c
}

// Changed returns the outputs computed on both sides whose values differ, in the before
// side's order.
func (d *Diff) Changed() []output.Key {
	var out []output.Key
	for _, k := range d.Before.keys {
		if c := d.Get(k); c.Has && differs(c) {
			out = append(out, k)
		}
	}
	return out
}

// Compare simulates a and b with one config snapshot for both. Builds of one shape (tree
// variant, class, ascendancy, level) run on one sheet, switching only the sources that
// differ.
func (s *Sim) Compare(a, b *build.Build, opts ...Option) (*Diff, error) {
	co, err := readCallOptions(opts)
	if err != nil {
		return nil, err
	}
	keys, err := outputKeys(co.outputs)
	if err != nil {
		return nil, err
	}
	snapshot := s.Config()
	ra, err := s.prepare(a, co, snapshot)
	if err != nil {
		return nil, err
	}
	rb, err := s.prepare(b, co, snapshot)
	if err != nil {
		return nil, err
	}
	sa, sb, err := s.runPair(ra, rb, keys)
	if err != nil {
		return nil, err
	}
	return sa.Diff(sb)
}

func (s *Sim) runPair(ra, rb *run, keys []output.Key) (*Stats, *Stats, error) {
	sh, err := s.checkout(ra.checked)
	if err != nil {
		return nil, nil, err
	}
	sa, err := s.statsOf(ra, sh, keys)
	if err != nil {
		s.checkin(ra.shape, sh)
		return nil, nil, err
	}
	if ra.shape != rb.shape {
		s.checkin(ra.shape, sh)
		if sh, err = s.checkout(rb.checked); err != nil {
			return nil, nil, err
		}
	}
	defer s.checkin(rb.shape, sh)
	sb, err := s.statsOf(rb, sh, keys)
	if err != nil {
		return nil, nil, err
	}
	return sa, sb, nil
}

// Diff compares st (before) with other (after), which may come from another sim: another
// dataset, enemy or config. Across datasets it lists the allocated nodes whose data changed.
func (st *Stats) Diff(other *Stats) (*Diff, error) {
	d := &Diff{Before: st, After: other}
	if edits, err := build.Diff(st.build, other.build); err == nil {
		d.Edits = orEmpty(edits)
	}
	if st.sim.store != other.sim.store {
		changes, err := nodeChanges(st, other)
		if err != nil {
			return nil, err
		}
		d.DataChanges = changes
	}
	return d, nil
}

// nodeChanges lists the nodes either side allocates whose data changed between the two
// datasets. Builds on different tree variants have none: their nodes are not the same nodes.
func nodeChanges(a, b *Stats) ([]NodeChange, error) {
	variant := a.checked.Build().Variant
	if b.checked.Build().Variant != variant {
		return nil, nil
	}
	td, err := dataset.CompareTree(a.sim.store, b.sim.store, variant)
	if err != nil {
		return nil, fmt.Errorf("simulator: diff: %w", err)
	}
	allocated := map[uint32]bool{}
	for _, n := range slices.Concat(a.checked.Nodes(), b.checked.Nodes()) {
		allocated[n.ID] = true
	}
	var out []NodeChange
	for _, nd := range td.Changed {
		if !allocated[nd.ID] {
			continue
		}
		before, err := nodeStats(a.sim.store, variant, nd.ID)
		if err != nil {
			return nil, err
		}
		after, err := nodeStats(b.sim.store, variant, nd.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, NodeChange{Node: nd.ID, Fields: nd.Fields, Before: before, After: after})
	}
	slices.SortFunc(out, func(x, y NodeChange) int { return int(x.Node) - int(y.Node) })
	return out, nil
}

// nodeStats returns a node's stats by stat id.
func nodeStats(s *dataset.Store, variant string, id uint32) (map[string]int32, error) {
	stats, err := s.Stats()
	if err != nil {
		return nil, fmt.Errorf("simulator: diff: %w", err)
	}
	tree, err := s.Tree(variant)
	if err != nil {
		return nil, fmt.Errorf("simulator: diff: %w", err)
	}
	for _, n := range tree.Nodes {
		if n.ID != id {
			continue
		}
		out := make(map[string]int32, len(n.Stats))
		for _, sv := range n.Stats {
			out[stats[sv.Stat].ID] = sv.Value
		}
		return out, nil
	}
	return nil, fmt.Errorf("simulator: diff: node %d in the %s tree: %w", id, variant, dataset.ErrNotInDataset)
}
