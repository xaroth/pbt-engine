package core_test

import (
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/xaroth/pbt-engine/core"
)

// nSources is the number of sources every test session starts with, so modifiers can name
// sources 0 to nSources-1 directly.
const nSources = 10

func newSession(tb testing.TB, m *core.Model, actors int) *core.Session {
	tb.Helper()
	s, err := core.NewSession(m, actors)
	if err != nil {
		tb.Fatal(err)
	}
	for range nSources {
		s.NewSource()
	}
	return s
}

func add(tb testing.TB, s *core.Session, m core.Modifier) core.ModID {
	tb.Helper()
	id, err := s.AddModifier(m)
	if err != nil {
		tb.Fatal(err)
	}
	return id
}

func addAll(tb testing.TB, s *core.Session, mods []core.Modifier) []core.ModID {
	tb.Helper()
	ids := make([]core.ModID, len(mods))
	for i, m := range mods {
		ids[i] = add(tb, s, m)
	}
	return ids
}

func get(tb testing.TB, s *core.Session, actor core.ActorID, stat core.StatID, ctx core.CtxID) float64 {
	tb.Helper()
	v, err := s.Get(actor, stat, ctx)
	if err != nil {
		tb.Fatal(err)
	}
	return v
}

// want compares bits, so -0 fails against 0.
func want(tb testing.TB, s *core.Session, stat core.StatID, exp float64) {
	tb.Helper()
	if v := get(tb, s, 0, stat, core.GlobalCtx); math.Float64bits(v) != math.Float64bits(exp) {
		def, _ := s.Model().StatDef(stat)
		tb.Errorf("%s: got %v, want %v", def.Name, v, exp)
	}
}

func must(tb testing.TB, err error) {
	tb.Helper()
	if err != nil {
		tb.Fatal(err)
	}
}

func setFormula(tb testing.TB, b *core.ModelBuilder, id core.StatID, f core.Formula) {
	tb.Helper()
	must(tb, b.SetFormula(id, f))
}

// increased100 is 100 with pct percent increased, computed at run time as the evaluator does;
// a constant expression would be exact.
func increased100(pct float64) float64 { return 100 * (1 + pct/100) }

// more applies each of pcts percent more, multiplied together first as the evaluator does.
func more(base float64, pcts ...float64) float64 {
	m := 1.0
	for _, p := range pcts {
		m *= 1 + p/100
	}
	return base * m
}

// evalCase adds mods to a fresh session and reads stats of actor 0 in the context of ctx.
type evalCase struct {
	name string
	mods []core.Modifier
	ctx  core.Mask
	want map[core.StatID]float64
}

func runEval(t *testing.T, m *core.Model, actors int, testCases []evalCase) {
	t.Helper()
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			s := newSession(t, m, actors)
			addAll(t, s, testCase.mods)
			ctx := s.InternContext(testCase.ctx)
			for stat, exp := range testCase.want {
				if v := get(t, s, 0, stat, ctx); math.Float64bits(v) != math.Float64bits(exp) {
					def, _ := m.StatDef(stat)
					t.Errorf("%s: got %v, want %v", def.Name, v, exp)
				}
			}
		})
	}
}

