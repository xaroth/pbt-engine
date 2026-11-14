package core_test

import (
	"errors"
	"math"
	"math/rand/v2"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/xaroth/pbt-engine/internal/core"
)

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

// A formula reads a stat in its own context with more bits set: a spell's total is the sum of
// its fire and cold parts, and follows a change to one part.
func TestStatWith(t *testing.T) {
	t.Parallel()
	var b core.ModelBuilder
	dmg := b.MustAddStat(core.StatDef{Name: "damage"})
	types, err := b.AddDimension("type", "fire", "cold", "spell")
	if err != nil {
		t.Fatal(err)
	}
	fire, cold, spell := types.Bit(0), types.Bit(1), types.Bit(2)
	total := b.MustAddStat(core.StatDef{Name: "total", Formula: func(r core.Reader, _ core.Agg) float64 {
		return r.StatWith(dmg, core.MaskOf(fire)) + r.StatWith(dmg, core.MaskOf(cold))
	}})
	s := newSession(t, b.Build(), 1)
	add(t, s, core.Modifier{Stat: dmg, Op: core.OpBase, Value: 10, Pred: core.Predicate{Req: []core.Mask{core.MaskOf(spell)}}})
	id := add(t, s, core.Modifier{Stat: dmg, Op: core.OpInc, Value: 100, Pred: core.Predicate{Req: []core.Mask{core.MaskOf(fire)}}})
	n := core.Node{Stat: total, Ctx: s.InternContext(core.MaskOf(spell))}
	read := func(want float64) {
		t.Helper()
		if got, err := s.Get(n); err != nil || got != want {
			t.Errorf("total: got %v, %v, want %v", got, err, want)
		}
	}
	read(30)
	if err := s.SetModifierValue(id, 50); err != nil {
		t.Fatal(err)
	}
	read(25)
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
			return wrap(t, "add modifier", err)
		}, 72, 1},
		{"read stat, same rounded value", func(s *core.Session, str, _ core.StatID) error {
			_, err := s.AddModifier(core.Modifier{Stat: str, Op: core.OpBase, Value: 0.4, Source: 4})
			return wrap(t, "add modifier", err)
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
				_, err := s.Get(core.Node{Stat: testCase.get})
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
			_, err := s.Get(core.Node{Stat: x})
			return wrap(t, "get", err)
		}},
		{"Explain", func(s *core.Session, x core.StatID) error {
			_, err := s.Explain(core.Node{Stat: x})
			return wrap(t, "explain", err)
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

// sheetWorld approximates a character sheet over a full passive tree: 2400 sources with
// 1 or 2 modifiers each over attributes, pools, defences and filler stats, a tenth of them
// conditional or scaled.
type sheetWorld struct {
	m       *core.Model
	mods    []core.Modifier
	outputs []core.StatID
	life    core.StatID
	nSrc    int
}

func newSheetWorld(b *testing.B) *sheetWorld {
	b.Helper()
	w := &sheetWorld{nSrc: 2400}
	var mb core.ModelBuilder
	st := w.addStats(b, &mb)
	r := rand.New(rand.NewPCG(1, 2))
	for src := range w.nSrc {
		for range 1 + r.IntN(2) {
			w.mods = append(w.mods, st.randMod(r, src))
		}
	}
	for _, a := range st.attrs { // class base
		w.mods = append(w.mods, core.Modifier{Stat: a, Op: core.OpBase, Value: 20, Source: core.SourceID(w.nSrc)})
	}
	w.m = mb.Build()
	return w
}

// sheetStats are the stats random modifiers of a sheetWorld target.
type sheetStats struct {
	attrs, targets, filler []core.StatID
	low                    core.StatID
}

func (w *sheetWorld) addStats(b *testing.B, mb *core.ModelBuilder) sheetStats {
	b.Helper()
	str := mb.MustAddStat(core.StatDef{Name: "str", CycleFamily: 1, Round: core.RoundFloor})
	dex := mb.MustAddStat(core.StatDef{Name: "dex", CycleFamily: 1, Round: core.RoundFloor})
	in := mb.MustAddStat(core.StatDef{Name: "int", CycleFamily: 1, Round: core.RoundFloor})
	pool := func(name string, attr core.StatID, div float64) core.StatID {
		id := mb.MustAddStat(core.StatDef{Name: name, Round: core.RoundHalf})
		setFormula(b, mb, id, func(r core.Reader, a core.Agg) float64 {
			return (a.Base + math.Floor(r.Stat(attr)/div)) * (1 + a.Inc/100) * a.More
		})
		return id
	}
	w.life = pool("life", str, 2)
	mana := pool("mana", in, 2)
	es := pool("es", in, 10)
	eva := pool("evasion", dex, 5)
	arm := mb.MustAddStat(core.StatDef{Name: "armour", Round: core.RoundHalf})
	ele := mb.MustAddStat(core.StatDef{Name: "ele_res"})
	elements := []string{"fire", "cold", "lightning"}
	res := make([]core.StatID, 0, len(elements))
	for _, n := range elements {
		res = append(res, mb.MustAddStat(core.StatDef{Name: n + "_res", Includes: []core.StatID{ele}, HasMax: true, Max: 75}))
	}
	regen := mb.MustAddStat(core.StatDef{Name: "life_regen", Round: core.RoundHalf, Places: 1})
	setFormula(b, mb, regen, func(r core.Reader, a core.Agg) float64 { return r.Stat(w.life) * a.Base / 100 })
	low := mb.MustAddStat(core.StatDef{Name: "low_life", Flag: true})
	const nFiller = 60
	filler := make([]core.StatID, 0, nFiller)
	for i := range nFiller {
		filler = append(filler, mb.MustAddStat(core.StatDef{Name: "filler" + string(rune('a'+i/26)) + string(rune('a'+i%26))}))
	}
	w.outputs = append([]core.StatID{str, dex, in, w.life, mana, es, eva, arm, regen}, res...)
	return sheetStats{
		attrs:   []core.StatID{str, dex, in},
		targets: append([]core.StatID{w.life, mana, es, eva, arm, ele, regen}, res...),
		filler:  filler,
		low:     low,
	}
}

func (st sheetStats) randMod(r *rand.Rand, src int) core.Modifier {
	mod := core.Modifier{Source: core.SourceID(src), Value: float64(5 + r.IntN(20))}
	switch k := r.IntN(10); {
	case k < 3:
		mod.Stat = st.attrs[r.IntN(3)]
		mod.Value = 10
	case k < 7:
		mod.Stat = st.targets[r.IntN(len(st.targets))]
		mod.Op = core.Op(r.IntN(2))
	default:
		mod.Stat = st.filler[r.IntN(len(st.filler))]
		mod.Op = core.Op(r.IntN(3))
	}
	if r.IntN(10) == 0 {
		if r.IntN(2) == 0 {
			mod.Conds = []core.Cond{{Ref: core.SelfRef(st.low), Cmp: core.CmpGE, Threshold: 1}}
		} else {
			mod.Scales = []core.Scale{{Ref: core.SelfRef(st.attrs[r.IntN(3)]), Step: 10, Cap: 5, HasCap: true}}
		}
	}
	return mod
}

func (w *sheetWorld) session(b *testing.B) *core.Session {
	b.Helper()
	s, err := core.NewSession(w.m, 2)
	if err != nil {
		b.Fatal(err)
	}
	for range w.nSrc + 1 {
		s.NewSource()
	}
	for _, m := range w.mods {
		if _, err := s.AddModifier(m); err != nil {
			b.Fatal(err)
		}
	}
	s.Commit()
	return s
}

func (w *sheetWorld) sheet(b *testing.B, s *core.Session) {
	b.Helper()
	for _, o := range w.outputs {
		if _, err := s.Get(core.Node{Stat: o}); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkAssemble builds a session from a sheet's worth of modifiers, part of opening a
// session from a build (target for the whole open: < 10 ms native).
func BenchmarkAssemble(b *testing.B) {
	w := newSheetWorld(b)
	b.ReportAllocs()
	for b.Loop() {
		w.session(b)
	}
	b.ReportMetric(float64(len(w.mods)), "mods")
}

// BenchmarkSheetCold evaluates every sheet output from cold caches (target for a typical
// build: < 3 ms native).
func BenchmarkSheetCold(b *testing.B) {
	w := newSheetWorld(b)
	s := w.session(b)
	w.sheet(b, s)
	b.ReportAllocs()
	for b.Loop() {
		s.Invalidate()
		w.sheet(b, s)
	}
}

// BenchmarkNodePower disables each source in turn, reads life and reverts, as node power does
// (target per what-if: < 0.2 ms native; ns/whatif).
func BenchmarkNodePower(b *testing.B) {
	w := newSheetWorld(b)
	s := w.session(b)
	w.sheet(b, s)
	b.ReportAllocs()
	whatIfs := 0
	for b.Loop() {
		for src := range core.SourceID(w.nSrc) {
			cp := s.Mark()
			if err := s.SetSourceEnabled(src, false); err != nil {
				b.Fatal(err)
			}
			if _, err := s.Get(core.Node{Stat: w.life}); err != nil {
				b.Fatal(err)
			}
			if err := s.Revert(cp); err != nil {
				b.Fatal(err)
			}
			whatIfs++
		}
	}
	b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(whatIfs), "ns/whatif")
}

func TestMoreRule(t *testing.T) {
	t.Parallel()
	var b core.ModelBuilder
	generic := b.MustAddStat(core.StatDef{Name: "generic_damage"})
	scale := b.MustAddStat(core.StatDef{Name: "scale"})
	damage := b.MustAddStat(core.StatDef{Name: "damage", Includes: []core.StatID{generic}})
	var seen [][]core.MoreProduct
	b.SetMoreRule(func(ps []core.MoreProduct) float64 {
		seen = append(seen, append([]core.MoreProduct(nil), ps...))
		m := 1.0
		for _, p := range ps {
			m *= math.Floor(p.Value*100+0.5) / 100
		}
		return m
	})
	s := newSession(t, b.Build(), 1)
	addAll(t, s, []core.Modifier{
		{Stat: damage, Op: core.OpBase, Value: 100, Source: 1},
		{Stat: damage, Op: core.OpMore, Value: 39, Source: 1, Group: 1},
		// Its value reads scale, whose evaluation nests inside damage's.
		{Stat: damage, Op: core.OpMore, Value: 39, Source: 2, Group: 1, Scales: []core.Scale{{Ref: core.SelfRef(scale), Step: 1}}},
		{Stat: damage, Op: core.OpMore, Value: 39, Source: 3, Group: 2},
		{Stat: generic, Op: core.OpMore, Value: 39, Source: 4, Group: 1},
		{Stat: scale, Op: core.OpBase, Value: 1, Source: 5},
		{Stat: scale, Op: core.OpMore, Value: 0.4, Source: 5},
	})
	if got, want := get(t, s, 0, damage, core.GlobalCtx), 100*1.93*1.39*1.39; !near(got, want) {
		t.Errorf("damage %v, want %v", got, want)
	}
	want := []core.MoreProduct{{Stat: damage, Group: 1, Value: 1.39 * 1.39}, {Stat: damage, Group: 2, Value: 1.39}, {Stat: generic, Group: 1, Value: 1.39}}
	if diff := cmp.Diff(want, seen[len(seen)-1], cmp.Comparer(near)); diff != "" {
		t.Errorf("products (-want +got):\n%s", diff)
	}
}
