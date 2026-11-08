// Package poe1 holds the rules of Path of Exile 1: the engine stats of the character sheet,
// the rules that compute what a modifier cannot express, and the names semantics tables may
// refer to.
package poe1

import "slices"

// engineStats are the engine stats of the character sheet, named after the game's words.
// Resistances are uncapped values; the maximum_ stats are their caps. Regeneration is per
// second: life_regenerated_per_second is a flat amount ("Regenerate 10 Life per second"),
// percent_of_life_regenerated_per_second a share of maximum life ("Regenerate 1% of Life per
// second"), and life_regeneration_rate holds the increases and more multipliers of the
// "Life Regeneration rate"; the same for mana.
func engineStats() []string {
	return []string{
		"armour",
		"chaos_resistance",
		"cold_resistance",
		"dexterity",
		"evasion_rating",
		"fire_resistance",
		"intelligence",
		"level",
		"life_regenerated_per_second",
		"life_regeneration_rate",
		"lightning_resistance",
		"mana_regenerated_per_second",
		"mana_regeneration_rate",
		"maximum_chaos_resistance",
		"maximum_cold_resistance",
		"maximum_energy_shield",
		"maximum_fire_resistance",
		"maximum_life",
		"maximum_lightning_resistance",
		"maximum_mana",
		"percent_of_life_regenerated_per_second",
		"percent_of_mana_regenerated_per_second",
		"strength",
	}
}

// ruleNames are the rules the table refers to for mechanics a modifier cannot express.
func ruleNames() []string {
	return []string{
		// The bonuses of strength, dexterity and intelligence, which some stats turn off.
		"attribute_bonuses",
		// Maximum resistances and the caps they put on resistances.
		"resistance_caps",
		"chaos_resistance_doubled",
		"convert_evasion_rating_to_armour",
		"convert_life_to_energy_shield",
		"energy_shield_from_spell_block_chance",
		"fire_resistance_modifiers_also_apply",
		// "Gain X% of a value as Extra Y": a share of one final value added to another.
		"gain_as_extra",
		// Whether and where life regeneration recovers.
		"life_regeneration",
	}
}

// Vocabulary is the engine stats and rules of the character sheet, for semantics.Load.
type Vocabulary struct{}

// IsEngineStat reports whether name is an engine stat of the character sheet.
func (Vocabulary) IsEngineStat(name string) bool { return slices.Contains(engineStats(), name) }

// IsRule reports whether name is a rule of the character sheet.
func (Vocabulary) IsRule(name string) bool { return slices.Contains(ruleNames(), name) }