func TestAggregation(t *testing.T) {
	t.Parallel()
	var b core.ModelBuilder
	life := b.MustAddStat(core.StatDef{Name: "life"})
	ele := b.MustAddStat(core.StatDef{Name: "elemental_res"})
	fire := b.MustAddStat(core.StatDef{Name: "fire_res", Includes: []core.StatID{ele}})
	onslaught := b.MustAddStat(core.StatDef{Name: "onslaught", Flag: true})
	runEval(t, b.Build(), 1, []evalCase{
		{name: "no modifiers", want: map[core.StatID]float64{life: 0, onslaught: 0}},
		{
			name: "base sums",
			mods: []core.Modifier{
				{Stat: life, Op: core.OpBase, Value: 38, Source: 1},
				{Stat: life, Op: core.OpBase, Value: 12, Source: 2},
			},
			want: map[core.StatID]float64{life: 50},
		},
		{
			name: "increased sums",
			mods: []core.Modifier{
				{Stat: life, Op: core.OpBase, Value: 50, Source: 1},
				{Stat: life, Op: core.OpInc, Value: 30, Source: 3},
				{Stat: life, Op: core.OpInc, Value: -10, Source: 3},
			},
			want: map[core.StatID]float64{life: 60},
		},
		{
			name: "more multiplies",
			mods: []core.Modifier{
				{Stat: life, Op: core.OpBase, Value: 50, Source: 1},
				{Stat: life, Op: core.OpMore, Value: 50, Source: 4},
				{Stat: life, Op: core.OpMore, Value: -20, Source: 5},
			},
			want: map[core.StatID]float64{life: more(50, 50, -20)},
		},
		{
			name: "last override in source order wins",
			mods: []core.Modifier{
				{Stat: life, Op: core.OpBase, Value: 50, Source: 1},
				{Stat: life, Op: core.OpOverride, Value: 1, Source: 9},
				{Stat: life, Op: core.OpOverride, Value: 7, Source: 8},
			},
			want: map[core.StatID]float64{life: 1},
		},
		{
			name: "includes",
			mods: []core.Modifier{
				{Stat: fire, Op: core.OpBase, Value: 30, Source: 0},
				{Stat: ele, Op: core.OpBase, Value: 12, Source: 1},
			},
			want: map[core.StatID]float64{fire: 42, ele: 12},
		},
		{
			name: "flag",
			mods: []core.Modifier{
				{Stat: onslaught, Op: core.OpFlag, Source: 2},
				{Stat: onslaught, Op: core.OpFlag, Source: 3},
			},
			want: map[core.StatID]float64{onslaught: 1},
		},
	})
}

func TestClampAndRounding(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name  string
		def   core.StatDef
		value float64
		want  float64
	}{
		{"max", core.StatDef{HasMin: true, Min: -60, HasMax: true, Max: 75}, 90, 75},
		{"min", core.StatDef{HasMin: true, Min: -60, HasMax: true, Max: 75}, -100, -60},
		{"half up", core.StatDef{Round: core.RoundHalf}, 2.5, 3},
		{"half negative", core.StatDef{Round: core.RoundHalf}, -2.5, -2},
		{"floor", core.StatDef{Round: core.RoundFloor}, -2.5, -3},
		{"trunc", core.StatDef{Round: core.RoundTrunc}, -2.5, -2},
		{"places", core.StatDef{Round: core.RoundHalf, Places: 1}, 12.345, 12.3},
		{"negative zero", core.StatDef{Round: core.RoundTrunc}, -0.5, 0},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			var b core.ModelBuilder
			testCase.def.Name = "x"
			x := b.MustAddStat(testCase.def)
			s := newSession(t, b.Build(), 1)
			add(t, s, core.Modifier{Stat: x, Op: core.OpBase, Value: testCase.value})
			want(t, s, x, testCase.want)
		})
	}
}

func TestModelBuilderErrors(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name string
		add  func(b *core.ModelBuilder) error
		want error
	}{
		{"duplicate stat", func(b *core.ModelBuilder) error {
			_, err := b.AddStat(core.StatDef{Name: "x"})
			return err
		}, core.ErrDuplicateName},
		{"unknown include", func(b *core.ModelBuilder) error {
			_, err := b.AddStat(core.StatDef{Name: "y", Includes: []core.StatID{7}})
			return err
		}, core.ErrBadHandle},
		{"places", func(b *core.ModelBuilder) error {
			_, err := b.AddStat(core.StatDef{Name: "y", Places: 16})
			return err
		}, core.ErrOutOfRange},
		{"rounding", func(b *core.ModelBuilder) error {
			_, err := b.AddStat(core.StatDef{Name: "y", Round: 9})
			return err
		}, core.ErrOutOfRange},
		{"formula of unknown stat", func(b *core.ModelBuilder) error {
			return b.SetFormula(7, func(core.Reader, core.Agg) float64 { return 0 })
		}, core.ErrBadHandle},
		{"duplicate dimension", func(b *core.ModelBuilder) error {
			_, err := b.AddDimension("d", "b")
			return err
		}, core.ErrDuplicateName},
		{"duplicate dimension value", func(b *core.ModelBuilder) error {
			_, err := b.AddDimension("e", "a", "a")
			return err
		}, core.ErrDuplicateName},
		{"dimension value name taken", func(b *core.ModelBuilder) error {
			if _, err := b.AddDimension("p.q", "r"); err != nil {
				return err
			}
			_, err := b.AddDimension("p", "q.r")
			return err
		}, core.ErrDuplicateName},
		{"too many bits", func(b *core.ModelBuilder) error {
			_, err := b.AddDimension("big", make([]string, core.MaxBits)...)
			return err
		}, core.ErrOutOfRange},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			var b core.ModelBuilder
			b.MustAddStat(core.StatDef{Name: "x"})
			if _, err := b.AddDimension("d", "a"); err != nil {
				t.Fatal(err)
			}
			if err := testCase.add(&b); !errors.Is(err, testCase.want) {
				t.Errorf("got %v, want %v", err, testCase.want)
			}
		})
	}
}

