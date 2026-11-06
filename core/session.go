package core

import (
	"fmt"
	"math"
	"slices"
	"strings"
)

// Node addresses a value: a stat of an actor in a context. Uncond nodes are the unconditional
// values used by the CycleBreak rule.
type Node struct {
	Actor  ActorID
	Stat   StatID
	Ctx    CtxID
	Uncond bool
}

type rec struct {
	value                    float64
	epoch, verified, changed uint32
	busy                     bool // being computed or verified; reaching it again is a cycle
	deps                     []uint32
}

type block struct {
	actor  ActorID
	ctx    CtxID
	uncond bool
	recs   []rec
}

// Session is one build: a modifier store over a Model, evaluated lazily and memoised.
// Edits mark what they touch; the next read re-verifies only nodes whose inputs changed.
// Not safe for concurrent use.
type Session struct {
	m       *Model
	nActors int
	nStats  int

	masks  []Mask
	ctxIdx map[Mask]CtxID

	// Modifier store, struct of arrays by ModID.
	mStat   []StatID
	mActor  []ActorID
	mOp     []Op
	mValue  []float64
	mSource []SourceID
	mAlive  []bool
	mPred   []int32
	mCond   []uint32 // offsets into conds, len(mods)+1
	mScale  []uint32 // offsets into scales, len(mods)+1
	conds   []Cond
	scales  []Scale
	preds   []Predicate
	buckets [][]ModID // [stat*nActors+actor], sorted by (source, id)
	srcMods [][]ModID
	srcOff  []bool

	stamp []uint32 // [stat*nActors+actor] revision of the last edit touching that bucket
	rev   uint32
	epoch uint32

	blocks   []block
	blockIdx []int32 // [(ctx*nActors+actor)*2+uncond] -> index into blocks, -1 if absent
	stack    []uint32
	err      error

	journal []edit
}

type editKind uint8

const (
	editSource editKind = iota
	editAdd
	editRemove
	editValue
)

type edit struct {
	kind editKind
	id   uint32
	old  float64
	on   bool
}

// NewSession creates a session with actors 0 to actors-1. The model is frozen from here on.
func NewSession(m *Model, actors int) *Session {
	if actors < 1 || actors > math.MaxUint16 {
		panic(fmt.Sprintf("core: %d actors", actors))
	}
	m.frozen = true
	s := &Session{
		m:       m,
		nActors: actors,
		nStats:  len(m.stats),
		ctxIdx:  map[Mask]CtxID{},
		rev:     1,
		epoch:   1,
		mCond:   []uint32{0},
		mScale:  []uint32{0},
	}
	s.buckets = make([][]ModID, s.nStats*actors)
	s.stamp = make([]uint32, s.nStats*actors)
	s.Context(Mask{})
	return s
}

// Model returns the session's model.
func (s *Session) Model() *Model { return s.m }

// NumActors returns the number of actors.
func (s *Session) NumActors() int { return s.nActors }

// Context interns a context mask. The empty mask is GlobalCtx.
func (s *Session) Context(mask Mask) CtxID {
	if id, ok := s.ctxIdx[mask]; ok {
		return id
	}
	id := CtxID(len(s.masks))
	s.masks = append(s.masks, mask)
	s.ctxIdx[mask] = id
	for range 2 * s.nActors {
		s.blockIdx = append(s.blockIdx, -1)
	}
	return id
}

// ContextMask returns the mask of an interned context.
func (s *Session) ContextMask(c CtxID) Mask { return s.masks[c] }

// ---- evaluation ----

func (s *Session) nodeID(n Node) uint32 {
	k := (int(n.Ctx)*s.nActors + int(n.Actor)) * 2
	if n.Uncond {
		k++
	}
	b := s.blockIdx[k]
	if b < 0 {
		b = int32(len(s.blocks))
		s.blocks = append(s.blocks, block{actor: n.Actor, ctx: n.Ctx, uncond: n.Uncond, recs: make([]rec, s.nStats)})
		s.blockIdx[k] = b
	}
	return uint32(b)*uint32(s.nStats) + uint32(n.Stat)
}

