package poe1_test

import (
	"slices"
	"testing"

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
