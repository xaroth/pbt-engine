package core_test

import (
	"testing"

	"github.com/xaroth/pbt-engine/core"
)

func TestFormulaSeesOverride(t *testing.T) {
	t.Parallel()
	var b core.ModelBuilder
	life := b.MustAddStat(core.StatDef{Name: "life"})
	incSeen := b.MustAddStat(core.StatDef{
		Name: "life_increases", Includes: []core.StatID{life}, FormulaSeesOverride: true,
		Formula: func(_ core.Reader, a core.Agg) float64 { return a.Inc },
	})
	incHidden := b.MustAddStat(core.StatDef{
		Name: "life_increases_or_override", Includes: []core.StatID{life},
		Formula: func(_ core.Reader, a core.Agg) float64 { return a.Inc },
	})
	plain := b.MustAddStat(core.StatDef{Name: "plain", FormulaSeesOverride: true})
	mods := []core.Modifier{
		{Stat: life, Op: core.OpBase, Value: 100, Source: 1},
		{Stat: life, Op: core.OpInc, Value: 30, Source: 2},
		{Stat: life, Op: core.OpOverride, Value: 1, Source: 3},
		{Stat: plain, Op: core.OpBase, Value: 5, Source: 1},
		{Stat: plain, Op: core.OpOverride, Value: 2, Source: 3},
	}
	runEval(t, b.Build(), 1, []evalCase{{
		name: "override",
		mods: mods,
		// the formula that sees the override still returns the increases; without the flag, or
		// without a formula, the override wins
		want: map[core.StatID]float64{life: 1, incSeen: 30, incHidden: 1, plain: 2},
	}})
}
