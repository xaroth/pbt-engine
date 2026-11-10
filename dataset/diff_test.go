package dataset_test

import (
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/xaroth/pbt-engine/dataset"
)

// compare writes a and b and compares them.
func compare(t *testing.T, a, b *dataset.Dataset) dataset.Diff {
	t.Helper()
	x, err := dataset.Open(written(t, a))
	if err != nil {
		t.Fatal(err)
	}
	y, err := dataset.Open(written(t, b))
	if err != nil {
		t.Fatal(err)
	}
	d, err := dataset.Compare(x, y)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestCompareSame(t *testing.T) {
	t.Parallel()
	d := compare(t, sample(), sample())
	want := dataset.Diff{
		Trees: []dataset.TreeDiff{
			{Variant: "ruthless"},
			{Variant: "standard", NodesCompared: 2},
		},
	}
	if diff := cmp.Diff(want, d, cmpopts.EquateEmpty()); diff != "" {
		t.Errorf("Compare mismatch (-want +got):\n%s", diff)
	}
}

func TestCompare(t *testing.T) {
	t.Parallel()
	a, b := sample(), sample()
	// b moves stat handles around and changes content; ids stay the key.
	b.Stats = []dataset.Stat{{ID: "base_maximum_all_resistances_%"}, {ID: "life_per_level"}, {ID: "new_stat"}}
	b.Characters[0].Stats = []dataset.StatValue{{Stat: 0, Value: -75}, {Stat: 1, Value: 12}}
	b.Characters = b.Characters[:1]
	b.Texts = nil // names would refer to the removed node and character
	b.Constants = []dataset.Constant{{ID: "life_per_strength", Value: 1, Per: 3}, {ID: "new", Value: 1, Per: 1}}
	b.Trees = b.Trees[:1]
	b.Trees = append(b.Trees, dataset.Tree{Variant: "alternate"})
	nodes := b.Trees[0].Nodes
	nodes[0].Neighbours = nil
	nodes[1].Neighbours = nil
	nodes[1].Stats = []dataset.StatValue{{Stat: 1, Value: 15}}
	nodes[1].Buffs = nil
	nodes[1].MasteryEffects = []dataset.MasteryEffect{{ID: 8}}
	nodes[1].SkillPoints = 0
	nodes[1].Kind = dataset.Keystone
	nodes[1].Ascendancy = ""
	b.Trees[0].Nodes = []dataset.Node{nodes[1], {ID: 3}}
	b.Trees[0].Classes, b.Trees[0].JewelSlots = nil, nil

	got := compare(t, a, b)
	want := dataset.Diff{
		StatsAdded:          []string{"new_stat"},
		StatsRemoved:        []string{"level"},
		CharactersChanged:   []string{"Metadata/Characters/Dex/Dex", "Metadata/Characters/Str/Str"},
		ConstantsAdded:      []string{"new"},
		ConstantsChanged:    []string{"life_per_strength"},
		TreeVariantsAdded:   []string{"alternate"},
		TreeVariantsRemoved: []string{"ruthless"},
		Trees: []dataset.TreeDiff{{
			Variant:        "standard",
			Added:          []uint32{3},
			Removed:        []uint32{1},
			EffectsAdded:   []uint32{8},
			EffectsRemoved: []uint32{7},
			NodesCompared:  1,
			Changed: []dataset.NodeDiff{{
				ID:          2,
				Fields:      []string{"kind", "ascendancy", "stats", "buffs", "skill_points", "mastery_effects", "neighbours"},
				StatsBefore: map[string]int32{"life_per_level": 10},
				StatsAfter:  map[string]int32{"life_per_level": 15},
			}},
		}},
	}
	if diff := cmp.Diff(want, got, cmpopts.EquateEmpty()); diff != "" {
		t.Errorf("Compare mismatch (-want +got):\n%s", diff)
	}
}

func TestCompareTree(t *testing.T) {
	t.Parallel()
	a, b := sample(), sample()
	b.Trees[0].Nodes[1].SkillPoints = 0
	x, err := dataset.Open(written(t, a))
	if err != nil {
		t.Fatal(err)
	}
	y, err := dataset.Open(written(t, b))
	if err != nil {
		t.Fatal(err)
	}
	got, err := dataset.CompareTree(x, y, "standard")
	if err != nil {
		t.Fatal(err)
	}
	want := dataset.TreeDiff{Variant: "standard", NodesCompared: 2, Changed: []dataset.NodeDiff{{ID: 2, Fields: []string{"skill_points"}}}}
	if diff := cmp.Diff(want, got, cmpopts.EquateEmpty()); diff != "" {
		t.Errorf("CompareTree mismatch (-want +got):\n%s", diff)
	}
	if _, err := dataset.CompareTree(x, y, "alternate"); !errors.Is(err, dataset.ErrNotInDataset) {
		t.Errorf("missing variant: got %v, want ErrNotInDataset", err)
	}
}

func TestNodeKindString(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		kind dataset.NodeKind
		want string
	}{
		{dataset.Normal, "normal"}, {dataset.Notable, "notable"}, {dataset.Keystone, "keystone"},
		{dataset.Mastery, "mastery"}, {dataset.JewelSocket, "jewel socket"}, {dataset.ClassStart, "class start"},
		{dataset.AscendancyStart, "ascendancy start"}, {dataset.Proxy, "proxy"}, {dataset.NodeKind(99), "unknown"},
	}
	for _, testCase := range testCases {
		t.Run(testCase.want, func(t *testing.T) {
			t.Parallel()
			if got := testCase.kind.String(); got != testCase.want {
				t.Errorf("String(): got %q, want %q", got, testCase.want)
			}
		})
	}
}
