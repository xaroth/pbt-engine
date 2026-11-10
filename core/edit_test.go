package core_test

import (
	"errors"
	"math"
	"math/rand/v2"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/xaroth/pbt-engine/core"
)

func TestEdits(t *testing.T) {
	t.Parallel()
	var b core.ModelBuilder
	life := b.MustAddStat(core.StatDef{Name: "life"})
	str := b.MustAddStat(core.StatDef{Name: "str"})
	lowLife := b.MustAddStat(core.StatDef{Name: "low_life", Flag: true})
	m := b.Build()
	mods := []core.Modifier{
		{Stat: life, Op: core.OpBase, Value: 100, Source: 1},
		{Stat: life, Op: core.OpOverride, Value: 7, Source: 8},
		{Stat: life, Op: core.OpOverride, Value: 1, Source: 9},
		{Stat: str, Op: core.OpBase, Value: 55, Source: 2},
		{Stat: life, Op: core.OpInc, Value: 1, Source: 3, Scales: []core.Scale{{Ref: core.SelfRef(str), Step: 10}}},
		{Stat: lowLife, Op: core.OpFlag, Source: 4},
		{Stat: life, Op: core.OpInc, Value: 20, Source: 5, Conds: []core.Cond{{Ref: core.SelfRef(lowLife), Cmp: core.CmpGE, Threshold: 1}}},
	}
	const override7, override1, strBase, lowLifeFlag = 1, 2, 3, 5
	testCases := []struct {
		name string
		edit func(s *core.Session, ids []core.ModID) error
		want float64
	}{
		{"no edit", func(*core.Session, []core.ModID) error { return nil }, 1},
		{"remove the winning override", func(s *core.Session, ids []core.ModID) error {
			return s.RemoveModifier(ids[override1])
		}, 7},
		{"remove twice", func(s *core.Session, ids []core.ModID) error {
			return errors.Join(s.RemoveModifier(ids[override1]), s.RemoveModifier(ids[override1]))
		}, 7},
		{"disable the override sources", func(s *core.Session, _ []core.ModID) error {
			return errors.Join(s.SetSourceEnabled(8, false), s.SetSourceEnabled(9, false))
		}, increased100(5 + 20)},
		{"disable and enable a source", func(s *core.Session, _ []core.ModID) error {
			return errors.Join(s.SetSourceEnabled(9, false), s.SetSourceEnabled(9, true))
		}, 1},
		{"change a scaled stat", func(s *core.Session, ids []core.ModID) error {
			return errors.Join(s.SetSourceEnabled(8, false), s.SetSourceEnabled(9, false), s.SetModifierValue(ids[strBase], 20))
		}, increased100(2 + 20)},
		{"remove a condition's input", func(s *core.Session, ids []core.ModID) error {
			return errors.Join(s.RemoveModifier(ids[override1]), s.RemoveModifier(ids[override7]), s.RemoveModifier(ids[lowLifeFlag]))
		}, increased100(5)},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			s := newSession(t, m, 1)
			ids := addAll(t, s, mods)
			want(t, s, life, 1)
			must(t, testCase.edit(s, ids))
			want(t, s, life, testCase.want)
		})
	}
}

