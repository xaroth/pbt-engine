package poe1_test

import (
	"slices"
	"testing"

	"github.com/xaroth/pbt-engine/internal/resolve"
)

func TestSpellBlock(t *testing.T) {
	t.Parallel()
	// the characters' own maximum and power charges, from Character.ot
	limits := []resolve.StatValue{{Stat: "base_maximum_spell_block_%", Value: 75}, {Stat: "max_power_charges", Value: 3}}
	testCases := []struct {
		name  string
		extra []resolve.StatValue
		want  map[string]float64
	}{
		{name: "no spell block", want: map[string]float64{"spell_block_chance_capped": 0, "maximum_spell_block_chance": 75}},
		// Mystic Bulwark (18174): 1% increased mana regeneration rate per 1%, 0.8225 * 1.06 = 0.87185
		{
			name: "mana regeneration per spell block",
			extra: []resolve.StatValue{
				{Stat: "base_spell_block_%", Value: 6}, {Stat: "mana_regeneration_rate_+%_per_1%_spell_block_chance", Value: 1},
			},
			want: map[string]float64{"spell_block_chance_capped": 6, "mana_regeneration": 0.9},
		},
		{
			name:  "capped by maximum",
			extra: []resolve.StatValue{{Stat: "base_spell_block_%", Value: 80}},
			want:  map[string]float64{"spell_block_chance": 80, "spell_block_chance_capped": 75},
		},
		{
			name:  "maximum capped at 90",
			extra: []resolve.StatValue{{Stat: "base_spell_block_%", Value: 100}, {Stat: "base_maximum_spell_block_%", Value: 20}},
			want:  map[string]float64{"maximum_spell_block_chance": 90, "spell_block_chance_capped": 90},
		},
		{
			name:  "Glancing Blows doubles",
			extra: []resolve.StatValue{{Stat: "base_spell_block_%", Value: 30}, {Stat: "keystone_glancing_blows", Value: 1}},
			want:  map[string]float64{"spell_block_chance_capped": 60},
		},
		{
			name:  "Versatile Combatant lowers the maximum",
			extra: []resolve.StatValue{{Stat: "base_spell_block_%", Value: 70}, {Stat: "keystone_versatile_combatant", Value: 1}},
			want:  map[string]float64{"maximum_spell_block_chance": 65, "spell_block_chance_capped": 65},
		},
		{
			name: "per power charge",
			extra: []resolve.StatValue{
				{Stat: "base_minimum_power_charges", Value: 2}, {Stat: "additional_spell_block_%_per_power_charge", Value: 5},
			},
			want: map[string]float64{"spell_block_chance_capped": 10},
		},
		// Vile Bastion (37492): 200 energy shield, 1% increased from intelligence and 30% from spell block
		{
			name: "energy shield increased by spell block",
			extra: []resolve.StatValue{
				{Stat: "base_maximum_energy_shield", Value: 200}, {Stat: "base_spell_block_%", Value: 30},
				{Stat: "maximum_energy_shield_increased_by_spell_block_chance", Value: 1},
			},
			want: map[string]float64{"maximum_energy_shield": 262},
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
