package resolve_test

import (
	"os"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/resolve"
)

const marauder = "Metadata/Characters/Str/Str"

func testVersion(t *testing.T) *dataset.Version {
	t.Helper()
	d := &dataset.Dataset{
		Manifest: dataset.Manifest{Game: "poe1", Version: "3.29.3.3"},
		Stats: []dataset.Stat{
			{ID: "level"}, {ID: "life_per_level"}, {ID: "base_maximum_all_resistances_%"},
			{ID: "additional_strength"}, {ID: "maximum_life_+%"}, {ID: "aura_damage_+%"},
		},
		Characters: []dataset.Character{{
			ID: marauder, BaseStrength: 32, BaseDexterity: 14, BaseIntelligence: 14, BaseMaxLife: 38, BaseMaxMana: 34,
			Stats: []dataset.StatValue{{Stat: 0, Value: 1}, {Stat: 1, Value: 12}, {Stat: 2, Value: 75}},
		}},
		Constants: []dataset.Constant{{ID: "life_per_strength", Value: 1, Per: 2}, {ID: resolve.ResistancePenalty, Value: -60, Per: 1}},
		Trees: []dataset.Tree{{
			Variant: "standard",
			Classes: []dataset.Class{{Character: marauder, StartNode: 1, Ascendancies: []dataset.Ascendancy{{ID: "Juggernaut"}}}},
			Nodes: []dataset.Node{
				{ID: 1, Kind: dataset.ClassStart, Neighbours: []uint32{4, 7}},
				{ID: 4, Neighbours: []uint32{1}, Stats: []dataset.StatValue{{Stat: 3, Value: 10}}},
				{ID: 7, Kind: dataset.Notable, Neighbours: []uint32{1, 9}, Stats: []dataset.StatValue{{Stat: 3, Value: 20}, {Stat: 4, Value: 8}},
					Buffs: [][]dataset.StatValue{{{Stat: 5, Value: 5}}}},
				{ID: 9, Kind: dataset.Mastery, Neighbours: []uint32{7}, MasteryEffects: []dataset.MasteryEffect{{ID: 1, Stats: []dataset.StatValue{{Stat: 4, Value: 10}}}}},
			},
		}},
	}
	dir := t.TempDir()
	if _, err := dataset.Write(dir, d); err != nil {
		t.Fatal(err)
	}
	v, err := dataset.Open(os.DirFS(dir))
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func resolveDump(t *testing.T, v *dataset.Version, b *build.Build) string {
	t.Helper()
	c, err := build.Check(b, v)
	if err != nil {
		t.Fatal(err)
	}
	r, err := resolve.Resolve(c)
	if err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	if err := r.Dump(&out); err != nil {
		t.Fatal(err)
	}
	return out.String()
}

func TestResolve(t *testing.T) {
	t.Parallel()
	v := testVersion(t)
	b := &build.Build{Format: 1, Game: "poe1", Version: "3.29", Class: marauder, Level: 90, Nodes: []uint32{7, 1, 4}}
	want := `dataset poe1 3.29.3.3
constant life_per_strength = 1 per 2 (dataset)
constant resistance_penalty_% = -60 per 1 (dataset)
bundle player global character Metadata/Characters/Str/Str
  base_strength = 32
  base_dexterity = 14
  base_intelligence = 14
  base_maximum_life = 38
  base_maximum_mana = 34
  life_per_level = 12
  base_maximum_all_resistances_% = 75
bundle player global level
  level = 90
bundle player global node 4
  additional_strength = 10
bundle player global node 7
  additional_strength = 20
  maximum_life_+% = 8
bundle player buff node 7
  aura_damage_+% = 5
`
	if diff := cmp.Diff(want, resolveDump(t, v, b)); diff != "" {
		t.Errorf("dump (-want +got):\n%s", diff)
	}
}

func TestResolveScenarioAndNotes(t *testing.T) {
	t.Parallel()
	v := testVersion(t)
	testCases := []struct {
		name     string
		scenario build.Scenario
		nodes    []uint32
		want     []string
	}{
		{"act 5 penalty", build.Scenario{Penalty: build.AfterAct5}, nil,
			[]string{"constant resistance_penalty_% = -30 per 1 (scenario resistancePenalty)\n"}},
		{"no penalty", build.Scenario{Penalty: build.NoPenalty}, nil,
			[]string{"constant resistance_penalty_% = 0 per 1 (scenario resistancePenalty)\n"}},
		{"bandit", build.Scenario{Bandit: build.Alira}, nil,
			[]string{"note bandit alira: reward stats are not in the dataset yet\n"}},
		{"mastery without a choice", build.Scenario{}, []uint32{1, 7, 9},
			[]string{"note node 9: mastery effect not chosen; builds have no mastery choices yet\n"}},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			b := &build.Build{Format: 1, Game: "poe1", Version: "3.29", Class: marauder, Level: 1, Nodes: testCase.nodes, Scenario: testCase.scenario}
			got := resolveDump(t, v, b)
			for _, w := range testCase.want {
				if !strings.Contains(got, w) {
					t.Errorf("got:\n%s\nwant a line %q", got, w)
				}
			}
		})
	}
}