func TestAddModifierErrors(t *testing.T) {
	t.Parallel()
	var b core.ModelBuilder
	x := b.MustAddStat(core.StatDef{Name: "x"})
	m := b.Build()
	testCases := []struct {
		name string
		mod  core.Modifier
		want error
	}{
		{"stat", core.Modifier{Stat: 9}, core.ErrBadHandle},
		{"actor", core.Modifier{Stat: x, Actor: 1}, core.ErrBadHandle},
		{"source", core.Modifier{Stat: x, Source: nSources}, core.ErrBadHandle},
		{"op", core.Modifier{Stat: x, Op: 99}, core.ErrOutOfRange},
		{"condition actor", core.Modifier{Stat: x, Conds: []core.Cond{{Ref: core.Ref{Actor: 4, Stat: x, Ctx: core.Self}}}}, core.ErrBadHandle},
		{"condition stat", core.Modifier{Stat: x, Conds: []core.Cond{{Ref: core.SelfRef(9)}}}, core.ErrBadHandle},
		{"condition context", core.Modifier{Stat: x, Conds: []core.Cond{{Ref: core.Ref{Actor: core.Self, Stat: x, Ctx: 3}}}}, core.ErrBadHandle},
		{"scale step", core.Modifier{Stat: x, Scales: []core.Scale{{Ref: core.SelfRef(x)}}}, core.ErrOutOfRange},
		{"scale ref", core.Modifier{Stat: x, Scales: []core.Scale{{Ref: core.SelfRef(9), Step: 1}}}, core.ErrBadHandle},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			s := newSession(t, m, 1)
			if _, err := s.AddModifier(testCase.mod); !errors.Is(err, testCase.want) {
				t.Errorf("got %v, want %v", err, testCase.want)
			}
		})
	}
}

func TestModifierCopy(t *testing.T) {
	t.Parallel()
	var b core.ModelBuilder
	x := b.MustAddStat(core.StatDef{Name: "x"})
	y := b.MustAddStat(core.StatDef{Name: "y"})
	m := b.Build()
	testCases := []struct {
		name   string
		mutate func(m *core.Modifier)
	}{
		{"condition", func(m *core.Modifier) { m.Conds[0].Threshold = 5 }},
		{"scale", func(m *core.Modifier) { m.Scales[0].Step = 100 }},
		{"predicate", func(m *core.Modifier) { m.Pred.Req[0] = core.MaskOf(1) }},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			s := newSession(t, m, 1)
			add(t, s, core.Modifier{Stat: y, Value: 4})
			id := add(t, s, core.Modifier{Stat: x, Value: 1,
				Conds:  []core.Cond{{Ref: core.SelfRef(y), Cmp: core.CmpGE, Threshold: 0}},
				Scales: []core.Scale{{Ref: core.SelfRef(y), Step: 1}},
				Pred:   core.Predicate{Req: []core.Mask{core.MaskOf(2)}, Excl: core.MaskOf(3)}})
			got, removed, err := s.Modifier(id)
			if err != nil || removed {
				t.Fatalf("Modifier: got removed %v, %v, want false, nil", removed, err)
			}
			testCase.mutate(&got)
			again, _, err := s.Modifier(id)
			must(t, err)
			if diff := cmp.Diff(again, got); diff == "" {
				t.Error("mutating the copy changed the stored modifier")
			}
		})
	}
}

func TestCounterWraparound(t *testing.T) {
	t.Parallel()
	m, str, _, life := attrModel(t, nil)
	s := newSession(t, m, 1)
	add(t, s, core.Modifier{Stat: life, Op: core.OpBase, Value: 50, Source: 0})
	add(t, s, core.Modifier{Stat: str, Op: core.OpBase, Value: 20, Source: 1})
	want(t, s, life, 60)
	core.NearWrap(s)
	for range 4 {
		must(t, s.SetSourceEnabled(1, false))
		want(t, s, life, 50)
		must(t, s.SetSourceEnabled(1, true))
		want(t, s, life, 60)
		s.Invalidate()
	}
}

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
		id := b.MustAddStat(randStatDef(r, i))
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

