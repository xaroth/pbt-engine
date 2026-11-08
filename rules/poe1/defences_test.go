package poe1_test

import (
	"testing"

	"github.com/xaroth/pbt-engine/resolve"
)

func TestDefences(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name  string
		stats []resolve.StatValue
		stat  string
		want  float64
	}{
		// 100 armour, 25% increased: 125
		{
			name: "armour", stat: "armour", want: 125,
			stats: []resolve.StatValue{{Stat: "base_physical_damage_reduction_rating", Value: 100}, {Stat: "physical_damage_reduction_rating_+%", Value: 25}},
		},
		// 15 base evasion and 33 more, 2% from dexterity and 30% increased: 48 * 1.32 = 63.36 rounds to 63
		{
			name: "evasion rounds", stat: "evasion_rating", want: 63,
			stats: []resolve.StatValue{{Stat: "base_evasion_rating", Value: 33}, {Stat: "evasion_rating_+%", Value: 30}},
		},
		// increases to "Evasion Rating and Armour" reach both: 10 * 1.15 = 11.5 rounds to 12
		{
			name: "armour and evasion increase", stat: "armour", want: 12,
			stats: []resolve.StatValue{{Stat: "base_physical_damage_reduction_rating", Value: 10}, {Stat: "evasion_and_physical_damage_reduction_rating_+%", Value: 15}},
		},
		// 30 energy shield, 1% from intelligence and 50% increased: 30 * 1.51 = 45.3 rounds to 45
		{
			name: "energy shield rounds", stat: "maximum_energy_shield", want: 45,
			stats: []resolve.StatValue{{Stat: "base_maximum_energy_shield", Value: 30}, {Stat: "maximum_energy_shield_+%", Value: 50}},
		},
		// "Removes all Energy Shield" overrides it to 0
		{
			name: "energy shield removed", stat: "maximum_energy_shield", want: 0,
			stats: []resolve.StatValue{{Stat: "base_maximum_energy_shield", Value: 30}, {Stat: "no_energy_shield", Value: 1}},
		},
		// negative armour stops at 0
		{
			name: "armour at least 0", stat: "armour", want: 0,
			stats: []resolve.StatValue{{Stat: "base_physical_damage_reduction_rating", Value: -50}},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			if got := value(t, open(t, resolved(marauder(1, testCase.stats...)...)), testCase.stat); got != testCase.want {
				t.Errorf("%s: got %v, want %v", testCase.stat, got, testCase.want)
			}
		})
	}
}
