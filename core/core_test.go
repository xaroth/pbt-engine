package core

import (
	"errors"
	"math"
	"testing"
)

func newModel(t testing.TB, defs ...StatDef) (*Model, []StatID) {
	t.Helper()
	m := NewModel()
	ids := make([]StatID, len(defs))
	for i, d := range defs {
		id, err := m.AddStat(d)
		if err != nil {
			t.Fatal(err)
		}
		ids[i] = id
	}
	return m, ids
}

func add(t testing.TB, s *Session, m Modifier) ModID {
	t.Helper()
	id, err := s.AddModifier(m)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func get(t testing.TB, s *Session, actor ActorID, stat StatID, ctx CtxID) float64 {
	t.Helper()
	v, err := s.Get(actor, stat, ctx)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func want(t testing.TB, s *Session, stat StatID, exp float64) {
	t.Helper()
	if v := get(t, s, 0, stat, GlobalCtx); math.Float64bits(v) != math.Float64bits(exp) {
		t.Errorf("%s = %v, want %v", s.m.stats[stat].Name, v, exp)
	}
}

func TestAggregation(t *testing.T) {
	m, ids := newModel(t, StatDef{Name: "life"}, StatDef{Name: "none"})
	life := ids[0]
	s := NewSession(m, 1)
	want(t, s, ids[1], 0)
	add(t, s, Modifier{Stat: life, Op: OpBase, Value: 38, Source: 1})
	add(t, s, Modifier{Stat: life, Op: OpBase, Value: 12, Source: 2})
	want(t, s, life, 50)
	add(t, s, Modifier{Stat: life, Op: OpInc, Value: 30, Source: 3})
	add(t, s, Modifier{Stat: life, Op: OpInc, Value: -10, Source: 3})
	want(t, s, life, 60)
	add(t, s, Modifier{Stat: life, Op: OpMore, Value: 50, Source: 4})
	add(t, s, Modifier{Stat: life, Op: OpMore, Value: -20, Source: 5})
	more := 1.5
	more *= 0.8 // runtime float64, not an exact constant
	want(t, s, life, 50*1.2*more)
	ov := add(t, s, Modifier{Stat: life, Op: OpOverride, Value: 1, Source: 9})
	add(t, s, Modifier{Stat: life, Op: OpOverride, Value: 7, Source: 8})
	want(t, s, life, 1) // last in source order wins
	s.RemoveModifier(ov)
	want(t, s, life, 7)
}

func TestFlagAndIncludes(t *testing.T) {
	m := NewModel()
	ele := m.MustAddStat(StatDef{Name: "elemental_res"})
	fire := m.MustAddStat(StatDef{Name: "fire_res", Includes: []StatID{ele}})
	onslaught := m.MustAddStat(StatDef{Name: "onslaught", Flag: true})
	s := NewSession(m, 1)
	add(t, s, Modifier{Stat: fire, Op: OpBase, Value: 30, Source: 1})
	add(t, s, Modifier{Stat: ele, Op: OpBase, Value: 12, Source: 2})
	want(t, s, fire, 42)
	want(t, s, ele, 12)
	want(t, s, onslaught, 0)
	add(t, s, Modifier{Stat: onslaught, Op: OpFlag, Source: 3})
	add(t, s, Modifier{Stat: onslaught, Op: OpFlag, Source: 4})
	want(t, s, onslaught, 1)
}

func TestClampAndRounding(t *testing.T) {
	m, ids := newModel(t,
		StatDef{Name: "res", HasMin: true, Min: -60, HasMax: true, Max: 75},
		StatDef{Name: "half", Round: RoundHalf},
		StatDef{Name: "floor", Round: RoundFloor},
		StatDef{Name: "trunc", Round: RoundTrunc},
		StatDef{Name: "regen", Round: RoundHalf, Places: 1},
		StatDef{Name: "negzero", Round: RoundTrunc},
	)
	s := NewSession(m, 1)
	r := add(t, s, Modifier{Stat: ids[0], Op: OpBase, Value: 90, Source: 1})
	want(t, s, ids[0], 75)
	s.SetModifierValue(r, -100)
	want(t, s, ids[0], -60)
	for i, v := range []float64{2.5, -2.5, -2.5} {
		add(t, s, Modifier{Stat: ids[1+i], Op: OpBase, Value: v, Source: 1})
	}
	want(t, s, ids[1], 3)
	want(t, s, ids[2], -3)
	want(t, s, ids[3], -2)
	add(t, s, Modifier{Stat: ids[4], Op: OpBase, Value: 12.345, Source: 1})
	want(t, s, ids[4], 12.3)
	add(t, s, Modifier{Stat: ids[5], Op: OpBase, Value: -0.5, Source: 1})
	if v := get(t, s, 0, ids[5], GlobalCtx); math.Signbit(v) {
		t.Errorf("negzero = -0")
	}
}

func TestContexts(t *testing.T) {
	m := NewModel()
	dmg := m.MustAddStat(StatDef{Name: "damage"})
	types, err := m.AddDimension("type", "fire", "cold", "physical")
	if err != nil {
		t.Fatal(err)
	}
	delivery, _ := m.AddDimension("delivery", "attack", "spell")
	fire, cold, phys := types.Bit(0), types.Bit(1), types.Bit(2)
	attack, spell := delivery.Bit(0), delivery.Bit(1)
	if b, ok := m.Bit("delivery.spell"); !ok || b != spell {
		t.Fatalf("Bit lookup = %v %v", b, ok)
	}
	if _, err := m.AddDimension("type"); err == nil {
		t.Error("duplicate dimension accepted")
	}
	s := NewSession(m, 1)
	fireSpell := s.Context(MaskOf(fire, spell))
	coldAttack := s.Context(MaskOf(cold, attack))
	physAttack := s.Context(MaskOf(phys, attack))
	if s.Context(MaskOf(spell, fire)) != fireSpell {
		t.Error("context not interned")
	}
	add(t, s, Modifier{Stat: dmg, Op: OpBase, Value: 10, Source: 1})
	add(t, s, Modifier{Stat: dmg, Op: OpInc, Value: 100, Source: 2,
		Pred: Predicate{Req: []Mask{MaskOf(fire, cold)}}}) // elemental
	add(t, s, Modifier{Stat: dmg, Op: OpInc, Value: 50, Source: 3,
		Pred: Predicate{Req: []Mask{MaskOf(cold), MaskOf(attack)}}})
	add(t, s, Modifier{Stat: dmg, Op: OpMore, Value: 100, Source: 4,
		Pred: Predicate{Excl: MaskOf(spell)}})
	for _, c := range []struct {
		ctx CtxID
		exp float64
	}{{GlobalCtx, 20}, {fireSpell, 20}, {coldAttack, 50}, {physAttack, 20}} {
		if v := get(t, s, 0, dmg, c.ctx); v != c.exp {
			t.Errorf("ctx %v: %v, want %v", s.ContextMask(c.ctx), v, c.exp)
		}
	}
}

func TestConditions(t *testing.T) {
	m := NewModel()
	life := m.MustAddStat(StatDef{Name: "life"})
	lowLife := m.MustAddStat(StatDef{Name: "low_life", Flag: true})
	enemyShocked := m.MustAddStat(StatDef{Name: "shocked", Flag: true})
	dmg := m.MustAddStat(StatDef{Name: "damage"})
	const player, enemy = 0, 1
	s := NewSession(m, 2)
	add(t, s, Modifier{Stat: life, Op: OpBase, Value: 100, Source: 1})
	add(t, s, Modifier{Stat: dmg, Op: OpBase, Value: 10, Source: 1})
	add(t, s, Modifier{Stat: life, Op: OpInc, Value: 20, Source: 2,
		Conds: []Cond{{Ref: SelfRef(lowLife), Cmp: CmpGE, Threshold: 1}}})
	add(t, s, Modifier{Stat: dmg, Op: OpInc, Value: 50, Source: 3,
		Conds: []Cond{{Ref: Ref{Actor: enemy, Stat: enemyShocked, Ctx: Self}, Cmp: CmpEQ, Threshold: 1}}})
	add(t, s, Modifier{Stat: dmg, Op: OpInc, Value: 7, Source: 4,
		Conds: []Cond{{Ref: SelfRef(life), Cmp: CmpGT, Threshold: 100}, {Ref: SelfRef(life), Cmp: CmpLE, Threshold: 120}}})
	add(t, s, Modifier{Stat: dmg, Op: OpInc, Value: 1000, Source: 5,
		Conds: []Cond{{Ref: SelfRef(life), Cmp: CmpLT, Threshold: 100}}})
	add(t, s, Modifier{Stat: dmg, Op: OpInc, Value: 3, Source: 6,
		Conds: []Cond{{Ref: SelfRef(lowLife), Cmp: CmpNE, Threshold: 1}}})
	want(t, s, life, 100)
	want(t, s, dmg, 10*1.03)

	ll := add(t, s, Modifier{Stat: lowLife, Actor: player, Op: OpFlag, Source: 7})
	sh := add(t, s, Modifier{Stat: enemyShocked, Actor: enemy, Op: OpFlag, Source: 8})
	want(t, s, life, 120)
	want(t, s, dmg, 10*(1+(50+7)/100.0))
	s.RemoveModifier(ll)
	s.RemoveModifier(sh)
	want(t, s, dmg, 10*1.03)
}

func TestScaling(t *testing.T) {
	m := NewModel()
	str := m.MustAddStat(StatDef{Name: "str"})
	dmg := m.MustAddStat(StatDef{Name: "damage"})
	s := NewSession(m, 1)
	sv := add(t, s, Modifier{Stat: str, Op: OpBase, Value: 55, Source: 1})
	add(t, s, Modifier{Stat: dmg, Op: OpBase, Value: 100, Source: 1})
	add(t, s, Modifier{Stat: dmg, Op: OpInc, Value: 1, Source: 2,
		Scales: []Scale{{Ref: SelfRef(str), Step: 10, Cap: 3}}}) // 1% per 10 str, up to 3%
	add(t, s, Modifier{Stat: dmg, Op: OpInc, Value: 2, Source: 3,
		Scales: []Scale{{Ref: SelfRef(str), Step: 5}, {Ref: SelfRef(str), Step: 50}}})
	want(t, s, dmg, 100*(1+(3+2*11*1)/100.0))
	s.SetModifierValue(sv, 20)
	want(t, s, dmg, 100*(1+(2+0)/100.0))
	if _, err := s.AddModifier(Modifier{Stat: dmg, Scales: []Scale{{Ref: SelfRef(str)}}}); err == nil {
		t.Error("zero step accepted")
	}
}

// attrModel has strength and dexterity granting each other ("+1 dex per 10 str" style),
// plus a life formula reading strength.
func attrModel() (*Model, StatID, StatID, StatID) {
	m := NewModel()
	str := m.MustAddStat(StatDef{Name: "str", CycleBreak: true, Round: RoundFloor})
	dex := m.MustAddStat(StatDef{Name: "dex", CycleBreak: true, Round: RoundFloor})
	life := m.MustAddStat(StatDef{Name: "life", Round: RoundHalf})
	m.SetFormula(life, func(r Reader, a Agg) float64 {
		b := a.Base + math.Floor(r.Stat(str)/2)
		return b * (1 + a.Inc/100) * a.More
	})
	return m, str, dex, life
}

func TestCycleBreak(t *testing.T) {
	m, str, dex, life := attrModel()
	s := NewSession(m, 1)
	add(t, s, Modifier{Stat: str, Op: OpBase, Value: 100, Source: 1})
	add(t, s, Modifier{Stat: dex, Op: OpBase, Value: 50, Source: 1})
	add(t, s, Modifier{Stat: life, Op: OpBase, Value: 50, Source: 1})
	// +1 dex per 10 str, +1 str per 10 dex: each sees the other's unconditional value.
	add(t, s, Modifier{Stat: dex, Op: OpBase, Value: 1, Source: 2, Scales: []Scale{{Ref: SelfRef(str), Step: 10}}})
	add(t, s, Modifier{Stat: str, Op: OpBase, Value: 1, Source: 3, Scales: []Scale{{Ref: SelfRef(dex), Step: 10}}})
	add(t, s, Modifier{Stat: str, Op: OpInc, Value: 10, Source: 4})
	want(t, s, str, math.Floor((100+5)*1.1))
	want(t, s, dex, 50+11)
	want(t, s, life, 50+57)
}

func TestCycleError(t *testing.T) {
	m := NewModel()
	a := m.MustAddStat(StatDef{Name: "a"})
	b := m.MustAddStat(StatDef{Name: "b"})
	s := NewSession(m, 1)
	add(t, s, Modifier{Stat: a, Op: OpBase, Value: 1, Source: 1, Scales: []Scale{{Ref: SelfRef(b), Step: 1}}})
	add(t, s, Modifier{Stat: b, Op: OpBase, Value: 1, Source: 1})
	want(t, s, a, 1)
	cyc := add(t, s, Modifier{Stat: b, Op: OpBase, Value: 1, Source: 2,
		Conds: []Cond{{Ref: SelfRef(a), Cmp: CmpGE, Threshold: 0}}})
	_, err := s.Get(0, a, GlobalCtx)
	var ce *CycleError
	if !errors.As(err, &ce) {
		t.Fatalf("err = %v, want cycle", err)
	}
	if len(ce.Path) != 3 || ce.Path[0].Stat != a || ce.Path[1].Stat != b || ce.Path[2].Stat != a {
		t.Errorf("path = %v (%v)", ce.Path, err)
	}
	if _, err := s.Get(0, b, GlobalCtx); err == nil {
		t.Error("second read: no error")
	}
	s.RemoveModifier(cyc)
	want(t, s, a, 1)
	want(t, s, b, 1)
}

func TestBadReads(t *testing.T) {
	m, ids := newModel(t, StatDef{Name: "x"})
	s := NewSession(m, 1)
	if _, err := s.Get(1, ids[0], GlobalCtx); err == nil {
		t.Error("actor out of range accepted")
	}
	if _, err := s.Get(0, 5, GlobalCtx); err == nil {
		t.Error("stat out of range accepted")
	}
	if _, err := s.Get(0, ids[0], 3); err == nil {
		t.Error("unknown context accepted")
	}
	add(t, s, Modifier{Stat: ids[0], Source: 1, Value: 1, Conds: []Cond{{Ref: Ref{Actor: 4, Stat: ids[0], Ctx: Self}}}})
	if _, err := s.Get(0, ids[0], GlobalCtx); err == nil {
		t.Error("condition on missing actor accepted")
	}
}

func TestEditRevert(t *testing.T) {
	m, str, dex, life := attrModel()
	calls := 0
	f := m.StatDef(life).Formula
	m.SetFormula(life, func(r Reader, a Agg) float64 { calls++; return f(r, a) })
	s := NewSession(m, 1)
	add(t, s, Modifier{Stat: str, Op: OpBase, Value: 20, Source: 1})
	add(t, s, Modifier{Stat: life, Op: OpBase, Value: 50, Source: 1})
	add(t, s, Modifier{Stat: str, Op: OpBase, Value: 10, Source: 2})
	add(t, s, Modifier{Stat: life, Op: OpInc, Value: 10, Source: 3})
	s.Commit()
	want(t, s, life, 72) // (50 + 30/2) * 1.1 = 71.5
	want(t, s, life, 72)
	if calls != 1 {
		t.Errorf("formula calls = %d, want 1 (memoised)", calls)
	}

	// An edit to an unrelated stat does not recompute life.
	cp := s.Mark()
	add(t, s, Modifier{Stat: dex, Op: OpBase, Value: 5, Source: 4})
	want(t, s, life, 72)
	if calls != 1 {
		t.Errorf("formula calls = %d after unrelated edit", calls)
	}
	// An edit that leaves str's value unchanged stops at str (str is floored: 30.4 -> 30).
	add(t, s, Modifier{Stat: str, Op: OpBase, Value: 0.4, Source: 4})
	want(t, s, life, 72)
	if calls != 1 {
		t.Errorf("formula calls = %d after no-op edit", calls)
	}

	s.SetSourceEnabled(2, false)
	want(t, s, life, 66) // str 20 -> (50 + 10) * 1.1
	mv := add(t, s, Modifier{Stat: life, Op: OpMore, Value: 100, Source: 5})
	want(t, s, life, 132)
	s.SetModifierValue(mv, 50)
	want(t, s, life, 99)
	s.RemoveModifier(mv)
	want(t, s, life, 66)

	inner := s.Mark()
	s.SetSourceEnabled(1, false)
	want(t, s, life, 0)
	s.Revert(inner)
	want(t, s, life, 66)

	s.Revert(cp)
	want(t, s, life, 72)
	if n := s.NumModifiers(); n != 4 {
		t.Errorf("modifiers after revert = %d, want 4", n)
	}
	if !s.SourceEnabled(2) {
		t.Error("source 2 still disabled after revert")
	}
	want(t, s, dex, 0)
}

func TestRevertPanicsOnBadCheckpoint(t *testing.T) {
	m, _ := newModel(t, StatDef{Name: "x"})
	s := NewSession(m, 1)
	defer func() {
		if recover() == nil {
			t.Error("no panic")
		}
	}()
	s.Revert(1)
}

func TestModelFrozen(t *testing.T) {
	m, _ := newModel(t, StatDef{Name: "x"})
	if _, err := m.AddStat(StatDef{Name: "x"}); err == nil {
		t.Error("duplicate stat accepted")
	}
	if _, err := m.AddStat(StatDef{Name: "y", Includes: []StatID{7}}); err == nil {
		t.Error("unknown include accepted")
	}
	NewSession(m, 1)
	defer func() {
		if recover() == nil {
			t.Error("no panic")
		}
	}()
	m.MustAddStat(StatDef{Name: "z"})
}
