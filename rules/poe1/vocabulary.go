// Package poe1 holds the rules of Path of Exile 1: the engine stats of the character sheet,
// the rules that compute what a modifier cannot express, and the names semantics tables may
// refer to.
package poe1

import "slices"

// Engine stat names.
const (
	statLifeRegeneration                  = "life_regeneration"
	statManaRegeneration                  = "mana_regeneration"
	statElementalResistance               = "elemental_resistance"
	statMaximumElementalResistance        = "maximum_elemental_resistance"
	statMaximumAllResistances             = "maximum_all_resistances"
	statFireResistanceCapped              = "fire_resistance_capped"
	statColdResistanceCapped              = "cold_resistance_capped"
	statLightningResistanceCapped         = "lightning_resistance_capped"
	statChaosResistanceCapped             = "chaos_resistance_capped"
	statArmour                            = "armour"
	statChaosResistance                   = "chaos_resistance"
	statColdResistance                    = "cold_resistance"
	statDexterity                         = "dexterity"
	statEvasionRating                     = "evasion_rating"
	statFireResistance                    = "fire_resistance"
	statIntelligence                      = "intelligence"
	statLevel                             = "level"
	statLifeRegeneratedPerSecond          = "life_regenerated_per_second"
	statLifeRegenerationRate              = "life_regeneration_rate"
	statLightningResistance               = "lightning_resistance"
	statManaRegeneratedPerSecond          = "mana_regenerated_per_second"
	statManaRegenerationRate              = "mana_regeneration_rate"
	statMaximumChaosResistance            = "maximum_chaos_resistance"
	statMaximumColdResistance             = "maximum_cold_resistance"
	statMaximumEnergyShield               = "maximum_energy_shield"
	statMaximumFireResistance             = "maximum_fire_resistance"
	statMaximumLife                       = "maximum_life"
	statMaximumLightningResistance        = "maximum_lightning_resistance"
	statMaximumMana                       = "maximum_mana"
	statPercentOfLifeRegeneratedPerSecond = "percent_of_life_regenerated_per_second"
	statPercentOfManaRegeneratedPerSecond = "percent_of_mana_regenerated_per_second"
	statStrength                          = "strength"
)

// engineStats are the engine stats of the character sheet, named after the game's words, in
// registration order: a stat follows the stats it includes or reads.
//
// Resistances are uncapped totals; the maximum_ stats are their caps and the _capped stats the
// values the sheet shows. elemental_resistance and the maximum_ stats for all elemental and all
// resistances hold modifiers to several resistances at once, which each resistance includes.
//
// Regeneration is per second: life_regenerated_per_second is a flat amount ("Regenerate 10 Life
// per second"), percent_of_life_regenerated_per_second a share of maximum life ("Regenerate 1%
// of Life per second"), and life_regeneration_rate holds the increases and more multipliers of
// the "Life Regeneration rate"; life_regeneration is the total the sheet shows. The same for
// mana.
func engineStats() []string {
	return []string{
		statElementalResistance,
		statMaximumElementalResistance,
		statMaximumAllResistances,
		statArmour,
		statChaosResistance,
		statColdResistance,
		statDexterity,
		statEvasionRating,
		statFireResistance,
		statIntelligence,
		statLevel,
		statLifeRegeneratedPerSecond,
		statLifeRegenerationRate,
		statLightningResistance,
		statManaRegeneratedPerSecond,
		statManaRegenerationRate,
		statMaximumChaosResistance,
		statMaximumColdResistance,
		statMaximumEnergyShield,
		statMaximumFireResistance,
		statMaximumLife,
		statMaximumLightningResistance,
		statMaximumMana,
		statPercentOfLifeRegeneratedPerSecond,
		statPercentOfManaRegeneratedPerSecond,
		statStrength,
		statFireResistanceCapped,
		statColdResistanceCapped,
		statLightningResistanceCapped,
		statChaosResistanceCapped,
		statLifeRegeneration,
		statManaRegeneration,
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
