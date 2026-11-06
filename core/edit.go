package core

import (
	"fmt"
	"iter"
	"math"
	"slices"
)

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

// SetSourceEnabled enables or disables every modifier of a source.
func (s *Session) SetSourceEnabled(src SourceID, on bool) error {
	if int(src) >= len(s.srcOff) {
		return fmt.Errorf("%w: source %d", ErrBadHandle, src)
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

func (s *Session) checkRef(what string, r Ref) error {
	if r.Actor != Self && (r.Actor < 0 || int(r.Actor) >= s.nActors) {
		return fmt.Errorf("%w: %s actor %d", ErrBadHandle, what, r.Actor)
	}
	if int(r.Stat) >= s.nStats {
		return fmt.Errorf("%w: %s stat %d", ErrBadHandle, what, r.Stat)
	}
	if r.Ctx != Self && (r.Ctx < 0 || r.Ctx >= int64(len(s.masks))) {
		return fmt.Errorf("%w: %s context %d", ErrBadHandle, what, r.Ctx)
	}
	return nil
}

func (s *Session) checkModifier(m *Modifier) error {
	if int(m.Stat) >= s.nStats {
		return fmt.Errorf("%w: modifier stat %d", ErrBadHandle, m.Stat)
	}
	if int(m.Actor) >= s.nActors {
		return fmt.Errorf("%w: modifier actor %d", ErrBadHandle, m.Actor)
	}
	if int(m.Source) >= len(s.srcOff) {
		return fmt.Errorf("%w: modifier source %d", ErrBadHandle, m.Source)
	}
	if m.Op > OpFlag {
		return fmt.Errorf("%w: modifier op %d", ErrOutOfRange, m.Op)
	}
	for _, c := range m.Conds {
		if err := s.checkRef("condition", c.Ref); err != nil {
			return err
		}
	}
	for _, sc := range m.Scales {
		if err := s.checkRef("scale", sc.Ref); err != nil {
			return err
		}
		if !(sc.Step > 0) {
			return fmt.Errorf("%w: scale step %v, want > 0", ErrOutOfRange, sc.Step)
		}
	}
	return nil
}

// AddModifier adds a modifier and returns its id. Refs must name existing actors, stats and
// interned contexts, and the source must come from NewSource.
func (s *Session) AddModifier(m Modifier) (ModID, error) {
	if err := s.checkModifier(&m); err != nil {
		return ModID{}, err
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
	i, _ := slices.BinarySearchFunc(*b, m.Source, func(slot uint32, src SourceID) int {
		if s.mSource[slot] > src {
			return 1
		}
		return -1 // past equal sources, so they keep insertion order
	})
	*b = slices.Insert(*b, i, slot)
	s.srcMods[m.Source] = append(s.srcMods[m.Source], slot)

	s.record(edit{kind: editAdd, id: slot})
	s.bump()
	s.touch(m.Stat, m.Actor)
	return s.modID(slot), nil
}

func (s *Session) modID(slot uint32) ModID { return ModID{slot: slot, gen: s.slotGen[slot]} }

func (s *Session) slot(id ModID) (uint32, error) {
	if int(id.slot) >= len(s.mStat) || s.slotGen[id.slot] != id.gen {
		return 0, fmt.Errorf("%w: modifier %v", ErrBadHandle, id)
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

// Modifiers yields the ids of all modifiers added and not dropped by Revert, removed ones
// included, in the order they were added.
func (s *Session) Modifiers() iter.Seq[ModID] {
	return func(yield func(ModID) bool) {
		for mi := range uint32(len(s.mStat)) {
			if !yield(s.modID(mi)) {
				return
			}
		}
	}
}
