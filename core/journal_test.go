package core_test

import (
	"errors"
	"testing"

	"github.com/xaroth/pbt-engine/core"
)

func TestRevert(t *testing.T) {
	t.Parallel()
	m, str, dex, life := attrModel(t, nil)
	testCases := []struct {
		name string
		edit func(s *core.Session, ids []core.ModID) error
		want float64 // life after the edit, before Revert
	}{
		{"add", func(s *core.Session, _ []core.ModID) error {
			_, err := s.AddModifier(core.Modifier{Stat: life, Op: core.OpMore, Value: 100, Source: 5})
			return wrap(t, "add modifier", err)
		}, 143},
		{"add to an empty stat", func(s *core.Session, _ []core.ModID) error {
			_, err := s.AddModifier(core.Modifier{Stat: dex, Op: core.OpBase, Value: 5, Source: 4})
			return wrap(t, "add modifier", err)
		}, 72},
		{"remove", func(s *core.Session, ids []core.ModID) error { return s.RemoveModifier(ids[2]) }, 66},
		{"value", func(s *core.Session, ids []core.ModID) error { return s.SetModifierValue(ids[3], 20) }, 78},
		{"disable source", func(s *core.Session, _ []core.ModID) error { return s.SetSourceEnabled(2, false) }, 66},
		{"several edits of one modifier", func(s *core.Session, ids []core.ModID) error {
			return errors.Join(s.SetModifierValue(ids[2], 40), s.RemoveModifier(ids[2]), s.SetModifierValue(ids[2], 50))
		}, 66},
		{"add, edit and disable its source", func(s *core.Session, _ []core.ModID) error {
			id, err := s.AddModifier(core.Modifier{Stat: str, Op: core.OpBase, Value: 40, Source: 6})
			return errors.Join(err, s.SetModifierValue(id, 60), s.SetSourceEnabled(6, false), s.SetSourceEnabled(1, false))
		}, 6},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			s := newSession(t, m, 1)
			ids := addAll(t, s, []core.Modifier{
				{Stat: str, Op: core.OpBase, Value: 20, Source: 1},
				{Stat: life, Op: core.OpBase, Value: 50, Source: 1},
				{Stat: str, Op: core.OpBase, Value: 10, Source: 2},
				{Stat: life, Op: core.OpInc, Value: 10, Source: 3},
			})
			s.Commit()
			want(t, s, life, 72) // (50 + 30/2) * 1.1 = 71.5
			before := snapshot(t, s, false)
			cp := s.Mark()
			must(t, testCase.edit(s, ids))
			want(t, s, life, testCase.want)
			must(t, s.Revert(cp))
			equalBits(t, 0, snapshot(t, s, true), before)
			for src := range core.SourceID(nSources) {
				if !s.SourceEnabled(src) {
					t.Errorf("source %d: got disabled, want enabled", src)
				}
			}
			n := 0
			for id := range s.Modifiers() {
				_, removed, err := s.Modifier(id)
				if err != nil || removed {
					t.Errorf("modifier %d: got removed %v, %v, want false, nil", n, removed, err)
				}
				n++
			}
			if n != len(ids) {
				t.Errorf("modifiers: got %d, want %d", n, len(ids))
			}
		})
	}
}

