package poe1_test

import (
	"testing"

	"github.com/xaroth/pbt-engine/resolve"
)

func TestRegeneration(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name  string
		extra []resolve.StatValue
		stat  string
		want  float64
	}{
		// 47 mana at 1.75% per second: 0.8225 rounds to 0.8
		{name: "base mana regeneration", stat: "mana_regeneration", want: 0.8},
		// 0.8225 * 1.3 = 1.06925 rounds to 1.1
		{
			name: "increased mana regeneration rate", stat: "mana_regeneration", want: 1.1,
			extra: []resolve.StatValue{{Stat: "mana_regeneration_rate_+%", Value: 30}},
		},
		{name: "no life regeneration", stat: "life_regeneration", want: 0},
		// 120 per minute is 2% of 66 life per second, plus 600 per minute flat: 1.32 + 10 = 11.32
		{
			name: "percent and flat life regeneration", stat: "life_regeneration", want: 11.3,
			extra: []resolve.StatValue{{Stat: "life_regeneration_rate_per_minute_%", Value: 120}, {Stat: "base_life_regeneration_rate_per_minute", Value: 600}},
		},
		// Eternal Youth: 50% less life regeneration rate, 11.32 * 0.5 = 5.66
		{
			name: "less life regeneration rate", stat: "life_regeneration", want: 5.7,
			extra: []resolve.StatValue{
				{Stat: "life_regeneration_rate_per_minute_%", Value: 120},
				{Stat: "base_life_regeneration_rate_per_minute", Value: 600},
				{Stat: "keystone_eternal_youth", Value: 1},
			},
		},
		{
			name: "life regeneration not applied", stat: "life_regeneration", want: 0,
			extra: []resolve.StatValue{
				{Stat: "base_life_regeneration_rate_per_minute", Value: 600}, {Stat: "life_recovery_from_regeneration_is_not_applied", Value: 1},
			},
		},
		{
			name: "life regeneration to energy shield: none to life", stat: "life_regeneration", want: 0,
			extra: []resolve.StatValue{
				{Stat: "base_life_regeneration_rate_per_minute", Value: 600}, {Stat: "regenerate_energy_shield_instead_of_life", Value: 1},
			},
		},
		// 10 flat plus 2% of 200 energy shield (1% from intelligence: 202), without the 50% increased
		// life regeneration rate: 10 + 4.04
		{
			name: "life regeneration to energy shield", stat: "energy_shield_regeneration", want: 14,
			extra: []resolve.StatValue{
				{Stat: "base_life_regeneration_rate_per_minute", Value: 600}, {Stat: "life_regeneration_rate_per_minute_%", Value: 120},
				{Stat: "life_regeneration_rate_+%", Value: 50}, {Stat: "base_maximum_energy_shield", Value: 200},
				{Stat: "regenerate_energy_shield_instead_of_life", Value: 1},
			},
		},
		{name: "no energy shield regeneration", stat: "energy_shield_regeneration", want: 0},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			if got := value(t, open(t, resolved(marauder(1, testCase.extra...)...)), testCase.stat); got != testCase.want {
				t.Errorf("%s: got %v, want %v", testCase.stat, got, testCase.want)
			}
		})
	}
}
