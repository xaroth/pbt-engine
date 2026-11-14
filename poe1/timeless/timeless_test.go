package timeless_test

import (
	"slices"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/internal/realdata"
	"github.com/xaroth/pbt-engine/poe1/timeless"
	"github.com/xaroth/pbt-engine/simulator"
)

// TestConquerInGame checks a jewel as the game client showed it on 3.30: Elegant Hubris,
// "Commissioned 67340 coins to commemorate Caspiro", in the Large radius of socket 26196.
func TestConquerInGame(t *testing.T) {
	t.Parallel()
	store := realdata.Open(t, realdata.InGame)
	sim, err := simulator.New(store)
	if err != nil {
		t.Fatal(err)
	}
	inRadius, err := sim.NodesInRadius(build.DefaultVariant, 26196, simulator.Radius{ID: "Large"})
	if err != nil {
		t.Fatal(err)
	}
	tree, err := store.Tree(build.DefaultVariant)
	if err != nil {
		t.Fatal(err)
	}
	names, err := store.Names("en")
	if err != nil {
		t.Fatal(err)
	}
	i := slices.IndexFunc(tree.TimelessVersions, func(v dataset.TimelessVersion) bool { return v.ID == "Eternal" })
	if i < 0 {
		t.Fatal("no Eternal version in the dataset")
	}
	version := &tree.TimelessVersions[i]
	conquer, err := timeless.NewTree(tables(version))
	if err != nil {
		t.Fatal(err)
	}
	// Replacements are named by their display name: some row ids do not match their stats.
	nameOf := map[string]string{}
	for _, n := range names.TimelessNames {
		nameOf[n.ID] = n.Name
	}
	// Caspiro is Chitus's second version.
	jewel := timeless.Jewel{Seed: 67340, Conqueror: timeless.Conqueror{Index: 3, Version: 1}}

	type became struct {
		Name  string
		Rolls [4]int32
	}
	testCases := []struct {
		name string
		node uint32
		want became
	}{
		{name: "Overcharge", node: 34173, want: became{Name: "Slum Lord", Rolls: [4]int32{80}}},
		{name: "Holy Dominion", node: 33435, want: became{Name: "Slum Lord", Rolls: [4]int32{80}}},
		{name: "Light of Divinity", node: 54694, want: became{Name: "Eternal Resilience", Rolls: [4]int32{1}}},
		{name: "Divine Fervour", node: 30693, want: became{Name: "Robust Diet", Rolls: [4]int32{10}}},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			if !slices.Contains(inRadius, testCase.node) {
				t.Fatalf("node %d is not in the Large radius of socket 26196", testCase.node)
			}
			res := conquer.Conquer(jewel, timeless.Node{ID: testCase.node, Kind: timeless.Notable})
			if !res.Replaced {
				t.Fatalf("node %d not replaced", testCase.node)
			}
			got := became{Name: nameOf[version.Replacements[res.Replacement].ID], Rolls: res.Rolls}
			if diff := cmp.Diff(testCase.want, got); diff != "" {
				t.Errorf("node %d (-want +got):\n%s", testCase.node, diff)
			}
		})
	}
}

func tables(v *dataset.TimelessVersion) timeless.Tables {
	t := timeless.Tables{Version: timeless.Version{
		ID:                       v.ID,
		SmallAttributeReplaced:   v.SmallAttributeReplaced,
		SmallNormalReplaced:      v.SmallNormalReplaced,
		SmallAttributeAdditions:  timeless.Range(v.SmallAttributeAdditions),
		SmallNormalAdditions:     timeless.Range(v.SmallNormalAdditions),
		NotableAdditions:         timeless.Range(v.NotableAdditions),
		NotableReplacementChance: v.NotableReplacementChance,
	}}
	for _, r := range v.Replacements {
		t.Replacements = append(t.Replacements, timeless.Replacement{
			Kinds: kinds(r.Kinds), Stats: ranges(r.Stats), Weight: r.Weight,
			Additions: timeless.Range(r.Additions), Conqueror: timeless.Conqueror(r.Conqueror),
		})
	}
	for _, a := range v.Additions {
		t.Additions = append(t.Additions, timeless.Addition{Kinds: kinds(a.Kinds), Stats: ranges(a.Stats), Weight: a.Weight})
	}
	return t
}

func kinds(ks []dataset.TimelessKind) []timeless.Kind {
	out := make([]timeless.Kind, 0, len(ks))
	for _, k := range ks {
		out = append(out, timeless.Kind(k))
	}
	return out
}

func ranges(rs []dataset.StatRange) []timeless.Range {
	out := make([]timeless.Range, 0, len(rs))
	for _, r := range rs {
		out = append(out, timeless.Range{Min: r.Min, Max: r.Max})
	}
	return out
}