func (s *Session) node(id uint32) Node {
	b := &s.blocks[id/uint32(s.nStats)]
	return Node{Actor: b.actor, Stat: StatID(id % uint32(s.nStats)), Ctx: b.ctx, Uncond: b.uncond}
}

func (s *Session) rec(id uint32) *rec {
	return &s.blocks[id/uint32(s.nStats)].recs[id%uint32(s.nStats)]
}

// Get returns the value of a stat of an actor in a context.
func (s *Session) Get(actor ActorID, stat StatID, ctx CtxID) (float64, error) {
	return s.GetNode(Node{Actor: actor, Stat: stat, Ctx: ctx})
}

// GetNode returns the value of a node.
func (s *Session) GetNode(n Node) (float64, error) {
	if err := s.checkNode(n); err != nil {
		return 0, err
	}
	v := s.get(s.nodeID(n))
	if s.err != nil {
		err := s.err
		s.err = nil
		s.stack = s.stack[:0]
		s.Invalidate()
		return 0, err
	}
	return v, nil
}

func (s *Session) checkNode(n Node) error {
	if int(n.Actor) >= s.nActors {
		return fmt.Errorf("core: actor %d out of range", n.Actor)
	}
	if int(n.Stat) >= s.nStats {
		return fmt.Errorf("core: stat %d out of range", n.Stat)
	}
	if int(n.Ctx) >= len(s.masks) {
		return fmt.Errorf("core: context %d not interned", n.Ctx)
	}
	return nil
}

