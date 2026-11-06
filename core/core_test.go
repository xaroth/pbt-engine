package core_test

import (
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/xaroth/pbt-engine/core"
)

func newSession(tb testing.TB, m *core.Model, actors int) *core.Session {
	tb.Helper()
	m.Freeze()
	s, err := core.NewSession(m, actors)
	if err != nil {
		tb.Fatal(err)
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

func sources(s *core.Session, n int) []core.SourceID {
	out := make([]core.SourceID, n)
	for i := range out {
		out[i] = s.NewSource()
	}
	return out
}

func get(tb testing.TB, s *core.Session, actor core.ActorID, stat core.StatID, ctx core.CtxID) float64 {
	tb.Helper()
	v, err := s.Get(actor, stat, ctx)
	if err != nil {
		tb.Fatal(err)
	}
	return v
}

func want(tb testing.TB, s *core.Session, stat core.StatID, exp float64) {
	tb.Helper()
	if v := get(tb, s, 0, stat, core.GlobalCtx); math.Float64bits(v) != math.Float64bits(exp) {
		tb.Errorf("%s: got %v, want %v", s.Model().StatDef(stat).Name, v, exp)
	}
}

func must(tb testing.TB, err error) {
	tb.Helper()
	if err != nil {
		tb.Fatal(err)
	}
}

func TestAggregation(t *testing.T) {
	t.Parallel()
	m := core.NewModel()
	life := m.MustAddStat(core.StatDef{Name: "life"})
	empty := m.MustAddStat(core.StatDef{Name: "empty"})
	s := newSession(t, m, 1)
	src := sources(s, 10)
	want(t, s, empty, 0)
	add(t, s, core.Modifier{Stat: life, Op: core.OpBase, Value: 38, Source: src[1]})
	add(t, s, core.Modifier{Stat: life, Op: core.OpBase, Value: 12, Source: src[2]})
	want(t, s, life, 50)
	add(t, s, core.Modifier{Stat: life, Op: core.OpInc, Value: 30, Source: src[3]})
	add(t, s, core.Modifier{Stat: life, Op: core.OpInc, Value: -10, Source: src[3]})
	want(t, s, life, 60)
	add(t, s, core.Modifier{Stat: life, Op: core.OpMore, Value: 50, Source: src[4]})
	add(t, s, core.Modifier{Stat: life, Op: core.OpMore, Value: -20, Source: src[5]})
	more := 1.5
	more *= 0.8 // runtime float64, not an exact constant
	want(t, s, life, 50*1.2*more)
	ov := add(t, s, core.Modifier{Stat: life, Op: core.OpOverride, Value: 1, Source: src[9]})
	add(t, s, core.Modifier{Stat: life, Op: core.OpOverride, Value: 7, Source: src[8]})
	want(t, s, life, 1) // last in source order wins
	must(t, s.RemoveModifier(ov))
	want(t, s, life, 7)
}

func TestFlagAndIncludes(t *testing.T) {
	t.Parallel()
	m := core.NewModel()
	ele := m.MustAddStat(core.StatDef{Name: "elemental_res"})
	fire := m.MustAddStat(core.StatDef{Name: "fire_res", Includes: []core.StatID{ele}})
	onslaught := m.MustAddStat(core.StatDef{Name: "onslaught", Flag: true})
	s := newSession(t, m, 1)
	src := sources(s, 4)
	add(t, s, core.Modifier{Stat: fire, Op: core.OpBase, Value: 30, Source: src[0]})
	add(t, s, core.Modifier{Stat: ele, Op: core.OpBase, Value: 12, Source: src[1]})
	want(t, s, fire, 42)
	want(t, s, ele, 12)
	want(t, s, onslaught, 0)
	add(t, s, core.Modifier{Stat: onslaught, Op: core.OpFlag, Source: src[2]})
	add(t, s, core.Modifier{Stat: onslaught, Op: core.OpFlag, Source: src[3]})
	want(t, s, onslaught, 1)
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
			m := core.NewModel()
			testCase.def.Name = "x"
			x := m.MustAddStat(testCase.def)
			s := newSession(t, m, 1)
			add(t, s, core.Modifier{Stat: x, Op: core.OpBase, Value: testCase.value, Source: s.NewSource()})
			want(t, s, x, testCase.want) // compares bits, so -0 fails against 0
		})
	}
}