func TestBuildCopies(t *testing.T) {
	t.Parallel()
	var b core.ModelBuilder
	x := b.MustAddStat(core.StatDef{Name: "x"})
	m := b.Build()
	b.MustAddStat(core.StatDef{Name: "y"})
	setFormula(t, &b, x, func(core.Reader, core.Agg) float64 { return 9 })
	if _, err := b.AddDimension("d", "a"); err != nil {
		t.Fatal(err)
	}
	if n := m.NumStats(); n != 1 {
		t.Errorf("NumStats: got %d, want 1", n)
	}
	if _, ok := m.Stat("y"); ok {
		t.Error("Stat(y): got found, want not found")
	}
	if _, ok := m.Bit("d.a"); ok {
		t.Error("Bit(d.a): got found, want not found")
	}
	s := newSession(t, m, 1)
	add(t, s, core.Modifier{Stat: x, Value: 2})
	want(t, s, x, 2)
}

func TestNewSessionErrors(t *testing.T) {
	t.Parallel()
	var b core.ModelBuilder
	m := b.Build()
	for _, actors := range []int{0, math.MaxUint16 + 1} {
		if _, err := core.NewSession(m, actors); !errors.Is(err, core.ErrOutOfRange) {
			t.Errorf("%d actors: got %v, want ErrOutOfRange", actors, err)
		}
	}
}

func TestContexts(t *testing.T) {
	t.Parallel()
	var b core.ModelBuilder
	dmg := b.MustAddStat(core.StatDef{Name: "damage"})
	types, err := b.AddDimension("type", "fire", "cold", "physical")
	if err != nil {
		t.Fatal(err)
	}
	delivery, err := b.AddDimension("delivery", "attack", "spell")
	if err != nil {
		t.Fatal(err)
	}
	m := b.Build()
	fire, cold, phys := types.Bit(0), types.Bit(1), types.Bit(2)
	attack, spell := delivery.Bit(0), delivery.Bit(1)
	if got, ok := m.Bit("delivery.spell"); !ok || got != spell {
		t.Fatalf("Bit(delivery.spell): got %v, %v, want %v, true", got, ok, spell)
	}
	mods := []core.Modifier{
		{Stat: dmg, Op: core.OpBase, Value: 10, Source: 0},
		{Stat: dmg, Op: core.OpInc, Value: 100, Source: 1,
			Pred: core.Predicate{Req: []core.Mask{core.MaskOf(fire, cold)}}},
		{Stat: dmg, Op: core.OpInc, Value: 50, Source: 2,
			Pred: core.Predicate{Req: []core.Mask{core.MaskOf(cold), core.MaskOf(attack)}}},
		{Stat: dmg, Op: core.OpMore, Value: 100, Source: 3,
			Pred: core.Predicate{Excl: core.MaskOf(spell)}},
	}
	runEval(t, m, 1, []evalCase{
		{name: "global", mods: mods, want: map[core.StatID]float64{dmg: 20}},
		{name: "fire spell", mods: mods, ctx: core.MaskOf(fire, spell), want: map[core.StatID]float64{dmg: 20}},
		{name: "cold attack", mods: mods, ctx: core.MaskOf(cold, attack), want: map[core.StatID]float64{dmg: 50}},
		{name: "physical attack", mods: mods, ctx: core.MaskOf(phys, attack), want: map[core.StatID]float64{dmg: 20}},
	})
}

