package core

import (
	"fmt"
	"math"
	"sort"
)

// Checkpoint is a position in the edit journal.
type Checkpoint int

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

// SourceEnabled reports whether a source's modifiers apply. Sources are enabled by default.
func (s *Session) SourceEnabled(src SourceID) bool {
	return int(src) >= len(s.srcOff) || !s.srcOff[src]
}

// SetSourceEnabled enables or disables every modifier of a source.
func (s *Session) SetSourceEnabled(src SourceID, on bool) {
	if s.SourceEnabled(src) == on {
		return
	}
	s.journal = append(s.journal, edit{kind: editSource, id: uint32(src), on: !on})
	s.setSource(src, on)
}

func (s *Session) setSource(src SourceID, on bool) {
	for int(src) >= len(s.srcOff) {
		s.srcOff = append(s.srcOff, false)
	}
	s.srcOff[src] = !on
	s.bump()
	if int(src) < len(s.srcMods) {
		for _, mi := range s.srcMods[src] {
			s.touch(s.mStat[mi], s.mActor[mi])
		}
	}
}

// AddModifier adds a modifier and returns its id.
func (s *Session) AddModifier(m Modifier) (ModID, error) {
	if int(m.Stat) >= s.nStats {
		return 0, fmt.Errorf("core: modifier stat %d out of range", m.Stat)
	}
	if int(m.Actor) >= s.nActors {
		return 0, fmt.Errorf("core: modifier actor %d out of range", m.Actor)
	}
	if m.Op > OpFlag {
		return 0, fmt.Errorf("core: modifier op %d unknown", m.Op)
	}
	for _, sc := range m.Scales {
		if !(sc.Step > 0) {
			return 0, fmt.Errorf("core: modifier on %s: scale step %v", s.m.stats[m.Stat].Name, sc.Step)
		}
	}
	id := ModID(len(s.mStat))
	s.mStat = append(s.mStat, m.Stat)
	s.mActor = append(s.mActor, m.Actor)
	s.mOp = append(s.mOp, m.Op)
	s.mValue = append(s.mValue, m.Value)
	s.mSource = append(s.mSource, m.Source)
	s.mAlive = append(s.mAlive, true)
	pred := int32(-1)
	if len(m.Pred.Req) > 0 || !m.Pred.Excl.IsZero() {
		pred = int32(len(s.preds))
		s.preds = append(s.preds, Predicate{Req: append([]Mask(nil), m.Pred.Req...), Excl: m.Pred.Excl})
	}
	s.mPred = append(s.mPred, pred)
	s.conds = append(s.conds, m.Conds...)
	s.mCond = append(s.mCond, uint32(len(s.conds)))
	s.scales = append(s.scales, m.Scales...)
	s.mScale = append(s.mScale, uint32(len(s.scales)))

	b := &s.buckets[int(m.Stat)*s.nActors+int(m.Actor)]
	i := sort.Search(len(*b), func(i int) bool { return s.mSource[(*b)[i]] > m.Source })
	*b = append(*b, 0)
	copy((*b)[i+1:], (*b)[i:])
	(*b)[i] = id

	for int(m.Source) >= len(s.srcMods) {
		s.srcMods = append(s.srcMods, nil)
	}
	s.srcMods[m.Source] = append(s.srcMods[m.Source], id)

	s.journal = append(s.journal, edit{kind: editAdd, id: uint32(id)})
	s.bump()
	s.touch(m.Stat, m.Actor)
	return id, nil
}

// RemoveModifier stops a modifier from applying. Revert restores it.
func (s *Session) RemoveModifier(id ModID) {
	if !s.mAlive[id] {
		return
	}
	s.journal = append(s.journal, edit{kind: editRemove, id: uint32(id)})
	s.setAlive(id, false)
}

func (s *Session) setAlive(id ModID, on bool) {
	s.mAlive[id] = on
	s.bump()
	s.touch(s.mStat[id], s.mActor[id])
}

// SetModifierValue changes a modifier's value (a scenario field, a roll).
func (s *Session) SetModifierValue(id ModID, v float64) {
	if math.Float64bits(s.mValue[id]) == math.Float64bits(v) {
		return
	}
	s.journal = append(s.journal, edit{kind: editValue, id: uint32(id), old: s.mValue[id]})
	s.setValue(id, v)
}

func (s *Session) setValue(id ModID, v float64) {
	s.mValue[id] = v
	s.bump()
	s.touch(s.mStat[id], s.mActor[id])
}

// Modifier returns a copy of a modifier as stored, and whether it applies.
func (s *Session) Modifier(id ModID) (Modifier, bool) {
	m := Modifier{
		Stat:   s.mStat[id],
		Actor:  s.mActor[id],
		Op:     s.mOp[id],
		Value:  s.mValue[id],
		Source: s.mSource[id],
		Conds:  s.conds[s.mCond[id]:s.mCond[id+1]:s.mCond[id+1]],
		Scales: s.scales[s.mScale[id]:s.mScale[id+1]:s.mScale[id+1]],
	}
	if p := s.mPred[id]; p >= 0 {
		m.Pred = s.preds[p]
	}
	return m, s.mAlive[id]
}

// NumModifiers returns the number of modifiers added, including removed ones.
func (s *Session) NumModifiers() int { return len(s.mStat) }

// Mark returns the current journal position for Revert.
func (s *Session) Mark() Checkpoint { return Checkpoint(len(s.journal)) }

// Revert undoes every edit made after cp, newest first.
func (s *Session) Revert(cp Checkpoint) {
	if cp < 0 || int(cp) > len(s.journal) {
		panic(fmt.Sprintf("core: checkpoint %d outside journal of %d", cp, len(s.journal)))
	}
	for i := len(s.journal) - 1; i >= int(cp); i-- {
		e := s.journal[i]
		switch e.kind {
		case editSource:
			s.setSource(SourceID(e.id), e.on)
		case editRemove:
			s.setAlive(ModID(e.id), true)
		case editValue:
			s.setValue(ModID(e.id), e.old)
		case editAdd:
			s.popModifier(ModID(e.id))
		}
	}
	s.journal = s.journal[:cp]
}

// Commit forgets the journal; earlier checkpoints become invalid.
func (s *Session) Commit() { s.journal = s.journal[:0] }

// popModifier removes the most recently added modifier.
func (s *Session) popModifier(id ModID) {
	stat, actor, src := s.mStat[id], s.mActor[id], s.mSource[id]
	b := &s.buckets[int(stat)*s.nActors+int(actor)]
	for i, x := range *b {
		if x == id {
			*b = append((*b)[:i], (*b)[i+1:]...)
			break
		}
	}
	sm := s.srcMods[src]
	s.srcMods[src] = sm[:len(sm)-1]
	if s.mPred[id] >= 0 {
		s.preds = s.preds[:s.mPred[id]]
	}
	s.conds = s.conds[:s.mCond[id]]
	s.scales = s.scales[:s.mScale[id]]
	s.mStat = s.mStat[:id]
	s.mActor = s.mActor[:id]
	s.mOp = s.mOp[:id]
	s.mValue = s.mValue[:id]
	s.mSource = s.mSource[:id]
	s.mAlive = s.mAlive[:id]
	s.mPred = s.mPred[:id]
	s.mCond = s.mCond[:id+1]
	s.mScale = s.mScale[:id+1]
	s.bump()
	s.touch(stat, actor)
}
