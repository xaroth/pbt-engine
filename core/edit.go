package core

import (
	"errors"
	"fmt"
	"iter"
	"math"
	"slices"
	"sort"
)

type editKind uint8

const (
	editSource editKind = iota
	editAdd
	editRemove
	editValue
)

type edit struct {
	kind editKind
	id   uint32 // source or slot
	old  float64
	on   bool
	seq  uint64
}

// Checkpoint is a position in the edit journal, valid until Commit or until a Revert to an
// earlier checkpoint drops it.
type Checkpoint struct {
	gen uint32
	pos int
	seq uint64 // seq of the entry before pos, 0 at the start
}

// ErrStaleCheckpoint is returned by Revert for a checkpoint dropped by Commit or an earlier
// Revert.
var ErrStaleCheckpoint = errors.New("core: stale checkpoint")

func (s *Session) bump() {
	if s.rev == math.MaxUint32-1 {
		// Revisions are compared against cached records; restart both.
		clear(s.stamp)
		s.rev = 1
		s.Invalidate()
	}
	s.rev++
}

func (s *Session) touch(stat StatID, actor ActorID) {
	s.stamp[int(stat)*s.nActors+int(actor)] = s.rev
}

func (s *Session) record(e edit) {
	s.seq++
	e.seq = s.seq
	s.journal = append(s.journal, e)
}

// SetSourceEnabled enables or disables every modifier of a source.
func (s *Session) SetSourceEnabled(src SourceID, on bool) error {
	if int(src) >= len(s.srcOff) {
		return fmt.Errorf("core: unknown source %d", src)
	}
	if !s.srcOff[src] == on {
		return nil
	}
	s.record(edit{kind: editSource, id: uint32(src), on: !on})
	s.setSource(src, on)
	return nil
}

func (s *Session) setSource(src SourceID, on bool) {
	s.srcOff[src] = !on
	s.bump()
	for _, mi := range s.srcMods[src] {
		s.touch(s.mStat[mi], s.mActor[mi])
	}
}

func (s *Session) checkRef(r Ref) error {
	if r.Actor != Self && (r.Actor < 0 || int(r.Actor) >= s.nActors) {
		return fmt.Errorf("actor %d out of range", r.Actor)
	}
	if int(r.Stat) >= s.nStats {
		return fmt.Errorf("stat %d out of range", r.Stat)
	}
	if r.Ctx != Self && (r.Ctx < 0 || r.Ctx >= int64(len(s.masks))) {
		return fmt.Errorf("context %d not interned", r.Ctx)
	}
	return nil
}

func (s *Session) checkModifier(m *Modifier) error {
	if int(m.Stat) >= s.nStats {
		return fmt.Errorf("stat %d out of range", m.Stat)
	}
	if int(m.Actor) >= s.nActors {
		return fmt.Errorf("actor %d out of range", m.Actor)
	}
	if m.Op > OpFlag {
		return fmt.Errorf("op %d unknown", m.Op)
	}
	if int(m.Source) >= len(s.srcOff) {
		return fmt.Errorf("unknown source %d", m.Source)
	}
	for _, c := range m.Conds {
		if err := s.checkRef(c.Ref); err != nil {
			return fmt.Errorf("condition: %w", err)
		}
	}
	for _, sc := range m.Scales {
		if err := s.checkRef(sc.Ref); err != nil {
			return fmt.Errorf("scale: %w", err)
		}
		if !(sc.Step > 0) {
			return fmt.Errorf("scale step %v, want > 0", sc.Step)
		}
	}
	return nil
}

// AddModifier adds a modifier and returns its id. Refs must name existing actors, stats and
// interned contexts, and the source must come from NewSource.
func (s *Session) AddModifier(m Modifier) (ModID, error) {
	if err := s.checkModifier(&m); err != nil {
		return ModID{}, fmt.Errorf("core: modifier: %w", err)
	}
	slot := uint32(len(s.mStat))
	s.mStat = append(s.mStat, m.Stat)
	s.mActor = append(s.mActor, m.Actor)
	s.mOp = append(s.mOp, m.Op)
	s.mValue = append(s.mValue, m.Value)
	s.mSource = append(s.mSource, m.Source)
	s.mAlive = append(s.mAlive, true)
	pred := int32(-1)
	if len(m.Pred.Req) > 0 || !m.Pred.Excl.IsZero() {
		pred = int32(len(s.preds))
		s.preds = append(s.preds, Predicate{Req: slices.Clone(m.Pred.Req), Excl: m.Pred.Excl})
	}
	s.mPred = append(s.mPred, pred)
	s.conds = append(s.conds, m.Conds...)
	s.mCond = append(s.mCond, uint32(len(s.conds)))
	s.scales = append(s.scales, m.Scales...)
	s.mScale = append(s.mScale, uint32(len(s.scales)))
	if int(slot) == len(s.slotGen) {
		s.slotGen = append(s.slotGen, 1)
	}

	b := &s.buckets[int(m.Stat)*s.nActors+int(m.Actor)]
	i := sort.Search(len(*b), func(i int) bool { return s.mSource[(*b)[i]] > m.Source })
	*b = slices.Insert(*b, i, slot)
	s.srcMods[m.Source] = append(s.srcMods[m.Source], slot)

	s.record(edit{kind: editAdd, id: slot})
	s.bump()
	s.touch(m.Stat, m.Actor)
	return ModID{slot: slot, gen: s.slotGen[slot]}, nil
}

