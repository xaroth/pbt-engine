package poe1_test

import (
	"math"
	"slices"
	"testing"

	"github.com/xaroth/pbt-engine/config"
	"github.com/xaroth/pbt-engine/internal/poe1"
	"github.com/xaroth/pbt-engine/internal/resolve"
)

func TestChargesAndFortification(t *testing.T) {
	t.Parallel()
	// the characters' own limits, from Character.ot
	limits := []resolve.StatValue{
		{Stat: "max_power_charges", Value: 3}, {Stat: "max_frenzy_charges", Value: 3}, {Stat: "max_endurance_charges", Value: 3},
		{Stat: "base_max_fortification", Value: 20},
	}
	testCases := []struct {
		name  string
		extra []resolve.StatValue
		want  map[string]float64
	}{
		{
			name: "no minimum charges, no fortification",
			want: map[string]float64{"power_charges": 0, "frenzy_charges": 0, "endurance_charges": 0, "fortification": 0, "fortification_above_20": 0},
		},
		{
			name:  "minimum charges",
			extra: []resolve.StatValue{{Stat: "base_minimum_frenzy_charges", Value: 1}},
			want:  map[string]float64{"frenzy_charges": 1, "power_charges": 0},
		},
		// Hierophant: 4 + 1 minimum, 3 + 1 maximum; 8% increased mana regeneration rate per charge,
		// 0.8225 * 1.32 = 1.0857
		{
			name: "minimum capped by maximum",
			extra: []resolve.StatValue{
				{Stat: "base_minimum_power_charges", Value: 5}, {Stat: "maximum_power_and_endurance_charges_+", Value: 1},
				{Stat: "mana_regeneration_rate_+%_per_power_charge", Value: 8},
			},
			want: map[string]float64{"power_charges": 4, "maximum_endurance_charges": 4, "mana_regeneration": 1.1},
		},
		{
			name: "maximum endurance charges equal to maximum frenzy charges",
			extra: []resolve.StatValue{
				{Stat: "max_frenzy_charges", Value: 2}, {Stat: "base_minimum_endurance_charges", Value: 9},
				{Stat: "maximum_endurance_charges_is_equal_to_maximum_frenzy_charges", Value: 1},
			},
			want: map[string]float64{"maximum_endurance_charges": 5, "endurance_charges": 5},
		},
		// Steadfast: 23 Fortification, 0.1% of 66 life per second each: 1.518
		{
			name: "maximum fortification",
			extra: []resolve.StatValue{
				{Stat: "you_have_your_maximum_fortification", Value: 1}, {Stat: "base_max_fortification", Value: 3},
				{Stat: "life_regeneration_per_minute_%_per_fortification", Value: 6},
			},
			want: map[string]float64{"fortification": 23, "fortification_above_20": 3, "life_regeneration": 1.5},
		},
		{
			name:  "fortification not held",
			extra: []resolve.StatValue{{Stat: "life_regeneration_per_minute_%_per_fortification", Value: 6}},
			want:  map[string]float64{"fortification": 0, "life_regeneration": 0},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			sh := open(t, resolved(marauder(1, slices.Concat(limits, testCase.extra)...)...))
			for stat, want := range testCase.want {
				if got := value(t, sh, stat); got != want {
					t.Errorf("%s: got %v, want %v", stat, got, want)
				}
			}
		})
	}
}

// A spell that hits for 100 fire against 50% fire resistance deals 50. Frenzy Charges give 4%
// more damage each (Character.ot), and the stat here 10% increased spell damage per Power Charge.
// The character has its maximum charges of a kind it uses, else its minimum. 100 * 1.3 * 1.04
// is 135.2, rounded to 135 before resistance, as PoB does.
func TestUsedCharges(t *testing.T) {
	t.Parallel()
	stats := []resolve.StatValue{
		{Stat: "max_power_charges", Value: 3}, {Stat: "max_frenzy_charges", Value: 3}, {Stat: "max_endurance_charges", Value: 3},
		{Stat: "base_minimum_frenzy_charges", Value: 1},
		{Stat: "object_inherent_damage_+%_final_per_frenzy_charge", Value: 4},
		{Stat: "spell_damage_+%_per_power_charge", Value: 10},
	}
	for _, c := range []struct {
		name    string
		uses    []config.Key[bool]
		charges map[string]float64
		want    float64
	}{
		{name: "none used", charges: map[string]float64{"power_charges": 0, "frenzy_charges": 1, "endurance_charges": 0}, want: 52},
		{name: "frenzy", uses: []config.Key[bool]{poe1.FrenzyCharges}, charges: map[string]float64{"frenzy_charges": 3}, want: 56},
		{
			name: "power and endurance", uses: []config.Key[bool]{poe1.PowerCharges, poe1.EnduranceCharges},
			charges: map[string]float64{"power_charges": 3, "frenzy_charges": 1, "endurance_charges": 3}, want: 67.5,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			var values config.Values
			for _, k := range c.uses {
				if err := config.Set(&values, k, true); err != nil {
					t.Fatal(err)
				}
			}
			sh := openWith(t, []resolve.Bundle{bundle(resolve.NodeSource{ID: 1}, resolve.Global, stats...)}, values)
			setSkills(t, sh, elementalSpell())
			for stat, want := range c.charges {
				if got := value(t, sh, stat); got != want {
					t.Errorf("%s: got %v, want %v", stat, got, want)
				}
			}
			if got := outputOf(t, sh, poe1.FireMinimum); math.Abs(got-c.want) > 1e-9 {
				t.Errorf("fire minimum: got %v, want %v", got, c.want)
			}
		})
	}
}
