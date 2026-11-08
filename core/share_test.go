package core_test

import (
	"testing"

	"github.com/xaroth/pbt-engine/core"
)

type shareModel struct {
	m                     *core.Model
	ele, fire, rate, cold core.StatID
}

func newShareModel() shareModel {
	var b core.ModelBuilder
	var w shareModel
	w.ele = b.MustAddStat(core.StatDef{Name: "elemental_res"})
	w.fire = b.MustAddStat(core.StatDef{Name: "fire_res", Includes: []core.StatID{w.ele}})
	w.rate = b.MustAddStat(core.StatDef{Name: "fire_applies_to_cold_%"})
	w.cold = b.MustAddStat(core.StatDef{
		Name: "cold_res", Includes: []core.StatID{w.ele},
		Shares: []core.Share{{Stat: w.fire, Rate: core.SelfRef(w.rate)}},
	})
	w.m = b.Build()
	return w
}

func TestShares(t *testing.T) {
	t.Parallel()
	w := newShareModel()
	fireMods := [...]core.Modifier{
		{Stat: w.fire, Op: core.OpBase, Value: 30, Source: 1},
		{Stat: w.fire, Op: core.OpInc, Value: 10, Source: 1},
		{Stat: w.fire, Op: core.OpMore, Value: 20, Source: 1},
		{Stat: w.ele, Op: core.OpBase, Value: 12, Source: 2},
		{Stat: w.cold, Op: core.OpBase, Value: 5, Source: 3},
	}
	runEval(t, w.m, 1, []evalCase{
		{
			// fire's own modifiers at 50%: base 5 + 15, 5% increased, 10% more; elemental reaches
			// cold through its include, not through fire
			name: "half of fire's own modifiers",
			mods: append(fireMods[:], core.Modifier{Stat: w.rate, Op: core.OpBase, Value: 50, Source: 4}),
			want: map[core.StatID]float64{w.cold: more((5+12+15)*(1+5/100.0), 10)},
		},
		{name: "rate 0 adds nothing", mods: fireMods[:], want: map[core.StatID]float64{w.cold: 17}},
		{
			name: "fire's overrides do not apply",
			mods: []core.Modifier{
				{Stat: w.fire, Op: core.OpOverride, Value: 90, Source: 1},
				{Stat: w.cold, Op: core.OpBase, Value: 5, Source: 3},
				{Stat: w.rate, Op: core.OpBase, Value: 50, Source: 4},
			},
			want: map[core.StatID]float64{w.cold: 5, w.fire: 90},
		},
	})
}

func TestSharesFollowEdits(t *testing.T) {
	t.Parallel()
	w := newShareModel()
	s := newSession(t, w.m, 1)
	fire := add(t, s, core.Modifier{Stat: w.fire, Op: core.OpBase, Value: 40, Source: 1})
	rate := add(t, s, core.Modifier{Stat: w.rate, Op: core.OpBase, Value: 50, Source: 2})
	want(t, s, w.cold, 20)
	must(t, s.SetModifierValue(fire, 60))
	want(t, s, w.cold, 30)
	must(t, s.SetModifierValue(rate, 25))
	want(t, s, w.cold, 15)
	must(t, s.SetSourceEnabled(1, false))
	want(t, s, w.cold, 0)
}
