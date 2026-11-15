package build_test

import (
	"slices"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/internal/realdata"
)

// The stats of timelessStore, by handle.
const (
	tlLife = iota
	tlStrength
	tlEternalKeystone
	tlEternalKeystoneV2
	tlKaruiKeystone
	tlKaruiStrength
)

// timelessStore is a Marauder tree on a line, with each node's x: start 1 (0), small strength
// node 2 (1000), socket 3 (2000), notable 4 (3000), keystone 5 (3500), small strength node 6
// (4000), each joined to the next. A Large radius reaches 1800 layout units, so 6 is out of the
// socket's. Its alternate tree versions are None, Karui and Eternal: the Eternal one blanks
// small passives, replaces notables by +10 life and has a keystone for conqueror 1 and its
// second revision; the Karui one adds strength to small passives and notables and has a
// keystone for conqueror 1. Every roll is fixed, so no seed changes the outcome.
func timelessStore(t *testing.T) *dataset.Store {
	t.Helper()
	str := []dataset.StatValue{{Stat: tlStrength, Value: 10}}
	xs := []float64{0, 1000, 2000, 3000, 3500, 4000}
	layout := make([]dataset.LayoutNode, 0, len(xs))
	groups := make([]dataset.LayoutGroup, 0, len(xs))
	for i, x := range xs {
		id := uint32(i + 1)
		groups = append(groups, dataset.LayoutGroup{ID: id, X: x})
		layout = append(layout, dataset.LayoutNode{ID: id, Group: id, Icon: "n.png"})
	}
	return openStore(t, &dataset.Dataset{
		Manifest: dataset.Manifest{Game: "poe1", Version: "3.29.3.3"},
		Stats: []dataset.Stat{
			{ID: "base_maximum_life"}, {ID: "base_strength"}, {ID: "eternal_keystone"}, {ID: "eternal_keystone_v2"},
			{ID: "karui_keystone"}, {ID: "karui_strength"},
		},
		Characters: []dataset.Character{{ID: marauder}},
		Trees: []dataset.Tree{{
			Variant: "standard",
			Classes: []dataset.Class{{Character: marauder, StartNode: 1}},
			Nodes: []dataset.Node{
				{ID: 1, Kind: dataset.ClassStart, Neighbours: []uint32{2}},
				{ID: 2, Neighbours: []uint32{1, 3}, Stats: str},
				{ID: 3, Kind: dataset.JewelSocket, Neighbours: []uint32{2, 4}},
				{ID: 4, Kind: dataset.Notable, Neighbours: []uint32{3, 5}, Stats: []dataset.StatValue{{Stat: tlLife, Value: 5}}},
				{ID: 5, Kind: dataset.Keystone, Neighbours: []uint32{4, 6}},
				{ID: 6, Neighbours: []uint32{5}, Stats: str},
			},
			Overrides: []dataset.NodeOverride{
				{ID: "Valako4", Icon: "v.png", Target: dataset.SmallAttribute, Stats: []dataset.StatValue{{Stat: tlLife, Value: 20}}},
			},
			JewelRadii:       []dataset.JewelRadius{{ID: "Small", Radius: 800}, {ID: "Large", Radius: 1500}},
			TimelessVersions: timelessVersions(),
		}},
		Layouts: []dataset.Layout{{Variant: "standard", Orbits: []dataset.Orbit{{Slots: 1}}, Groups: groups, Nodes: layout}},
	})
}

