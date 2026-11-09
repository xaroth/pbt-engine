package core

import (
	"fmt"
	"math"
	"slices"
	"strings"
)

// percent is what a percentage value is divided by to give a fraction.
const percent = 100.0

func (s *Session) get(id uint32) float64 {
	r := s.rec(id)
	if r.busy {
		s.cycle(id)
		return 0
	}
	if s.err != nil {
		return 0
	}
	if r.epoch == s.epoch {
		if r.verified == s.rev {
			return r.value
		}
		r.busy = true
		s.stack = append(s.stack, id)
		changed := s.changedSince(id, r)
		s.stack = s.stack[:len(s.stack)-1]
		r.busy = false
		if !changed {
			r.verified = s.rev
			return r.value
		}
	}
	s.compute(id, r)
	return r.value
}

func (s *Session) changedSince(id uint32, r *rec) bool {
	n := s.node(id)
	a := int(n.Actor)
	if s.stamp[int(n.Stat)*s.nActors+a] > r.verified {
		return true
	}
	def := &s.m.stats[n.Stat]
	for _, t := range def.Includes {
		if s.stamp[int(t)*s.nActors+a] > r.verified {
			return true
		}
	}
	for i := range def.Shares {
		if s.stamp[int(def.Shares[i].Stat)*s.nActors+a] > r.verified {
			return true
		}
	}
	for _, dep := range r.deps {
		s.get(dep)
		if s.err != nil || s.rec(dep).changed > r.verified {
			return true
		}
	}
	return false
}

func (s *Session) compute(id uint32, r *rec) {
	valid := r.epoch == s.epoch
	old := math.Float64bits(r.value)
	r.busy = true
	s.stack = append(s.stack, id)
	r.deps = r.deps[:0]
	v := s.eval(id, &r.deps, nil)
	s.stack = s.stack[:len(s.stack)-1]
	r.busy = false
	r.value = v
	r.epoch = s.epoch
	if !valid || math.Float64bits(v) != old {
		r.changed = s.rev
	}
	r.verified = s.rev
}

// CycleError reports a dependency cycle not resolved by a CycleFamily.
type CycleError struct {
	Path  []Node // first and last are the same node
	names []string
}

func (e *CycleError) Error() string {
	var b strings.Builder
	b.WriteString("core: dependency cycle: ")
	for i, n := range e.Path {
		if i > 0 {
			b.WriteString(" -> ")
		}
		fmt.Fprintf(&b, "%s[actor %d ctx %d", e.names[i], n.Actor, n.Ctx)
		if n.Uncond {
			b.WriteString(" uncond")
		}
		b.WriteString("]")
	}
	return b.String()
}

func (s *Session) cycle(id uint32) {
	if s.err != nil {
		return
	}
	start := slices.Index(s.stack, id)
	e := &CycleError{}
	for _, x := range append(s.stack[start:len(s.stack):len(s.stack)], id) {
		n := s.node(x)
		e.Path = append(e.Path, n)
		e.names = append(e.names, s.m.stats[n.Stat].Name)
	}
	s.err = e
}

// Term is one modifier considered for a node.
type Term struct {
	Mod    ModID
	Source SourceID
	Op     Op
	Value  float64 // after scaling and the share rate; 0 when Held is false
	// Stat is the stat the modifier is on: the node's own, one it includes or one it shares.
	Stat StatID
	// Rate is the share rate as a fraction for a shared stat's modifier, else 1.
	Rate float64
	// Held is false for a modifier whose condition failed; Terms leaves those out.
	Held bool
}

// collector receives what an evaluation saw, for Terms and Explain.
type collector struct {
	terms  []Term
	failed bool // also collect terms whose condition failed
	agg    Agg
	total  float64
}

