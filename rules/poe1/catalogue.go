package poe1

import (
	"errors"
	"fmt"
)

// ErrUnknownOutput is returned for an output name not in the catalogue.
var ErrUnknownOutput = errors.New("unknown output")

// Units of outputs.
const (
	UnitNone      = ""
	UnitPercent   = "%"
	UnitPerSecond = "/s"
)

// Output is a named value of the sheet a consumer can ask for and render without knowing the
// engine stats: its engine stat, a label, a unit and the decimal places it shows.
type Output struct {
	Name   string
	Label  string
	Unit   string
	Places int
	stat   string
}

// Outputs returns the catalogue of the character sheet, in display order.
func Outputs() []Output {
	return []Output{
		{Name: "player.strength", Label: "Strength", stat: statStrength},
		{Name: "player.dexterity", Label: "Dexterity", stat: statDexterity},
		{Name: "player.intelligence", Label: "Intelligence", stat: statIntelligence},
		{Name: "player.life", Label: "Life", stat: statMaximumLife},
		{Name: "player.mana", Label: "Mana", stat: statMaximumMana},
		{Name: "player.energy_shield", Label: "Energy Shield", stat: statMaximumEnergyShield},
		{Name: "player.armour", Label: "Armour", stat: statArmour},
		{Name: "player.evasion_rating", Label: "Evasion Rating", stat: statEvasionRating},
		{Name: "player.fire_resistance", Label: "Fire Resistance", Unit: UnitPercent, stat: statFireResistanceCapped},
		{Name: "player.cold_resistance", Label: "Cold Resistance", Unit: UnitPercent, stat: statColdResistanceCapped},
		{Name: "player.lightning_resistance", Label: "Lightning Resistance", Unit: UnitPercent, stat: statLightningResistanceCapped},
		{Name: "player.chaos_resistance", Label: "Chaos Resistance", Unit: UnitPercent, stat: statChaosResistanceCapped},
		{Name: "player.maximum_fire_resistance", Label: "Maximum Fire Resistance", Unit: UnitPercent, stat: statEffectiveMaximumFireResistance},
		{Name: "player.maximum_cold_resistance", Label: "Maximum Cold Resistance", Unit: UnitPercent, stat: statEffectiveMaximumColdResistance},
		{
			Name: "player.maximum_lightning_resistance", Label: "Maximum Lightning Resistance", Unit: UnitPercent,
			stat: statEffectiveMaximumLightningResistance,
		},
		{Name: "player.maximum_chaos_resistance", Label: "Maximum Chaos Resistance", Unit: UnitPercent, stat: statMaximumChaosResistance},
		{Name: "player.uncapped_fire_resistance", Label: "Uncapped Fire Resistance", Unit: UnitPercent, stat: statFireResistance},
		{Name: "player.uncapped_cold_resistance", Label: "Uncapped Cold Resistance", Unit: UnitPercent, stat: statColdResistance},
		{Name: "player.uncapped_lightning_resistance", Label: "Uncapped Lightning Resistance", Unit: UnitPercent, stat: statLightningResistance},
		{Name: "player.uncapped_chaos_resistance", Label: "Uncapped Chaos Resistance", Unit: UnitPercent, stat: statChaosResistance},
		{Name: "player.life_regeneration", Label: "Life Regeneration", Unit: UnitPerSecond, Places: regenerationPlaces, stat: statLifeRegeneration},
		{Name: "player.mana_regeneration", Label: "Mana Regeneration", Unit: UnitPerSecond, Places: regenerationPlaces, stat: statManaRegeneration},
		{
			Name: "player.energy_shield_regeneration", Label: "Energy Shield Regeneration", Unit: UnitPerSecond, Places: regenerationPlaces,
			stat: statEnergyShieldRegeneration,
		},
	}
}

// Output returns the value of catalogue output name.
func (sh *Sheet) Output(name string) (float64, error) {
	for _, o := range Outputs() {
		if o.Name == name {
			return sh.Value(o.stat)
		}
	}
	return 0, fmt.Errorf("%w %q", ErrUnknownOutput, name)
}