func TestInternContext(t *testing.T) {
	t.Parallel()
	var b core.ModelBuilder
	s := newSession(t, b.Build(), 1)
	a := s.InternContext(core.MaskOf(1, 5))
	if got := s.InternContext(core.MaskOf(5, 1)); got != a {
		t.Errorf("same mask: got %d, want %d", got, a)
	}
	if got := s.InternContext(core.Mask{}); got != core.GlobalCtx {
		t.Errorf("empty mask: got %d, want GlobalCtx", got)
	}
	if mask, ok := s.ContextMask(a); !ok || mask != core.MaskOf(1, 5) {
		t.Errorf("ContextMask: got %v, %v, want the interned mask, true", mask, ok)
	}
	if n := s.NumContexts(); n != 2 {
		t.Errorf("NumContexts: got %d, want 2", n)
	}
}

func TestConditions(t *testing.T) {
	t.Parallel()
	var b core.ModelBuilder
	life := b.MustAddStat(core.StatDef{Name: "life"})
	shocked := b.MustAddStat(core.StatDef{Name: "shocked", Flag: true})
	dmg := b.MustAddStat(core.StatDef{Name: "damage"})
	const enemy = 1
	onLife := func(c core.Cmp, threshold float64) []core.Modifier {
		return []core.Modifier{
			{Stat: life, Op: core.OpBase, Value: 100},
			{Stat: dmg, Op: core.OpBase, Value: 10},
			{Stat: dmg, Op: core.OpInc, Value: 50, Source: 1,
				Conds: []core.Cond{{Ref: core.SelfRef(life), Cmp: c, Threshold: threshold}}},
		}
	}
	applies := map[core.StatID]float64{dmg: 15}
	skipped := map[core.StatID]float64{dmg: 10}
	enemyShocked := core.Cond{Ref: core.Ref{Actor: enemy, Stat: shocked, Ctx: core.Self}, Cmp: core.CmpEQ, Threshold: 1}
	runEval(t, b.Build(), 2, []evalCase{
		{name: "GE equal", mods: onLife(core.CmpGE, 100), want: applies},
		{name: "GE below", mods: onLife(core.CmpGE, 101), want: skipped},
		{name: "GT equal", mods: onLife(core.CmpGT, 100), want: skipped},
		{name: "GT above", mods: onLife(core.CmpGT, 99), want: applies},
		{name: "LE equal", mods: onLife(core.CmpLE, 100), want: applies},
		{name: "LE above", mods: onLife(core.CmpLE, 99), want: skipped},
		{name: "LT equal", mods: onLife(core.CmpLT, 100), want: skipped},
		{name: "LT below", mods: onLife(core.CmpLT, 101), want: applies},
		{name: "EQ equal", mods: onLife(core.CmpEQ, 100), want: applies},
		{name: "EQ other", mods: onLife(core.CmpEQ, 99), want: skipped},
		{name: "NE equal", mods: onLife(core.CmpNE, 100), want: skipped},
		{name: "NE other", mods: onLife(core.CmpNE, 99), want: applies},
		{
			name: "all conditions hold",
			mods: []core.Modifier{
				{Stat: life, Op: core.OpBase, Value: 110},
				{Stat: dmg, Op: core.OpBase, Value: 10},
				{Stat: dmg, Op: core.OpInc, Value: 50, Source: 1, Conds: []core.Cond{
					{Ref: core.SelfRef(life), Cmp: core.CmpGT, Threshold: 100},
					{Ref: core.SelfRef(life), Cmp: core.CmpLE, Threshold: 120},
				}},
			},
			want: applies,
		},
		{
			name: "one condition fails",
			mods: []core.Modifier{
				{Stat: life, Op: core.OpBase, Value: 130},
				{Stat: dmg, Op: core.OpBase, Value: 10},
				{Stat: dmg, Op: core.OpInc, Value: 50, Source: 1, Conds: []core.Cond{
					{Ref: core.SelfRef(life), Cmp: core.CmpGT, Threshold: 100},
					{Ref: core.SelfRef(life), Cmp: core.CmpLE, Threshold: 120},
				}},
			},
			want: skipped,
		},
		{
			name: "other actor",
			mods: []core.Modifier{
				{Stat: dmg, Op: core.OpBase, Value: 10},
				{Stat: dmg, Op: core.OpInc, Value: 50, Source: 1, Conds: []core.Cond{enemyShocked}},
				{Stat: shocked, Actor: enemy, Op: core.OpFlag, Source: 2},
			},
			want: applies,
		},
		{
			name: "other actor not shocked",
			mods: []core.Modifier{
				{Stat: dmg, Op: core.OpBase, Value: 10},
				{Stat: dmg, Op: core.OpInc, Value: 50, Source: 1, Conds: []core.Cond{enemyShocked}},
				{Stat: shocked, Op: core.OpFlag, Source: 2},
			},
			want: skipped,
		},
	})
}

