package core

import "slices"

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
// earlier checkpoint drops it. The zero Checkpoint is never valid.
type Checkpoint struct {
	gen uint32
	pos int
	seq uint64 // seq of the entry before pos, 0 at the start
}

// record journals e while a checkpoint is live; nothing could revert it otherwise.
func (s *Session) record(e edit) {
	if !s.marked {
		return
	}
	s.seq++
	e.seq = s.seq
	s.journal = append(s.journal, e)
}

// Mark returns the current journal position for Revert. Edits are journalled from the first
// Mark until Commit.
func (s *Session) Mark() Checkpoint {
	s.marked = true
	cp := Checkpoint{gen: s.journalGen, pos: len(s.journal)}
	if cp.pos > 0 {
		cp.seq = s.journal[cp.pos-1].seq
	}
	return cp
}

// Revert undoes every edit made after cp, newest first. Checkpoints taken after cp become
// stale; cp stays valid. It returns ErrStaleCheckpoint for a stale cp and changes nothing.
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

// Commit keeps every edit made so far: it forgets the journal and stops journalling until the
// next Mark, and every earlier checkpoint becomes stale. Call it when no checkpoint is needed
// any more; until then the journal grows with every edit.
func (s *Session) Commit() {
	s.journal = s.journal[:0]
	s.journalGen++
	s.marked = false
}

// popModifier removes the most recently added modifier.
func (s *Session) popModifier(mi uint32) {
	stat, actor, src := s.mStat[mi], s.mActor[mi], s.mSource[mi]
	b := &s.buckets[int(stat)*s.nActors+int(actor)]
	i := slices.Index(*b, mi)
	*b = slices.Delete(*b, i, i+1)
	s.srcMods[src] = s.srcMods[src][:len(s.srcMods[src])-1]
	if s.mPred[mi] >= 0 {
		s.preds = s.preds[:s.mPred[mi]]
	}
	s.conds = s.conds[:s.mCond[mi]]
	s.scales = s.scales[:s.mScale[mi]]
	s.mStat = s.mStat[:mi]
	s.mActor = s.mActor[:mi]
	s.mOp = s.mOp[:mi]
	s.mGroup = s.mGroup[:mi]
	s.mValue = s.mValue[:mi]
	s.mSource = s.mSource[:mi]
	s.mAlive = s.mAlive[:mi]
	s.mPred = s.mPred[:mi]
	s.mCond = s.mCond[:mi+1]
	s.mScale = s.mScale[:mi+1]
	s.slotGen[mi]++
	if s.slotGen[mi] == 0 { // keep the zero ModID invalid
		s.slotGen[mi] = 1
	}
	s.bump()
	s.touch(stat, actor)
}
