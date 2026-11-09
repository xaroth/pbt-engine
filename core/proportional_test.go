package core_test

import (
	"errors"
	"math"
	"math/rand/v2"
	"testing"

	"github.com/xaroth/pbt-engine/core"
)

// sumParts returns the sum of parts' values.
func sumParts(parts []core.Part) float64 {
	v := 0.0
	for _, p := range parts {
		v += p.Value
	}
	return v
}

func near(a, b float64) bool { return math.Abs(a-b) <= 1e-9*max(1, math.Abs(b)) }

func proportional(tb testing.TB, s *core.Session, n core.Node) core.Proportion {
	tb.Helper()
	p, err := s.Proportional(n)
	if err != nil {
		tb.Fatal(err)
	}
	return p
}

// bySource sums parts per source.
func bySource(parts []core.Part) map[core.SourceID]float64 {
	m := map[core.SourceID]float64{}
	for _, p := range parts {
		m[p.Source] += p.Value
	}
	return m
}

func TestProportionalDefaultRule(t *testing.T) {
	t.Parallel()
	var b core.ModelBuilder
	life := b.MustAddStat(core.StatDef{Name: "life", Round: core.RoundHalf})
	s := newSession(t, b.Build(), 1)
	addAll(t, s, []core.Modifier{
		{Stat: life, Op: core.OpBase, Value: 100, Source: 0},
		{Stat: life, Op: core.OpBase, Value: 50, Source: 1},
		{Stat: life, Op: core.OpInc, Value: 30, Source: 2},
		{Stat: life, Op: core.OpInc, Value: 10, Source: 3},
		{Stat: life, Op: core.OpMore, Value: 20, Source: 4},
		{Stat: life, Op: core.OpMore, Value: -10, Source: 5},
	})
	p := proportional(t, s, core.Node{Stat: life})
	// 150 * 1.4 = 210 before more; more part 210 * (1.2 * 0.9 - 1) = 16.8, by ln weight.
	total := 150 * 1.4 * 1.2 * 0.9
	ratio := (1.08 - 1) / math.Log(1.08)
	want := map[core.SourceID]float64{
		0: 100, 1: 50, 2: 45, 3: 15,
		4: 210 * math.Log(1.2) * ratio, 5: 210 * math.Log(0.9) * ratio,
	}
	got := bySource(p.Parts)
	for src, v := range want {
		if !near(got[src], v) {
			t.Errorf("source %d: got %v, want %v", src, got[src], v)
		}
	}
	if !near(p.Amount, total) || !near(sumParts(p.Parts), total) || p.Value != 227 {
		t.Errorf("amount %v, sum %v, value %v; want %v, %v, 227", p.Amount, sumParts(p.Parts), p.Value, total, total)
	}
}

func TestProportionalMoreEdgeCases(t *testing.T) {
	t.Parallel()
	for name, more := range map[string][]float64{
		"cancel out": {100, -50},
		"minus 100":  {-100, 50},
		"below -100": {-150, 20},
	} {
		var b core.ModelBuilder
		x := b.MustAddStat(core.StatDef{Name: "x"})
		s := newSession(t, b.Build(), 1)
		add(t, s, core.Modifier{Stat: x, Op: core.OpBase, Value: 10, Source: 0})
		for i, m := range more {
			add(t, s, core.Modifier{Stat: x, Op: core.OpMore, Value: m, Source: core.SourceID(1 + i)})
		}
		p := proportional(t, s, core.Node{Stat: x})
		if !near(sumParts(p.Parts), p.Amount) || p.Amount != get(t, s, 0, x, core.GlobalCtx) {
			t.Errorf("%s: sum %v, amount %v", name, sumParts(p.Parts), p.Amount)
		}
	}
}

// Q5: a shared modifier is credited to the sources granting the share, keeping the modifier.
// Caps are a stage: the parts explain the uncapped amount.
func TestProportionalShareAndCap(t *testing.T) {
	t.Parallel()
	w := newShareModel()
	s := newSession(t, w.m, 1)
	ids := addAll(t, s, []core.Modifier{
		{Stat: w.fire, Op: core.OpBase, Value: 30, Source: 1},
		{Stat: w.rate, Op: core.OpBase, Value: 30, Source: 2},
		{Stat: w.rate, Op: core.OpBase, Value: 20, Source: 3},
		{Stat: w.cold, Op: core.OpBase, Value: 10, Source: 4},
	})
	p := proportional(t, s, core.Node{Stat: w.cold})
	got := bySource(p.Parts)
	if !near(got[2], 9) || !near(got[3], 6) || got[4] != 10 || got[1] != 0 || p.Amount != 25 {
		t.Errorf("parts %v, amount %v", got, p.Amount)
	}
	for _, pt := range p.Parts {
		if pt.Source == 2 && (pt.Mod != ids[0] || len(pt.Via) != 1 || pt.Via[0].Stat != w.rate) {
			t.Errorf("shared part: %+v", pt)
		}
	}

	var b core.ModelBuilder
	res := b.MustAddStat(core.StatDef{Name: "res", HasMax: true, Max: 75})
	s = newSession(t, b.Build(), 1)
	addAll(t, s, []core.Modifier{{Stat: res, Op: core.OpBase, Value: 150, Source: 0}, {Stat: res, Op: core.OpBase, Value: 50, Source: 1}})
	if p := proportional(t, s, core.Node{Stat: res}); p.Amount != 200 || p.Effective != 75 || p.Value != 75 || sumParts(p.Parts) != 200 {
		t.Errorf("capped: %+v", p)
	}
}