func TestScaling(t *testing.T) {
	t.Parallel()
	var b core.ModelBuilder
	str := b.MustAddStat(core.StatDef{Name: "str"})
	dmg := b.MustAddStat(core.StatDef{Name: "damage"})
	perStr := func(step float64, scales ...core.Scale) []core.Modifier {
		return []core.Modifier{
			{Stat: str, Op: core.OpBase, Value: step},
			{Stat: dmg, Op: core.OpBase, Value: 100},
			{Stat: dmg, Op: core.OpInc, Value: 2, Source: 1, Scales: scales},
		}
	}
	runEval(t, b.Build(), 1, []evalCase{
		{
			name: "step count floors",
			mods: perStr(55, core.Scale{Ref: core.SelfRef(str), Step: 10}),
			want: map[core.StatID]float64{dmg: increased100(2 * 5)},
		},
		{
			name: "cap limits the step count",
			mods: perStr(55, core.Scale{Ref: core.SelfRef(str), Step: 10, Cap: 3, HasCap: true}),
			want: map[core.StatID]float64{dmg: increased100(2 * 3)},
		},
		{
			name: "cap of zero steps",
			mods: perStr(55, core.Scale{Ref: core.SelfRef(str), Step: 10, HasCap: true}),
			want: map[core.StatID]float64{dmg: 100},
		},
		{
			name: "scales multiply",
			mods: perStr(55, core.Scale{Ref: core.SelfRef(str), Step: 5}, core.Scale{Ref: core.SelfRef(str), Step: 50}),
			want: map[core.StatID]float64{dmg: increased100(2 * 11 * 1)},
		},
	})
}

func TestTerms(t *testing.T) {
	t.Parallel()
	var b core.ModelBuilder
	str := b.MustAddStat(core.StatDef{Name: "str"})
	dmg := b.MustAddStat(core.StatDef{Name: "damage", Round: core.RoundFloor})
	s := newSession(t, b.Build(), 1)
	ids := addAll(t, s, []core.Modifier{
		{Stat: str, Op: core.OpBase, Value: 25, Source: 0},
		{Stat: dmg, Op: core.OpInc, Value: 3, Source: 5, Scales: []core.Scale{{Ref: core.SelfRef(str), Step: 10}}},
		{Stat: dmg, Op: core.OpBase, Value: 10.5, Source: 2},
		{Stat: dmg, Op: core.OpMore, Value: 50, Source: 7, Conds: []core.Cond{{Ref: core.SelfRef(str), Cmp: core.CmpGT, Threshold: 30}}},
	})
	terms, v, err := s.Terms(core.Node{Stat: dmg})
	if err != nil {
		t.Fatal(err)
	}
	wantTerms := []core.Term{
		{Mod: ids[2], Source: 2, Op: core.OpBase, Value: 10.5},
		{Mod: ids[1], Source: 5, Op: core.OpInc, Value: 6},
	}
	if diff := cmp.Diff(wantTerms, terms, cmp.Comparer(func(a, b core.ModID) bool { return a == b })); diff != "" {
		t.Errorf("terms (-want +got):\n%s", diff)
	}
	if v != 11 {
		t.Errorf("value: got %v, want 11", v)
	}
}

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

// attrModel has strength and dexterity in one CycleFamily, plus a life formula reading
// strength. calls, when not nil, counts life formula calls.
func attrModel(tb testing.TB, calls *int) (m *core.Model, str, dex, life core.StatID) {
	tb.Helper()
	var b core.ModelBuilder
	str = b.MustAddStat(core.StatDef{Name: "str", CycleFamily: 1, Round: core.RoundFloor})
	dex = b.MustAddStat(core.StatDef{Name: "dex", CycleFamily: 1, Round: core.RoundFloor})
	life = b.MustAddStat(core.StatDef{Name: "life", Round: core.RoundHalf})
	setFormula(tb, &b, life, func(r core.Reader, a core.Agg) float64 {
		if calls != nil {
			*calls++
		}
		base := a.Base + math.Floor(r.Stat(str)/2)
		return base * (1 + a.Inc/100) * a.More
	})
	return b.Build(), str, dex, life
}