func (s *Session) slot(id ModID) (uint32, error) {
	if int(id.slot) >= len(s.mStat) || s.slotGen[id.slot] != id.gen {
		return 0, fmt.Errorf("core: unknown modifier %v", id)
	}
	return id.slot, nil
}

// RemoveModifier stops a modifier from applying. Revert restores it.
func (s *Session) RemoveModifier(id ModID) error {
	mi, err := s.slot(id)
	if err != nil {
		return err
	}
	if !s.mAlive[mi] {
		return nil
	}
	s.record(edit{kind: editRemove, id: mi})
	s.setAlive(mi, false)
	return nil
}

func (s *Session) setAlive(mi uint32, on bool) {
	s.mAlive[mi] = on
	s.bump()
	s.touch(s.mStat[mi], s.mActor[mi])
}

// SetModifierValue changes a modifier's value (a scenario field, a roll).
func (s *Session) SetModifierValue(id ModID, v float64) error {
	mi, err := s.slot(id)
	if err != nil {
		return err
	}
	if math.Float64bits(s.mValue[mi]) == math.Float64bits(v) {
		return nil
	}
	s.record(edit{kind: editValue, id: mi, old: s.mValue[mi]})
	s.setValue(mi, v)
	return nil
}

func (s *Session) setValue(mi uint32, v float64) {
	s.mValue[mi] = v
	s.bump()
	s.touch(s.mStat[mi], s.mActor[mi])
}

// Modifier returns a copy of a modifier as stored, and whether RemoveModifier removed it.
func (s *Session) Modifier(id ModID) (m Modifier, removed bool, err error) {
	mi, err := s.slot(id)
	if err != nil {
		return Modifier{}, false, err
	}
	m = Modifier{
		Stat:   s.mStat[mi],
		Actor:  s.mActor[mi],
		Op:     s.mOp[mi],
		Value:  s.mValue[mi],
		Source: s.mSource[mi],
		Conds:  slices.Clone(s.conds[s.mCond[mi]:s.mCond[mi+1]]),
		Scales: slices.Clone(s.scales[s.mScale[mi]:s.mScale[mi+1]]),
	}
	if p := s.mPred[mi]; p >= 0 {
		m.Pred = Predicate{Req: slices.Clone(s.preds[p].Req), Excl: s.preds[p].Excl}
	}
	return m, !s.mAlive[mi], nil
}

// Modifiers yields the ids of all modifiers added and not dropped by Revert, in the order
// they were added.
func (s *Session) Modifiers() iter.Seq[ModID] {
	return func(yield func(ModID) bool) {
		for mi := range uint32(len(s.mStat)) {
			if !yield(ModID{slot: mi, gen: s.slotGen[mi]}) {
				return
			}
		}
	}
}

// Mark returns the current journal position for Revert.
func (s *Session) Mark() Checkpoint {
	cp := Checkpoint{gen: s.journalGen, pos: len(s.journal)}
	if cp.pos > 0 {
		cp.seq = s.journal[cp.pos-1].seq
	}
	return cp
}

// Revert undoes every edit made after cp, newest first. Checkpoints taken after cp become
// stale; cp stays valid.
func (s *Session) Revert(cp Checkpoint) error {
	if cp.gen != s.journalGen || cp.pos > len(s.journal) || (cp.pos > 0 && s.journal[cp.pos-1].seq != cp.seq) {
		return ErrStaleCheckpoint
	}
	for i := len(s.journal) - 1; i >= cp.pos; i-- {
		e := s.journal[i]
		switch e.kind {
		case editSource:
			s.setSource(SourceID(e.id), e.on)
		case editRemove:
			s.setAlive(e.id, true)
		case editValue:
			s.setValue(e.id, e.old)
		case editAdd:
			s.popModifier(e.id)
		}
	}
	s.journal = s.journal[:cp.pos]
	return nil
}

// Commit forgets the journal; every earlier checkpoint becomes stale.
func (s *Session) Commit() {
	s.journal = s.journal[:0]
	s.journalGen++
}

// popModifier removes the most recently added modifier.
func (s *Session) popModifier(mi uint32) {
	stat, actor, src := s.mStat[mi], s.mActor[mi], s.mSource[mi]
	b := &s.buckets[int(stat)*s.nActors+int(actor)]
	*b = slices.Delete(*b, slices.Index(*b, mi), slices.Index(*b, mi)+1)
	s.srcMods[src] = s.srcMods[src][:len(s.srcMods[src])-1]
	if s.mPred[mi] >= 0 {
		s.preds = s.preds[:s.mPred[mi]]
	}
	s.conds = s.conds[:s.mCond[mi]]
	s.scales = s.scales[:s.mScale[mi]]
	s.mStat = s.mStat[:mi]
	s.mActor = s.mActor[:mi]
	s.mOp = s.mOp[:mi]
	s.mValue = s.mValue[:mi]
	s.mSource = s.mSource[:mi]
	s.mAlive = s.mAlive[:mi]
	s.mPred = s.mPred[:mi]
	s.mCond = s.mCond[:mi+1]
	s.mScale = s.mScale[:mi+1]
	s.slotGen[mi]++
	s.bump()
	s.touch(stat, actor)
}