func randStatDef(r *rand.Rand, i int) core.StatDef {
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
	if i > flagStat+1 && r.IntN(3) == 0 {
		def.Shares = []core.Share{{
			Stat: core.StatID(flagStat + 1 + r.IntN(i-flagStat-1)),
			Rate: core.SelfRef(core.StatID(r.IntN(i))),
		}}
	}
	return def
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

// numStats counts the stats of m.
func numStats(m *core.Model) int {
	n := 0
	for ; ; n++ {
		if _, ok := m.StatDef(core.StatID(n)); !ok {
			return n
		}
	}
}

// snapshot reads every node, in forward or reverse order.
func snapshot(tb testing.TB, s *core.Session, reverse bool) []uint64 {
	tb.Helper()
	nStats := numStats(s.Model())
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

// TestIncrementalEqualsFresh applies random edits, reverts and commits, reading random nodes
// in between, and checks every node against a session built from scratch.
func TestIncrementalEqualsFresh(t *testing.T) {
	t.Parallel()
	for seed := range uint64(40) {
		t.Run("", func(t *testing.T) {
			t.Parallel()
			r := rand.New(rand.NewPCG(seed, 7))
			w := newRandWorld(t, r)
			e := &editRun{tb: t, w: w, s: w.session(t), r: r}
			for range 40 {
				e.ids = append(e.ids, add(t, e.s, w.mod(r)))
			}
			e.s.Commit()
			base := snapshot(t, e.s, false)
			equalBits(t, -1, snapshot(t, fresh(t, w, e.s), true), base)
			start := e.s.Mark()

			for step := range 60 {
				e.apply(step)
				for range 3 {
					ctx := core.CtxID(r.IntN(len(w.ctxs)))
					get(t, e.s, core.ActorID(r.IntN(w.nActor)), core.StatID(r.IntN(w.nStat)), ctx)
				}
				if step%5 == 4 {
					equalBits(t, step, snapshot(t, e.s, step%2 == 0), snapshot(t, fresh(t, w, e.s), false))
				}
			}
			if !e.committed {
				must(t, e.s.Revert(start))
				equalBits(t, 99, snapshot(t, e.s, true), base)
			}
		})
	}
}

// editRun is the state of one random edit sequence.
type editRun struct {
	tb        testing.TB
	w         *randWorld
	s         *core.Session
	r         *rand.Rand
	ids       []core.ModID
	marks     []core.Checkpoint
	snaps     [][]uint64
	committed bool
}

// apply performs one random edit.
func (e *editRun) apply(step int) {
	t, s, r := e.tb, e.s, e.r
	switch k := r.IntN(20); {
	case k < 6:
		e.ids = append(e.ids, add(t, s, e.w.mod(r)))
	case k < 10:
		must(t, s.SetSourceEnabled(core.SourceID(r.IntN(nSources)), r.IntN(2) == 0))
	case k < 12:
		droppedOK(t, s.RemoveModifier(e.ids[r.IntN(len(e.ids))]))
	case k < 14:
		droppedOK(t, s.SetModifierValue(e.ids[r.IntN(len(e.ids))], float64(r.IntN(41)-5)))
	case k < 16:
		e.marks = append(e.marks, s.Mark())
		e.snaps = append(e.snaps, snapshot(t, s, false))
	case k < 19:
		e.revert(step)
	default:
		e.commit(step)
	}
}

func (e *editRun) revert(step int) {
	if len(e.marks) == 0 {
		return
	}
	i := e.r.IntN(len(e.marks))
	must(e.tb, e.s.Revert(e.marks[i]))
	equalBits(e.tb, step, snapshot(e.tb, e.s, false), e.snaps[i])
	e.marks, e.snaps = e.marks[:i+1], e.snaps[:i+1]
}

func (e *editRun) commit(step int) {
	e.s.Commit()
	e.committed = true
	for _, cp := range e.marks {
		if err := e.s.Revert(cp); !errors.Is(err, core.ErrStaleCheckpoint) {
			e.tb.Fatalf("step %d: revert after commit: got %v, want ErrStaleCheckpoint", step, err)
		}
	}
	e.marks, e.snaps = nil, nil
}

// droppedOK accepts ErrBadHandle: ids of modifiers dropped by Revert stay in the test's list.
func droppedOK(tb testing.TB, err error) {
	tb.Helper()
	if err != nil && !errors.Is(err, core.ErrBadHandle) {
		tb.Fatal(err)
	}
}
