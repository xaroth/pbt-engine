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
	statEnergyShieldRegeneratedPerSecond                 = "energy_shield_regenerated_per_second"
	statEnergyShieldRegenerationRate                     = "energy_shield_regeneration_rate"
	statPercentOfEnergyShieldRegeneratedPerSecond        = "percent_of_energy_shield_regenerated_per_second"
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
	statNumberOfAllocatedMasteries                       = "number_of_allocated_masteries"
	statPercentOfLifeRegeneratedPerSecond                = "percent_of_life_regenerated_per_second"
	statPercentOfManaRegeneratedPerSecond                = "percent_of_mana_regenerated_per_second"
	statStrength                                         = "strength"
	statMaximumPowerCharges                              = "maximum_power_charges"
	statMaximumFrenzyCharges                             = "maximum_frenzy_charges"
	statMaximumEnduranceCharges                          = "maximum_endurance_charges"
	statMinimumPowerCharges                              = "minimum_power_charges"
	statMinimumFrenzyCharges                             = "minimum_frenzy_charges"
	statMinimumEnduranceCharges                          = "minimum_endurance_charges"
	statPowerCharges                                     = "power_charges"
	statFrenzyCharges                                    = "frenzy_charges"
	statEnduranceCharges                                 = "endurance_charges"
	statMaximumEnduranceChargesAreMaximumFrenzyCharges   = "maximum_endurance_charges_are_maximum_frenzy_charges"
	statMaximumFortification                             = "maximum_fortification"
	statHasMaximumFortification                          = "has_maximum_fortification"
	statFortification                                    = "fortification"
	statFortificationAbove20                             = "fortification_above_20"
	statMaximumInspirationCharges                        = "maximum_inspiration_charges"
	statInspirationCharges                               = "inspiration_charges"
	statSpellBlockChance                                 = "spell_block_chance"
	statMaximumSpellBlockChance                          = "maximum_spell_block_chance"
	statSpellBlockChanceCapped                           = "spell_block_chance_capped"
	statMovementSpeed                                    = "movement_speed"
	statDamage                                           = "damage"
	statMinimumDamage                                    = "minimum_damage"
	statMaximumDamage                                    = "maximum_damage"
	statReservation                                      = "reservation"
	statReservationEfficiency                            = "reservation_efficiency"
	statManaReservationEfficiency                        = "mana_reservation_efficiency"
	statLifeReservationEfficiency                        = "life_reservation_efficiency"
	statSkillReservesLifeInsteadOfMana                   = "skill_reserves_life_instead_of_mana"
	statSkillHasNoReservation                            = "skill_has_no_reservation"
	statManaReservationBase                              = "mana_reservation_base"
	statManaReservationPermyriadBase                     = "mana_reservation_permyriad_base"
	statLifeReservationBase                              = "life_reservation_base"
	statLifeReservationPermyriadBase                     = "life_reservation_permyriad_base"
	statManaReservation                                  = "mana_reservation"
	statManaReservationPermyriad                         = "mana_reservation_permyriad"
	statLifeReservation                                  = "life_reservation"
	statLifeReservationPermyriad                         = "life_reservation_permyriad"
	statReservedMana                                     = "reserved_mana"
	statReservedManaPermyriad                            = "reserved_mana_permyriad"
	statReservedLife                                     = "reserved_life"
	statReservedLifePermyriad                            = "reserved_life_permyriad"
	statUnreservedMana                                   = "unreserved_mana"
	statUnreservedLife                                   = "unreserved_life"
	statCostEfficiency                                   = "cost_efficiency"
	statManaCostEfficiency                               = "mana_cost_efficiency"
	statLifeCostEfficiency                               = "life_cost_efficiency"
	statSkillCostsLifeInsteadOfMana                      = "skill_costs_life_instead_of_mana"
	statManaCostBase                                     = "mana_cost_base"
	statLifeCostBase                                     = "life_cost_base"
	statCost                                             = "cost"
	statManaCost                                         = "mana_cost"
	statLifeCost                                         = "life_cost"
	statEnemyResistance                                  = "enemy_resistance"
	statEnemyArmour                                      = "enemy_armour"
	statBaseMinimumDamage                                = "base_minimum_damage"
	statBaseMaximumDamage                                = "base_maximum_damage"
	statAddedMinimumDamage                               = "added_minimum_damage"
	statAddedMaximumDamage                               = "added_maximum_damage"
	statBaseDamageMultiplier                             = "base_damage_multiplier"
	statDamageEffectiveness                              = "damage_effectiveness"
	statMinimumHitDamage                                 = "minimum_hit_damage"
	statMaximumHitDamage                                 = "maximum_hit_damage"
	statEffectiveMinimumHitDamage                        = "effective_minimum_hit_damage"
	statEffectiveMaximumHitDamage                        = "effective_maximum_hit_damage"
	statCriticalStrikeChance                             = "critical_strike_chance"
	statCriticalStrikeMultiplier                         = "critical_strike_multiplier"
	statAverageHit                                       = "average_hit"
	statHitChance                                        = "hit_chance"
	statCastTime                                         = "cast_time"
	statCastSpeed                                        = "cast_speed"
	statAttackRate                                       = "attack_rate"
	statAttackSpeed                                      = "attack_speed"
	statHitsPerSecond                                    = "hits_per_second"
	statHitDPS                                           = "hit_dps"
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
// mana and energy shield.
//
// power_charges, frenzy_charges and endurance_charges are the charges the character has: its
// minimum, at most its maximum, as PoB counts them by default (charges not in use).
// fortification is the character's Fortification: its maximum while has_maximum_fortification
// is set, else none, as PoB counts it by default (not Fortified); fortification_above_20 is
// the part above 20. inspiration_charges are the Inspiration Charges the character has: its
// maximum_inspiration_charges, as PoB counts them by default.
//
// spell_block_chance is the uncapped chance to block spell damage, maximum_spell_block_chance
// its cap and spell_block_chance_capped the value the sheet shows.
//
// movement_speed is the character's movement speed as a percentage of the base speed: 100
// before modifiers.
//
// damage is a skill's damage as a percentage of its base damage: 100 before modifiers. Read in
// a skill's context for one damage type, it holds the increases and more multipliers that
// apply to that part of the skill's damage.
//
// Costs and reservation are read in a skill's context. mana_reservation_base and
// mana_reservation_permyriad_base are what the skill's level row reserves with its supports'
// multipliers, flat and in hundredths of a percent of maximum mana; mana_reservation and
// mana_reservation_permyriad are what it reserves, after the increases and more multipliers
// of "Mana Reservation" and the mana_reservation_efficiency; reservation and
// reservation_efficiency hold those to both pools. reserved_mana and reserved_mana_permyriad, read on the sheet, total every
// reserving skill's, and unreserved_mana is what is left of maximum mana. The same for life.
// mana_cost_base is the skill's level row mana cost with its supports' multipliers, mana_cost
// the cost after its modifiers and mana_cost_efficiency; cost and cost_efficiency hold those to
// every cost. The skill_ flags are its "Reserves Life instead of Mana", "has no Reservation"
// and "Costs Life instead of Mana".
//
// A skill's hit is read in its context. enemy_resistance, read for one damage type, is the
// enemy's resistance to it; enemy_armour its armour. base_minimum_damage and
// base_maximum_damage, read for one damage type, are the skill's own base damage of that type,
// and added_minimum_damage and added_maximum_damage the damage added to it; the skill's
// base_damage_multiplier scales the first and its damage_effectiveness the second, both in
// percent. minimum_hit_damage and maximum_hit_damage are the hit of that type after damage, and
// effective_minimum_hit_damage and effective_maximum_hit_damage what the enemy takes of a hit
// that is not critical. critical_strike_chance and critical_strike_multiplier are in percent;
// average_hit is what the enemy takes from an average hit of every type, critical strikes
// weighed in. hit_chance is the chance to hit in percent. cast_time is the skill's in seconds
// and cast_speed its speed in percent of it, 100 before modifiers; hits_per_second follows, and
// hit_dps is the damage the enemy takes per second from hits.
//
// number_of_allocated_masteries is the allocated Mastery passives: the sum of the tree's
// counters, since every allocatable mastery carries its group's count at 1.
func engineStats() []string {
	return slices.Concat([]string{
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
		statEnergyShieldRegeneratedPerSecond,
		statEnergyShieldRegenerationRate,
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
		statNumberOfAllocatedMasteries,
		statPercentOfEnergyShieldRegeneratedPerSecond,
		statPercentOfLifeRegeneratedPerSecond,
		statPercentOfManaRegeneratedPerSecond,
		statStrength,
	}, chargeStats(), []string{
		statSpellBlockChance,
		statMaximumSpellBlockChance,
		statSpellBlockChanceCapped,
		statMovementSpeed,
		statDamage,
	}, reservationStats(), hitStats(), []string{
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
	})
}

