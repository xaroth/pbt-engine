package build_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/dataset"
)

// overrideStore is a Marauder tree: start 1, small strength node 2 joined to 1, 3 and 5,
// notable 3 (+30 strength) with mastery 4, keystone 5, and small dexterity node 6 joining
// nothing. Its overrides: Valako4 on a small attribute node, Picky on one with four or more
// neighbours, runegraft_8 on a mastery, Keystone1 on any keystone but 5, Strong on a strength
// notable.
func overrideStore(t *testing.T) *dataset.Store {
	t.Helper()
	const life, str, dex = 0, 1, 2
	return openStore(t, &dataset.Dataset{
		Manifest:   dataset.Manifest{Game: "poe1", Version: "3.29.3.3"},
		Stats:      []dataset.Stat{{ID: "base_maximum_life"}, {ID: "base_strength"}, {ID: "base_dexterity"}},
		Characters: []dataset.Character{{ID: marauder}},
		Trees: []dataset.Tree{{
			Variant: "standard",
			Classes: []dataset.Class{{Character: marauder, StartNode: 1}},
			Nodes: []dataset.Node{
				{ID: 1, Kind: dataset.ClassStart, Neighbours: []uint32{2}},
				{ID: 2, Neighbours: []uint32{1, 3, 5}, Stats: []dataset.StatValue{{Stat: str, Value: 10}}},
				{ID: 3, Kind: dataset.Notable, Neighbours: []uint32{2, 4}, Stats: []dataset.StatValue{{Stat: life, Value: 5}, {Stat: str, Value: 30}}},
				{ID: 4, Kind: dataset.Mastery, Neighbours: []uint32{3}, MasteryEffects: []dataset.MasteryEffect{{ID: 7}}},
				{ID: 5, Kind: dataset.Keystone, Neighbours: []uint32{2}},
				{ID: 6, Stats: []dataset.StatValue{{Stat: dex, Value: 10}}},
			},
			Overrides: []dataset.NodeOverride{
				{ID: "Keystone1", Icon: "k.png", Target: dataset.KeystoneTarget, ExcludedNodes: []uint32{5}},
				{ID: "Picky", Icon: "p.png", Target: dataset.SmallAttribute, MinNeighbours: 4},
				{ID: "Strong", Icon: "s.png", Target: dataset.StrengthNotable},
				{ID: "Valako4", Icon: "v.png", Target: dataset.SmallAttribute, Stats: []dataset.StatValue{{Stat: life, Value: 20}}},
				{ID: "runegraft_8", Kind: dataset.Runegraft, Icon: "r.png", Target: dataset.MasteryTarget},
			},
		}},
	})
}

func TestCheckOverrides(t *testing.T) {
	t.Parallel()
	s := overrideStore(t)
	testCases := []struct {
		name      string
		overrides map[uint32]string
		masteries map[uint32]uint32
		kept      map[uint32]string
		want      []build.Warning
	}{
		{
			name:      "a tattoo and a runegraft",
			overrides: map[uint32]string{2: "Valako4", 4: "runegraft_8", 3: "Strong"},
			kept:      map[uint32]string{2: "Valako4", 4: "runegraft_8", 3: "Strong"},
		},
		{
			name:      "unknown",
			overrides: map[uint32]string{2: "Nope"},
			want:      []build.Warning{{Code: build.OverrideUnknown, Subject: "node 2", Message: `override "Nope" is not in the standard tree of 3.29.3.3; dropped`}},
		},
		{
			name:      "on a node not allocated",
			overrides: map[uint32]string{6: "Valako4"},
			want:      []build.Warning{{Code: build.OverrideNotAllocated, Subject: "node 6", Message: "override Valako4 on a node that is not allocated; dropped"}},
		},
		{
			name:      "on another kind of node",
			overrides: map[uint32]string{2: "runegraft_8", 5: "Keystone1", 3: "Valako4"},
			want: []build.Warning{
				{Code: build.OverrideNotPlaceable, Subject: "node 2", Message: "override runegraft_8 replaces only a mastery node; dropped"},
				{Code: build.OverrideNotPlaceable, Subject: "node 3", Message: "override Valako4 replaces only a small attribute node; dropped"},
				{Code: build.OverrideNotPlaceable, Subject: "node 5", Message: "override Keystone1 may not replace this node; dropped"},
			},
		},
		{
			name:      "too few neighbours",
			overrides: map[uint32]string{2: "Picky"},
			want: []build.Warning{
				{Code: build.OverrideNotPlaceable, Subject: "node 2", Message: "override Picky needs at least 4 neighbours, the node has 3; dropped"},
			},
		},
		{
			name:      "a mastery choice under a runegraft",
			overrides: map[uint32]string{4: "runegraft_8"},
			masteries: map[uint32]uint32{4: 7},
			kept:      map[uint32]string{4: "runegraft_8"},
			want:      []build.Warning{{Code: build.MasteryOverridden, Subject: "node 4", Message: "effect 7 chosen on a mastery an override replaces; dropped"}},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			b := &build.Build{
				Format: build.Format, Game: "poe1", Version: "3.29", Class: marauder, Level: 90,
				Nodes: []uint32{1, 2, 3, 5}, Overrides: testCase.overrides, Masteries: testCase.masteries,
			}
			// Mastery 4 is allocated only where a case puts something on it.
			if _, ok := testCase.overrides[4]; ok {
				b.Nodes = append(b.Nodes, 4)
			}
			c, warnings, err := build.Check(b, s)
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(testCase.want, warnings); diff != "" {
				t.Errorf("warnings (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(testCase.kept, c.Build().Overrides); diff != "" {
				t.Errorf("overrides kept (-want +got):\n%s", diff)
			}
			if got := c.EditWarnings([]build.Edit{build.SetOverride(6, "Valako4")}); len(got) != 1 || got[0].Code != build.OverrideNotAllocated {
				t.Errorf("EditWarnings of an override on node 6 = %v", got)
			}
		})
	}
}
