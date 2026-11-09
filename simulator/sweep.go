package simulator

import (
	"cmp"
	"fmt"
	"iter"
	"maps"
	"math"
	"runtime"
	"slices"
	"sync"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/config"
	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/output"
	"github.com/xaroth/pbt-engine/rules/poe1"
)

// Result is one variant of a sweep: its index in the variants, its edits, the change of each
// output against the base build, and its warnings. A variant that cannot be simulated has Err
// and no changes; the sweep goes on.
type Result struct {
	Index    int
	Edits    []build.Edit
	Changes  map[output.Key]Change
	Warnings []Warning
	Err      error
}

// Sweep simulates each variant (edits applied to a clone of base) and returns the results in
// variant order, or ranked with RankBy. One config snapshot serves the whole sweep. It fails
// only for the base build (ErrUnreconcilable) or a bad option.
func (s *Sim) Sweep(base *build.Build, variants [][]build.Edit, opts ...Option) ([]Result, error) {
	seq, err := s.SweepEach(base, variants, opts...)
	if err != nil {
		return nil, err
	}
	out := make([]Result, len(variants))
	for r := range seq {
		out[r.Index] = r
	}
	co, _ := readCallOptions(opts)
	if co.rankBy != "" {
		slices.SortStableFunc(out, func(a, b Result) int {
			return cmp.Compare(rankValue(b, co.rankBy), rankValue(a, co.rankBy))
		})
	}
	return out, nil
}

// rankValue is a result's change in k; failed results sort last.
func rankValue(r Result, k output.Key) float64 {
	if c, ok := r.Changes[k]; ok && r.Err == nil {
		return c.Abs
	}
	return math.Inf(-1)
}

// SweepEach is Sweep streamed: results arrive as workers finish them, each with its Index.
// Stopping the iteration stops the workers.
func (s *Sim) SweepEach(base *build.Build, variants [][]build.Edit, opts ...Option) (iter.Seq[Result], error) {
	sw, err := s.newSweep(base, opts)
	if err != nil {
		return nil, err
	}
	return func(yield func(Result) bool) {
		jobs := make(chan int)
		results := make(chan Result)
		done := make(chan struct{})
		var wg sync.WaitGroup
		for range min(sw.workers, max(len(variants), 1)) {
			wg.Go(func() { sw.work(variants, jobs, results, done) })
		}
		go func() {
			defer close(jobs)
			for i := range variants {
				select {
				case jobs <- i:
				case <-done:
					return
				}
			}
		}()
		go func() { wg.Wait(); close(results) }()
		defer func() {
			close(done)
			// drain so the workers can finish
			for range results {
			}
		}()
		for r := range results {
			if !yield(r) {
				return
			}
		}
	}, nil
}

// sweep is a sweep's shared state: the base run, its values, the outputs and the options.
type sweep struct {
	sim      *Sim
	base     *run
	keys     []output.Key
	before   map[output.Key]float64
	co       callOptions
	snapshot config.Values
	workers  int
	tree     *dataset.Tree
}

func (s *Sim) newSweep(base *build.Build, opts []Option) (*sweep, error) {
	co, err := readCallOptions(opts)
	if err != nil {
		return nil, err
	}
	asked := co.outputs
	if co.rankBy != "" && len(asked) > 0 && !slices.Contains(asked, co.rankBy) {
		asked = append(slices.Clone(asked), co.rankBy)
	}
	keys, err := outputKeys(asked)
	if err != nil {
		return nil, err
	}
	snapshot := s.Config()
	r, err := s.prepare(base, co, snapshot)
	if err != nil {
		return nil, err
	}
	sh, err := s.checkout(r.checked)
	if err != nil {
		return nil, err
	}
	st, err := s.statsOf(r, sh, keys)
	s.checkin(r.shape, sh)
	if err != nil {
		return nil, err
	}
	workers := co.workers
	if workers <= 0 {
		workers = runtime.GOMAXPROCS(0)
	}
	return &sweep{sim: s, base: r, keys: keys, before: st.values, co: co, snapshot: snapshot, workers: workers, tree: r.checked.Tree()}, nil
}

// work runs variants from jobs on its own sheet of the base's shape, switched to the base.
func (sw *sweep) work(variants [][]build.Edit, jobs <-chan int, results chan<- Result, done <-chan struct{}) {
	sh, err := sw.sim.checkout(sw.base.checked)
	if err == nil {
		err = switchTo(sw.base, sh)
	}
	clean := true
	defer func() {
		if sh != nil && clean {
			sw.sim.checkin(sw.base.shape, sh)
		}
	}()
	for i := range jobs {
		var r Result
		if err != nil {
			r = Result{Err: err}
		} else {
			clean = false
			r = sw.variant(sh, variants[i])
			clean = true
		}
		r.Index, r.Edits = i, slices.Clone(variants[i])
		select {
		case results <- r:
		case <-done:
			return
		}
	}
}

// variant runs one variant: on the fast path when its edits only switch nodes and mastery
// effects in a way the build check would not change, else through a full simulation.
func (sw *sweep) variant(sh *poe1.Sheet, edits []build.Edit) Result {
	if plan, ok := sw.fastPlan(edits); ok {
		return sw.fast(sh, plan)
	}
	return sw.full(edits)
}

// switchPlan is what a fast variant switches on the base sheet, and the warnings of edits the
// check would drop.
type switchPlan struct {
	nodesOn, nodesOff     []uint32
	effectsOn, effectsOff [][2]uint32
	warnings              []Warning
}

