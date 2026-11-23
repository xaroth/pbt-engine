package poe1

import "github.com/xaroth/pbt-engine/internal/core"

// Engine stats of life, mana and the defences. armour_and_evasion_rating holds modifiers to
// both armour and evasion rating, which both include.
const (
	statArmourAndEvasionRating = "armour_and_evasion_rating"
	statArmour                 = "armour"
	statEvasionRating          = "evasion_rating"
	statMaximumLife            = "maximum_life"
	statMaximumMana            = "maximum_mana"
	statMaximumEnergyShield    = "maximum_energy_shield"
)

// A minion's damage reduction, in percent: what its endurance charges give it. Nothing reads
// them yet.
const (
	statPhysicalDamageReduction  = "physical_damage_reduction"
	statElementalDamageReduction = "elemental_damage_reduction"
)

// mechanicDefences names the defences mechanic for those that come after it.
const mechanicDefences = "defences"

func defenceMechanic() mechanic {
	return mechanic{name: mechanicDefences, stats: []string{
		statArmourAndEvasionRating,
		statArmour,
		statEvasionRating,
		statMaximumLife,
		statMaximumMana,
		statMaximumEnergyShield,
		statPhysicalDamageReduction,
		statElementalDamageReduction,
	}, def: defenceDef}
}

// defenceDef completes def when it is a pool or a defence. Rounding and minimums follow PoB
// (CalcDefence.lua); the game text states none of them, so all are unvalidated.
func defenceDef(def core.StatDef, ids map[string]core.StatID) core.StatDef {
	switch def.Name {
	case statMaximumLife:
		def.Round, def.HasMin, def.Min = core.RoundHalf, true, 1
	case statMaximumMana:
		def.Round = core.RoundHalf
	case statMaximumEnergyShield:
		def.Round, def.HasMin = core.RoundHalf, true
	case statArmour, statEvasionRating:
		def.Round, def.HasMin = core.RoundHalf, true
		def.Includes = []core.StatID{mustID(ids, statArmourAndEvasionRating)}
	}
	return def
}