func TestModelErrors(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name string
		add  func(m *core.Model) error
	}{
		{"duplicate stat", func(m *core.Model) error {
			_, err := m.AddStat(core.StatDef{Name: "x"})
			return err
		}},
		{"unknown include", func(m *core.Model) error {
			_, err := m.AddStat(core.StatDef{Name: "y", Includes: []core.StatID{7}})
			return err
		}},
		{"places", func(m *core.Model) error {
			_, err := m.AddStat(core.StatDef{Name: "y", Places: 16})
			return err
		}},
		{"duplicate dimension", func(m *core.Model) error {
			_, err := m.AddDimension("d", "a")
			return err
		}},
		{"too many bits", func(m *core.Model) error {
			_, err := m.AddDimension("big", make([]string, core.MaxBits)...)
			return err
		}},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			m := core.NewModel()
			m.MustAddStat(core.StatDef{Name: "x"})
			if _, err := m.AddDimension("d", "a"); err != nil {
				t.Fatal(err)
			}
			if err := testCase.add(m); err == nil {
				t.Error("got nil error, want error")
			}
		})
	}
}

func TestModelFrozen(t *testing.T) {
	t.Parallel()
	m := core.NewModel()
	if _, err := core.NewSession(m, 1); err == nil {
		t.Error("NewSession on unfrozen model: got nil error, want error")
	}
	m.Freeze()
	if _, err := core.NewSession(m, 0); err == nil {
		t.Error("NewSession with 0 actors: got nil error, want error")
	}
	defer func() {
		if recover() == nil {
			t.Error("AddStat after Freeze: got no panic, want panic")
		}
	}()
	m.MustAddStat(core.StatDef{Name: "z"})
}

func TestContexts(t *testing.T) {
	t.Parallel()
	m := core.NewModel()
	dmg := m.MustAddStat(core.StatDef{Name: "damage"})
	types, err := m.AddDimension("type", "fire", "cold", "physical")
	if err != nil {
		t.Fatal(err)
	}
	delivery, _ := m.AddDimension("delivery", "attack", "spell")
	fire, cold, phys := types.Bit(0), types.Bit(1), types.Bit(2)
	attack, spell := delivery.Bit(0), delivery.Bit(1)
	if b, ok := m.Bit("delivery.spell"); !ok || b != spell {
		t.Fatalf("Bit(delivery.spell): got %v, %v, want %v, true", b, ok, spell)
	}
	s := newSession(t, m, 1)
	src := sources(s, 4)
	fireSpell := s.Context(core.MaskOf(fire, spell))
	coldAttack := s.Context(core.MaskOf(cold, attack))
	physAttack := s.Context(core.MaskOf(phys, attack))
	if got := s.Context(core.MaskOf(spell, fire)); got != fireSpell {
		t.Errorf("Context of same mask: got %d, want %d", got, fireSpell)
	}
	add(t, s, core.Modifier{Stat: dmg, Op: core.OpBase, Value: 10, Source: src[0]})
	add(t, s, core.Modifier{Stat: dmg, Op: core.OpInc, Value: 100, Source: src[1],
		Pred: core.Predicate{Req: []core.Mask{core.MaskOf(fire, cold)}}}) // elemental
	add(t, s, core.Modifier{Stat: dmg, Op: core.OpInc, Value: 50, Source: src[2],
		Pred: core.Predicate{Req: []core.Mask{core.MaskOf(cold), core.MaskOf(attack)}}})
	add(t, s, core.Modifier{Stat: dmg, Op: core.OpMore, Value: 100, Source: src[3],
		Pred: core.Predicate{Excl: core.MaskOf(spell)}})
	testCases := []struct {
		name string
		ctx  core.CtxID
		want float64
	}{
		{"global", core.GlobalCtx, 20},
		{"fire spell", fireSpell, 20},
		{"cold attack", coldAttack, 50},
		{"physical attack", physAttack, 20},
	}
	for _, testCase := range testCases {
		if got := get(t, s, 0, dmg, testCase.ctx); got != testCase.want {
			t.Errorf("%s: got %v, want %v", testCase.name, got, testCase.want)
		}
	}
}

