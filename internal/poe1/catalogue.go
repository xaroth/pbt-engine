package poe1

import (
	"errors"
	"fmt"

	"github.com/xaroth/pbt-engine/output"
)

// Errors of the catalogue and explanations.
var (
	// ErrUnknownOutput is returned for an output name not in the catalogue.
	ErrUnknownOutput = errors.New("unknown output")
	// ErrUnknownView is returned for a View that is not Tree, Proportional or Marginal.
	ErrUnknownView = errors.New("unknown view")
)

// Units of outputs.
const (
	UnitNone      = ""
	UnitPercent   = "%"
	UnitPerSecond = "/s"
)

// Groups of outputs: the panel a consumer shows an output in.
const (
	// GroupSheet is the character sheet: attributes, pools, armour and evasion, resistances
	// and regeneration.
	GroupSheet = "sheet"
)

// Output is a named value of the sheet a consumer can ask for and render without knowing the
// engine stats: its engine stat, a label, a unit, the decimal places it shows, its group and
// its display order.
type Output struct {
	Key    output.Key `json:"key"`
	Label  string     `json:"label"`
	Unit   string     `json:"unit"`
	Places int        `json:"places"`
	Group  string     `json:"group"`
	// Order is the output's position in the catalogue; lower shows first.
	Order int `json:"order"`
	stat  string
}

// Outputs returns the catalogue of outputs, in display order.
func Outputs() []Output {
	out := group(GroupSheet, []Output{
		{Key: Strength, Label: "Strength", stat: statStrength},
		{Key: Dexterity, Label: "Dexterity", stat: statDexterity},
		{Key: Intelligence, Label: "Intelligence", stat: statIntelligence},
		{Key: Life, Label: "Life", stat: statMaximumLife},
		{Key: Mana, Label: "Mana", stat: statMaximumMana},
		{Key: EnergyShield, Label: "Energy Shield", stat: statMaximumEnergyShield},
		{Key: Armour, Label: "Armour", stat: statArmour},
		{Key: EvasionRating, Label: "Evasion Rating", stat: statEvasionRating},
		{Key: FireResistance, Label: "Fire Resistance", Unit: UnitPercent, stat: statFireResistanceCapped},
		{Key: ColdResistance, Label: "Cold Resistance", Unit: UnitPercent, stat: statColdResistanceCapped},
		{Key: LightningResistance, Label: "Lightning Resistance", Unit: UnitPercent, stat: statLightningResistanceCapped},
		{Key: ChaosResistance, Label: "Chaos Resistance", Unit: UnitPercent, stat: statChaosResistanceCapped},
		{Key: MaximumFireResistance, Label: "Maximum Fire Resistance", Unit: UnitPercent, stat: statEffectiveMaximumFireResistance},
		{Key: MaximumColdResistance, Label: "Maximum Cold Resistance", Unit: UnitPercent, stat: statEffectiveMaximumColdResistance},
		{
			Key: MaximumLightningResistance, Label: "Maximum Lightning Resistance", Unit: UnitPercent,
			stat: statEffectiveMaximumLightningResistance,
		},
		{Key: MaximumChaosResistance, Label: "Maximum Chaos Resistance", Unit: UnitPercent, stat: statMaximumChaosResistance},
		{Key: UncappedFireResistance, Label: "Uncapped Fire Resistance", Unit: UnitPercent, stat: statFireResistance},
		{Key: UncappedColdResistance, Label: "Uncapped Cold Resistance", Unit: UnitPercent, stat: statColdResistance},
		{Key: UncappedLightningResistance, Label: "Uncapped Lightning Resistance", Unit: UnitPercent, stat: statLightningResistance},
		{Key: UncappedChaosResistance, Label: "Uncapped Chaos Resistance", Unit: UnitPercent, stat: statChaosResistance},
		{Key: LifeRegeneration, Label: "Life Regeneration", Unit: UnitPerSecond, Places: regenerationPlaces, stat: statLifeRegeneration},
		{Key: ManaRegeneration, Label: "Mana Regeneration", Unit: UnitPerSecond, Places: regenerationPlaces, stat: statManaRegeneration},
		{
			Key: EnergyShieldRegeneration, Label: "Energy Shield Regeneration", Unit: UnitPerSecond, Places: regenerationPlaces,
			stat: statEnergyShieldRegeneration,
		},
	})
	for i := range out {
		out[i].Order = i
	}
	return out
}

// group returns outputs with their group set to g.
func group(g string, outputs []Output) []Output {
	for i := range outputs {
		outputs[i].Group = g
	}
	return outputs
}

// Output returns the value of catalogue output k.
func (sh *Sheet) Output(k output.Key) (float64, error) {
	o, ok := outputByKey(k)
	if !ok {
		return 0, fmt.Errorf("%w %q", ErrUnknownOutput, k)
	}
	return sh.Value(o.stat)
}

func outputByKey(k output.Key) (Output, bool) {
	for _, o := range Outputs() {
		if o.Key == k {
			return o, true
		}
	}
	return Output{}, false
}
