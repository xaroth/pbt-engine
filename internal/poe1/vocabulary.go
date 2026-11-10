package poe1

import "slices"

// Engine stat names.
const (
	statFireResistanceModifiersToColdAndLightning        = "fire_resistance_modifiers_to_cold_and_lightning_%"
	statMaximumFireResistanceModifiersToColdAndLightning = "maximum_fire_resistance_modifiers_to_cold_and_lightning_%"
	statArmourAndEvasionRating                           = "armour_and_evasion_rating"
	statChaosResistanceIsDoubled                         = "chaos_resistance_is_doubled"
	statElementalMaximumResistancesAreHighest            = "elemental_maximum_resistances_are_highest"
	statLifeRegenerationIsNotApplied                     = "life_regeneration_is_not_applied"
	statLifeRegenerationAppliesToEnergyShield            = "life_regeneration_applies_to_energy_shield"
	statEffectiveMaximumFireResistance                   = "effective_maximum_fire_resistance"
	statEffectiveMaximumColdResistance                   = "effective_maximum_cold_resistance"
	statEffectiveMaximumLightningResistance              = "effective_maximum_lightning_resistance"
	statEnergyShieldRegeneration                         = "energy_shield_regeneration"
	statNoStrengthBonusToMaximumLife                     = "no_strength_bonus_to_maximum_life"
	statNoIntelligenceBonusToMaximumMana                 = "no_intelligence_bonus_to_maximum_mana"
	statNoDexterityBonusToEvasionRating                  = "no_dexterity_bonus_to_evasion_rating"
	statNoIntelligenceBonusToMaximumEnergyShield         = "no_intelligence_bonus_to_maximum_energy_shield"
	statLifeRegeneration                                 = "life_regeneration"
	statManaRegeneration                                 = "mana_regeneration"
	statElementalResistance                              = "elemental_resistance"
	statMaximumElementalResistance                       = "maximum_elemental_resistance"
	statMaximumAllResistances                            = "maximum_all_resistances"
	statFireResistanceCapped                             = "fire_resistance_capped"
	statColdResistanceCapped                             = "cold_resistance_capped"
	statLightningResistanceCapped                        = "lightning_resistance_capped"
	statChaosResistanceCapped                            = "chaos_resistance_capped"
	statArmour                                           = "armour"
	statChaosResistance                                  = "chaos_resistance"
	statColdResistance                                   = "cold_resistance"
	statDexterity                                        = "dexterity"
	statEvasionRating                                    = "evasion_rating"
	statFireResistance                                   = "fire_resistance"
	statIntelligence                                     = "intelligence"
	statLevel                                            = "level"
	statLifeRegeneratedPerSecond                         = "life_regenerated_per_second"
	statLifeRegenerationRate                             = "life_regeneration_rate"
	statLightningResistance                              = "lightning_resistance"
	statManaRegeneratedPerSecond                         = "mana_regenerated_per_second"
	statManaRegenerationRate                             = "mana_regeneration_rate"
	statMaximumChaosResistance                           = "maximum_chaos_resistance"
	statMaximumColdResistance                            = "maximum_cold_resistance"
	statMaximumEnergyShield                              = "maximum_energy_shield"
	statMaximumFireResistance                            = "maximum_fire_resistance"
	statMaximumLife                                      = "maximum_life"
	statMaximumLightningResistance                       = "maximum_lightning_resistance"
	statMaximumMana                                      = "maximum_mana"
	statPercentOfLifeRegeneratedPerSecond                = "percent_of_life_regenerated_per_second"
	statPercentOfManaRegeneratedPerSecond                = "percent_of_mana_regenerated_per_second"
	statStrength                                         = "strength"
)

// engineStats are the engine stats of the character sheet, named after the game's words, in
// registration order: a stat follows the stats it includes or reads.
//
// Resistances are uncapped totals; the maximum_ stats are their caps and the _capped stats the
// values the sheet shows. elemental_resistance and the maximum_ stats for all elemental and all
// resistances hold modifiers to several resistances at once, which each resistance includes;
// armour_and_evasion_rating holds those to both, which armour and evasion rating include. The
// _to_cold_and_lightning_% stats are the share of fire's own modifiers, or maximum fire's,
// that cold and lightning take.
//
// The no_ stats are flags that turn off one inherent attribute bonus each; the _is_ and _are_
// stats are flags of one-off mechanics. effective_maximum_ elemental resistances are the
// maximums after "capped by your highest Maximum Elemental Resistance".
//
// Regeneration is per second: life_regenerated_per_second is a flat amount ("Regenerate 10 Life
// per second"), percent_of_life_regenerated_per_second a share of maximum life ("Regenerate 1%
// of Life per second"), and life_regeneration_rate holds the increases and more multipliers of
// the "Life Regeneration rate"; life_regeneration is the total the sheet shows. The same for
// mana.
func engineStats() []string {
	return []string{
		statChaosResistanceIsDoubled,
		statElementalMaximumResistancesAreHighest,
		statLifeRegenerationIsNotApplied,
		statLifeRegenerationAppliesToEnergyShield,
		statNoStrengthBonusToMaximumLife,
		statNoIntelligenceBonusToMaximumMana,
		statNoDexterityBonusToEvasionRating,
		statNoIntelligenceBonusToMaximumEnergyShield,
		statArmourAndEvasionRating,
		statElementalResistance,
		statMaximumElementalResistance,
		statMaximumAllResistances,
		statFireResistanceModifiersToColdAndLightning,
		statMaximumFireResistanceModifiersToColdAndLightning,
		statFireResistance,
		statMaximumFireResistance,
		statArmour,
		statChaosResistance,
		statColdResistance,
		statDexterity,
		statEvasionRating,
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
		statMaximumLife,
		statMaximumLightningResistance,
		statMaximumMana,
		statPercentOfLifeRegeneratedPerSecond,
		statPercentOfManaRegeneratedPerSecond,
		statStrength,
		statEffectiveMaximumFireResistance,
		statEffectiveMaximumColdResistance,
		statEffectiveMaximumLightningResistance,
		statFireResistanceCapped,
		statColdResistanceCapped,
		statLightningResistanceCapped,
		statChaosResistanceCapped,
		statLifeRegeneration,
		statManaRegeneration,
		statEnergyShieldRegeneration,
	}
}

// ruleNames are the rules the table refers to for mechanics a modifier cannot express.
func ruleNames() []string {
	return []string{
		ruleConvert,
		"energy_shield_from_spell_block_chance",
		ruleGainAsExtra,
	}
}

// Vocabulary is the engine stats and rules of the character sheet, for semantics.Load.
type Vocabulary struct{}

// IsEngineStat reports whether name is an engine stat of the character sheet.
func (Vocabulary) IsEngineStat(name string) bool { return slices.Contains(engineStats(), name) }

// RuleParams returns the parameters of rule name of the character sheet, and false for an
// unknown rule.
func (Vocabulary) RuleParams(name string) ([]string, bool) {
	switch name {
	case ruleGainAsExtra, ruleConvert:
		return []string{"from", "to"}, true
	}
	return nil, slices.Contains(ruleNames(), name)
}