// read resolves ref against the node being evaluated, records the dependency and returns the
// node's value. A read of a stat in family fam (not 0) sees the unconditional value.
func (s *Session) read(deps *[]uint32, ref Ref, fam uint8, actor ActorID, ctx CtxID) float64 {
	// Check before converting: ActorID and CtxID are narrower than Ref's fields.
	if err := s.checkRef("read", ref); err != nil {
		if s.err == nil {
			s.err = err
		}
		return 0
	}
	n := Node{Actor: actor, Stat: ref.Stat, Ctx: ctx}
	if ref.Actor != Self {
		n.Actor = ActorID(ref.Actor) //nolint:gosec // checkRef above rejected values outside ActorID
	}
	if ref.Ctx != Self {
		n.Ctx = CtxID(ref.Ctx) //nolint:gosec // checkRef above rejected values outside CtxID
	}
	n.Uncond = fam != 0 && s.m.stats[n.Stat].CycleFamily == fam
	id := s.nodeID(n)
	if deps != nil {
		*deps = append(*deps, id)
	}
	return s.get(id)
}

// eval computes node id. deps receives every node read; c, when not nil, the terms, aggregate
// and total.
func (s *Session) eval(id uint32, deps *[]uint32, c *collector) float64 {
	n := s.node(id)
	def := &s.m.stats[n.Stat]
	mask := s.masks[n.Ctx]
	agg := Agg{More: 1}
	for k := -1; k < len(def.Includes); k++ {
		t := n.Stat
		if k >= 0 {
			t = def.Includes[k]
		}
		s.sumStat(&agg, t, n, &mask, deps, c)
	}
	for i := range def.Shares {
		sh := &def.Shares[i]
		if rate := s.read(deps, sh.Rate, def.CycleFamily, n.Actor, n.Ctx) / percent; rate != 0 {
			s.sumShare(&agg, sh.Stat, rate, n, &mask, deps, c)
		}
	}
	total := s.total(def, agg, n, deps)
	if c != nil {
		c.agg, c.total = agg, total
	}
	return def.finish(total)
}

// add records modifier mi of stat t as a term of the node being collected; c is not nil.
func (c *collector) add(s *Session, mi uint32, t StatID, op Op, v, rate float64, held bool) {
	if held || c.failed {
		c.terms = append(c.terms, Term{Mod: s.modID(mi), Source: s.mSource[mi], Op: op, Value: v, Stat: t, Rate: rate, Held: held})
	}
}

// sumShare adds to agg the base, increased and more modifiers of stat t that apply to node n,
// at rate times their value. It is sumStat's loop apart, so sumStat stays small enough to
// inline its checks.
func (s *Session) sumShare(agg *Agg, t StatID, rate float64, n Node, mask *Mask, deps *[]uint32, c *collector) {
	fam := s.m.stats[t].CycleFamily
	for _, mi := range s.buckets[int(t)*s.nActors+int(n.Actor)] {
		op := s.mOp[mi]
		if op == OpOverride || op == OpFlag || !s.modApplies(mi, n, mask) {
			continue
		}
		v := s.mValue[mi]
		if !s.modPlain(mi) {
			var ok bool
			if v, ok = s.modValue(mi, n, fam, deps); !ok {
				if c != nil {
					c.add(s, mi, t, op, 0, rate, false)
				}
				continue
			}
		}
		v = float64(v * rate)
		aggregate(agg, op, v)
		if c != nil {
			c.add(s, mi, t, op, v, rate, true)
		}
	}
}

// sumStat adds to agg the modifiers of stat t that apply to node n.
func (s *Session) sumStat(agg *Agg, t StatID, n Node, mask *Mask, deps *[]uint32, c *collector) {
	fam := s.m.stats[t].CycleFamily
	for _, mi := range s.buckets[int(t)*s.nActors+int(n.Actor)] {
		if !s.modLive(mi) || !s.predHolds(mi, mask) || (n.Uncond && !s.modPlain(mi)) {
			continue
		}
		v := s.mValue[mi]
		if !s.modPlain(mi) {
			var ok bool
			if v, ok = s.modValue(mi, n, fam, deps); !ok {
				if c != nil {
					c.add(s, mi, t, s.mOp[mi], 0, 1, false)
				}
				continue
			}
		}
		aggregate(agg, s.mOp[mi], v)
		if c != nil {
			c.add(s, mi, t, s.mOp[mi], v, 1, true)
		}
	}
}