func TestConditions(t *testing.T) {
	t.Parallel()
	m := core.NewModel()
	life := m.MustAddStat(core.StatDef{Name: "life"})
	lowLife := m.MustAddStat(core.StatDef{Name: "low_life", Flag: true})
	enemyShocked := m.MustAddStat(core.StatDef{Name: "shocked", Flag: true})
	dmg := m.MustAddStat(core.StatDef{Name: "damage"})
	const player, enemy = 0, 1
	s := newSession(t, m, 2)
	src := sources(s, 9)
	add(t, s, core.Modifier{Stat: life, Op: core.OpBase, Value: 100, Source: src[1]})
	add(t, s, core.Modifier{Stat: dmg, Op: core.OpBase, Value: 10, Source: src[1]})
	add(t, s, core.Modifier{Stat: life, Op: core.OpInc, Value: 20, Source: src[2],
		Conds: []core.Cond{{Ref: core.SelfRef(lowLife), Cmp: core.CmpGE, Threshold: 1}}})
	add(t, s, core.Modifier{Stat: dmg, Op: core.OpInc, Value: 50, Source: src[3],
		Conds: []core.Cond{{Ref: core.Ref{Actor: enemy, Stat: enemyShocked, Ctx: core.Self}, Cmp: core.CmpEQ, Threshold: 1}}})
	add(t, s, core.Modifier{Stat: dmg, Op: core.OpInc, Value: 7, Source: src[4],
		Conds: []core.Cond{{Ref: core.SelfRef(life), Cmp: core.CmpGT, Threshold: 100}, {Ref: core.SelfRef(life), Cmp: core.CmpLE, Threshold: 120}}})
	add(t, s, core.Modifier{Stat: dmg, Op: core.OpInc, Value: 1000, Source: src[5],
		Conds: []core.Cond{{Ref: core.SelfRef(life), Cmp: core.CmpLT, Threshold: 100}}})
	add(t, s, core.Modifier{Stat: dmg, Op: core.OpInc, Value: 3, Source: src[6],
		Conds: []core.Cond{{Ref: core.SelfRef(lowLife), Cmp: core.CmpNE, Threshold: 1}}})
	want(t, s, life, 100)
	want(t, s, dmg, 10*1.03)

	ll := add(t, s, core.Modifier{Stat: lowLife, Actor: player, Op: core.OpFlag, Source: src[7]})
	sh := add(t, s, core.Modifier{Stat: enemyShocked, Actor: enemy, Op: core.OpFlag, Source: src[8]})
	want(t, s, life, 120)
	want(t, s, dmg, 10*(1+(50+7)/100.0))
	must(t, s.RemoveModifier(ll))
	must(t, s.RemoveModifier(sh))
	want(t, s, dmg, 10*1.03)
}

func TestScaling(t *testing.T) {
	t.Parallel()
	m := core.NewModel()
	str := m.MustAddStat(core.StatDef{Name: "str"})
	dmg := m.MustAddStat(core.StatDef{Name: "damage"})
	s := newSession(t, m, 1)
	src := sources(s, 3)
	sv := add(t, s, core.Modifier{Stat: str, Op: core.OpBase, Value: 55, Source: src[0]})
	add(t, s, core.Modifier{Stat: dmg, Op: core.OpBase, Value: 100, Source: src[0]})
	add(t, s, core.Modifier{Stat: dmg, Op: core.OpInc, Value: 1, Source: src[1],
		Scales: []core.Scale{{Ref: core.SelfRef(str), Step: 10, Cap: 3}}}) // 1% per 10 str, up to 3%
	add(t, s, core.Modifier{Stat: dmg, Op: core.OpInc, Value: 2, Source: src[2],
		Scales: []core.Scale{{Ref: core.SelfRef(str), Step: 5}, {Ref: core.SelfRef(str), Step: 50}}})
	want(t, s, dmg, 100*(1+(3+2*11*1)/100.0))
	must(t, s.SetModifierValue(sv, 20))
	want(t, s, dmg, 100*(1+(2+0)/100.0))
}

