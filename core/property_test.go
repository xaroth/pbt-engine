package core

import (
	"math"
	"math/rand/v2"
	"testing"
)

// randWorld is a random model and modifier set without undeclared cycles: stats 0-2 are a
// CycleBreak family whose conditional reads stay inside the family; every other stat's
// modifiers and formula read only lower stats.
type randWorld struct {
	m      *Model
	ctxs   []Mask
	nStat  int
	nActor int
}

const nCycle = 3

func newRandWorld(r *rand.Rand) *randWorld {
	w := &randWorld{m: NewModel(), nStat: 12, nActor: 2}
	dim, _ := w.m.AddDimension("d", "a", "b", "c", "d")
	for i := range w.nStat {
		def := StatDef{Name: string(rune('a' + i)), Round: Rounding(r.IntN(4))}
		if i < nCycle {
			def.CycleBreak = true
			def.Round = RoundFloor
		} else if r.IntN(3) == 0 {
			def.Includes = []StatID{StatID(nCycle + r.IntN(i-nCycle+1))}
			if def.Includes[0] == StatID(i) {
				def.Includes = nil
			}
		}
		if i >= nCycle && r.IntN(4) == 0 {
			def.HasMax, def.Max = true, float64(50+r.IntN(200))
		}
		id := w.m.MustAddStat(def)
		if i > nCycle && r.IntN(3) == 0 {
			in := StatID(r.IntN(i))
			coef := float64(1 + r.IntN(3))
			w.m.SetFormula(id, func(rd Reader, a Agg) float64 {
				return (a.Base + float64(coef*math.Floor(rd.Stat(in)/4))) * (1 + a.Inc/100) * a.More
			})
		}
	}
	w.ctxs = []Mask{{}, MaskOf(dim.Bit(0), dim.Bit(2)), MaskOf(dim.Bit(1)), MaskOf(dim.Bit(1), dim.Bit(3))}
	return w
}

func (w *randWorld) ref(r *rand.Rand, below int) Ref {
	ref := Ref{Actor: Self, Stat: StatID(r.IntN(below)), Ctx: Self}
	if r.IntN(4) == 0 {
		ref.Actor = int32(r.IntN(w.nActor))
	}
	if r.IntN(4) == 0 {
		ref.Ctx = int64(r.IntN(len(w.ctxs)))
	}
	return ref
}

func (w *randWorld) mod(r *rand.Rand) Modifier {
	stat := r.IntN(w.nStat)
	m := Modifier{
		Stat:   StatID(stat),
		Actor:  ActorID(r.IntN(w.nActor)),
		Op:     Op(r.IntN(4)),
		Value:  float64(r.IntN(41) - 5),
		Source: SourceID(r.IntN(10)),
	}
	if m.Op == OpOverride && r.IntN(2) == 0 {
		m.Op = OpBase
	}
	// Reads from modifiers stay below the stat, or inside the CycleBreak family.
	limit := stat
	if stat < nCycle {
		limit = nCycle
	}
	if limit > 0 && r.IntN(3) == 0 {
		m.Conds = []Cond{{Ref: w.ref(r, limit), Cmp: Cmp(r.IntN(6)), Threshold: float64(r.IntN(30))}}
	}
	if limit > 0 && r.IntN(3) == 0 {
		m.Scales = []Scale{{Ref: w.ref(r, limit), Step: float64(1 + r.IntN(10)), Cap: float64(r.IntN(4))}}
	}
	if r.IntN(3) == 0 {
		m.Pred.Req = []Mask{w.ctxs[1+r.IntN(3)]}
	}
	if r.IntN(5) == 0 {
		m.Pred.Excl = w.ctxs[1+r.IntN(3)]
	}
	return m
}

func (w *randWorld) session() *Session {
	s := NewSession(w.m, w.nActor)
	for _, c := range w.ctxs {
		s.Context(c)
	}
	return s
}

// snapshot reads every node, in forward or reverse order.
func snapshot(t testing.TB, s *Session, reverse bool) []uint64 {
	t.Helper()
	n := s.nActors * s.nStats * len(s.masks)
	out := make([]uint64, n)
	for k := range n {
		i := k
		if reverse {
			i = n - 1 - k
		}
		ctx := i / (s.nActors * s.nStats)
		actor := i / s.nStats % s.nActors
		v, err := s.Get(ActorID(actor), StatID(i%s.nStats), CtxID(ctx))
		if err != nil {
			t.Fatal(err)
		}
		out[i] = math.Float64bits(v)
	}
	return out
}

// fresh rebuilds s's live state in a new session.
func fresh(t testing.TB, w *randWorld, s *Session) *Session {
	f := w.session()
	for i := range s.NumModifiers() {
		m, alive := s.Modifier(ModID(i))
		if alive {
			add(t, f, m)
		}
	}
	for src := range SourceID(10) {
		f.SetSourceEnabled(src, s.SourceEnabled(src))
	}
	return f
}

func equalBits(t *testing.T, step int, got, exp []uint64) {
	t.Helper()
	for i := range exp {
		if got[i] != exp[i] {
			t.Fatalf("step %d node %d: %v, want %v", step, i, math.Float64frombits(got[i]), math.Float64frombits(exp[i]))
		}
	}
}

// TestIncrementalEqualsFresh applies random edits and reverts, reading random nodes in
// between, and checks every node against a session built from scratch.
func TestIncrementalEqualsFresh(t *testing.T) {
	for seed := range uint64(40) {
		r := rand.New(rand.NewPCG(seed, 7))
		w := newRandWorld(r)
		s := w.session()
		for range 40 {
			add(t, s, w.mod(r))
		}
		s.Commit()
		base := snapshot(t, s, false)
		equalBits(t, -1, snapshot(t, fresh(t, w, s), true), base)

		var marks []Checkpoint
		var snaps [][]uint64
		for step := range 60 {
			switch k := r.IntN(10); {
			case k < 3:
				add(t, s, w.mod(r))
			case k < 5:
				s.SetSourceEnabled(SourceID(r.IntN(10)), r.IntN(2) == 0)
			case k < 6:
				s.RemoveModifier(ModID(r.IntN(s.NumModifiers())))
			case k < 7:
				s.SetModifierValue(ModID(r.IntN(s.NumModifiers())), float64(r.IntN(41)-5))
			case k < 8:
				marks = append(marks, s.Mark())
				snaps = append(snaps, snapshot(t, s, false))
			default:
				if len(marks) > 0 {
					i := r.IntN(len(marks))
					s.Revert(marks[i])
					equalBits(t, step, snapshot(t, s, false), snaps[i])
					marks, snaps = marks[:i], snaps[:i]
				}
			}
			for range 3 {
				ctx := CtxID(r.IntN(len(w.ctxs)))
				if _, err := s.Get(ActorID(r.IntN(w.nActor)), StatID(r.IntN(w.nStat)), ctx); err != nil {
					t.Fatal(err)
				}
			}
			if step%5 == 4 {
				equalBits(t, step, snapshot(t, s, step%2 == 0), snapshot(t, fresh(t, w, s), false))
			}
		}
		s.Revert(0)
		equalBits(t, 99, snapshot(t, s, true), base)
	}
}
