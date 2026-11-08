package core

import (
	"fmt"
	"math"
)

// Node addresses a value: a stat of an actor in a context. Uncond nodes are the unconditional
// values used by the CycleFamily rule.
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

// Session is one build: a modifier store over a Model, evaluated lazily and memoised. Edits
// mark what they touch; the next read re-verifies only nodes whose inputs changed. Edits are
// journalled from the first Mark until Commit, so Revert can undo them. A Session
// is not safe for concurrent use; sessions over the same Model are independent.
type Session struct {
	m       *Model
	nActors int
	nStats  int

	masks  []Mask
	ctxIdx map[Mask]CtxID

	// Modifier store, struct of arrays by slot.
	mStat   []StatID
	mActor  []ActorID
	mOp     []Op
	mValue  []float64
	mSource []SourceID
	mAlive  []bool
	mPred   []int32
	mCond   []uint32 // offsets into conds, len(slots)+1
	mScale  []uint32 // offsets into scales, len(slots)+1
	slotGen []uint32 // never shrinks, so a ModID of a reverted add stays invalid
	conds   []Cond
	scales  []Scale
	preds   []Predicate
	buckets [][]uint32 // [stat*nActors+actor] slots, sorted by (source, slot)
	srcMods [][]uint32
	srcOff  []bool

	stamp []uint32 // [stat*nActors+actor] revision of the last edit touching that bucket
	rev   uint32
	epoch uint32

	blocks   []block
	blockIdx []int32 // [(ctx*nActors+actor)*2+uncond] -> index into blocks, -1 if absent
	stack    []uint32
	err      error

	journal    []edit
	journalGen uint32 // starts at 1, so the zero Checkpoint is stale
	seq        uint64
	marked     bool // a Mark since the last Commit
}

// NewSession creates a session with actors 0 to actors-1 over m.
func NewSession(m *Model, actors int) (*Session, error) {
	if actors < 1 || actors > math.MaxUint16 {
		return nil, fmt.Errorf("%w: %d actors, want 1 to %d", ErrOutOfRange, actors, math.MaxUint16)
	}
	s := &Session{
		m:       m,
		nActors: actors,
		nStats:  len(m.stats),
		ctxIdx:  map[Mask]CtxID{},
		rev:     1,
		epoch:   1,
		mCond:   []uint32{0},
		mScale:  []uint32{0},

		journalGen: 1,
	}
	s.buckets = make([][]uint32, s.nStats*actors)
	s.stamp = make([]uint32, s.nStats*actors)
	s.InternContext(Mask{})
	return s, nil
}

// Model returns the session's model.
func (s *Session) Model() *Model { return s.m }

// NumActors returns the number of actors.
func (s *Session) NumActors() int { return s.nActors }

// InternContext returns the id of a context mask, adding it to the session if it is new. The
// empty mask is GlobalCtx.
func (s *Session) InternContext(mask Mask) CtxID {
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

// ContextMask returns the mask of an interned context, and false for an unknown id.
func (s *Session) ContextMask(c CtxID) (Mask, bool) {
	if int(c) >= len(s.masks) {
		return Mask{}, false
	}
	return s.masks[c], true
}

// NumContexts returns the number of interned contexts; ids run from 0 to NumContexts()-1.
func (s *Session) NumContexts() int { return len(s.masks) }

// NewSource returns a new source id, enabled. Sources are dense: ids run from 0 to
// NumSources()-1.
func (s *Session) NewSource() SourceID {
	s.srcOff = append(s.srcOff, false)
	s.srcMods = append(s.srcMods, nil)
	return SourceID(len(s.srcOff) - 1)
}

// NumSources returns the number of sources created.
func (s *Session) NumSources() int { return len(s.srcOff) }

// SourceEnabled reports whether a source's modifiers apply; false for an unknown source.
func (s *Session) SourceEnabled(src SourceID) bool {
	return int(src) < len(s.srcOff) && !s.srcOff[src]
}

// Get returns the value of a stat of an actor in a context.
func (s *Session) Get(actor ActorID, stat StatID, ctx CtxID) (float64, error) {
	return s.GetNode(Node{Actor: actor, Stat: stat, Ctx: ctx})
}

// GetNode returns the value of a node. If a formula panics, the session drops its cached
// values and the panic continues.
func (s *Session) GetNode(n Node) (float64, error) {
	if err := s.checkNode(n); err != nil {
		return 0, err
	}
	defer s.recoverEval()
	v := s.get(s.nodeID(n))
	if err := s.takeErr(); err != nil {
		return 0, err
	}
	return v, nil
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

// Terms returns the modifiers matching a node with their scaled values, in evaluation order,
// and the node's value.
func (s *Session) Terms(n Node) ([]Term, float64, error) {
	if _, err := s.GetNode(n); err != nil {
		return nil, 0, err
	}
	defer s.recoverEval()
	var terms []Term
	v := s.eval(s.nodeID(n), nil, &terms)
	if err := s.takeErr(); err != nil {
		return nil, 0, err
	}
	return terms, v, nil
}

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

func (s *Session) checkNode(n Node) error {
	if int(n.Actor) >= s.nActors {
		return fmt.Errorf("%w: actor %d", ErrBadHandle, n.Actor)
	}
	if int(n.Stat) >= s.nStats {
		return fmt.Errorf("%w: stat %d", ErrBadHandle, n.Stat)
	}
	if int(n.Ctx) >= len(s.masks) {
		return fmt.Errorf("%w: context %d", ErrBadHandle, n.Ctx)
	}
	return nil
}

func (s *Session) recoverEval() {
	if p := recover(); p != nil {
		s.abort()
		panic(p)
	}
}

func (s *Session) takeErr() error {
	err := s.err
	if err != nil {
		s.abort()
	}
	return err
}

// abort resets evaluation state after an error or panic; partial results are discarded.
func (s *Session) abort() {
	for _, id := range s.stack {
		s.rec(id).busy = false
	}
	s.stack = s.stack[:0]
	s.err = nil
	s.Invalidate()
}