func TestMemoisation(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name      string
		edit      func(s *core.Session, str, dex core.StatID) error
		want      float64
		wantCalls int
	}{
		{"no edit", func(*core.Session, core.StatID, core.StatID) error { return nil }, 72, 1},
		{"unrelated stat", func(s *core.Session, _, dex core.StatID) error {
			_, err := s.AddModifier(core.Modifier{Stat: dex, Op: core.OpBase, Value: 5, Source: 4})
			return err
		}, 72, 1},
		{"read stat, same rounded value", func(s *core.Session, str, _ core.StatID) error {
			_, err := s.AddModifier(core.Modifier{Stat: str, Op: core.OpBase, Value: 0.4, Source: 4})
			return err
		}, 72, 1},
		{"read stat changes", func(s *core.Session, _, _ core.StatID) error {
			return s.SetSourceEnabled(2, false)
		}, 66, 2},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			calls := 0
			m, str, dex, life := attrModel(t, &calls)
			s := newSession(t, m, 1)
			addAll(t, s, []core.Modifier{
				{Stat: str, Op: core.OpBase, Value: 20, Source: 1},
				{Stat: life, Op: core.OpBase, Value: 50, Source: 1},
				{Stat: str, Op: core.OpBase, Value: 10, Source: 2},
				{Stat: life, Op: core.OpInc, Value: 10, Source: 3},
			})
			want(t, s, life, 72) // (50 + 30/2) * 1.1 = 71.5
			must(t, testCase.edit(s, str, dex))
			want(t, s, life, testCase.want)
			want(t, s, life, testCase.want)
			if calls != testCase.wantCalls {
				t.Errorf("formula calls: got %d, want %d", calls, testCase.wantCalls)
			}
		})
	}
}

func TestCycleFamily(t *testing.T) {
	t.Parallel()
	m, str, dex, life := attrModel(t, nil)
	runEval(t, m, 1, []evalCase{
		{
			// +1 dex per 10 str, +1 str per 10 dex: each sees the other's unconditional value.
			name: "scales read the unconditional value",
			mods: []core.Modifier{
				{Stat: str, Op: core.OpBase, Value: 100, Source: 1},
				{Stat: dex, Op: core.OpBase, Value: 50, Source: 1},
				{Stat: life, Op: core.OpBase, Value: 50, Source: 1},
				{Stat: dex, Op: core.OpBase, Value: 1, Source: 2, Scales: []core.Scale{{Ref: core.SelfRef(str), Step: 10}}},
				{Stat: str, Op: core.OpBase, Value: 1, Source: 3, Scales: []core.Scale{{Ref: core.SelfRef(dex), Step: 10}}},
				{Stat: str, Op: core.OpInc, Value: 10, Source: 4},
			},
			want: map[core.StatID]float64{str: math.Floor((100 + 5) * 1.1), dex: 50 + 11, life: 50 + 57},
		},
		{
			name: "conditions read the unconditional value",
			mods: []core.Modifier{
				{Stat: str, Op: core.OpBase, Value: 10, Source: 1},
				{Stat: dex, Op: core.OpBase, Value: 5, Source: 2, Conds: []core.Cond{{Ref: core.SelfRef(str), Cmp: core.CmpGE, Threshold: 20}}},
				{Stat: str, Op: core.OpBase, Value: 10, Source: 3, Conds: []core.Cond{{Ref: core.SelfRef(dex), Cmp: core.CmpGE, Threshold: 0}}},
			},
			want: map[core.StatID]float64{str: 20, dex: 0},
		},
	})

	var b core.ModelBuilder
	fStr := b.MustAddStat(core.StatDef{Name: "str", CycleFamily: 1})
	fDex := b.MustAddStat(core.StatDef{Name: "dex", CycleFamily: 1})
	other := b.MustAddStat(core.StatDef{Name: "other", CycleFamily: 2})
	z := b.MustAddStat(core.StatDef{Name: "z"})
	setFormula(t, &b, fStr, func(r core.Reader, a core.Agg) float64 { return a.Base + r.Stat(fDex) + r.Stat(other) })
	runEval(t, b.Build(), 1, []evalCase{
		{
			// str's formula reads unconditional dex (0) but full other (6), being another family.
			name: "formulas read the unconditional value",
			mods: []core.Modifier{
				{Stat: fStr, Op: core.OpBase, Value: 10},
				{Stat: fDex, Op: core.OpBase, Value: 1, Scales: []core.Scale{{Ref: core.SelfRef(fStr), Step: 1}}},
				{Stat: z, Op: core.OpBase, Value: 1},
				{Stat: other, Op: core.OpBase, Value: 5},
				{Stat: other, Op: core.OpBase, Value: 1, Conds: []core.Cond{{Ref: core.SelfRef(z), Cmp: core.CmpGE, Threshold: 1}}},
			},
			want: map[core.StatID]float64{fStr: 16, fDex: 16, other: 6},
		},
	})
}

