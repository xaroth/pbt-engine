package itemdefence_test

import (
	"testing"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/internal/poe1/itemdefence"
)

func defence(t *testing.T, key build.Defence) itemdefence.Defence {
	t.Helper()
	for _, d := range itemdefence.All() {
		if d.Key == key {
			return d
		}
	}
	t.Fatalf("no defence %s", key)
	return itemdefence.Defence{}
}

func TestValueAndRoll(t *testing.T) {
	t.Parallel()
	line := func(stat string, v int32) build.ModLine {
		return build.ModLine{Stats: []build.StatValue{{Stat: stat, Value: v}}}
	}
	// Armour and evasion 82 to 95, 39% increased Armour and Evasion, 20% quality.
	boots := &dataset.ArmourValues{
		Armour: dataset.ValueRange{Min: 82, Max: 95}, Evasion: dataset.ValueRange{Min: 82, Max: 95},
	}
	wrath := build.Item{Quality: 20, Mods: []build.ModLine{line("local_armour_and_evasion_+%", 39)}}
	hybrid := build.Item{Mods: []build.ModLine{line("local_evasion_rating_and_energy_shield", 10), line("local_energy_shield_+%", 50)}}
	esBase := &dataset.ArmourValues{EnergyShield: dataset.ValueRange{Min: 20, Max: 30}}
	noQuality := build.Item{Quality: 20, Mods: []build.ModLine{line("local_quality_does_not_increase_defences", 1)}}
	testCases := []struct {
		name     string
		defence  build.Defence
		item     build.Item
		base     *dataset.ArmourValues
		printed  int
		wantRoll float64
		want     float64
	}{
		// 155 / (1.39 * 1.2) = 92.93; (92.93 - 82) / 13 = 0.8404; (82 + 13 * 0.8404) * 1.668 = 154.99
		{name: "roll from the printed value", defence: build.DefenceArmour, item: wrath, base: boots, printed: 155, wantRoll: 0.8404, want: 155},
		{name: "above the highest roll", defence: build.DefenceArmour, item: wrath, base: boots, printed: 200, wantRoll: 1, want: 158},
		{name: "below the lowest roll", defence: build.DefenceArmour, item: wrath, base: boots, printed: 100, wantRoll: 0, want: 137},
		// 53 / 1.5 = 35.33, less 20 and the 10 shared with evasion: (35.33 - 30) / 10 = 0.5333
		{name: "hybrid flat", defence: build.DefenceEnergyShield, item: hybrid, base: esBase, printed: 53, wantRoll: 0.5333, want: 53},
		{name: "a base without the defence", defence: build.DefenceEvasion, item: hybrid, base: esBase, printed: 10, wantRoll: 1, want: 10},
		{name: "quality does not increase defences", defence: build.DefenceEnergyShield, item: noQuality, base: esBase, printed: 30, wantRoll: 1, want: 30},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			d, l := defence(t, tc.defence), itemdefence.LocalOf(&tc.item)
			roll := d.Roll(tc.base, l, tc.printed)
			if roll != tc.wantRoll {
				t.Errorf("roll %v, want %v", roll, tc.wantRoll)
			}
			if got := d.Value(tc.base, l, roll); got != tc.want {
				t.Errorf("value %v, want %v", got, tc.want)
			}
		})
	}
}
