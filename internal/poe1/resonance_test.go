package poe1_test

import (
	"math"
	"testing"

	"github.com/xaroth/pbt-engine/config"
	"github.com/xaroth/pbt-engine/internal/poe1"
	"github.com/xaroth/pbt-engine/internal/resolve"
)

// fireAgainstResonance returns the minimum fire damage of a spell that hits for 100 against 50%
// resistance, with the stat and the lowest Resonance the config gives.
func fireAgainstResonance(t *testing.T, stat string, value float64, resonance poe1.Resonance) float64 {
	t.Helper()
	var values config.Values
	if err := config.Set(&values, poe1.LowestResonance, resonance); err != nil {
		t.Fatal(err)
	}
	sh := openWith(t, []resolve.Bundle{bundle(resolve.NodeSource{ID: 1}, resolve.Global, resolve.StatValue{Stat: stat, Value: value})}, values)
	setSkills(t, sh, spellOf(0, "fire"))
	return outputOf(t, sh, poe1.FireMinimum)
}

// Trinity's more Elemental Damage is 3% per whole 5 of the lowest Resonance, which stops at 50:
// 100 fire damage against 50% resistance is 50 without it.
func TestTrinityDamagePerResonance(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		resonance poe1.Resonance
		want      float64
	}{
		{0, 50},
		{4, 50},
		{5, 51.5},
		{24, 56},
		{50, 65},
		{80, 65},
	} {
		got := fireAgainstResonance(t, "elemental_damage_+%_final_per_5_lowest_resonance", 3, c.resonance)
		if math.Abs(got-c.want) > 1e-9 {
			t.Errorf("resonance %d: fire minimum: got %v, want %v", c.resonance, got, c.want)
		}
	}
}

// Trinity's penetration holds from 25 of the lowest Resonance.
func TestTrinityPenetrationAtResonance(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		resonance poe1.Resonance
		want      float64
	}{{0, 50}, {24, 50}, {25, 60}, {50, 60}} {
		got := fireAgainstResonance(t, "damage_penetrates_%_elemental_resistances_while_all_resonance_is_25", 10, c.resonance)
		if math.Abs(got-c.want) > 1e-9 {
			t.Errorf("resonance %d: fire minimum: got %v, want %v", c.resonance, got, c.want)
		}
	}
}