func TestCycleError(t *testing.T) {
	t.Parallel()
	var b core.ModelBuilder
	a := b.MustAddStat(core.StatDef{Name: "a"})
	bb := b.MustAddStat(core.StatDef{Name: "b"})
	f := b.MustAddStat(core.StatDef{Name: "f"})
	setFormula(t, &b, f, func(r core.Reader, ag core.Agg) float64 { return ag.Base + r.Stat(a) })
	famA := b.MustAddStat(core.StatDef{Name: "fam_a", CycleFamily: 1})
	famC := b.MustAddStat(core.StatDef{Name: "fam_c", CycleFamily: 1})
	setFormula(t, &b, famA, func(r core.Reader, ag core.Agg) float64 { return ag.Base + r.Stat(famC) })
	setFormula(t, &b, famC, func(r core.Reader, ag core.Agg) float64 { return ag.Base + r.Stat(famA) })
	m := b.Build()
	testCases := []struct {
		name     string
		get      core.StatID
		mods     []core.Modifier
		wantPath []core.Node
		wantMsg  string
	}{
		{
			name:     "self",
			get:      a,
			mods:     []core.Modifier{{Stat: a, Value: 1, Scales: []core.Scale{{Ref: core.SelfRef(a), Step: 1}}}},
			wantPath: []core.Node{{Stat: a}, {Stat: a}},
			wantMsg:  "core: dependency cycle: a[actor 0 ctx 0] -> a[actor 0 ctx 0]",
		},
		{
			name: "scale and condition",
			get:  a,
			mods: []core.Modifier{
				{Stat: a, Value: 1, Scales: []core.Scale{{Ref: core.SelfRef(bb), Step: 1}}},
				{Stat: bb, Value: 1, Conds: []core.Cond{{Ref: core.SelfRef(a), Cmp: core.CmpGE}}},
			},
			wantPath: []core.Node{{Stat: a}, {Stat: bb}, {Stat: a}},
			wantMsg:  "a[actor 0 ctx 0] -> b[actor 0 ctx 0] -> a[actor 0 ctx 0]",
		},
		{
			name: "formula across actors",
			get:  a,
			mods: []core.Modifier{
				{Stat: a, Value: 1, Conds: []core.Cond{{Ref: core.Ref{Actor: 1, Stat: f, Ctx: core.Self}, Cmp: core.CmpGE}}},
				{Stat: a, Actor: 1, Value: 1, Conds: []core.Cond{{Ref: core.Ref{Actor: 1, Stat: f, Ctx: core.Self}, Cmp: core.CmpGE}}},
			},
			wantPath: []core.Node{{Actor: 1, Stat: f}, {Actor: 1, Stat: a}, {Actor: 1, Stat: f}},
			wantMsg:  "f[actor 1 ctx 0] -> a[actor 1 ctx 0] -> f[actor 1 ctx 0]",
		},
		{
			// Unconditional nodes still run their formulas.
			name:     "formulas in one family",
			get:      famA,
			wantPath: []core.Node{{Stat: famC, Uncond: true}, {Stat: famA, Uncond: true}, {Stat: famC, Uncond: true}},
			wantMsg:  "fam_c[actor 0 ctx 0 uncond] -> fam_a[actor 0 ctx 0 uncond] -> fam_c[actor 0 ctx 0 uncond]",
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			s := newSession(t, m, 2)
			addAll(t, s, testCase.mods)
			for range 2 { // a failed read leaves no partial state behind
				_, err := s.Get(0, testCase.get, core.GlobalCtx)
				ce, ok := errors.AsType[*core.CycleError](err)
				if !ok {
					t.Fatalf("got %v, want CycleError", err)
				}
				if diff := cmp.Diff(testCase.wantPath, ce.Path); diff != "" {
					t.Errorf("path (-want +got):\n%s", diff)
				}
				if !strings.Contains(err.Error(), testCase.wantMsg) {
					t.Errorf("message: got %q, want it to contain %q", err, testCase.wantMsg)
				}
			}
		})
	}
}

