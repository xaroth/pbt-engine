package core_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/xaroth/pbt-engine/core"
)

func TestExplain(t *testing.T) {
	t.Parallel()
	var b core.ModelBuilder
	str := b.MustAddStat(core.StatDef{Name: "str"})
	all := b.MustAddStat(core.StatDef{Name: "all"})
	fire := b.MustAddStat(core.StatDef{Name: "fire"})
	rate := b.MustAddStat(core.StatDef{Name: "rate"})
	cold := b.MustAddStat(core.StatDef{
		Name: "cold", Includes: []core.StatID{all}, Shares: []core.Share{{Stat: fire, Rate: core.SelfRef(rate)}},
		Round: core.RoundTrunc, HasMax: true, Max: 75,
	})
	s := newSession(t, b.Build(), 1)
	ids := addAll(t, s, []core.Modifier{
		{Stat: str, Op: core.OpBase, Value: 25, Source: 0},
		{Stat: cold, Op: core.OpBase, Value: 40.5, Source: 1},
		{Stat: cold, Op: core.OpBase, Value: 9, Source: 2, Conds: []core.Cond{{Ref: core.SelfRef(str), Cmp: core.CmpGT, Threshold: 30}}},
		{Stat: all, Op: core.OpBase, Value: 20, Source: 3},
		{Stat: fire, Op: core.OpBase, Value: 30, Source: 4},
		{Stat: rate, Op: core.OpBase, Value: 50, Source: 5},
	})
	e, err := s.Explain(core.Node{Stat: cold})
	if err != nil {
		t.Fatal(err)
	}
	want := core.Explanation{
		Node: core.Node{Stat: cold}, Stat: "cold",
		Terms: []core.Term{
			{Mod: ids[1], Source: 1, Op: core.OpBase, Value: 40.5, Stat: cold, Rate: 1, Held: true},
			{Mod: ids[2], Source: 2, Op: core.OpBase, Value: 0, Stat: cold, Rate: 1, Held: false},
			{Mod: ids[3], Source: 3, Op: core.OpBase, Value: 20, Stat: all, Rate: 1, Held: true},
			{Mod: ids[4], Source: 4, Op: core.OpBase, Value: 15, Stat: fire, Rate: 0.5, Held: true, Shared: true},
		},
		Agg:   core.Agg{Base: 75.5, More: 1},
		Reads: []core.Node{{Stat: str}, {Stat: rate}},
		Total: 75.5, Capped: 75, Value: 75,
	}
	if diff := cmp.Diff(want, e, cmp.Comparer(func(a, b core.ModID) bool { return a == b })); diff != "" {
		t.Errorf("explanation (-want +got):\n%s", diff)
	}
}

func TestExplainFormulaAndOverride(t *testing.T) {
	t.Parallel()
	var b core.ModelBuilder
	x := b.MustAddStat(core.StatDef{Name: "x"})
	doubled := b.MustAddStat(core.StatDef{
		Name: "doubled", FormulaName: "double",
		Formula: func(r core.Reader, a core.Agg) float64 { return 2 * (a.Default() + r.Stat(x)) },
		Round:   core.RoundHalf,
	})
	s := newSession(t, b.Build(), 1)
	addAll(t, s, []core.Modifier{
		{Stat: x, Op: core.OpBase, Value: 1.1, Source: 0},
		{Stat: doubled, Op: core.OpBase, Value: 2, Source: 1},
	})
	e, err := s.Explain(core.Node{Stat: doubled})
	if err != nil {
		t.Fatal(err)
	}
	if e.Formula != "double" || e.Overridden || e.Total != 6.2 || e.Capped != 6.2 || e.Value != 6 ||
		len(e.Reads) != 1 || e.Reads[0].Stat != x {
		t.Errorf("formula: got %+v", e)
	}
	add(t, s, core.Modifier{Stat: doubled, Op: core.OpOverride, Value: 3, Source: 2})
	if e, err = s.Explain(core.Node{Stat: doubled}); err != nil {
		t.Fatal(err)
	}
	if !e.Overridden || e.Total != 3 || e.Value != 3 || len(e.Reads) != 0 {
		t.Errorf("override: got %+v", e)
	}
}

// Explain must not change what later reads see.
func TestExplainLeavesMemo(t *testing.T) {
	t.Parallel()
	var b core.ModelBuilder
	x := b.MustAddStat(core.StatDef{Name: "x"})
	y := b.MustAddStat(core.StatDef{Name: "y", Formula: func(r core.Reader, a core.Agg) float64 { return a.Default() + r.Stat(x) }})
	s := newSession(t, b.Build(), 1)
	addAll(t, s, []core.Modifier{{Stat: x, Op: core.OpBase, Value: 1, Source: 0}, {Stat: y, Op: core.OpBase, Value: 2, Source: 1}})
	if _, err := s.Explain(core.Node{Stat: y}); err != nil {
		t.Fatal(err)
	}
	add(t, s, core.Modifier{Stat: x, Op: core.OpBase, Value: 4, Source: 2})
	if v, err := s.Get(core.Node{Stat: y}); err != nil || v != 7 {
		t.Errorf("y after explain and edit: got %v, %v, want 7", v, err)
	}
}