func TestCheckpoints(t *testing.T) {
	t.Parallel()
	var b core.ModelBuilder
	x := b.MustAddStat(core.StatDef{Name: "x"})
	m := b.Build()
	addX := func(s *core.Session) { add(t, s, core.Modifier{Stat: x, Value: 1}) }
	testCases := []struct {
		name      string
		run       func(s *core.Session) core.Checkpoint // returns the checkpoint to revert to
		wantErr   error
		wantValue float64
	}{
		{"revert to the start", func(s *core.Session) core.Checkpoint {
			cp := s.Mark()
			addX(s)
			return cp
		}, nil, 10},
		{"revert to the same checkpoint twice", func(s *core.Session) core.Checkpoint {
			cp := s.Mark()
			addX(s)
			must(t, s.Revert(cp))
			addX(s)
			addX(s)
			return cp
		}, nil, 10},
		{"inner, then outer", func(s *core.Session) core.Checkpoint {
			outer := s.Mark()
			addX(s)
			inner := s.Mark()
			addX(s)
			must(t, s.Revert(inner))
			return outer
		}, nil, 10},
		{"checkpoint before Commit", func(s *core.Session) core.Checkpoint {
			cp := s.Mark()
			addX(s)
			s.Commit()
			return cp
		}, core.ErrStaleCheckpoint, 11},
		{"zero checkpoint", func(s *core.Session) core.Checkpoint {
			addX(s)
			return core.Checkpoint{}
		}, core.ErrStaleCheckpoint, 11},
		{"zero checkpoint after a mark", func(s *core.Session) core.Checkpoint {
			s.Mark()
			addX(s)
			return core.Checkpoint{}
		}, core.ErrStaleCheckpoint, 11},
		{"edits before the mark stay", func(s *core.Session) core.Checkpoint {
			addX(s)
			cp := s.Mark()
			addX(s)
			return cp
		}, nil, 11},
		{"checkpoint after the reverted one", func(s *core.Session) core.Checkpoint {
			start := s.Mark()
			addX(s)
			later := s.Mark()
			must(t, s.Revert(start))
			addX(s)
			addX(s)
			return later
		}, core.ErrStaleCheckpoint, 12},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			s := newSession(t, m, 1)
			add(t, s, core.Modifier{Stat: x, Value: 10})
			s.Commit()
			cp := testCase.run(s)
			if err := s.Revert(cp); !errors.Is(err, testCase.wantErr) {
				t.Errorf("Revert: got %v, want %v", err, testCase.wantErr)
			}
			want(t, s, x, testCase.wantValue)
		})
	}
}

func TestJournalOnlyWhileMarked(t *testing.T) {
	t.Parallel()
	var b core.ModelBuilder
	x := b.MustAddStat(core.StatDef{Name: "x"})
	s := newSession(t, b.Build(), 1)
	steps := []struct {
		name string
		do   func()
		want int
	}{
		{"edit before any mark", func() { add(t, s, core.Modifier{Stat: x, Value: 1}) }, 0},
		{"mark", func() { s.Mark() }, 0},
		{"edit after mark", func() { add(t, s, core.Modifier{Stat: x, Value: 1}) }, 1},
		{"commit", s.Commit, 0},
		{"edit after commit", func() { add(t, s, core.Modifier{Stat: x, Value: 1}) }, 0},
	}
	for _, step := range steps {
		step.do()
		if got := core.JournalLen(s); got != step.want {
			t.Errorf("%s: journal has %d edits, want %d", step.name, got, step.want)
		}
	}
}

func TestRevertedModifierID(t *testing.T) {
	t.Parallel()
	var b core.ModelBuilder
	x := b.MustAddStat(core.StatDef{Name: "x"})
	m := b.Build()
	testCases := []struct {
		name string
		call func(s *core.Session, dropped core.ModID) error
	}{
		{"remove", func(s *core.Session, id core.ModID) error { return s.RemoveModifier(id) }},
		{"value", func(s *core.Session, id core.ModID) error { return s.SetModifierValue(id, 3) }},
		{"modifier", func(s *core.Session, id core.ModID) error {
			_, _, err := s.Modifier(id)
			return wrap(t, "modifier", err)
		}},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			s := newSession(t, m, 1)
			cp := s.Mark()
			dropped := add(t, s, core.Modifier{Stat: x, Value: 1})
			must(t, s.Revert(cp))
			reused := add(t, s, core.Modifier{Stat: x, Value: 2})
			if dropped == reused {
				t.Fatalf("id of reverted modifier reused: got %v twice", reused)
			}
			if err := testCase.call(s, dropped); !errors.Is(err, core.ErrBadHandle) {
				t.Errorf("got %v, want ErrBadHandle", err)
			}
			want(t, s, x, 2)
		})
	}
}