func TestAddModifierErrors(t *testing.T) {
	t.Parallel()
	m := core.NewModel()
	x := m.MustAddStat(core.StatDef{Name: "x"})
	s := newSession(t, m, 1)
	src := s.NewSource()
	testCases := []struct {
		name string
		mod  core.Modifier
	}{
		{"stat", core.Modifier{Stat: 9, Source: src}},
		{"actor", core.Modifier{Stat: x, Actor: 1, Source: src}},
		{"op", core.Modifier{Stat: x, Op: 99, Source: src}},
		{"source", core.Modifier{Stat: x, Source: 5}},
		{"condition actor", core.Modifier{Stat: x, Source: src, Conds: []core.Cond{{Ref: core.Ref{Actor: 4, Stat: x, Ctx: core.Self}}}}},
		{"condition stat", core.Modifier{Stat: x, Source: src, Conds: []core.Cond{{Ref: core.SelfRef(9)}}}},
		{"condition context", core.Modifier{Stat: x, Source: src, Conds: []core.Cond{{Ref: core.Ref{Actor: core.Self, Stat: x, Ctx: 3}}}}},
		{"scale step", core.Modifier{Stat: x, Source: src, Scales: []core.Scale{{Ref: core.SelfRef(x)}}}},
		{"scale ref", core.Modifier{Stat: x, Source: src, Scales: []core.Scale{{Ref: core.SelfRef(9), Step: 1}}}},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			if _, err := s.AddModifier(testCase.mod); err == nil {
				t.Error("got nil error, want error")
			}
		})
	}
}

func TestBadHandles(t *testing.T) {
	t.Parallel()
	m := core.NewModel()
	x := m.MustAddStat(core.StatDef{Name: "x"})
	s := newSession(t, m, 1)
	src := s.NewSource()
	cp := s.Mark()
	dropped := add(t, s, core.Modifier{Stat: x, Value: 1, Source: src})
	must(t, s.Revert(cp))
	reused := add(t, s, core.Modifier{Stat: x, Value: 2, Source: src})
	if dropped == reused {
		t.Fatalf("id of reverted modifier reused: got %v twice", reused)
	}
	testCases := []struct {
		name string
		err  error
	}{
		{"get actor", func() error { _, err := s.Get(1, x, core.GlobalCtx); return err }()},
		{"get stat", func() error { _, err := s.Get(0, 5, core.GlobalCtx); return err }()},
		{"get context", func() error { _, err := s.Get(0, x, 3); return err }()},
		{"remove dropped", s.RemoveModifier(dropped)},
		{"value dropped", s.SetModifierValue(dropped, 3)},
		{"modifier dropped", func() error { _, _, err := s.Modifier(dropped); return err }()},
		{"remove zero id", s.RemoveModifier(core.ModID{})},
		{"source", s.SetSourceEnabled(7, false)},
	}
	for _, testCase := range testCases {
		if testCase.err == nil {
			t.Errorf("%s: got nil error, want error", testCase.name)
		}
	}
	want(t, s, x, 2)
}

