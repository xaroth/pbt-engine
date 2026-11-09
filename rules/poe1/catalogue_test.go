package poe1_test

import (
	"errors"
	"testing"

	"github.com/xaroth/pbt-engine/output"
	rules "github.com/xaroth/pbt-engine/rules/poe1"
)

func TestOutputs(t *testing.T) {
	t.Parallel()
	sh := open(t, resolved(endgame()...))
	seen := map[output.Key]bool{}
	for _, o := range rules.Outputs() {
		if seen[o.Key] {
			t.Errorf("output %s listed twice", o.Key)
		}
		seen[o.Key] = true
		if _, err := sh.Output(o.Key); err != nil {
			t.Errorf("%s: %v", o.Key, err)
		}
	}
	for name, want := range map[output.Key]float64{rules.Life: 1134, rules.FireResistance: -60, rules.MaximumFireResistance: 75} {
		if got, err := sh.Output(name); err != nil || got != want {
			t.Errorf("%s: got %v, %v, want %v", name, got, err, want)
		}
	}
	if _, err := sh.Output("player.lifes"); !errors.Is(err, rules.ErrUnknownOutput) {
		t.Errorf("unknown output: got %v, want %v", err, rules.ErrUnknownOutput)
	}
}

// Every catalogue entry has its key constant: the keys are the catalogue, in order.
func TestOutputKeys(t *testing.T) {
	t.Parallel()
	keys := []output.Key{
		rules.Strength, rules.Dexterity, rules.Intelligence, rules.Life, rules.Mana, rules.EnergyShield, rules.Armour,
		rules.EvasionRating, rules.FireResistance, rules.ColdResistance, rules.LightningResistance, rules.ChaosResistance,
		rules.MaximumFireResistance, rules.MaximumColdResistance, rules.MaximumLightningResistance, rules.MaximumChaosResistance,
		rules.UncappedFireResistance, rules.UncappedColdResistance, rules.UncappedLightningResistance, rules.UncappedChaosResistance,
		rules.LifeRegeneration, rules.ManaRegeneration, rules.EnergyShieldRegeneration,
	}
	outputs := rules.Outputs()
	if len(outputs) != len(keys) {
		t.Fatalf("%d outputs, %d keys", len(outputs), len(keys))
	}
	for i, o := range outputs {
		if o.Key != keys[i] {
			t.Errorf("output %d: key %s, want %s", i, o.Key, keys[i])
		}
	}
}
