package core_test

import (
	"math"
	"math/rand/v2"
	"testing"

	"github.com/xaroth/pbt-engine/core"
)

// randWorld is a random model and modifier set without undeclared cycles: stats 0-2 are one
// CycleFamily whose conditional and formula reads stay inside the family; every other stat's
// modifiers and formula read only lower stats. Stat 3 is a flag stat.
type randWorld struct {
	m      *core.Model
	ctxs   []core.Mask
	nStat  int
	nActor int
}

const (
	nFamily  = 3
	flagStat = 3
)

func newRandWorld(tb testing.TB, r *rand.Rand) *randWorld {
	tb.Helper()
	w := &randWorld{nStat: 12, nActor: 2}
	var b core.ModelBuilder
	dim, err := b.AddDimension("d", "a", "b", "c", "d")
	if err != nil {
		tb.Fatal(err)
	}
	for i := range w.nStat {
		def := core.StatDef{Name: string(rune('a' + i)), Round: core.Rounding(r.IntN(4))}
		switch {
		case i < nFamily:
			def.CycleFamily = 1
			def.Round = core.RoundFloor
		case i == flagStat:
			def.Flag = true
		case r.IntN(3) == 0:
			if inc := core.StatID(flagStat + 1 + r.IntN(i-flagStat)); int(inc) != i {
				def.Includes = []core.StatID{inc}
			}
		}
		if i > flagStat && r.IntN(4) == 0 {
			def.HasMax, def.Max = true, float64(50+r.IntN(200))
		}
		id := b.MustAddStat(def)
		// Family formulas read the family (unconditional); others read any lower stat.
		if i == 1 || (i > flagStat && r.IntN(3) == 0) {
			in := core.StatID(r.IntN(i))
			if i == 1 {
				in = core.StatID(2 * r.IntN(2)) // not itself: its unconditional node would read itself
			}
			coef := float64(1 + r.IntN(3))
			setFormula(tb, &b, id, func(rd core.Reader, a core.Agg) float64 {
				return (a.Base + float64(coef*math.Floor(rd.Stat(in)/4))) * (1 + a.Inc/100) * a.More
			})
		}
	}
	w.ctxs = []core.Mask{{}, core.MaskOf(dim.Bit(0), dim.Bit(2)), core.MaskOf(dim.Bit(1)), core.MaskOf(dim.Bit(1), dim.Bit(3))}
	w.m = b.Build()
	return w
}

func (w *randWorld) ref(r *rand.Rand, below int) core.Ref {
	ref := core.Ref{Actor: core.Self, Stat: core.StatID(r.IntN(below)), Ctx: core.Self}
	if r.IntN(4) == 0 {
		ref.Actor = int32(r.IntN(w.nActor))
	}
	if r.IntN(4) == 0 {
		ref.Ctx = int64(r.IntN(len(w.ctxs)))
	}
	return ref
}

func (w *randWorld) mod(r *rand.Rand) core.Modifier {
	stat := r.IntN(w.nStat)
	m := core.Modifier{
		Stat:   core.StatID(stat),
		Actor:  core.ActorID(r.IntN(w.nActor)),
		Op:     core.Op(r.IntN(5)),
		Value:  float64(r.IntN(41) - 5),
		Source: core.SourceID(r.IntN(nSources)),
	}
	if m.Op == core.OpOverride && r.IntN(2) == 0 {
		m.Op = core.OpBase
	}
	if stat == flagStat {
		m.Op = core.OpFlag
	}
	limit := max(stat, nFamily)
	if limit > 0 && r.IntN(3) == 0 {
		m.Conds = []core.Cond{{Ref: w.ref(r, limit), Cmp: core.Cmp(r.IntN(6)), Threshold: float64(r.IntN(30))}}
	}
	if limit > 0 && r.IntN(3) == 0 {
		m.Scales = []core.Scale{{Ref: w.ref(r, limit), Step: float64(1 + r.IntN(10)), Cap: float64(r.IntN(4)), HasCap: r.IntN(2) == 0}}
	}
	if r.IntN(3) == 0 {
		m.Pred.Req = []core.Mask{w.ctxs[1+r.IntN(3)]}
	}
	if r.IntN(5) == 0 {
		m.Pred.Excl = w.ctxs[1+r.IntN(3)]
	}
	return m
}

func (w *randWorld) session(tb testing.TB) *core.Session {
	tb.Helper()
	s := newSession(tb, w.m, w.nActor)
	for _, c := range w.ctxs {
		s.InternContext(c)
	}
	return s
}

// snapshot reads every node, in forward or reverse order.
func snapshot(tb testing.TB, s *core.Session, reverse bool) []uint64 {
	tb.Helper()
	nStats := s.Model().NumStats()
	n := s.NumActors() * nStats * s.NumContexts()
	out := make([]uint64, n)
	for k := range n {
		i := k
		if reverse {
			i = n - 1 - k
		}
		ctx := i / (s.NumActors() * nStats)
		actor := i / nStats % s.NumActors()
		out[i] = math.Float64bits(get(tb, s, core.ActorID(actor), core.StatID(i%nStats), core.CtxID(ctx)))
	}
	return out
}

// fresh rebuilds s's live state in a new session.
func fresh(tb testing.TB, w *randWorld, s *core.Session) *core.Session {
	tb.Helper()
	f := w.session(tb)
	for id := range s.Modifiers() {
		m, removed, err := s.Modifier(id)
		if err != nil {
			tb.Fatal(err)
		}
		if !removed {
			add(tb, f, m)
		}
	}
	for src := range core.SourceID(nSources) {
		must(tb, f.SetSourceEnabled(src, s.SourceEnabled(src)))
	}
	return f
}

func equalBits(tb testing.TB, step int, got, exp []uint64) {
	tb.Helper()
	for i := range exp {
		if got[i] != exp[i] {
			tb.Fatalf("step %d node %d: got %v, want %v", step, i, math.Float64frombits(got[i]), math.Float64frombits(exp[i]))
		}
	}
}

// TestIncrementalEqualsFresh applies random edits, reading random nodes in between, and checks
// every node against a session built from scratch.
func TestIncrementalEqualsFresh(t *testing.T) {
	t.Parallel()
	for seed := range uint64(40) {
		t.Run("", func(t *testing.T) {
			t.Parallel()
			r := rand.New(rand.NewPCG(seed, 7))
			w := newRandWorld(t, r)
			s := w.session(t)
			var ids []core.ModID
			for range 40 {
				ids = append(ids, add(t, s, w.mod(r)))
			}
			equalBits(t, -1, snapshot(t, fresh(t, w, s), true), snapshot(t, s, false))

			for step := range 60 {
				switch k := r.IntN(14); {
				case k < 6:
					ids = append(ids, add(t, s, w.mod(r)))
				case k < 10:
					must(t, s.SetSourceEnabled(core.SourceID(r.IntN(nSources)), r.IntN(2) == 0))
				case k < 12:
					must(t, s.RemoveModifier(ids[r.IntN(len(ids))]))
				default:
					must(t, s.SetModifierValue(ids[r.IntN(len(ids))], float64(r.IntN(41)-5)))
				}
				for range 3 {
					ctx := core.CtxID(r.IntN(len(w.ctxs)))
					get(t, s, core.ActorID(r.IntN(w.nActor)), core.StatID(r.IntN(w.nStat)), ctx)
				}
				if step%5 == 4 {
					equalBits(t, step, snapshot(t, s, step%2 == 0), snapshot(t, fresh(t, w, s), false))
				}
			}
		})
	}
}
