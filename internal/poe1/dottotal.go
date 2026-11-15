package poe1

import (
	"github.com/xaroth/pbt-engine/internal/core"
	"github.com/xaroth/pbt-engine/internal/resolve"
	"github.com/xaroth/pbt-engine/output"
)

// statTotalDamageOverTimeDPS, read in a skill's context without a form of damage, is all the
// damage over time per second the skill deals: its own damage over time and its ignite, bleeding
// and poisons, at most dotDPSCap.
const statTotalDamageOverTimeDPS = "total_damage_over_time_dps"

// formulaTotalDamageOverTimeDPS is the formula name shown in explanations.
const formulaTotalDamageOverTimeDPS = "all damage over time per second"

func totalDotMechanic() mechanic {
	return mechanic{
		name: "total damage over time", after: []string{mechanicDot, mechanicIgnite, mechanicBleed, mechanicPoison},
		stats: []string{statTotalDamageOverTimeDPS}, def: totalDotDef,
	}
}

func totalDotDef(def core.StatDef, ids map[string]core.StatID) core.StatDef {
	if def.Name == statTotalDamageOverTimeDPS {
		d := totalDot{
			dot: mustID(ids, statDamageOverTimeDPS), masks: newDotMasks(ids),
			ailments: []core.StatID{mustID(ids, statIgniteDPS), mustID(ids, statBleedDPS), mustID(ids, statPoisonDPS)},
			hit:      core.MaskOf(modelContexts().form.Bit(int(formHit) - 1)),
		}
		def.FormulaName, def.Formula, def.Attribute = formulaTotalDamageOverTimeDPS, d.formula(), d.attribute()
	}
	return def
}

// dotOrAilmentSkill reports whether the sheet gives sk's total damage over time with hands: a
// skill that deals damage over time and does not hit, or one whose ailments it gives. A hit
// whose ailments it does not give would leave them out.
func dotOrAilmentSkill(sk *resolve.Skill, h Hands) bool {
	return (dotSkill(sk) && !Hit(sk, h)) || ailmentSkill(sk, h)
}

// dotOutputs are the main skill's damage over time outputs.
func dotOutputs() []Output {
	return []Output{
		{
			Key: DotDPS, Label: "Damage over Time DPS", Better: output.Higher, Places: hitPlaces,
			stat: statTotalDamageOverTimeDPS, needs: needsDot,
		},
	}
}

// totalDot is a skill's damage over time per second and its ailments', read in its context
// without a form: the first in its context for damage over time (dotMasks), the ailments in its
// hit's.
type totalDot struct {
	dot      core.StatID
	ailments []core.StatID
	masks    dotMasks
	hit      core.Mask
}

// at returns the masks the parts are read at, from the skill's context m, with stat reading a
// stat in m.
func (d totalDot) at(m core.Mask, stat func(core.StatID) float64) (dot, hit core.Mask) {
	return d.masks.of(m, stat), m.Or(d.hit)
}

func (d totalDot) formula() core.Formula {
	return func(r core.Reader, _ core.Agg) float64 {
		dot, hit := d.at(r.Mask(), r.Stat)
		v := r.StatAt(d.dot, dot)
		for _, s := range d.ailments {
			v += r.StatAt(s, hit)
		}
		return min(v, dotDPSCap)
	}
}

// attribute credits each part to its sources, scaled down when the cap applies.
func (d totalDot) attribute() core.Attribute {
	return func(a *core.Attributor) core.Attribution {
		dot, hit := d.at(a.Mask(), a.Stat)
		parts, sum := a.Of(a.RefAt(d.dot, dot))
		for _, s := range d.ailments {
			p, v := a.Of(a.RefAt(s, hit))
			parts, sum = append(parts, p...), sum+v
		}
		total := a.Explanation().Total
		if sum != 0 && sum != total {
			parts = scaled(parts, total/sum)
		}
		return core.Attribution{Parts: parts, Amount: total}
	}
}
