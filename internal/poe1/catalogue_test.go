package poe1_test

import (
	"errors"
	"maps"
	"slices"
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
		poe1.Strength, poe1.Dexterity, poe1.Intelligence,
		poe1.Life, poe1.ReservedLife, poe1.UnreservedLife, poe1.LifeRegeneration,
		poe1.Mana, poe1.ReservedMana, poe1.UnreservedMana, poe1.ManaRegeneration,
		poe1.EnergyShield, poe1.EnergyShieldRegeneration,
		poe1.FireResistance, poe1.MaximumFireResistance, poe1.UncappedFireResistance,
		poe1.ColdResistance, poe1.MaximumColdResistance, poe1.UncappedColdResistance,
		poe1.LightningResistance, poe1.MaximumLightningResistance, poe1.UncappedLightningResistance,
		poe1.ChaosResistance, poe1.MaximumChaosResistance, poe1.UncappedChaosResistance,
		poe1.Armour, poe1.EvasionRating, poe1.BlockChance, poe1.SpellBlockChance, poe1.MovementSpeed,
		poe1.PhysicalDamageModifier, poe1.FireDamageModifier, poe1.ColdDamageModifier, poe1.LightningDamageModifier,
		poe1.ChaosDamageModifier, poe1.ManaCost, poe1.LifeCost, poe1.TotemLife,
		poe1.AverageHit, poe1.MainHandAverageHit, poe1.OffHandAverageHit, poe1.AccuracyRating,
		poe1.MainHandAccuracyRating, poe1.OffHandAccuracyRating, poe1.HitChance,
		poe1.CriticalStrikeChance, poe1.HitsPerSecond, poe1.HitDPS, poe1.TotemsHitDPS,
		poe1.PhysicalMinimum, poe1.PhysicalMaximum, poe1.FireMinimum, poe1.FireMaximum, poe1.ColdMinimum, poe1.ColdMaximum,
		poe1.LightningMinimum, poe1.LightningMaximum, poe1.ChaosMinimum, poe1.ChaosMaximum,
		poe1.MainHandPhysicalMinimum, poe1.MainHandPhysicalMaximum, poe1.MainHandFireMinimum, poe1.MainHandFireMaximum,
		poe1.MainHandColdMinimum, poe1.MainHandColdMaximum, poe1.MainHandLightningMinimum, poe1.MainHandLightningMaximum,
		poe1.MainHandChaosMinimum, poe1.MainHandChaosMaximum, poe1.OffHandPhysicalMinimum, poe1.OffHandPhysicalMaximum,
		poe1.OffHandFireMinimum, poe1.OffHandFireMaximum, poe1.OffHandColdMinimum, poe1.OffHandColdMaximum,
		poe1.OffHandLightningMinimum, poe1.OffHandLightningMaximum, poe1.OffHandChaosMinimum, poe1.OffHandChaosMaximum,
		poe1.DotDPS, poe1.IgniteDPS, poe1.BleedDPS, poe1.PoisonDPS,
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

// Every output has a group, the groups come in topic order, each once, and an output's display
// order is its position in the catalogue.
func TestOutputGroupsAndOrder(t *testing.T) {
	t.Parallel()
	var groups []string
	for i, o := range poe1.Outputs() {
		if o.Group == "" {
			t.Errorf("%s has no group", o.Key)
		}
		if o.Order != i {
			t.Errorf("%s: order %d, want %d", o.Key, o.Order, i)
		}
		if len(groups) == 0 || groups[len(groups)-1] != o.Group {
			groups = append(groups, o.Group)
		}
	}
	want := []string{
		poe1.GroupAttributes, poe1.GroupLife, poe1.GroupMana, poe1.GroupEnergyShield, poe1.GroupResistances,
		poe1.GroupDefences, poe1.GroupSkill,
	}
	if !slices.Equal(groups, want) {
		t.Errorf("groups %v, want %v", groups, want)
	}
}

// Each resistance relates to its maximum and its uncapped value, and only resistances relate to
// others.
func TestOutputRelated(t *testing.T) {
	t.Parallel()
	want := map[output.Key]map[string]output.Key{
		poe1.FireResistance: {poe1.RelationMaximum: poe1.MaximumFireResistance, poe1.RelationUncapped: poe1.UncappedFireResistance},
		poe1.ColdResistance: {poe1.RelationMaximum: poe1.MaximumColdResistance, poe1.RelationUncapped: poe1.UncappedColdResistance},
		poe1.LightningResistance: {
			poe1.RelationMaximum: poe1.MaximumLightningResistance, poe1.RelationUncapped: poe1.UncappedLightningResistance,
		},
		poe1.ChaosResistance: {poe1.RelationMaximum: poe1.MaximumChaosResistance, poe1.RelationUncapped: poe1.UncappedChaosResistance},
	}
	outputs := poe1.Outputs()
	for _, o := range outputs {
		if !maps.Equal(o.Related, want[o.Key]) {
			t.Errorf("%s: related %v, want %v", o.Key, o.Related, want[o.Key])
		}
		for rel, k := range o.Related {
			if !slices.ContainsFunc(outputs, func(r poe1.Output) bool { return r.Key == k && r.Group == o.Group }) {
				t.Errorf("%s: %s %s is not an output of its group", o.Key, rel, k)
			}
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