func TestModifierCopy(t *testing.T) {
	t.Parallel()
	m := core.NewModel()
	x := m.MustAddStat(core.StatDef{Name: "x"})
	y := m.MustAddStat(core.StatDef{Name: "y"})
	s := newSession(t, m, 1)
	src := s.NewSource()
	id := add(t, s, core.Modifier{Stat: x, Value: 1, Source: src,
		Conds: []core.Cond{{Ref: core.SelfRef(y), Cmp: core.CmpGE, Threshold: 0}},
		Pred:  core.Predicate{Excl: core.MaskOf(3)}})
	want(t, s, x, 1)
	got, removed, err := s.Modifier(id)
	if err != nil || removed {
		t.Fatalf("Modifier: got removed %v, %v, want false, nil", removed, err)
	}
	got.Conds[0].Threshold = 5
	want(t, s, x, 1)
}

// attrModel has strength and dexterity in one CycleFamily, plus a life formula reading strength.
func attrModel() (m *core.Model, str, dex, life core.StatID) {
	m = core.NewModel()
	str = m.MustAddStat(core.StatDef{Name: "str", CycleFamily: 1, Round: core.RoundFloor})
	dex = m.MustAddStat(core.StatDef{Name: "dex", CycleFamily: 1, Round: core.RoundFloor})
	life = m.MustAddStat(core.StatDef{Name: "life", Round: core.RoundHalf})
	m.SetFormula(life, func(r core.Reader, a core.Agg) float64 {
		b := a.Base + math.Floor(r.Stat(str)/2)
		return b * (1 + a.Inc/100) * a.More
	})
	return m, str, dex, life
}

func TestCycleFamily(t *testing.T) {
	t.Parallel()
	m, str, dex, life := attrModel()
	s := newSession(t, m, 1)
	src := sources(s, 5)
	add(t, s, core.Modifier{Stat: str, Op: core.OpBase, Value: 100, Source: src[1]})
	add(t, s, core.Modifier{Stat: dex, Op: core.OpBase, Value: 50, Source: src[1]})
	add(t, s, core.Modifier{Stat: life, Op: core.OpBase, Value: 50, Source: src[1]})
	// +1 dex per 10 str, +1 str per 10 dex: each sees the other's unconditional value.
	add(t, s, core.Modifier{Stat: dex, Op: core.OpBase, Value: 1, Source: src[2], Scales: []core.Scale{{Ref: core.SelfRef(str), Step: 10}}})
	add(t, s, core.Modifier{Stat: str, Op: core.OpBase, Value: 1, Source: src[3], Scales: []core.Scale{{Ref: core.SelfRef(dex), Step: 10}}})
	add(t, s, core.Modifier{Stat: str, Op: core.OpInc, Value: 10, Source: src[4]})
	want(t, s, str, math.Floor((100+5)*1.1))
	want(t, s, dex, 50+11)
	want(t, s, life, 50+57)
}

func TestCycleFamilyFormula(t *testing.T) {
	t.Parallel()
	m := core.NewModel()
	str := m.MustAddStat(core.StatDef{Name: "str", CycleFamily: 1})
	dex := m.MustAddStat(core.StatDef{Name: "dex", CycleFamily: 1})
	other := m.MustAddStat(core.StatDef{Name: "other", CycleFamily: 2})
	z := m.MustAddStat(core.StatDef{Name: "z"})
	m.SetFormula(str, func(r core.Reader, a core.Agg) float64 { return a.Base + r.Stat(dex) + r.Stat(other) })
	s := newSession(t, m, 1)
	src := s.NewSource()
	add(t, s, core.Modifier{Stat: str, Op: core.OpBase, Value: 10, Source: src})
	add(t, s, core.Modifier{Stat: dex, Op: core.OpBase, Value: 1, Source: src, Scales: []core.Scale{{Ref: core.SelfRef(str), Step: 1}}})
	add(t, s, core.Modifier{Stat: z, Op: core.OpBase, Value: 1, Source: src})
	add(t, s, core.Modifier{Stat: other, Op: core.OpBase, Value: 5, Source: src})
	add(t, s, core.Modifier{Stat: other, Op: core.OpBase, Value: 1, Source: src, Conds: []core.Cond{{Ref: core.SelfRef(z), Cmp: core.CmpGE, Threshold: 1}}})
	// str's formula reads unconditional dex (0) but full other (6), being another family.
	want(t, s, str, 16)
	want(t, s, dex, 16)
	want(t, s, other, 6)
}

