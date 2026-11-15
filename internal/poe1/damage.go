package poe1

import "github.com/xaroth/pbt-engine/internal/core"

// Engine stats of a skill's damage, read in its context. damage is the skill's damage as a
// percentage of its base damage: 100 before modifiers. Read for one damage type, it holds the
// increases and more multipliers that apply to that part of the skill's damage.
// base_minimum_damage and base_maximum_damage, read for one damage type, are the skill's own base
// damage of that type, and added_minimum_damage and added_maximum_damage the damage added to it;
// the skill's base_damage_multiplier scales the first and its damage_effectiveness the second,
// both in percent. minimum_damage and maximum_damage hold the modifiers of the smallest or
// largest damage alone. critical_strike_chance and critical_strike_multiplier are in percent.
const (
	statDamage                   = "damage"
	statBaseMinimumDamage        = "base_minimum_damage"
	statBaseMaximumDamage        = "base_maximum_damage"
	statAddedMinimumDamage       = "added_minimum_damage"
	statAddedMaximumDamage       = "added_maximum_damage"
	statBaseDamageMultiplier     = "base_damage_multiplier"
	statDamageEffectiveness      = "damage_effectiveness"
	statMinimumDamage            = "minimum_damage"
	statMaximumDamage            = "maximum_damage"
	statCriticalStrikeChance     = "critical_strike_chance"
	statCriticalStrikeMultiplier = "critical_strike_multiplier"
)

// mechanicDamage names the damage mechanic for those that come after it.
const mechanicDamage = "damage"

func damageMechanic() mechanic {
	return mechanic{name: mechanicDamage, stats: []string{
		statDamage,
		statBaseMinimumDamage,
		statBaseMaximumDamage,
		statAddedMinimumDamage,
		statAddedMaximumDamage,
		statBaseDamageMultiplier,
		statDamageEffectiveness,
		statMinimumDamage,
		statMaximumDamage,
		statCriticalStrikeChance,
		statCriticalStrikeMultiplier,
	}, def: damageDef}
}

// damageDef completes def when it is a critical strike stat: the chance is from 0 to 100%, the
// multiplier at least 100%.
func damageDef(def core.StatDef, _ map[string]core.StatID) core.StatDef {
	switch def.Name {
	case statCriticalStrikeChance:
		def.HasMin, def.HasMax, def.Max = true, true, percentScale
	case statCriticalStrikeMultiplier:
		def.HasMin, def.Min = true, percentScale
	}
	return def
}
