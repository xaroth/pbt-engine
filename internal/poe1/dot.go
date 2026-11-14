package poe1

import (
	"github.com/xaroth/pbt-engine/internal/core"
	"github.com/xaroth/pbt-engine/internal/resolve"
	"github.com/xaroth/pbt-engine/output"
)

const typeDamageOverTime = "DamageOverTime"

// dotDPSCap is the most damage over time per second PoB gives: the largest 32-bit integer over
// 60 seconds (Data.lua DotDpsCap).
const dotDPSCap = 35791394

// dotSkill reports whether the sheet gives sk's damage over time: a skill of the DamageOverTime
// type.
func dotSkill(sk *resolve.Skill) bool {
	return hasTypes(sk, typeDamageOverTime)
}

// dotOutputs are the main skill's damage over time outputs.
func dotOutputs() []Output {
	return []Output{
		{Key: DotDPS, Label: "Damage over Time DPS", Better: output.Higher, Places: hitPlaces, stat: statDamageOverTimeDPS, needs: needsDot},
	}
}

// dotDef completes def when it is a damage over time stat.
//
// A skill's damage over time of one type, read in its context for damage over time of that
// type, is its base damage over time times the type's damage modifier and the Damage over Time
// Multiplier, reduced by the resistance it meets (physical damage meets none) and scaled by the
// enemy's damage taken. Its damage over time per second totals the types, at most dotDPSCap.
// The order follows PoB (CalcOffence.lua, the skill's damage over time); the game text gives
// none of it.
func dotDef(def core.StatDef, ids map[string]core.StatID) core.StatDef {
	switch def.Name {
	case statDamageOverTime:
		d := damageOverTime{
			base: mustID(ids, statBaseDamageOverTime), damage: mustID(ids, statDamage),
			multiplier: mustID(ids, statDamageOverTimeMultiplier), resistance: mustID(ids, statDamageOverTimeEnemyResistance),
			taken: mustID(ids, statEnemyDamageTaken),
		}
		def.FormulaName, def.Formula, def.Attribute = formulaDamageOverTime, d.formula(), d.attribute()
	case statDamageOverTimeDPS:
		d := dotDPS{dot: mustID(ids, statDamageOverTime), dim: damageDimension()}
		def.FormulaName, def.Formula, def.Attribute = formulaDamageOverTimeDPS, d.formula(), d.attribute()
	}
	return def
}

// damageOverTime is what the enemy takes of a skill's damage over time of one type per second.
type damageOverTime struct {
	base, damage, multiplier, resistance, taken core.StatID
}

func (d damageOverTime) formula() core.Formula {
	return func(r core.Reader, _ core.Agg) float64 {
		v := r.Stat(d.base) * r.Stat(d.damage) / percentScale * (1 + r.Stat(d.multiplier)/percentScale)
		return v * resistanceMultiplier(r.Stat(d.resistance)) * r.Stat(d.taken) / percentScale
	}
}

// attribute credits the base, then the damage modifier's increases and more multipliers over
// it, what the Damage over Time Multiplier and the resistance add or take off to their
// sources, and what damage taken changes to its sources.
func (d damageOverTime) attribute() core.Attribute {
	return func(a *core.Attributor) core.Attribution {
		total := a.Explanation().Total
		base := a.Stat(d.base)
		if base == 0 {
			return core.Attribution{Amount: total}
		}
		parts := a.LayersOver(core.SelfRef(d.damage), a.Spread(core.SelfRef(d.base), base, core.OpBase))
		v := base * a.Stat(d.damage) / percentScale
		if m := v * a.Stat(d.multiplier) / percentScale; m != 0 {
			parts = append(parts, a.Spread(core.SelfRef(d.multiplier), m, core.OpMore)...)
			v += m
		}
		if r := v*resistanceMultiplier(a.Stat(d.resistance)) - v; r != 0 {
			parts = append(parts, a.Spread(core.SelfRef(d.resistance), r, core.OpMore)...)
		}
		return core.Attribution{Parts: rest(a, parts, total, core.SelfRef(d.taken)), Amount: total}
	}
}

// dotDPS is a skill's damage over time per second of every type.
type dotDPS struct {
	dot core.StatID
	dim core.Dimension
}

func (d dotDPS) formula() core.Formula {
	return func(r core.Reader, _ core.Agg) float64 {
		total := 0.0
		for t := Physical; t <= Chaos; t++ {
			total += r.StatWith(d.dot, damageBit(d.dim, t))
		}
		return min(total, dotDPSCap)
	}
}

// attribute credits each type's damage over time to its sources, scaled down when the cap
// applies.
func (d dotDPS) attribute() core.Attribute {
	return func(a *core.Attributor) core.Attribution {
		var parts []core.Part
		sum := 0.0
		for t := Physical; t <= Chaos; t++ {
			p, v := a.Of(a.RefWith(d.dot, damageBit(d.dim, t)))
			parts, sum = append(parts, p...), sum+v
		}
		total := a.Explanation().Total
		if sum != 0 && sum != total {
			parts = scaled(parts, total/sum)
		}
		return core.Attribution{Parts: parts, Amount: total}
	}
}
