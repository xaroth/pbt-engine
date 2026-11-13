package poe1_test

import (
	"errors"
	"testing"

	"github.com/xaroth/pbt-engine/internal/poe1"
	"github.com/xaroth/pbt-engine/output"
)

func TestOutputs(t *testing.T) {
	t.Parallel()
	sh := open(t, resolved(endgame()...))
	setSkills(t, sh, skill(0, "Spell"))
	seen := map[output.Key]bool{}
	for _, o := range poe1.Outputs() {
		if seen[o.Key] {
			t.Errorf("output %s listed twice", o.Key)
		}
		seen[o.Key] = true
		if _, err := sh.Output(o.Key); err != nil {
			t.Errorf("%s: %v", o.Key, err)
		}
	}
	for name, want := range map[output.Key]float64{poe1.Life: 1134, poe1.UnreservedLife: 1134, poe1.FireResistance: -60, poe1.MaximumFireResistance: 75} {
		if got, err := sh.Output(name); err != nil || got != want {
			t.Errorf("%s: got %v, %v, want %v", name, got, err, want)
		}
	}
	if _, err := sh.Output("player.lifes"); !errors.Is(err, poe1.ErrUnknownOutput) {
		t.Errorf("unknown output: got %v, want %v", err, poe1.ErrUnknownOutput)
	}
}

// Every catalogue entry has its key constant: the keys are the catalogue, in order.
func TestOutputKeys(t *testing.T) {
	t.Parallel()
	keys := []output.Key{
		poe1.Strength, poe1.Dexterity, poe1.Intelligence, poe1.Life, poe1.UnreservedLife, poe1.Mana, poe1.UnreservedMana,
		poe1.EnergyShield, poe1.Armour,
		poe1.EvasionRating, poe1.FireResistance, poe1.ColdResistance, poe1.LightningResistance, poe1.ChaosResistance,
		poe1.MaximumFireResistance, poe1.MaximumColdResistance, poe1.MaximumLightningResistance, poe1.MaximumChaosResistance,
		poe1.UncappedFireResistance, poe1.UncappedColdResistance, poe1.UncappedLightningResistance, poe1.UncappedChaosResistance,
		poe1.LifeRegeneration, poe1.ManaRegeneration, poe1.EnergyShieldRegeneration, poe1.MovementSpeed, poe1.SpellBlockChance,
		poe1.PhysicalDamageModifier, poe1.FireDamageModifier, poe1.ColdDamageModifier, poe1.LightningDamageModifier,
		poe1.ChaosDamageModifier,
	}
	outputs := poe1.Outputs()
	if len(outputs) != len(keys) {
		t.Fatalf("%d outputs, %d keys", len(outputs), len(keys))
	}
	for i, o := range outputs {
		if o.Key != keys[i] {
			t.Errorf("output %d: key %s, want %s", i, o.Key, keys[i])
		}
	}
}

// Every output has a group and its display order is its position in the catalogue.
func TestOutputGroupsAndOrder(t *testing.T) {
	t.Parallel()
	for i, o := range poe1.Outputs() {
		if o.Group == "" {
			t.Errorf("%s has no group", o.Key)
		}
		if o.Order != i {
			t.Errorf("%s: order %d, want %d", o.Key, o.Order, i)
		}
	}
}

// Every output says which way is better: higher for life.
func TestOutputBetter(t *testing.T) {
	t.Parallel()
	for _, o := range poe1.Outputs() {
		if !o.Better.Valid() {
			t.Errorf("%s: better %q, want higher, lower or neither", o.Key, o.Better)
		}
		if o.Key == poe1.Life && o.Better != output.Higher {
			t.Errorf("%s: better %q, want %q", o.Key, o.Better, output.Higher)
		}
	}
}