func TestCycleError(t *testing.T) {
	t.Parallel()
	m := core.NewModel()
	a := m.MustAddStat(core.StatDef{Name: "a"})
	b := m.MustAddStat(core.StatDef{Name: "b"})
	s := newSession(t, m, 1)
	src := sources(s, 2)
	add(t, s, core.Modifier{Stat: a, Op: core.OpBase, Value: 1, Source: src[0], Scales: []core.Scale{{Ref: core.SelfRef(b), Step: 1}}})
	add(t, s, core.Modifier{Stat: b, Op: core.OpBase, Value: 1, Source: src[0]})
	want(t, s, a, 1)
	cyc := add(t, s, core.Modifier{Stat: b, Op: core.OpBase, Value: 1, Source: src[1],
		Conds: []core.Cond{{Ref: core.SelfRef(a), Cmp: core.CmpGE, Threshold: 0}}})
	_, err := s.Get(0, a, core.GlobalCtx)
	ce, ok := errors.AsType[*core.CycleError](err)
	if !ok {
		t.Fatalf("got %v, want CycleError", err)
	}
	if len(ce.Path) != 3 || ce.Path[0].Stat != a || ce.Path[1].Stat != b || ce.Path[2].Stat != a {
		t.Errorf("path: got %v, want a -> b -> a", ce.Path)
	}
	if !strings.Contains(err.Error(), "a[actor 0 ctx 0] -> b[actor 0 ctx 0] -> a") {
		t.Errorf("message: got %q", err)
	}
	if _, err := s.Get(0, b, core.GlobalCtx); err == nil {
		t.Error("second read: got nil error, want cycle")
	}
	must(t, s.RemoveModifier(cyc))
	want(t, s, a, 1)
	want(t, s, b, 1)
}

func TestFormulaPanic(t *testing.T) {
	t.Parallel()
	m := core.NewModel()
	x := m.MustAddStat(core.StatDef{Name: "x"})
	fail := true
	m.SetFormula(x, func(_ core.Reader, a core.Agg) float64 {
		if fail {
			panic("boom")
		}
		return a.Base
	})
	s := newSession(t, m, 1)
	add(t, s, core.Modifier{Stat: x, Value: 3, Source: s.NewSource()})
	func() {
		defer func() {
			if recover() == nil {
				t.Error("got no panic, want the formula's panic")
			}
		}()
		_, _ = s.Get(0, x, core.GlobalCtx)
	}()
	fail = false
	want(t, s, x, 3)
}

