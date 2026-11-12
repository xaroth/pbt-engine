package simulator_test

import (
	"errors"
	"fmt"
	"testing"

	gocmp "github.com/google/go-cmp/cmp"

	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/internal/realdata"
	"github.com/xaroth/pbt-engine/simulator"
)

func TestDataNodeNotInDataset(t *testing.T) {
	t.Parallel()
	s := newSim(t)
	testCases := []struct {
		name    string
		variant string
		id      uint32
	}{
		{"unknown variant", "ruthless", 2},
		{"unknown node", "standard", 99},
		{"no English text", "standard", 2},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			if _, err := s.Data().Node(testCase.variant, testCase.id); !errors.Is(err, dataset.ErrNotInDataset) {
				t.Errorf("got %v, want dataset.ErrNotInDataset", err)
			}
		})
	}
	if _, err := s.Data().Classes("ruthless"); !errors.Is(err, dataset.ErrNotInDataset) {
		t.Errorf("classes of an unknown variant: got %v, want dataset.ErrNotInDataset", err)
	}
}

func realSim(tb testing.TB) *simulator.Sim {
	tb.Helper()
	s, err := simulator.New(realdata.Open(tb, realdata.Current))
	if err != nil {
		tb.Fatal(err)
	}
	return s
}

// TestDataNodeReal compares nodes of the 3.29 standard tree with the stat lines of GGG's tree
// export for 3.29 (github.com/grindinggear/skilltree-export, 8bd138b, data.json), with its
// [id|text] markup shown as text. The export's reminder text is not in the dataset, and its
// skill point line, such as "Grants 2 Passive Skill Points", has no stat description.
func TestDataNodeReal(t *testing.T) {
	t.Parallel()
	s := realSim(t)
	abyssal := dataset.Name{ID: "Abyssal", Name: "Abyssal Bloodline"}
	testCases := []struct {
		name string
		want simulator.Node
	}{
		{"small passive", simulator.Node{ID: 60472, Name: "Strength", Stats: []string{"+10 to Strength"}}},
		{"keystone", simulator.Node{
			ID: 31961, Name: "Resolute Technique", Kind: dataset.Keystone,
			Stats: []string{"Your hits can't be Evaded\nNever deal Critical Strikes"},
		}},
		{"mastery", simulator.Node{
			ID: 44298, Name: "Elemental Mastery", Kind: dataset.Mastery,
			Effects: []simulator.MasteryEffect{
				{ID: 48385, Stats: []string{"Exposure you inflict applies at least -18% to the affected Resistance"}},
				{ID: 4119, Stats: []string{"Prevent +60% of Reflected Elemental Damage"}},
				{ID: 59343, Stats: []string{"50% reduced Effect of Exposure on you"}},
				{ID: 17942, Stats: []string{"Hits have 15% chance to treat Enemy Monster Elemental Resistance values as inverted"}},
				{ID: 30502, Stats: []string{"Critical Strikes against you do not inherently inflict Elemental Ailments"}},
				{ID: 25681, Stats: []string{"3% chance for Hits to deal 300% of Physical Damage as Extra Damage of a random Element"}},
			},
		}},
		{"ascendancy notable", simulator.Node{
			ID: 17988, Name: "Untiring", Kind: dataset.Notable, Ascendancy: dataset.Name{ID: "Juggernaut", Name: "Juggernaut"},
			Stats: []string{
				"40% increased Life Regeneration rate",
				"1.5% of Physical Damage prevented from Hits in the past\n10 seconds is Regenerated as Life per second",
			},
		}},
		{"two-line description and decimals", simulator.Node{
			ID: 9271, Name: "Defy Pain", Kind: dataset.Notable, Ascendancy: dataset.Name{ID: "Berserker", Name: "Berserker"},
			Stats: []string{
				"Gain Defiance for 10 seconds on losing Life to an Enemy Hit, no\nmore than once every 0.3 seconds",
				"Lose all Defiance when you reach 10 Defiance",
				"Gain 3% of Missing Unreserved Life before being Hit by an Enemy Per Defiance",
			},
		}},
		{"reminder text left out", simulator.Node{
			ID: 64963, Name: "Searching Gaze of the Lich", Ascendancy: abyssal,
			Stats: []string{
				"While you have at least 3 Searching Eye Jewels socketed:",
				"Targets affected by Maim you inflict cannot deal Critical Strikes",
				"Maim you inflict causes Hits against the target to have 20% more Critical Strike Chance",
			},
		}},
		{"value divided for display", simulator.Node{
			ID: 8592, Name: "Physical Damage, Melee Strike Range", Ascendancy: dataset.Name{ID: "Berserker", Name: "Berserker"},
			Stats: []string{"+0.2 metres to Melee Strike Range", "10% increased Physical Damage"},
		}},
		{"negative value", simulator.Node{
			ID: 58999, Name: "Curse Effect on Self", Ascendancy: abyssal,
			Stats: []string{"25% reduced Effect of Curses on you"},
		}},
		{"aura", simulator.Node{
			ID: 62634, Name: "Replenishing Presence", Kind: dataset.Notable,
			Stats: []string{"Non-Curse Aura Skills have 20% increased Duration", "You and nearby Allies Regenerate 1.00% of Life per second"},
		}},
		{"skill points", simulator.Node{
			ID: 54877, Name: "Path of the Templar", Kind: dataset.Notable,
			Ascendancy: dataset.Name{ID: "Ascendant", Name: "Ascendant"}, SkillPoints: 2,
			Stats: []string{"Can Allocate Passives from the Templar's starting point"},
		}},
		{"markup", simulator.Node{
			ID: 16287, Name: "Bone Feeders", Kind: dataset.Notable, Ascendancy: abyssal,
			Stats: []string{"Triggers Level 20 Summon Bone Feeders when Allocated"},
		}},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			got, err := s.Data().Node("standard", testCase.want.ID)
			if err != nil {
				t.Fatal(err)
			}
			if diff := gocmp.Diff(testCase.want, got); diff != "" {
				t.Errorf("node %d (-want +got):\n%s", testCase.want.ID, diff)
			}
		})
	}
}

