package poe1_test

import (
	"testing"

	"github.com/xaroth/pbt-engine/resolve"
)

// marauder returns the bundles of a Marauder at level with extra stats on one node, from the
// 3.29.3.3 Characters row and Character.ot: 32 strength, 14 dexterity, 14 intelligence, 38
// life plus 12 per level, 34 mana plus 6 per level, 15 evasion rating, 1.75% of mana
// regenerated per second (105 per minute).
func marauder(level float64, extra ...resolve.StatValue) []resolve.Bundle {
	return []resolve.Bundle{
		bundle(resolve.CharacterSource{ID: "Metadata/Characters/Str/Str"}, resolve.Global,
			resolve.StatValue{Stat: "base_strength", Value: 32},
			resolve.StatValue{Stat: "base_dexterity", Value: 14},
			resolve.StatValue{Stat: "base_intelligence", Value: 14},
			resolve.StatValue{Stat: "base_maximum_life", Value: 38},
			resolve.StatValue{Stat: "base_maximum_mana", Value: 34},
			resolve.StatValue{Stat: "life_per_level", Value: 12},
			resolve.StatValue{Stat: "mana_per_level", Value: 6},
			resolve.StatValue{Stat: "base_evasion_rating", Value: 15},
			resolve.StatValue{Stat: "mana_regeneration_rate_per_minute_%", Value: 105},
		),
		bundle(resolve.LevelSource{}, resolve.Global, resolve.StatValue{Stat: "level", Value: level}),
		bundle(resolve.NodeSource{ID: 1}, resolve.Global, extra...),
	}
}

func TestAttributes(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name    string
		bundles []resolve.Bundle
		stat    string
		want    float64
	}{
		// 38 + 12 at level 1 + floor(32 / 2)
		{name: "level 1 life", bundles: marauder(1), stat: "maximum_life", want: 66},
		// 38 + 12 x 90 + floor(32 / 2)
		{name: "level 90 life", bundles: marauder(90), stat: "maximum_life", want: 1134},
		// 34 + 6 at level 1 + floor(14 / 2)
		{name: "level 1 mana", bundles: marauder(1), stat: "maximum_mana", want: 47},
		// 33 strength gives floor(33 / 2) = 16, as 32 does
		{
			name: "strength bonus rounds down", stat: "maximum_life", want: 66,
			bundles: marauder(1, resolve.StatValue{Stat: "additional_strength", Value: 1}),
		},
		// 32 * 1.05 = 33.6 rounds to 34
		{
			name: "increased strength rounds half up", stat: "strength", want: 34,
			bundles: marauder(1, resolve.StatValue{Stat: "strength_+%", Value: 5}),
		},
		// the bonus reads the rounded 34 strength: 38 + 12 + 17
		{
			name: "life bonus from increased strength", stat: "maximum_life", want: 67,
			bundles: marauder(1, resolve.StatValue{Stat: "strength_+%", Value: 5}),
		},
		// 66 * 1.25 = 82.5 rounds to 83
		{
			name: "life rounds half up", stat: "maximum_life", want: 83,
			bundles: marauder(1, resolve.StatValue{Stat: "maximum_life_+%", Value: 25}),
		},
		// 15 base evasion with floor(14 / 5) = 2% increased: 15.3 rounds to 15
		{name: "evasion from dexterity", bundles: marauder(1), stat: "evasion_rating", want: 15},
		// 100 energy shield with floor(14 / 10) = 1% increased
		{
			name: "energy shield from intelligence", stat: "maximum_energy_shield", want: 101,
			bundles: marauder(1, resolve.StatValue{Stat: "base_maximum_energy_shield", Value: 100}),
		},
		// no life at all still leaves 1
		{name: "life at least 1", bundles: nil, stat: "maximum_life", want: 1},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			if got := value(t, open(t, resolved(testCase.bundles...)), testCase.stat); got != testCase.want {
				t.Errorf("%s: got %v, want %v", testCase.stat, got, testCase.want)
			}
		})
	}
}
