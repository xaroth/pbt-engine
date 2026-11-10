package poe1_test

import (
	"os"
	"testing"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/config"
	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/internal/poe1"
)

// budgetStore has campaign quests worth 1 (act 1), 1 (the bandit kill, act 2), 1 (act 7) and
// 2 (a10q3) passive points and one labyrinth worth 2 ascendancy points; the Marauder tree is a
// chain 1 (start) to 11, and Juggernaut nodes 21 to 23 hang off its start 20.
func budgetStore(t *testing.T) *dataset.Store {
	t.Helper()
	tree := dataset.Tree{
		Variant: "standard",
		Classes: []dataset.Class{{Character: "Metadata/Characters/Str/Str", StartNode: 1, Ascendancies: []dataset.Ascendancy{{ID: "Juggernaut"}}}},
	}
	for id := uint32(1); id <= 11; id++ {
		n := dataset.Node{ID: id, Neighbours: []uint32{id + 1}}
		if id == 1 {
			n.Kind = dataset.ClassStart
		} else {
			n.Neighbours = []uint32{id - 1, id + 1}
		}
		if id == 11 {
			n.Neighbours = []uint32{10}
		}
		tree.Nodes = append(tree.Nodes, n)
	}
	tree.Nodes = append(tree.Nodes, dataset.Node{ID: 20, Kind: dataset.AscendancyStart, Ascendancy: "Juggernaut", Neighbours: []uint32{21, 22, 23}})
	for id := uint32(21); id <= 23; id++ {
		tree.Nodes = append(tree.Nodes, dataset.Node{ID: id, Ascendancy: "Juggernaut", Neighbours: []uint32{20}})
	}
	d := &dataset.Dataset{
		Manifest:   dataset.Manifest{Game: "poe1", Version: "3.29.3.3"},
		Characters: []dataset.Character{{ID: "Metadata/Characters/Str/Str"}},
		Quests: []dataset.Quest{
			{Quest: "a1q7", Flag: "a1q7-UsedRewardBook", Act: 1, PassivePoints: 1},
			{Quest: "a2q7", Flag: "a2q7-QuestRewardTaken", Act: 2, PassivePoints: 1},
			{Quest: "a7q1", Flag: "a7q1-UsedRewardBook", Act: 7, PassivePoints: 1},
			{Quest: "a10q3", Flag: "a10q3-UsedRewardBook", Act: 10, PassivePoints: 2},
			{Flag: "LabyrinthComplete1", AscendancyPoints: 2},
		},
		Trees: []dataset.Tree{tree},
	}
	dir := t.TempDir()
	if _, err := dataset.Write(dir, d); err != nil {
		t.Fatal(err)
	}
	s, err := dataset.Open(os.DirFS(dir))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestPointBudget(t *testing.T) {
	t.Parallel()
	s := budgetStore(t)
	chain := func(n int) []uint32 {
		var out []uint32
		for id := uint32(2); id < uint32(2+n); id++ {
			out = append(out, id)
		}
		return out
	}
	testCases := []struct {
		name     string
		nodes    []uint32
		campaign poe1.CampaignStage
		bandits  poe1.BanditChoice
		codes    []build.Code
	}{
		// level 5: 4 points; complete and the kill: 5 quest points
		{name: "complete, kill all, at budget", nodes: chain(9), bandits: poe1.BanditsKillAll},
		{name: "complete, kill all, over", nodes: chain(10), bandits: poe1.BanditsKillAll, codes: []build.Code{poe1.PointsOverBudget}},
		{name: "complete without the kill", nodes: chain(9), codes: []build.Code{poe1.PointsOverBudget}},
		// acts 6 to 10: a1q7, a7q1 and the kill, not a10q3
		{name: "acts 6 to 10", nodes: chain(7), campaign: poe1.CampaignActs6To10, bandits: poe1.BanditsKillAll},
		{name: "acts 6 to 10, over", nodes: chain(8), campaign: poe1.CampaignActs6To10, bandits: poe1.BanditsKillAll,
			codes: []build.Code{poe1.PointsOverBudget}},
		{name: "acts 1 to 5", nodes: chain(5), campaign: poe1.CampaignActs1To5},
		{name: "ascendancy over", nodes: append(chain(1), 21, 22, 23), codes: []build.Code{poe1.AscendancyPointsOverBudget}},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			b := &build.Build{Format: 1, Game: "poe1", Version: "3.29", Class: "Metadata/Characters/Str/Str", Ascendancy: "Juggernaut",
				Level: 5, Nodes: testCase.nodes}
			c, warnings, err := build.Check(b, s)
			if err != nil || len(warnings) != 0 {
				t.Fatal(err, warnings)
			}
			var v config.Values
			if testCase.campaign != "" {
				must(t, config.Set(&v, poe1.Campaign, testCase.campaign))
			}
			must(t, config.Set(&v, poe1.Bandits, testCase.bandits))
			cfg, _ := config.Resolve(poe1.ConfigSpecs(), config.Layer{Source: config.FromBuild, Values: v})
			got, err := poe1.PointBudget(c, cfg)
			if err != nil {
				t.Fatal(err)
			}
			var codes []build.Code
			for _, w := range got {
				codes = append(codes, w.Code)
			}
			if len(codes) != len(testCase.codes) || (len(codes) > 0 && codes[0] != testCase.codes[0]) {
				t.Errorf("got %v, want %v", got, testCase.codes)
			}
		})
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

// The level 90 Chieftain tree from pbt-conformance (tree only, bandits killed) fits its budget.
func TestPointBudgetRealTree(t *testing.T) {
	t.Parallel()
	for _, version := range []string{"3.28.0.16", "3.29.3.3"} {
		b := &build.Build{Format: 1, Game: "poe1", Version: version, Class: "Metadata/Characters/Str/Str", Ascendancy: "Chieftain",
			Level: 90, Nodes: treeNodes(t, version)}
		_, s := realSheetStore(t, version, b)
		c, _, err := build.Check(b, s)
		if err != nil {
			t.Fatal(err)
		}
		var v config.Values
		must(t, config.Set(&v, poe1.Bandits, poe1.BanditsKillAll))
		cfg, _ := config.Resolve(poe1.ConfigSpecs(), config.Layer{Source: config.FromBuild, Values: v})
		ws, err := poe1.PointBudget(c, cfg)
		if err != nil || len(ws) != 0 {
			t.Errorf("%s: %v, %v", version, ws, err)
		}
		p, a, g := c.Points()
		t.Logf("%s: %d passive, %d ascendancy, %d granted", version, p, a, g)
	}
}