func TestEditRevert(t *testing.T) {
	t.Parallel()
	m, str, dex, life := attrModel()
	calls := 0
	f := m.StatDef(life).Formula
	m.SetFormula(life, func(r core.Reader, a core.Agg) float64 { calls++; return f(r, a) })
	s := newSession(t, m, 1)
	src := sources(s, 6)
	add(t, s, core.Modifier{Stat: str, Op: core.OpBase, Value: 20, Source: src[1]})
	add(t, s, core.Modifier{Stat: life, Op: core.OpBase, Value: 50, Source: src[1]})
	add(t, s, core.Modifier{Stat: str, Op: core.OpBase, Value: 10, Source: src[2]})
	add(t, s, core.Modifier{Stat: life, Op: core.OpInc, Value: 10, Source: src[3]})
	s.Commit()
	want(t, s, life, 72) // (50 + 30/2) * 1.1 = 71.5
	want(t, s, life, 72)
	if calls != 1 {
		t.Errorf("formula calls: got %d, want 1 (memoised)", calls)
	}

	cp := s.Mark()
	add(t, s, core.Modifier{Stat: dex, Op: core.OpBase, Value: 5, Source: src[4]})
	want(t, s, life, 72)
	if calls != 1 {
		t.Errorf("formula calls after unrelated edit: got %d, want 1", calls)
	}
	// str is floored: 30.4 -> 30, so life is not recomputed.
	add(t, s, core.Modifier{Stat: str, Op: core.OpBase, Value: 0.4, Source: src[4]})
	want(t, s, life, 72)
	if calls != 1 {
		t.Errorf("formula calls after no-op edit: got %d, want 1", calls)
	}

	must(t, s.SetSourceEnabled(src[2], false))
	want(t, s, life, 66) // str 20 -> (50 + 10) * 1.1
	mv := add(t, s, core.Modifier{Stat: life, Op: core.OpMore, Value: 100, Source: src[5]})
	want(t, s, life, 132)
	must(t, s.SetModifierValue(mv, 50))
	want(t, s, life, 99)
	must(t, s.RemoveModifier(mv))
	want(t, s, life, 66)

	inner := s.Mark()
	must(t, s.SetSourceEnabled(src[1], false))
	want(t, s, life, 0)
	must(t, s.Revert(inner))
	want(t, s, life, 66)

	must(t, s.Revert(cp))
	want(t, s, life, 72)
	if !s.SourceEnabled(src[2]) {
		t.Error("source 2 after revert: got disabled, want enabled")
	}
	want(t, s, dex, 0)
	if err := s.Revert(inner); !errors.Is(err, core.ErrStaleCheckpoint) {
		t.Errorf("revert to dropped checkpoint: got %v, want ErrStaleCheckpoint", err)
	}
}

func TestStaleCheckpoint(t *testing.T) {
	t.Parallel()
	m := core.NewModel()
	x := m.MustAddStat(core.StatDef{Name: "x"})
	s := newSession(t, m, 1)
	src := s.NewSource()
	add(t, s, core.Modifier{Stat: x, Value: 10, Source: src})
	add(t, s, core.Modifier{Stat: x, Value: 10, Source: src})
	beforeCommit := s.Mark()
	s.Commit()
	add(t, s, core.Modifier{Stat: x, Value: 1, Source: src})
	if err := s.Revert(beforeCommit); !errors.Is(err, core.ErrStaleCheckpoint) {
		t.Errorf("checkpoint before Commit: got %v, want ErrStaleCheckpoint", err)
	}

	start := s.Mark()
	add(t, s, core.Modifier{Stat: x, Value: 1, Source: src})
	later := s.Mark()
	must(t, s.Revert(start))
	add(t, s, core.Modifier{Stat: x, Value: 1, Source: src})
	add(t, s, core.Modifier{Stat: x, Value: 1, Source: src})
	if err := s.Revert(later); !errors.Is(err, core.ErrStaleCheckpoint) {
		t.Errorf("checkpoint after the reverted one: got %v, want ErrStaleCheckpoint", err)
	}
	must(t, s.Revert(start))
	want(t, s, x, 21)
}

func TestCounterWraparound(t *testing.T) {
	t.Parallel()
	m, str, _, life := attrModel()
	s := newSession(t, m, 1)
	src := sources(s, 2)
	add(t, s, core.Modifier{Stat: life, Op: core.OpBase, Value: 50, Source: src[0]})
	add(t, s, core.Modifier{Stat: str, Op: core.OpBase, Value: 20, Source: src[1]})
	want(t, s, life, 60)
	core.NearWrap(s)
	for range 4 {
		must(t, s.SetSourceEnabled(src[1], false))
		want(t, s, life, 50)
		must(t, s.SetSourceEnabled(src[1], true))
		want(t, s, life, 60)
		s.Invalidate()
	}
}

func TestConcurrentSessions(t *testing.T) {
	t.Parallel()
	m, str, _, life := attrModel()
	m.Freeze()
	for range 4 {
		t.Run("", func(t *testing.T) {
			t.Parallel()
			s, err := core.NewSession(m, 1)
			if err != nil {
				t.Fatal(err)
			}
			add(t, s, core.Modifier{Stat: str, Op: core.OpBase, Value: 20, Source: s.NewSource()})
			want(t, s, life, 10)
		})
	}
}
