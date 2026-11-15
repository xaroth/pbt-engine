package items_test

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/items"
)

// printed returns the defences it prints by key.
func printed(it *build.Item) map[build.Defence]int {
	out := map[build.Defence]int{}
	for d, v := range map[build.Defence]int{
		build.DefenceArmour: it.Armour, build.DefenceEvasion: it.Evasion, build.DefenceEnergyShield: it.EnergyShield, build.DefenceWard: it.Ward,
	} {
		if v != 0 {
			out[d] = v
		}
	}
	return out
}

// TestParseGameSampleDefences reads the game's samples: every defence they print is what the
// base, the roll, the quality and the local modifiers give, so each gets a roll and no warning.
func TestParseGameSampleDefences(t *testing.T) {
	t.Parallel()
	r, _ := realReader(t)
	names, err := filepath.Glob(filepath.Join("testdata", "game", "*.txt"))
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, path := range names {
		it, warnings, err := r.Parse(gameSample(t, filepath.Base(path)))
		if err != nil {
			continue
		}
		for _, w := range warnings {
			if w.Code == items.DefenceDiffers {
				t.Errorf("%s: %v", path, w)
			}
		}
		for d := range printed(&it) {
			n++
			if _, ok := it.DefenceRolls[d]; !ok {
				t.Errorf("%s: %s printed without a roll", path, d)
			}
		}
	}
	if n != 15 {
		t.Errorf("%d printed defences, want 15", n)
	}
}

// TestParseDefenceDiffers reads game text whose printed armour no roll gives: it keeps the printed
// value, no roll and warns; the evasion it prints keeps its roll.
func TestParseDefenceDiffers(t *testing.T) {
	t.Parallel()
	r, _ := realReader(t)
	text := strings.Replace(gameSample(t, "13-wrath-pace.txt"), "Armour: 155", "Armour: 400", 1)
	it, warnings, err := r.Parse(text)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(warnings, func(w build.Warning) bool { return w.Code == items.DefenceDiffers && w.Subject == "armour" }) {
		t.Errorf("warnings %v, want %s armour", warnings, items.DefenceDiffers)
	}
	if _, ok := it.DefenceRolls[build.DefenceArmour]; ok || it.Armour != 400 {
		t.Errorf("armour %d, rolls %v: want 400 without a roll", it.Armour, it.DefenceRolls)
	}
	if _, ok := it.DefenceRolls[build.DefenceEvasion]; !ok {
		t.Errorf("rolls %v: want evasion", it.DefenceRolls)
	}
}

// TestParseArmourBase reads Path of Building's text of armour: its base percentiles as rolls,
// and the variant of a base of one name by the defences the text prints where the implicits do
// not tell.
func TestParseArmourBase(t *testing.T) {
	t.Parallel()
	r, _ := realReader(t)
	fledgling := "Rarity: UNIQUE\nThe Fledgling\nLacquered Helmet\nArmour: 467\nArmourBasePercentile: 0.7553\n" +
		"Evasion: 467\nEvasionBasePercentile: 0.7553\nQuality: 20\nImplicits: 0\n158% increased Armour and Evasion\n"
	beacon := "Rarity: UNIQUE\nBeacon of Madness\nTwo-Toned Boots (Armour/Evasion)\nArmour: 458\nArmourBasePercentile: 0.1093\n" +
		"Evasion: 458\nEvasionBasePercentile: 0.1093\nQuality: 20\nImplicits: 1\n{vestigial}+1 to Maximum Frenzy Charges\n" +
		"198% increased Armour and Evasion\n"
	for _, tc := range []struct {
		name, text, base string
		rolls            map[build.Defence]float64
		stat             string
	}{
		{
			name: "The Fledgling", text: fledgling, base: "Metadata/Items/Armours/Helmets/HelmetStrDex7",
			rolls: map[build.Defence]float64{build.DefenceArmour: 0.7553, build.DefenceEvasion: 0.7553},
			stat:  "local_armour_and_evasion_+%",
		},
		// Its implicit is none of the three Two-Toned Boots'; it prints armour and evasion.
		{
			name: "Beacon of Madness", text: beacon, base: "Metadata/Items/Armours/Boots/BootsAtlas2",
			rolls: map[build.Defence]float64{build.DefenceArmour: 0.1093, build.DefenceEvasion: 0.1093},
			stat:  "local_armour_and_evasion_+%",
		},
	} {
		it, _, err := r.Parse(tc.text)
		if err != nil {
			t.Fatal(err)
		}
		if it.Base != tc.base {
			t.Errorf("%s: base %s, want %s", tc.name, it.Base, tc.base)
		}
		if diff := cmp.Diff(tc.rolls, it.DefenceRolls); diff != "" {
			t.Errorf("%s rolls (-want +got):\n%s", tc.name, diff)
		}
		last := it.Mods[len(it.Mods)-1]
		if len(last.Stats) != 1 || last.Stats[0].Stat != tc.stat {
			t.Errorf("%s: %q read as %v, want %s", tc.name, last.Text, last.Stats, tc.stat)
		}
	}
}