// TestDataNodeAllReal renders every node of every tree of the 3.29 dataset. Stats without
// text, such as those of cluster jewel nodes, render no line, as in the export.
func TestDataNodeAllReal(t *testing.T) {
	t.Parallel()
	s := realSim(t)
	for _, variant := range []string{"standard", "ruthless", "alternate"} {
		tree, err := s.Store().Tree(variant)
		if err != nil {
			t.Fatal(err)
		}
		for _, n := range tree.Nodes {
			got, err := s.Data().Node(variant, n.ID)
			if err != nil {
				t.Errorf("%s node %d: %v", variant, n.ID, err)
				continue
			}
			if got.Name == "" && n.Kind != dataset.ClassStart {
				t.Errorf("%s node %d: no name", variant, n.ID)
			}
			if n.Ascendancy != "" && got.Ascendancy.Name == "" {
				t.Errorf("%s node %d: ascendancy %s without a name", variant, n.ID, n.Ascendancy)
			}
		}
	}
}

func TestDataClassesReal(t *testing.T) {
	t.Parallel()
	classes, err := realSim(t).Data().Classes("standard")
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, 0, len(classes))
	for _, c := range classes {
		got = append(got, fmt.Sprintf("%s %s %v", c.ID, c.Name, c.Ascendancies))
	}
	want := []string{
		"Metadata/Characters/StrDexInt/StrDexInt Scion [{Ascendant Ascendant} {Reliquarian Reliquarian} {Luminary Luminary}]",
		"Metadata/Characters/Str/Str Marauder [{Juggernaut Juggernaut} {Berserker Berserker} {Chieftain Chieftain}]",
		"Metadata/Characters/Dex/Dex Ranger [{Raider Warden} {Deadeye Deadeye} {Pathfinder Pathfinder}]",
		"Metadata/Characters/Int/Int Witch [{Occultist Occultist} {Elementalist Elementalist} {Necromancer Necromancer}]",
		"Metadata/Characters/StrDex/StrDex Duelist [{Slayer Slayer} {Gladiator Gladiator} {Champion Champion}]",
		"Metadata/Characters/StrInt/StrInt Templar [{Inquisitor Inquisitor} {Hierophant Hierophant} {Guardian Guardian}]",
		"Metadata/Characters/DexInt/DexInt Shadow [{Assassin Assassin} {Trickster Trickster} {Saboteur Saboteur}]",
	}
	if diff := gocmp.Diff(want, got); diff != "" {
		t.Errorf("classes (-want +got):\n%s", diff)
	}
}

func TestNodeFilesReal(t *testing.T) {
	t.Parallel()
	for _, variant := range []string{"standard", "alternate"} {
		t.Run(variant, func(t *testing.T) {
			t.Parallel()
			rec, _, sim := recorded(t)
			if _, err := sim.Data().Node(variant, 62634); err != nil {
				t.Fatal(err)
			}
			if _, err := sim.Data().Classes(variant); err != nil {
				t.Fatal(err)
			}
			if diff := gocmp.Diff(sorted(simulator.NodeFiles(sim.Store().Manifest(), variant)), rec.Opened()); diff != "" {
				t.Errorf("node lookup (-NodeFiles +opened):\n%s", diff)
			}
		})
	}
}
