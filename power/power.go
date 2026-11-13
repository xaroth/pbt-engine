// Package power computes node power: for every node a build could allocate next, what it
// changes on its own and through the shortest path to it. What counts as power (life, a ratio,
// per point) is the caller's: each result carries the change of every asked output.
package power

import (
	"cmp"
	"fmt"
	"slices"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/output"
	"github.com/xaroth/pbt-engine/simulator"
)

// NodePower is one candidate: a node, or a mastery node with one of its effects.
type NodePower struct {
	Node uint32
	// Effect is the mastery effect, 0 for a node that is not a mastery.
	Effect uint32
	// Alone allocates only the candidate (and chooses the effect), connected or not.
	Alone simulator.Result
	// Path is the shortest path of unallocated nodes from the tree to the candidate, the
	// candidate last; empty for another effect of an allocated mastery. Its length is the
	// candidate's distance, which MaxDistance limits.
	Path []uint32
	// PassivePoints and AscendancyPoints are what the path costs from each budget (level and
	// quests; labyrinths): ascendancy nodes cost ascendancy points, the option of a choice node
	// costs nothing (its parent paid), every other node a passive point.
	PassivePoints, AscendancyPoints int
	// ViaPath allocates the whole path (and chooses the effect).
	ViaPath simulator.Result
}

// Option configures Nodes.
type Option func(*options)

type options struct {
	maxDistance int
	sweep       []simulator.Option
}

// MaxDistance keeps only candidates whose shortest path allocates at most n nodes, the
// candidate included, whatever budget each comes from. It applies to both results: a far candidate gets neither. 0 or less is
// no limit.
func MaxDistance(n int) Option { return func(o *options) { o.maxDistance = n } }

// Outputs names the outputs each result reports; default every output.
func Outputs(keys ...output.Key) Option {
	return func(o *options) { o.sweep = append(o.sweep, simulator.Outputs(keys...)) }
}

// Workers sets the sweep's workers, as simulator.Workers.
func Workers(n int) Option {
	return func(o *options) { o.sweep = append(o.sweep, simulator.Workers(n)) }
}

// Nodes sweeps every candidate of base: each node reachable on the tree from the allocated
// nodes and the class and ascendancy starts that is not allocated, and each effect of each
// such mastery and the other effects of allocated masteries. Results are sorted by node, then
// effect. It fails as simulator.Sweep does for the base.
func Nodes(sim *simulator.Sim, base *build.Build, opts ...Option) ([]NodePower, error) {
	var o options
	for _, f := range opts {
		f(&o)
	}
	c, _, err := build.Check(base, sim.Store())
	if err != nil {
		return nil, fmt.Errorf("power: %w: %w", simulator.ErrUnreconcilable, err)
	}
	cands := candidates(c, o.maxDistance)
	variants := make([][]build.Edit, 0, resultsPerCandidate*len(cands))
	for _, cd := range cands {
		variants = append(variants, cd.alone())
		if cd.hasPath() {
			variants = append(variants, cd.viaPath())
		}
	}
	results, err := sim.Sweep(base, variants, o.sweep...)
	if err != nil {
		return nil, fmt.Errorf("power: %w", err)
	}
	out := make([]NodePower, 0, len(cands))
	i := 0
	for _, cd := range cands {
		np := NodePower{
			Node: cd.node, Effect: cd.effect, Path: cd.path, PassivePoints: cd.passive, AscendancyPoints: cd.ascendancy, Alone: results[i],
		}
		i++
		np.ViaPath = np.Alone
		if cd.hasPath() {
			np.ViaPath = results[i]
			i++
		}
		out = append(out, np)
	}
	return out, nil
}

// resultsPerCandidate is alone and via the path.
const resultsPerCandidate = 2

// candidate is one node or mastery effect, its shortest path and what the path costs.
type candidate struct {
	node, effect        uint32
	path                []uint32
	passive, ascendancy int
}

func (cd candidate) alone() []build.Edit {
	edits := []build.Edit{build.Allocate(cd.node)}
	if cd.effect != 0 {
		edits = append(edits, build.SetMastery(cd.node, cd.effect))
	}
	return edits
}

// hasPath reports whether the path differs from the candidate alone.
func (cd candidate) hasPath() bool { return len(cd.path) > 1 }

func (cd candidate) viaPath() []build.Edit {
	edits := []build.Edit{build.Allocate(cd.path...)}
	if cd.effect != 0 {
		edits = append(edits, build.SetMastery(cd.node, cd.effect))
	}
	return edits
}

