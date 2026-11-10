package poe1_test

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/internal/poe1"
	"github.com/xaroth/pbt-engine/internal/resolve"
	"github.com/xaroth/pbt-engine/output"
)

const goldenDir = "testdata/explain"

// golden renders outputs of sh in every view and compares the text with testdata/explain/name.txt,
// or rewrites it with -update.
func golden(t *testing.T, name string, sh *poe1.Sheet, outputs ...output.Key) {
	t.Helper()
	var b strings.Builder
	for _, o := range outputs {
		for _, v := range []poe1.View{poe1.Tree, poe1.Proportional, poe1.Marginal} {
			b.WriteString(render(t, explainOf(t, sh, o, v)))
			b.WriteString("\n")
		}
	}
	path := filepath.Join(goldenDir, name+".txt")
	if *update {
		if err := os.WriteFile(path, []byte(b.String()), 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run with -update to create it)", err)
	}
	if diff := cmp.Diff(string(want), b.String()); diff != "" {
		t.Errorf("%s (-want +got):\n%s", path, diff)
	}
}

// TestGoldenExplanations covers each formula on hand-built sheets, with ids for names.
func TestGoldenExplanations(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name    string
		bundles []resolve.Bundle
		outputs []output.Key
	}{
		{"marauder-90", endgame(), []output.Key{"player.life", "player.mana", "player.evasion_rating"}},
		{"resistances", append(fireSheet(),
			bundle(resolve.NodeSource{ID: 3}, resolve.Global,
				resolve.StatValue{Stat: "base_chaos_damage_resistance_%", Value: 20}, resolve.StatValue{Stat: "chaos_damage_resistance_is_doubled", Value: 1})),
			[]output.Key{"player.fire_resistance", "player.cold_resistance", "player.chaos_resistance", "player.maximum_cold_resistance"}},
		{"melding", endgame(
			resolve.StatValue{Stat: "elemental_resistances_are_limited_by_highest_maximum_elemental_resistance", Value: 1},
			resolve.StatValue{Stat: "base_maximum_cold_damage_resistance_%", Value: 5}, resolve.StatValue{Stat: "base_resist_all_elements_%", Value: 200}),
			[]output.Key{"player.fire_resistance", "player.maximum_fire_resistance"}},
		{"regeneration", append(endgame(
			resolve.StatValue{Stat: "life_regeneration_rate_per_minute_%", Value: 120}, resolve.StatValue{Stat: "base_life_regeneration_rate_per_minute", Value: 600}),
			other(resolve.StatValue{Stat: "life_regeneration_rate_+%", Value: 50}, resolve.StatValue{Stat: "keystone_eternal_youth", Value: 1})),
			[]output.Key{"player.life_regeneration", "player.mana_regeneration"}},
		{"transfers", append(endgame(
			resolve.StatValue{Stat: "maximum_life_%_to_add_as_maximum_armour", Value: 10}, resolve.StatValue{Stat: "maximum_life_+%", Value: 20}),
			other(resolve.StatValue{Stat: "maximum_life_%_to_convert_to_maximum_energy_shield", Value: 30},
				resolve.StatValue{Stat: "physical_damage_reduction_rating_+%", Value: 30})),
			[]output.Key{"player.life", "player.energy_shield", "player.armour"}},
		{"iron-reflexes", endgame(
			resolve.StatValue{Stat: "keystone_iron_reflexes", Value: 1}, resolve.StatValue{Stat: "base_evasion_rating", Value: 85},
			resolve.StatValue{Stat: "evasion_and_physical_damage_reduction_rating_+%", Value: 20}),
			[]output.Key{"player.armour", "player.evasion_rating"}},
		{"chaos-inoculation", endgame(
			resolve.StatValue{Stat: "keystone_chaos_inoculation", Value: 1}, resolve.StatValue{Stat: "maximum_life_%_to_add_as_maximum_energy_shield", Value: 100},
			resolve.StatValue{Stat: "base_maximum_energy_shield", Value: 100}),
			[]output.Key{"player.life", "player.energy_shield"}},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			golden(t, testCase.name, open(t, resolved(testCase.bundles...)), testCase.outputs...)
		})
	}
}

// TestGoldenRealExplanations explains the 2.6 hand-worked builds and a level 90 Chieftain
// with a real tree (pbt-conformance 05-marauder-L90-chieftain-righteous-fire, tree only) on
// the real datasets, with names and stat text from the dataset. Skipped without
// PBT_DATASETS.
func TestGoldenRealExplanations(t *testing.T) {
	t.Parallel()
	const marauder, witch, ranger = "Metadata/Characters/Str/Str", "Metadata/Characters/Int/Int", "Metadata/Characters/Dex/Dex"
	testCases := []struct {
		name    string
		build   build.Build
		outputs []output.Key
	}{
		{"marauder-L1", build.Build{Class: marauder, Level: 1}, []output.Key{"player.life", "player.mana_regeneration"}},
		{"witch-L1", build.Build{Class: witch, Level: 1}, []output.Key{"player.mana"}},
		{"ranger-L90", build.Build{Class: ranger, Level: 90}, []output.Key{"player.evasion_rating"}},
		{
			"juggernaut-L90-life-node", build.Build{Class: marauder, Ascendancy: "Juggernaut", Level: 90, Nodes: []uint32{47175, 17765, 31628}},
			[]output.Key{"player.life"},
		},
		{
			"chieftain-L90", build.Build{Class: marauder, Ascendancy: "Chieftain", Level: 90},
			[]output.Key{"player.life", "player.fire_resistance", "player.life_regeneration"},
		},
	}
	for _, version := range []string{"3.28.0.16", "3.29.3.3"} {
		for _, testCase := range testCases {
			t.Run(version+"/"+testCase.name, func(t *testing.T) {
				t.Parallel()
				b := testCase.build
				b.Format, b.Game, b.Version = 1, "poe1", version
				if testCase.name == "chieftain-L90" {
					b.Nodes = treeNodes(t, version)
				}
				sh, _ := realSheetStore(t, version, &b)
				golden(t, "real-"+version+"-"+testCase.name, sh, testCase.outputs...)
			})
		}
	}
}

// treeNodes reads the Chieftain's node list for version's league.
func treeNodes(tb testing.TB, version string) []uint32 {
	tb.Helper()
	league := version[:strings.LastIndex(version[:strings.LastIndex(version, ".")], ".")]
	data, err := os.ReadFile(filepath.Join(goldenDir, "chieftain-L90-"+league+".nodes"))
	if err != nil {
		tb.Fatal(err)
	}
	var out []uint32
	for f := range strings.SplitSeq(strings.TrimSpace(string(data)), ",") {
		n, err := strconv.ParseUint(f, 10, 32)
		if err != nil {
			tb.Fatal(err)
		}
		out = append(out, uint32(n))
	}
	return out
}