// fastPlan turns node and mastery edits into switches, as build.Check would treat the edited
// build: unknown and other ascendancies' nodes do nothing (the sheet does not have them),
// mastery choices count only on allocated masteries. It reports false for any other edit, and
// for edits the check would change values for: an option of a multiple-choice node, or a
// mastery effect chosen twice.
func (sw *sweep) fastPlan(edits []build.Edit) (switchPlan, bool) {
	cb := sw.base.checked.Build()
	nodes := map[uint32]bool{}
	for _, n := range cb.Nodes {
		nodes[n] = true
	}
	masteries := make(map[uint32]uint32, len(cb.Masteries))
	maps.Copy(masteries, cb.Masteries)
	var p switchPlan
	for _, e := range edits {
		if !sw.applyFast(e, nodes, masteries, &p) {
			return p, false
		}
	}
	// a mastery effect chosen twice is the check's to resolve
	seen := map[uint32]bool{}
	for n, e := range masteries {
		if nodes[n] {
			if seen[e] {
				return p, false
			}
			seen[e] = true
		}
	}
	sw.diffPlan(cb, nodes, masteries, &p)
	return p, true
}

func (sw *sweep) applyFast(e build.Edit, nodes map[uint32]bool, masteries map[uint32]uint32, p *switchPlan) bool {
	switch e.Op {
	case build.OpAllocate:
		for _, id := range e.Nodes {
			n := sw.node(id)
			if n == nil {
				p.warnings = append(p.warnings, Warning{Code: build.NodeUnknown, Subject: fmt.Sprintf("node %d", id), Message: "not in the tree; dropped"})
				continue
			}
			if n.MultipleChoiceOption {
				return false
			}
			nodes[id] = true
		}
	case build.OpDeallocate:
		for _, id := range e.Nodes {
			delete(nodes, id)
		}
	case build.OpSetMastery:
		masteries[e.Node] = e.Effect
	case build.OpClearMastery:
		delete(masteries, e.Node)
	default:
		return false
	}
	return true
}

// diffPlan lists the switches from the base to nodes and masteries.
func (sw *sweep) diffPlan(cb build.Build, nodes map[uint32]bool, masteries map[uint32]uint32, p *switchPlan) {
	diffNodes(cb, nodes, p)
	diffEffects(cb, nodes, masteries, p)
}

func diffNodes(cb build.Build, nodes map[uint32]bool, p *switchPlan) {
	for id := range nodes {
		if !slices.Contains(cb.Nodes, id) {
			p.nodesOn = append(p.nodesOn, id)
		}
	}
	for _, id := range cb.Nodes {
		if !nodes[id] {
			p.nodesOff = append(p.nodesOff, id)
		}
	}
}

func diffEffects(cb build.Build, nodes map[uint32]bool, masteries map[uint32]uint32, p *switchPlan) {
	for n, e := range cb.Masteries {
		if masteries[n] != e || !nodes[n] {
			p.effectsOff = append(p.effectsOff, [2]uint32{n, e})
		}
	}
	for n, e := range masteries {
		if nodes[n] && (cb.Masteries[n] != e || !slices.Contains(cb.Nodes, n)) {
			p.effectsOn = append(p.effectsOn, [2]uint32{n, e})
		}
	}
}

func (sw *sweep) node(id uint32) *dataset.Node {
	j, ok := slices.BinarySearchFunc(sw.tree.Nodes, id, func(n dataset.Node, id uint32) int { return cmp.Compare(n.ID, id) })
	if !ok {
		return nil
	}
	return &sw.tree.Nodes[j]
}

// fast switches the plan on sh in a checkpoint, reads the outputs and reverts.
func (sw *sweep) fast(sh *poe1.Sheet, p switchPlan) Result {
	cp := sh.Mark()
	for _, id := range p.nodesOff {
		sh.SetNode(id, false)
	}
	for _, ne := range p.effectsOff {
		sh.SetMasteryEffect(ne[0], ne[1], false)
	}
	for _, id := range p.nodesOn {
		sh.SetNode(id, true)
	}
	for _, ne := range p.effectsOn {
		sh.SetMasteryEffect(ne[0], ne[1], true)
	}
	r := Result{Changes: make(map[output.Key]Change, len(sw.keys)), Warnings: p.warnings}
	for _, k := range sw.keys {
		v, err := sh.Output(k)
		if err != nil {
			r.Err = fmt.Errorf("simulator: %w", err)
			break
		}
		r.Changes[k] = change(sw.before[k], v)
	}
	if err := sh.Revert(cp); err != nil && r.Err == nil {
		r.Err = fmt.Errorf("simulator: %w", err)
	}
	sh.Commit()
	return r
}

// full simulates the edited build with the sweep's options and config snapshot.
func (sw *sweep) full(edits []build.Edit) Result {
	b := sw.base.build.Clone()
	if err := b.Apply(edits...); err != nil {
		return Result{Err: fmt.Errorf("simulator: %w", err)}
	}
	r0, err := sw.sim.prepare(b, sw.co, sw.snapshot)
	if err != nil {
		return Result{Err: err}
	}
	sh, err := sw.sim.checkout(r0.checked)
	if err != nil {
		return Result{Err: err}
	}
	st, err := sw.sim.statsOf(r0, sh, sw.keys)
	sw.sim.checkin(r0.shape, sh)
	if err != nil {
		return Result{Err: err}
	}
	r := Result{Changes: make(map[output.Key]Change, len(sw.keys)), Warnings: st.Warnings()}
	for _, k := range sw.keys {
		r.Changes[k] = change(sw.before[k], st.values[k])
	}
	return r
}

func change(before, after float64) Change {
	c := Change{Before: before, After: after, Abs: after - before, Has: true, Pct: math.NaN()}
	if before != 0 {
		c.Pct = (after - before) / math.Abs(before) * percent
	}
	return c
}
