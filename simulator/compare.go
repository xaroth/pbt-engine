package simulator

import (
	"fmt"
	"maps"
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
	// Edits turn the before build into the after build; nil when they cannot (another game
	// or tree variant).
	Edits []build.Edit
	// DataChanges are the nodes either build allocates whose data differs between the two
	// datasets; empty when both ran on one dataset. A node listed may change no output.
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
		if c := d.Get(k); c.Has && math.Float64bits(c.Before) != math.Float64bits(c.After) {
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
		d.Edits = edits
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

// nodeChanges lists the nodes either side allocates that dataset.Compare finds changed.
func nodeChanges(a, b *Stats) ([]NodeChange, error) {
	variant := a.checked.Build().Variant
	da, err := a.sim.store.Dataset(dataset.Options{Trees: []string{variant}})
	if err != nil {
		return nil, fmt.Errorf("simulator: diff: %w", err)
	}
	db, err := b.sim.store.Dataset(dataset.Options{Trees: []string{b.checked.Build().Variant}})
	if err != nil {
		return nil, fmt.Errorf("simulator: diff: %w", err)
	}
	allocated := map[uint32]bool{}
	for _, n := range slices.Concat(a.checked.Nodes(), b.checked.Nodes()) {
		allocated[n.ID] = true
	}
	var out []NodeChange
	for _, td := range dataset.Compare(da, db).Trees {
		if td.Variant != variant {
			continue
		}
		for _, nd := range td.Changed {
			if allocated[nd.ID] {
				out = append(out, NodeChange{Node: nd.ID, Fields: nd.Fields, Before: nodeStats(da, nd.ID), After: nodeStats(db, nd.ID)})
			}
		}
	}
	slices.SortFunc(out, func(x, y NodeChange) int { return int(x.Node) - int(y.Node) })
	return out, nil
}

func nodeStats(d *dataset.Dataset, id uint32) map[string]int32 {
	for _, n := range d.Trees[0].Nodes {
		if n.ID == id {
			return maps.Clone(d.StatMap(n.Stats))
		}
	}
	return nil
}