// The checks below run per modifier and are small enough for the compiler to inline.

// modApplies reports whether modifier mi is live, matches mask and, for an unconditional node,
// is plain.
func (s *Session) modApplies(mi uint32, n Node, mask *Mask) bool {
	return s.modLive(mi) && s.predHolds(mi, mask) && (!n.Uncond || s.modPlain(mi))
}

func (s *Session) modLive(mi uint32) bool { return s.mAlive[mi] && !s.srcOff[s.mSource[mi]] }

func (s *Session) predHolds(mi uint32, mask *Mask) bool {
	p := s.mPred[mi]
	return p < 0 || s.preds[p].matches(*mask)
}

// modPlain reports whether modifier mi has no conditions or scales.
func (s *Session) modPlain(mi uint32) bool {
	return s.mCond[mi] == s.mCond[mi+1] && s.mScale[mi] == s.mScale[mi+1]
}

// modValue returns modifier mi's value after scaling; ok is false when a condition fails.
func (s *Session) modValue(mi uint32, n Node, fam uint8, deps *[]uint32) (float64, bool) {
	cs := s.conds[s.mCond[mi]:s.mCond[mi+1]]
	for i := range cs {
		if !cs[i].holds(s.read(deps, cs[i].Ref, fam, n.Actor, n.Ctx)) {
			return 0, false
		}
	}
	v := s.mValue[mi]
	ss := s.scales[s.mScale[mi]:s.mScale[mi+1]]
	for i := range ss {
		sc := &ss[i]
		q := math.Floor(s.read(deps, sc.Ref, fam, n.Actor, n.Ctx) / sc.Step)
		if sc.HasCap {
			q = fmin(q, sc.Cap)
		}
		v = float64(v * q)
	}
	return v, true
}

func aggregate(agg *Agg, op Op, v float64) {
	switch op {
	case OpBase:
		agg.Base += v
	case OpInc:
		agg.Inc += v
	case OpMore:
		agg.More *= 1.0 + v/percent
	case OpOverride:
		agg.Override = v
		agg.HasOverride = true
	case OpFlag:
		agg.Flag = true
	}
}

// total returns a node's value from its aggregate, before the stat's clamp and rounding.
func (s *Session) total(def *StatDef, agg Agg, n Node, deps *[]uint32) float64 {
	switch {
	case agg.HasOverride && (def.Formula == nil || !def.FormulaSeesOverride):
		return agg.Override
	case def.Formula != nil:
		return def.Formula(Reader{s: s, deps: deps, actor: n.Actor, ctx: n.Ctx, fam: def.CycleFamily}, agg)
	case def.Flag:
		if agg.Flag {
			return 1
		}
		return 0
	}
	return agg.Default()
}

// Reader gives a formula access to other nodes. Every read becomes a dependency.
type Reader struct {
	s     *Session
	deps  *[]uint32
	actor ActorID
	ctx   CtxID
	fam   uint8
}

// Actor returns the actor of the node being evaluated.
func (r Reader) Actor() ActorID { return r.actor }

// Ctx returns the context of the node being evaluated.
func (r Reader) Ctx() CtxID { return r.ctx }

// Mask returns the context mask of the node being evaluated.
func (r Reader) Mask() Mask { return r.s.masks[r.ctx] }

// Read returns the value of the referenced node; a stat in the evaluated stat's CycleFamily
// reads its unconditional value.
func (r Reader) Read(ref Ref) float64 { return r.s.read(r.deps, ref, r.fam, r.actor, r.ctx) }

// Stat returns the value of stat on the same actor and context.
func (r Reader) Stat(stat StatID) float64 { return r.Read(SelfRef(stat)) }