// Invalidate drops every cached value; the next reads recompute from scratch.
func (s *Session) Invalidate() {
	s.epoch++
	if s.epoch == math.MaxUint32 {
		for i := range s.blocks {
			clear(s.blocks[i].recs)
		}
		s.epoch = 1
	}
}

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
	for _, t := range s.m.stats[n.Stat].Includes {
		if s.stamp[int(t)*s.nActors+a] > r.verified {
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

// CycleError reports a dependency cycle not resolved by a CycleBreak stat family.
type CycleError struct {
	Path []Node // first and last are the same node
	m    *Model
}

func (e *CycleError) Error() string {
	var b strings.Builder
	b.WriteString("core: dependency cycle: ")
	for i, n := range e.Path {
		if i > 0 {
			b.WriteString(" -> ")
		}
		fmt.Fprintf(&b, "%s[actor %d ctx %d", e.m.stats[n.Stat].Name, n.Actor, n.Ctx)
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
	e := &CycleError{m: s.m}
	for _, x := range s.stack[start:] {
		e.Path = append(e.Path, s.node(x))
	}
	e.Path = append(e.Path, s.node(id))
	s.err = e
}

// Term is one modifier that matched a node.
type Term struct {
	Mod    ModID
	Source SourceID
	Op     Op
	Value  float64 // after scaling
}

// read resolves ref against the node being evaluated, records the dependency and returns the
// node's value.
func (s *Session) read(deps *[]uint32, ref Ref, uncond bool, actor ActorID, ctx CtxID) float64 {
	n := Node{Actor: actor, Stat: ref.Stat, Ctx: ctx, Uncond: uncond}
	if ref.Actor != Self {
		n.Actor = ActorID(ref.Actor)
	}
	if ref.Ctx != Self {
		n.Ctx = CtxID(ref.Ctx)
	}
	if err := s.checkNode(n); err != nil {
		if s.err == nil {
			s.err = err
		}
		return 0
	}
	id := s.nodeID(n)
	if deps != nil {
		*deps = append(*deps, id)
	}
	return s.get(id)
}

// eval computes node id. deps receives every node read; terms, when not nil, every matching
// modifier.
func (s *Session) eval(id uint32, deps *[]uint32, terms *[]Term) float64 {
	n := s.node(id)
	def := &s.m.stats[n.Stat]
	mask := s.masks[n.Ctx]
	agg := Agg{More: 1}
	for k := -1; k < len(def.Includes); k++ {
		t := n.Stat
		if k >= 0 {
			t = def.Includes[k]
		}
		tCyc := s.m.stats[t].CycleBreak
		for _, mi := range s.buckets[int(t)*s.nActors+int(n.Actor)] {
			if !s.mAlive[mi] || !s.SourceEnabled(s.mSource[mi]) {
				continue
			}
			if p := s.mPred[mi]; p >= 0 && !s.preds[p].matches(mask) {
				continue
			}
			cs := s.conds[s.mCond[mi]:s.mCond[mi+1]]
			ss := s.scales[s.mScale[mi]:s.mScale[mi+1]]
			if n.Uncond && (len(cs) > 0 || len(ss) > 0) {
				continue
			}
			ok := true
			for i := range cs {
				c := &cs[i]
				u := tCyc && s.m.stats[c.Ref.Stat].CycleBreak
				if !c.holds(s.read(deps, c.Ref, u, n.Actor, n.Ctx)) {
					ok = false
					break
				}
			}
			if !ok {
				continue
			}
			v := s.mValue[mi]
			for i := range ss {
				sc := &ss[i]
				u := tCyc && s.m.stats[sc.Ref.Stat].CycleBreak
				x := s.read(deps, sc.Ref, u, n.Actor, n.Ctx)
				q := math.Floor(x / sc.Step)
				if sc.Cap > 0 {
					q = fmin(q, sc.Cap)
				}
				v = float64(v * q)
			}
			switch s.mOp[mi] {
			case OpBase:
				agg.Base += v
			case OpInc:
				agg.Inc += v
			case OpMore:
				agg.More *= 1.0 + v/100.0
			case OpOverride:
				agg.Override = v
				agg.HasOverride = true
			case OpFlag:
				agg.Flag = true
			}
			if terms != nil {
				*terms = append(*terms, Term{Mod: mi, Source: s.mSource[mi], Op: s.mOp[mi], Value: v})
			}
		}
	}
	var r float64
	switch {
	case agg.HasOverride:
		r = agg.Override
	case def.Formula != nil:
		r = def.Formula(Reader{s: s, deps: deps, actor: n.Actor, ctx: n.Ctx}, agg)
	case def.Flag:
		if agg.Flag {
			r = 1
		}
	default:
		r = agg.Default()
	}
	return def.finish(r)
}

// Reader gives a formula access to other nodes. Every read becomes a dependency.
type Reader struct {
	s     *Session
	deps  *[]uint32
	actor ActorID
	ctx   CtxID
}

// Actor returns the actor of the node being evaluated.
func (r Reader) Actor() ActorID { return r.actor }

// Ctx returns the context of the node being evaluated.
func (r Reader) Ctx() CtxID { return r.ctx }

// Mask returns the context mask of the node being evaluated.
func (r Reader) Mask() Mask { return r.s.masks[r.ctx] }

// Read returns the value of the referenced node.
func (r Reader) Read(ref Ref) float64 { return r.s.read(r.deps, ref, false, r.actor, r.ctx) }

// Stat returns the value of stat on the same actor and context.
func (r Reader) Stat(stat StatID) float64 { return r.Read(SelfRef(stat)) }

// Terms returns the modifiers matching a node with their scaled values, in evaluation order,
// and the node's value.
func (s *Session) Terms(n Node) ([]Term, float64, error) {
	if err := s.checkNode(n); err != nil {
		return nil, 0, err
	}
	if _, err := s.GetNode(n); err != nil {
		return nil, 0, err
	}
	var terms []Term
	v := s.eval(s.nodeID(n), nil, &terms)
	if s.err != nil {
		err := s.err
		s.err = nil
		s.stack = s.stack[:0]
		s.Invalidate()
		return nil, 0, err
	}
	return terms, v, nil
}