func TestFormulaPanic(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name string
		read func(s *core.Session, x core.StatID) error
	}{
		{"Get", func(s *core.Session, x core.StatID) error {
			_, err := s.Get(0, x, core.GlobalCtx)
			return err
		}},
		{"Terms", func(s *core.Session, x core.StatID) error {
			_, _, err := s.Terms(core.Node{Stat: x})
			return err
		}},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			var b core.ModelBuilder
			x := b.MustAddStat(core.StatDef{Name: "x"})
			fail := true
			setFormula(t, &b, x, func(_ core.Reader, a core.Agg) float64 {
				if fail {
					panic("boom")
				}
				return a.Base
			})
			s := newSession(t, b.Build(), 1)
			add(t, s, core.Modifier{Stat: x, Value: 3})
			func() {
				defer func() {
					if recover() == nil {
						t.Error("got no panic, want the formula's panic")
					}
				}()
				_ = testCase.read(s, x)
			}()
			fail = false
			want(t, s, x, 3)
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

func TestBadHandles(t *testing.T) {
	t.Parallel()
	var b core.ModelBuilder
	x := b.MustAddStat(core.StatDef{Name: "x"})
	badRead := func(name string, ref core.Ref) core.StatID {
		id := b.MustAddStat(core.StatDef{Name: name})
		setFormula(t, &b, id, func(r core.Reader, _ core.Agg) float64 { return r.Read(ref) })
		return id
	}
	readActor := badRead("read_actor", core.Ref{Actor: 3, Stat: x, Ctx: core.Self})
	// Past the range of ActorID and CtxID, these would convert to actor 0 and the global context.
	readWideActor := badRead("read_wide_actor", core.Ref{Actor: 1 << 16, Stat: x, Ctx: core.Self})
	readWideCtx := badRead("read_wide_ctx", core.Ref{Actor: core.Self, Stat: x, Ctx: 1 << 32})
	m := b.Build()
	testCases := []struct {
		name string
		call func(s *core.Session) error
	}{
		{"get actor", func(s *core.Session) error { _, err := s.Get(1, x, core.GlobalCtx); return err }},
		{"get stat", func(s *core.Session) error { _, err := s.Get(0, 5, core.GlobalCtx); return err }},
		{"get context", func(s *core.Session) error { _, err := s.Get(0, x, 3); return err }},
		{"terms", func(s *core.Session) error { _, _, err := s.Terms(core.Node{Stat: 5}); return err }},
		{"formula read actor", func(s *core.Session) error { _, err := s.Get(0, readActor, core.GlobalCtx); return err }},
		{"formula read actor past ActorID", func(s *core.Session) error { _, err := s.Get(0, readWideActor, core.GlobalCtx); return err }},
		{"formula read context past CtxID", func(s *core.Session) error { _, err := s.Get(0, readWideCtx, core.GlobalCtx); return err }},
		{"remove zero id", func(s *core.Session) error { return s.RemoveModifier(core.ModID{}) }},
		{"value zero id", func(s *core.Session) error { return s.SetModifierValue(core.ModID{}, 1) }},
		{"modifier zero id", func(s *core.Session) error { _, _, err := s.Modifier(core.ModID{}); return err }},
		{"source", func(s *core.Session) error { return s.SetSourceEnabled(nSources, false) }},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			s := newSession(t, m, 1)
			add(t, s, core.Modifier{Stat: x, Value: 2})
			if err := testCase.call(s); !errors.Is(err, core.ErrBadHandle) {
				t.Errorf("got %v, want ErrBadHandle", err)
			}
			want(t, s, x, 2)
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

func TestConcurrentSessions(t *testing.T) {
	t.Parallel()
	m, str, _, life := attrModel(t, nil)
	for range 4 {
		t.Run("", func(t *testing.T) {
			t.Parallel()
			s := newSession(t, m, 1)
			add(t, s, core.Modifier{Stat: str, Op: core.OpBase, Value: 20})
			want(t, s, life, 10)
		})
	}
}