// candidates walks the tree (see walk) and lists a candidate per reached node, per effect of
// each reached mastery, and per other effect of each allocated mastery, sorted by node and
// effect.
func candidates(c *build.Checked, limit int) []candidate {
	w := newWalk(c)
	reached := w.run(limit)
	var out []candidate
	for _, id := range reached {
		path := pathTo(id, w.parent, w.depth)
		passive, ascendancy := w.cost(path)
		n := w.node(id)
		if n.Kind != dataset.Mastery {
			out = append(out, candidate{node: id, path: path, passive: passive, ascendancy: ascendancy})
			continue
		}
		for _, e := range n.MasteryEffects {
			out = append(out, candidate{node: id, effect: e.ID, path: path, passive: passive, ascendancy: ascendancy})
		}
	}
	for _, m := range c.Masteries() {
		for _, e := range m.Node.MasteryEffects {
			if e.ID != m.Effect.ID {
				out = append(out, candidate{node: m.Node.ID, effect: e.ID})
			}
		}
	}
	slices.SortFunc(out, func(a, b candidate) int { return cmp.Or(cmp.Compare(a.node, b.node), cmp.Compare(a.effect, b.effect)) })
	return out
}

// walk is a breadth-first walk of the tree from the allocated nodes and the starts, over the
// nodes the class, ascendancy and bloodline can take, visiting neighbours in id order, so of
// equal-length paths the one through lower ids wins. Masteries end a path but are not walked
// through: their edges say which notables unlock them.
type walk struct {
	tree    *dataset.Tree
	checked *build.Checked
	parent  map[uint32]uint32
	depth   map[uint32]int
	queue   []uint32
}

func newWalk(c *build.Checked) *walk {
	cb := c.Build()
	w := &walk{tree: c.Tree(), checked: c, parent: map[uint32]uint32{}, depth: map[uint32]int{}}
	for _, id := range c.Starts() {
		w.seed(id)
	}
	for _, id := range cb.Nodes {
		w.seed(id)
	}
	return w
}

func (w *walk) seed(id uint32) {
	if _, ok := w.depth[id]; !ok {
		w.depth[id] = 0
		w.queue = append(w.queue, id)
	}
}

func (w *walk) node(id uint32) *dataset.Node {
	j, ok := slices.BinarySearchFunc(w.tree.Nodes, id, func(n dataset.Node, id uint32) int { return cmp.Compare(n.ID, id) })
	if !ok {
		return nil
	}
	return &w.tree.Nodes[j]
}

// cost returns the passive and ascendancy points path costs, as build.Checked.Points counts.
func (w *walk) cost(path []uint32) (passive, ascendancy int) {
	for _, id := range path {
		switch n := w.node(id); {
		case n.MultipleChoiceOption:
		case n.Ascendancy != "":
			ascendancy++
		default:
			passive++
		}
	}
	return passive, ascendancy
}

// takeable reports whether the walk may step from node from, a seed or not, to its neighbour n.
func (w *walk) takeable(from, n *dataset.Node, fromSeed bool) bool {
	return n != nil && n.Kind != dataset.ClassStart && n.Kind != dataset.AscendancyStart &&
		w.checked.OwnsAscendancy(n.Ascendancy) && build.Crosses(from, n, fromSeed)
}

// run walks to depth limit (no limit when 0 or less) and returns the nodes reached beyond the
// seeds, in walk order.
func (w *walk) run(limit int) []uint32 {
	var reached []uint32
	for len(w.queue) > 0 {
		id := w.queue[0]
		w.queue = w.queue[1:]
		n := w.node(id)
		if n == nil || n.Kind == dataset.Mastery || (limit > 0 && w.depth[id] >= limit) {
			continue
		}
		for _, nb := range n.Neighbours {
			if _, seen := w.depth[nb]; seen || !w.takeable(n, w.node(nb), w.depth[id] == 0) {
				continue
			}
			w.depth[nb], w.parent[nb] = w.depth[id]+1, id
			w.queue = append(w.queue, nb)
			reached = append(reached, nb)
		}
	}
	return reached
}

// pathTo follows parents from id back to the tree, returning the unallocated nodes on the way,
// id last.
func pathTo(id uint32, parent map[uint32]uint32, depth map[uint32]int) []uint32 {
	path := make([]uint32, 0, depth[id])
	for n := id; depth[n] > 0; n = parent[n] {
		path = append(path, n)
	}
	slices.Reverse(path)
	return path
}