// timelessVersions are timelessStore's alternate tree versions.
func timelessVersions() []dataset.TimelessVersion {
	keystone := func(stat uint32, c dataset.TimelessConqueror) dataset.TimelessReplacement {
		return dataset.TimelessReplacement{
			ID: "keystone", Kinds: []dataset.TimelessKind{dataset.TimelessKeystone},
			Stats: []dataset.StatRange{{Stat: stat, Min: 1, Max: 1}}, Weight: 100, Conqueror: c,
		}
	}
	small := []dataset.TimelessKind{dataset.TimelessSmallAttribute, dataset.TimelessSmallNormal}
	return []dataset.TimelessVersion{
		{ID: "None"},
		{
			ID: "Karui", SmallAttributeAdditions: dataset.ValueRange{Min: 1, Max: 1}, SmallNormalAdditions: dataset.ValueRange{Min: 1, Max: 1},
			NotableAdditions: dataset.ValueRange{Min: 1, Max: 1},
			Replacements:     []dataset.TimelessReplacement{keystone(tlKaruiKeystone, dataset.TimelessConqueror{Index: 1})},
			Additions: []dataset.TimelessAddition{{
				ID: "strength", Kinds: append(slices.Clone(small), dataset.TimelessNotable),
				Stats: []dataset.StatRange{{Stat: tlKaruiStrength, Min: 2, Max: 2}}, Weight: 100,
			}},
		},
		{
			ID: "Eternal", SmallAttributeReplaced: true, SmallNormalReplaced: true, NotableReplacementChance: 100,
			Replacements: []dataset.TimelessReplacement{
				{ID: "blank", Kinds: small, Weight: 100},
				{
					ID: "life", Kinds: []dataset.TimelessKind{dataset.TimelessNotable},
					Stats: []dataset.StatRange{{Stat: tlLife, Min: 10, Max: 10}}, Weight: 100,
				},
				keystone(tlEternalKeystone, dataset.TimelessConqueror{Index: 1}),
				keystone(tlEternalKeystoneV2, dataset.TimelessConqueror{Index: 1, Version: 1}),
			},
		},
	}
}

// timelessJewel is a jewel of the alternate tree version at index version, of conqueror 1 at
// revision.
func timelessJewel(version, revision int32) build.Item {
	stats := []build.StatValue{
		{Stat: "local_unique_jewel_alternate_tree_version", Value: version},
		{Stat: "local_unique_jewel_alternate_tree_seed", Value: 100},
		{Stat: "local_unique_jewel_alternate_tree_keystone", Value: 1},
	}
	if revision > 0 {
		stats = append(stats, build.StatValue{Stat: "local_unique_jewel_alternate_tree_internal_revision", Value: revision})
	}
	return build.Item{
		Rarity: build.Unique, Base: "Metadata/Items/Jewels/JewelTimeless",
		Mods: []build.ModLine{{Kind: build.Explicit, Text: "conquered", Stats: stats}},
	}
}

func TestCheckConquests(t *testing.T) {
	t.Parallel()
	s := timelessStore(t)
	const karui, eternal, abyss = 1, 2, 7
	replaced := func(node uint32, stats ...dataset.StatValue) build.Conquest {
		return build.Conquest{Node: &dataset.Node{ID: node}, Socket: 3, Replace: true, Stats: stats}
	}
	testCases := []struct {
		name  string
		nodes []uint32
		jewel build.Item
		want  []build.Conquest
		warns []build.Warning
	}{
		{
			name:  "Elegant Hubris",
			nodes: []uint32{1, 2, 3},
			jewel: timelessJewel(eternal, 0),
			want: []build.Conquest{
				replaced(2), replaced(4, dataset.StatValue{Stat: tlLife, Value: 10}), replaced(5, dataset.StatValue{Stat: tlEternalKeystone, Value: 1}),
			},
		},
		{
			name:  "second revision of the conqueror",
			nodes: []uint32{1, 2, 3},
			jewel: timelessJewel(eternal, 1),
			want: []build.Conquest{
				replaced(2), replaced(4, dataset.StatValue{Stat: tlLife, Value: 10}), replaced(5, dataset.StatValue{Stat: tlEternalKeystoneV2, Value: 1}),
			},
		},
		{
			name:  "keystone only, revision without its own",
			nodes: []uint32{1, 2, 3},
			jewel: timelessJewel(karui, 1),
			want:  []build.Conquest{replaced(5, dataset.StatValue{Stat: tlKaruiKeystone, Value: 1})},
			warns: []build.Warning{{Code: build.TimelessPartial, Subject: "jewel 3", Message: "Karui conquest of passives other than keystones not modelled; left out"}},
		},
		{name: "socket not allocated", nodes: []uint32{1, 2}, jewel: timelessJewel(eternal, 0)},
		{name: "version not in the tree", nodes: []uint32{1, 2, 3}, jewel: timelessJewel(abyss, 0)},
	}
	byID := cmp.Comparer(func(a, b *dataset.Node) bool { return a.ID == b.ID })
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			b := &build.Build{
				Format: build.Format, Game: "poe1", Variant: "standard", Class: marauder, Level: 90,
				Nodes: testCase.nodes, Jewels: build.Jewels{3: testCase.jewel},
			}
			c, warns := check(t, b, s, build.TimelessPartial)
			if diff := cmp.Diff(testCase.want, c.Conquests(), cmpopts.EquateEmpty(), byID); diff != "" {
				t.Errorf("Conquests (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(testCase.warns, warns, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("warnings (-want +got):\n%s", diff)
			}
		})
	}
}

