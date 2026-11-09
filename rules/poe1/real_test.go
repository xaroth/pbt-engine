package poe1_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/output"
	"github.com/xaroth/pbt-engine/resolve"
	rules "github.com/xaroth/pbt-engine/rules/poe1"
)

// realSheet opens the sheet of b on the real dataset of version under $PBT_DATASETS, a
// directory with one dataset per version, and skips the test when the variable is not set.
func realSheet(t *testing.T, version string, b *build.Build) *rules.Sheet {
	t.Helper()
	root := os.Getenv("PBT_DATASETS")
	if root == "" {
		t.Skip("PBT_DATASETS not set")
	}
	s, err := dataset.Open(os.DirFS(filepath.Join(root, version)))
	if err != nil {
		t.Fatalf("dataset %s: %v", version, err)
	}
	c, err := build.Check(b, s)
	if err != nil {
		t.Fatal(err)
	}
	r, err := resolve.Resolve(c)
	if err != nil {
		t.Fatal(err)
	}
	return open(t, r)
}

// TestRealSheets are mechanic cases on the real 3.28.0.16 and 3.29.3.3 datasets. Each value is
// worked out by hand from `pbt data show` and the formulas: life 38 + 12 per level + strength / 2,
// mana 34 + 6 per level + intelligence / 2, 15 evasion with 1% increased per 5 dexterity, mana
// regeneration 1.75% of mana per second, and -60% to every resistance after both Kitava quests
// (the default campaign stage) with a 75% maximum.
func TestRealSheets(t *testing.T) {
	t.Parallel()
	const (
		marauder = "Metadata/Characters/Str/Str"
		witch    = "Metadata/Characters/Int/Int"
		ranger   = "Metadata/Characters/Dex/Dex"
	)
	testCases := []struct {
		name  string
		build build.Build
		want  map[output.Key]float64
	}{
		{
			// 38 + 12 + 16; 34 + 6 + 7; 15 * 1.02; 47 * 0.0175 = 0.8225
			name:  "marauder level 1",
			build: build.Build{Class: marauder, Level: 1},
			want: map[output.Key]float64{
				"player.life": 66, "player.mana": 47, "player.evasion_rating": 15, "player.mana_regeneration": 0.8,
				"player.fire_resistance": -60, "player.chaos_resistance": -60, "player.maximum_cold_resistance": 75,
			},
		},
		{
			// 38 + 12 + 7; 34 + 6 + 16; 56 * 0.0175 = 0.98
			name:  "witch level 1",
			build: build.Build{Class: witch, Level: 1},
			want:  map[output.Key]float64{"player.life": 57, "player.mana": 56, "player.mana_regeneration": 1, "player.energy_shield": 0},
		},
		{
			// 38 + 1080 + 7; 34 + 540 + 7; 15 * 1.06 = 15.9
			name:  "ranger level 90",
			build: build.Build{Class: ranger, Level: 90},
			want:  map[output.Key]float64{"player.life": 1125, "player.mana": 581, "player.evasion_rating": 16},
		},
		{
			// node 31628: +16 to maximum Life; 38 + 1080 + 16 + 16
			name:  "juggernaut level 90 with a life node",
			build: build.Build{Class: marauder, Ascendancy: "Juggernaut", Level: 90, Nodes: []uint32{47175, 17765, 31628}},
			want:  map[output.Key]float64{"player.life": 1150, "player.strength": 32},
		},
	}
	for _, version := range []string{"3.28.0.16", "3.29.3.3"} {
		for _, testCase := range testCases {
			t.Run(version+"/"+testCase.name, func(t *testing.T) {
				t.Parallel()
				b := testCase.build
				b.Format, b.Game, b.Version = 1, "poe1", version
				sh := realSheet(t, version, &b)
				for name, want := range testCase.want {
					if got, err := sh.Output(name); err != nil || got != want {
						t.Errorf("%s: got %v, %v, want %v", name, got, err, want)
					}
				}
				for _, n := range sh.Notes {
					if n.Kind == rules.RuleMissing {
						t.Errorf("note: %s", n)
					}
				}
			})
		}
	}
}