// reservationStats are the engine stats of costs and reservation, in registration order.
func reservationStats() []string {
	return []string{
		statReservation,
		statReservationEfficiency,
		statManaReservationEfficiency,
		statLifeReservationEfficiency,
		statSkillReservesLifeInsteadOfMana,
		statSkillHasNoReservation,
		statManaReservationBase,
		statManaReservationPermyriadBase,
		statLifeReservationBase,
		statLifeReservationPermyriadBase,
		statManaReservation,
		statManaReservationPermyriad,
		statLifeReservation,
		statLifeReservationPermyriad,
		statReservedManaPermyriad,
		statReservedMana,
		statReservedLifePermyriad,
		statReservedLife,
		statUnreservedMana,
		statUnreservedLife,
		statCostEfficiency,
		statManaCostEfficiency,
		statLifeCostEfficiency,
		statSkillCostsLifeInsteadOfMana,
		statManaCostBase,
		statLifeCostBase,
		statCost,
		statManaCost,
		statLifeCost,
	}
}

// hitStats are the engine stats of a skill's hit, in registration order.
func hitStats() []string {
	return []string{
		statEnemyResistance,
		statEnemyArmour,
		statBaseMinimumDamage,
		statBaseMaximumDamage,
		statAddedMinimumDamage,
		statAddedMaximumDamage,
		statBaseDamageMultiplier,
		statDamageEffectiveness,
		statMinimumDamage,
		statMaximumDamage,
		statMinimumHitDamage,
		statMaximumHitDamage,
		statEffectiveMinimumHitDamage,
		statEffectiveMaximumHitDamage,
		statCriticalStrikeChance,
		statCriticalStrikeMultiplier,
		statAverageHit,
		statHitChance,
		statCastTime,
		statCastSpeed,
		statAttackRate,
		statAttackSpeed,
		statHitsPerSecond,
		statHitDPS,
	}
}

// chargeStats are the engine stats of charges and Fortification, in registration order.
func chargeStats() []string {
	return []string{
		statMaximumEnduranceChargesAreMaximumFrenzyCharges,
		statHasMaximumFortification,
		statMaximumPowerCharges,
		statMaximumFrenzyCharges,
		statMaximumEnduranceCharges,
		statMinimumPowerCharges,
		statMinimumFrenzyCharges,
		statMinimumEnduranceCharges,
		statPowerCharges,
		statFrenzyCharges,
		statEnduranceCharges,
		statMaximumFortification,
		statFortification,
		statFortificationAbove20,
		statMaximumInspirationCharges,
		statInspirationCharges,
	}
}

// ruleNames are the rules the table refers to for mechanics a modifier cannot express.
func ruleNames() []string {
	return []string{
		ruleConvert,
		ruleGainAsExtra,
	}
}

// Vocabulary is the engine stats, rules and contexts of the character sheet, for
// semantics.Load.
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

// IsContext reports whether name, as "dimension.value", is a context value a table modifier
// may require: a skill type (type.Spell), a damage type (damage.fire) or a form of damage
// (form.hit).
func (Vocabulary) IsContext(name string) bool { return isTableContext(name) }