// TestCheckConquestsInGame checks a jewel as the game client showed it on 3.30: Elegant Hubris,
// "Commissioned 67340 coins to commemorate Caspiro", in socket 26196.
func TestCheckConquestsInGame(t *testing.T) {
	t.Parallel()
	s := realdata.Open(t, realdata.InGame)
	tree, err := s.Tree(build.DefaultVariant)
	if err != nil {
		t.Fatal(err)
	}
	names, err := s.Names("en")
	if err != nil {
		t.Fatal(err)
	}
	b := &build.Build{Format: build.Format, Game: "poe1", Variant: build.DefaultVariant, Class: "Metadata/Characters/Int/Int", Level: 90}
	c, _, err := build.Check(b, s)
	if err != nil {
		t.Fatal(err)
	}
	toSocket, ok := c.AllocatePath(26196)
	if !ok {
		t.Fatal("no path to socket 26196")
	}
	// The 3.30 stat descriptions name conquerors by a handler the item reader lacks, so the
	// jewel is given as its stats; the seed stat is the shown seed over 20.
	jewel := timelessJewel(5, 1)
	jewel.Mods[0].Stats[1].Value = 67340 / 20
	jewel.Mods[0].Stats[2].Value = 3
	if err := b.Apply(toSocket, build.SocketJewel(26196, jewel)); err != nil {
		t.Fatal(err)
	}
	c, _, err = build.Check(b, s)
	if err != nil {
		t.Fatal(err)
	}
	// Replacements are found by their display name: some row ids do not match their stats.
	eternal := &tree.TimelessVersions[5]
	became := func(name string, value int32) []dataset.StatValue {
		for _, n := range names.TimelessNames {
			i := slices.IndexFunc(eternal.Replacements, func(r dataset.TimelessReplacement) bool { return r.ID == n.ID })
			if n.Name == name && i >= 0 {
				return []dataset.StatValue{{Stat: eternal.Replacements[i].Stats[0].Stat, Value: value}}
			}
		}
		t.Fatalf("no Eternal replacement named %s", name)
		return nil
	}
	want := map[uint32][]dataset.StatValue{
		34173: became("Slum Lord", 80),         // Overcharge
		33435: became("Slum Lord", 80),         // Holy Dominion
		54694: became("Eternal Resilience", 1), // Light of Divinity
		30693: became("Robust Diet", 10),       // Divine Fervour
	}
	got := map[uint32][]dataset.StatValue{}
	for _, q := range c.Conquests() {
		if _, ok := want[q.Node.ID]; ok && q.Replace {
			got[q.Node.ID] = q.Stats
		}
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("conquered notables (-want +got):\n%s", diff)
	}
}
