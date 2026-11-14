package poe1_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/internal/poe1"
	"github.com/xaroth/pbt-engine/internal/resolve"
)

func TestCompileItem(t *testing.T) {
	t.Parallel()
	line := func(stat string, v int32) build.ModLine {
		return build.ModLine{Kind: build.Explicit, Stats: []build.StatValue{{Stat: stat, Value: v}}}
	}
	helmet := build.Item{Quality: 20, Mods: []build.ModLine{
		line("local_base_physical_damage_reduction_rating", 68),
		line("base_maximum_life", 101),
		line("local_physical_damage_reduction_rating_+%", 82),
		line("base_fire_damage_resistance_%", 33),
	}}
	given := helmet
	given.Armour = 767
	hybrid := build.Item{Mods: []build.ModLine{
		line("local_evasion_and_energy_shield_+%", 50), line("local_energy_shield", 10), line("local_armour_and_evasion_+%", 7),
	}}
	ring := build.Item{Mods: []build.ModLine{line("local_energy_shield", 10), line("base_maximum_life", 5)}}
	strBase := &dataset.ItemBase{Armour: &dataset.ArmourValues{Armour: dataset.ValueRange{Min: 501, Max: 576}}}
	esEvBase := &dataset.ItemBase{Armour: &dataset.ArmourValues{
		Evasion: dataset.ValueRange{Min: 100, Max: 100}, EnergyShield: dataset.ValueRange{Min: 20, Max: 30},
	}}
	testCases := []struct {
		name        string
		item        build.Item
		base        *dataset.ItemBase
		want        []resolve.StatValue
		wantAssumed []string
	}{
		// (576 + 68) * 1.82 * 1.2 = 1406.496
		{
			name: "rolled armour", item: helmet, base: strBase, wantAssumed: []string{"armour"},
			want: []resolve.StatValue{
				{Stat: "base_physical_damage_reduction_rating", Value: 1406}, {Stat: "base_maximum_life", Value: 101},
				{Stat: "base_fire_damage_resistance_%", Value: 33},
			},
		},
		{
			name: "armour the text gives", item: given, base: strBase,
			want: []resolve.StatValue{
				{Stat: "base_physical_damage_reduction_rating", Value: 767}, {Stat: "base_maximum_life", Value: 101},
				{Stat: "base_fire_damage_resistance_%", Value: 33},
			},
		},
		// evasion 100 * 1.57 = 157, a fixed roll; energy shield (30 + 10) * 1.5 = 60
		{
			name: "hybrid", item: hybrid, base: esEvBase, wantAssumed: []string{"energy shield"},
			want: []resolve.StatValue{{Stat: "base_evasion_rating", Value: 157}, {Stat: "base_maximum_energy_shield", Value: 60}},
		},
		{
			name: "no defences", item: ring, base: &dataset.ItemBase{},
			want: []resolve.StatValue{{Stat: "base_maximum_life", Value: 5}},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := poe1.CompileItem(&tc.item, tc.base)
			if diff := cmp.Diff(tc.want, got.Stats); diff != "" {
				t.Errorf("stats (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(tc.wantAssumed, got.Assumed); diff != "" {
				t.Errorf("assumed (-want +got):\n%s", diff)
			}
		})
	}
}