func TestProportionalOverrideFlagAndFormula(t *testing.T) {
	t.Parallel()
	var b core.ModelBuilder
	x := b.MustAddStat(core.StatDef{Name: "x"})
	flag := b.MustAddStat(core.StatDef{Name: "flag", Flag: true})
	plain := b.MustAddStat(core.StatDef{Name: "plain", Formula: func(_ core.Reader, a core.Agg) float64 { return a.Default() }})
	// min(own, x): parts of whichever is smaller, capped by x when x is.
	capped := b.MustAddStat(core.StatDef{
		Name: "capped", FormulaName: "min",
		Formula: func(r core.Reader, a core.Agg) float64 { return min(a.Default(), r.Stat(x)) },
		Attribute: func(a *core.Attributor) core.Attribution {
			own := a.Default()
			at := core.Attribution{Parts: own, Amount: sumParts(own)}
			if c := a.Stat(x); c < at.Amount {
				at.CapBy = []core.Node{a.Node(core.SelfRef(x))}
			}
			return at
		},
	})
	s := newSession(t, b.Build(), 1)
	addAll(t, s, []core.Modifier{
		{Stat: x, Op: core.OpBase, Value: 5, Source: 0},
		{Stat: x, Op: core.OpOverride, Value: 7, Source: 1},
		{Stat: x, Op: core.OpOverride, Value: 8, Source: 2},
		{Stat: flag, Op: core.OpFlag, Source: 3},
		{Stat: flag, Op: core.OpFlag, Source: 4},
		{Stat: capped, Op: core.OpBase, Value: 20, Source: 5},
	})
	if p := proportional(t, s, core.Node{Stat: x}); len(p.Parts) != 1 || p.Parts[0].Source != 2 || p.Parts[0].Value != 8 {
		t.Errorf("override: %+v", p.Parts)
	}
	if got := bySource(proportional(t, s, core.Node{Stat: flag}).Parts); got[3] != 0.5 || got[4] != 0.5 {
		t.Errorf("flag: %v", got)
	}
	if _, err := s.Proportional(core.Node{Stat: plain}); !errors.Is(err, core.ErrNoAttribution) {
		t.Errorf("formula without attribution: got %v", err)
	}
	if p := proportional(t, s, core.Node{Stat: capped}); p.Amount != 20 || p.Effective != 8 || len(p.CapBy) != 1 || p.CapBy[0].Stat != x {
		t.Errorf("capped by formula: %+v", p)
	}
	if _, err := s.Proportional(core.Node{Stat: 99}); !errors.Is(err, core.ErrBadHandle) {
		t.Errorf("bad node: got %v", err)
	}
}

func TestProportionalAttributionCycle(t *testing.T) {
	t.Parallel()
	var b core.ModelBuilder
	var loop core.StatID
	loop = b.MustAddStat(core.StatDef{
		Name:    "loop",
		Formula: func(_ core.Reader, a core.Agg) float64 { return a.Default() },
		Attribute: func(a *core.Attributor) core.Attribution {
			parts, v := a.Of(core.SelfRef(loop))
			return core.Attribution{Parts: parts, Amount: v}
		},
	})
	s := newSession(t, b.Build(), 1)
	if _, err := s.Proportional(core.Node{Stat: loop}); !errors.Is(err, core.ErrAttributionCycle) {
		t.Errorf("got %v, want an attribution cycle", err)
	}
}

// TestProportionalSums checks on random sessions that parts sum to the amount, which for the
// default rule is the node's total before caps.
func TestProportionalSums(t *testing.T) {
	t.Parallel()
	checked := 0
	for seed := range uint64(40) {
		r := rand.New(rand.NewPCG(seed, 11))
		w := newRandWorld(t, r)
		s := w.session(t)
		for range 40 {
			add(t, s, w.mod(r))
		}
		for ctx := range len(w.ctxs) {
			for actor := range w.nActor {
				for stat := range w.nStat {
					n := core.Node{Actor: core.ActorID(actor), Stat: core.StatID(stat), Ctx: core.CtxID(ctx)}
					if checkSums(t, s, n) {
						checked++
					}
				}
			}
		}
	}
	if checked < 1000 {
		t.Errorf("only %d nodes checked", checked)
	}
}

// checkSums checks one node's parts and reports whether it could be attributed.
func checkSums(tb testing.TB, s *core.Session, n core.Node) bool {
	tb.Helper()
	p, err := s.Proportional(n)
	if errors.Is(err, core.ErrNoAttribution) {
		return false
	}
	if err != nil {
		tb.Fatalf("%+v: %v", n, err)
	}
	e, err := s.Explain(n)
	if err != nil {
		tb.Fatal(err)
	}
	if !near(sumParts(p.Parts), p.Amount) || p.Amount != e.Total || p.Value != get(tb, s, n.Actor, n.Stat, n.Ctx) {
		tb.Fatalf("%+v: sum %v, amount %v, total %v", n, sumParts(p.Parts), p.Amount, e.Total)
	}
	return true
}
