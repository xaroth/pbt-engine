package poe1_test

import (
	"testing"

	"github.com/xaroth/pbt-engine/resolve"
)

// endgame returns a level 90 Marauder after both Kitava quests (-30% to all elemental and
// chaos resistance each, QuestStaticRewards a5q6 and a10q3) with the 75% maximum of every
// resistance from Character.ot, plus extra stats on one node.
func endgame(extra ...resolve.StatValue) []resolve.Bundle {
	kitava := func(id string) resolve.Bundle {
		return bundle(resolve.QuestSource{ID: id}, resolve.Global,
			resolve.StatValue{Stat: "base_resist_all_elements_%", Value: -30},
			resolve.StatValue{Stat: "base_chaos_damage_resistance_%", Value: -30},
		)
	}
	b := marauder(90, extra...)
	b[0].Stats = append(b[0].Stats, resolve.StatValue{Stat: "base_maximum_all_resistances_%", Value: 75})
	return append(b, kitava("a5q6"), kitava("a10q3"))
}

func TestResistances(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name  string
		extra []resolve.StatValue
		want  map[string]float64
	}{
		{
			name: "kitava penalty",
			want: map[string]float64{
				"fire_resistance_capped": -60, "chaos_resistance_capped": -60,
				"maximum_fire_resistance": 75, "maximum_chaos_resistance": 75,
			},
		},
		{
			name:  "fire and all elemental",
			extra: []resolve.StatValue{{Stat: "base_fire_damage_resistance_%", Value: 40}, {Stat: "base_resist_all_elements_%", Value: 30}},
			want:  map[string]float64{"fire_resistance": 10, "cold_resistance": -30, "chaos_resistance": -60},
		},
		{
			name:  "capped at the maximum",
			extra: []resolve.StatValue{{Stat: "base_fire_damage_resistance_%", Value: 200}},
			want:  map[string]float64{"fire_resistance": 140, "fire_resistance_capped": 75},
		},
		{
			name:  "maximum of all elemental",
			extra: []resolve.StatValue{{Stat: "additional_maximum_all_elemental_resistances_%", Value: 2}, {Stat: "base_resist_all_elements_%", Value: 200}},
			want:  map[string]float64{"maximum_cold_resistance": 77, "cold_resistance_capped": 77, "maximum_chaos_resistance": 75},
		},
		{
			name:  "maximum at most 90",
			extra: []resolve.StatValue{{Stat: "base_maximum_fire_damage_resistance_%", Value: 20}},
			want:  map[string]float64{"maximum_fire_resistance": 90},
		},
		{
			// -60 + 20 doubled
			name:  "chaos resistance doubled",
			extra: []resolve.StatValue{{Stat: "base_chaos_damage_resistance_%", Value: 20}, {Stat: "chaos_damage_resistance_is_doubled", Value: 1}},
			want:  map[string]float64{"chaos_resistance": -80, "chaos_resistance_capped": -80},
		},
		{
			// one source doubles as two do
			name: "chaos resistance doubled once",
			extra: []resolve.StatValue{
				{Stat: "base_chaos_damage_resistance_%", Value: 100}, {Stat: "chaos_damage_resistance_is_doubled", Value: 1},
				{Stat: "chaos_damage_resistance_is_doubled", Value: 1},
			},
			want: map[string]float64{"chaos_resistance": 80},
		},
		{
			// Melding of the Flesh: cold's 80% maximum caps fire and lightning too; chaos keeps 75
			name: "capped by the highest maximum",
			extra: []resolve.StatValue{
				{Stat: "elemental_resistances_are_limited_by_highest_maximum_elemental_resistance", Value: 1},
				{Stat: "base_maximum_cold_damage_resistance_%", Value: 5}, {Stat: "base_resist_all_elements_%", Value: 200},
				{Stat: "base_chaos_damage_resistance_%", Value: 200},
			},
			want: map[string]float64{
				"maximum_fire_resistance": 75, "effective_maximum_fire_resistance": 80, "fire_resistance_capped": 80,
				"lightning_resistance_capped": 80, "chaos_resistance_capped": 75,
			},
		},
		{
			name:  "at least -200",
			extra: []resolve.StatValue{{Stat: "base_fire_damage_resistance_%", Value: -300}},
			want:  map[string]float64{"fire_resistance": -360, "fire_resistance_capped": -200},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			sh := open(t, resolved(endgame(testCase.extra...)...))
			for stat, want := range testCase.want {
				if got := value(t, sh, stat); got != want {
					t.Errorf("%s: got %v, want %v", stat, got, want)
				}
			}
		})
	}
}
