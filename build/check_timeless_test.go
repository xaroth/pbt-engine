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
	tlDevotion
	tlTemplarNotable
	tlTemplarKeystone
	tlVaalDamage
	tlVaalNotable
	tlVaalKeystone
)

// timelessStore is a Marauder tree on a line, with each node's x: start 1 (0), small strength
// node 2 (1000), socket 3 (2000), small life node 7 (2500), notable 4 (3000), keystone 5
// (3500), small strength node 6 (4000), each joined to the next. A Large radius reaches 1500
// layout units, so 6 is out of the socket's. Its alternate tree versions mirror the game's in
// shape, with a keystone each for conqueror 1:
//   - None changes nothing.
//   - Karui, as Lethal Pride and Brutal Restraint: +2 strength to small attribute passives, +4
//     to other small passives and +20 to notables.
//   - Eternal, as Elegant Hubris: blanks small passives, replaces notables by +10 life, and has
//     a keystone for the conqueror's second revision too.
//   - Templar, as Militant Faith: replaces small attribute passives by +10 devotion, adds +5
//     devotion to other small passives, and replaces a notable 1 time in 5, else adds +5
//     devotion to it.
//   - Vaal, as Glorious Vanity: replaces small passives by 7 to 12 damage and notables by 20 to
//     30 life, rolled.
func timelessStore(t *testing.T) *dataset.Store {
	t.Helper()
	str := []dataset.StatValue{{Stat: tlStrength, Value: 10}}
	xs := []float64{0, 1000, 2000, 3000, 3500, 4000, 2500}
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
			{ID: "karui_keystone"}, {ID: "karui_strength"}, {ID: "base_devotion"}, {ID: "templar_notable"},
			{ID: "templar_keystone"}, {ID: "vaal_damage"}, {ID: "vaal_notable"}, {ID: "vaal_keystone"},
		},
		Characters: []dataset.Character{{ID: marauder}},
		Trees: []dataset.Tree{{
			Variant: "standard",
			Classes: []dataset.Class{{Character: marauder, StartNode: 1}},
			Nodes: []dataset.Node{
				{ID: 1, Kind: dataset.ClassStart, Neighbours: []uint32{2}},
				{ID: 2, Neighbours: []uint32{1, 3}, Stats: str},
				{ID: 3, Kind: dataset.JewelSocket, Neighbours: []uint32{2, 7}},
				{ID: 4, Kind: dataset.Notable, Neighbours: []uint32{5, 7}, Stats: []dataset.StatValue{{Stat: tlLife, Value: 5}}},
				{ID: 5, Kind: dataset.Keystone, Neighbours: []uint32{4, 6}},
				{ID: 6, Neighbours: []uint32{5}, Stats: str},
				{ID: 7, Neighbours: []uint32{3, 4}, Stats: []dataset.StatValue{{Stat: tlLife, Value: 5}}},
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

// The alternate tree versions of timelessStore, by index.
const (
	tlKarui = iota + 1
	tlEternal
	tlTemplar
	tlVaal
)

// timelessVersions are timelessStore's alternate tree versions.
func timelessVersions() []dataset.TimelessVersion {
	keystone := func(stat uint32, c dataset.TimelessConqueror) dataset.TimelessReplacement {
		return dataset.TimelessReplacement{
			ID: "keystone", Kinds: []dataset.TimelessKind{dataset.TimelessKeystone},
			Stats: []dataset.StatRange{{Stat: stat, Min: 1, Max: 1}}, Weight: 100, Conqueror: c,
		}
	}
	fixed := func(stat uint32, v int32) []dataset.StatRange {
		return []dataset.StatRange{{Stat: stat, Min: v, Max: v}}
	}
	kinds := func(ks ...dataset.TimelessKind) []dataset.TimelessKind { return ks }
	one := dataset.ValueRange{Min: 1, Max: 1}
	attribute, normal, notable := dataset.TimelessSmallAttribute, dataset.TimelessSmallNormal, dataset.TimelessNotable
	return []dataset.TimelessVersion{
		{ID: "None"},
		{
			ID: "Karui", SmallAttributeAdditions: one, SmallNormalAdditions: one, NotableAdditions: one,
			Replacements: []dataset.TimelessReplacement{keystone(tlKaruiKeystone, dataset.TimelessConqueror{Index: 1})},
			Additions: []dataset.TimelessAddition{
				{ID: "attribute", Kinds: kinds(attribute), Stats: fixed(tlKaruiStrength, 2), Weight: 100},
				{ID: "small", Kinds: kinds(normal), Stats: fixed(tlKaruiStrength, 4), Weight: 100},
				{ID: "notable", Kinds: kinds(notable), Stats: fixed(tlKaruiStrength, 20), Weight: 100},
			},
		},
		{
			ID: "Eternal", SmallAttributeReplaced: true, SmallNormalReplaced: true, NotableReplacementChance: 100,
			Replacements: []dataset.TimelessReplacement{
				{ID: "blank", Kinds: kinds(attribute, normal), Weight: 100},
				{ID: "life", Kinds: kinds(notable), Stats: fixed(tlLife, 10), Weight: 100},
				keystone(tlEternalKeystone, dataset.TimelessConqueror{Index: 1}),
				keystone(tlEternalKeystoneV2, dataset.TimelessConqueror{Index: 1, Version: 1}),
			},
		},
		{
			ID: "Templar", SmallAttributeReplaced: true, SmallNormalAdditions: one, NotableAdditions: one, NotableReplacementChance: 20,
			Replacements: []dataset.TimelessReplacement{
				{ID: "devotion", Kinds: kinds(attribute), Stats: fixed(tlDevotion, 10), Weight: 100},
				{ID: "notable", Kinds: kinds(notable), Stats: fixed(tlTemplarNotable, 1), Weight: 100},
				keystone(tlTemplarKeystone, dataset.TimelessConqueror{Index: 1}),
			},
			Additions: []dataset.TimelessAddition{{ID: "devotion", Kinds: kinds(normal, notable), Stats: fixed(tlDevotion, 5), Weight: 100}},
		},
		{
			ID: "Vaal", SmallAttributeReplaced: true, SmallNormalReplaced: true, NotableReplacementChance: 100,
			Replacements: []dataset.TimelessReplacement{
				{ID: "damage", Kinds: kinds(attribute, normal), Stats: []dataset.StatRange{{Stat: tlVaalDamage, Min: 7, Max: 12}}, Weight: 100},
				{ID: "notable", Kinds: kinds(notable), Stats: []dataset.StatRange{{Stat: tlVaalNotable, Min: 20, Max: 30}}, Weight: 100},
				keystone(tlVaalKeystone, dataset.TimelessConqueror{Index: 1}),
			},
		},
	}
}

// timelessJewel is a jewel of the alternate tree version at index version with seed 100, of
// conqueror 1 at revision.
func timelessJewel(version, revision int32) build.Item {
	return seededJewel(version, revision, 100)
}

func seededJewel(version, revision, seed int32) build.Item {
	stats := []build.StatValue{
		{Stat: "local_unique_jewel_alternate_tree_version", Value: version},
		{Stat: "local_unique_jewel_alternate_tree_seed", Value: seed},
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

// Each version changes the small passives, notables and keystones in the radius as its tables
// say: rolled where the version rolls, by seed where it chooses between replacing and adding
// (seed 100 adds to the Templar notable, 103 replaces it).
func TestCheckConquests(t *testing.T) {
	t.Parallel()
	s := timelessStore(t)
	const abyss = 7
	sv := func(stat uint32, v int32) dataset.StatValue { return dataset.StatValue{Stat: stat, Value: v} }
	replaced := func(node uint32, stats ...dataset.StatValue) build.Conquest {
		return build.Conquest{Node: &dataset.Node{ID: node}, Socket: 3, Replace: true, Stats: stats}
	}
	added := func(node uint32, stats ...dataset.StatValue) build.Conquest {
		return build.Conquest{Node: &dataset.Node{ID: node}, Socket: 3, Stats: stats}
	}
	testCases := []struct {
		name  string
		jewel build.Item
		nodes []uint32
		want  []build.Conquest
	}{
		{
			name:  "Elegant Hubris",
			jewel: timelessJewel(tlEternal, 0),
			want:  []build.Conquest{replaced(2), replaced(4, sv(tlLife, 10)), replaced(5, sv(tlEternalKeystone, 1)), replaced(7)},
		},
		{
			name:  "second revision of the conqueror",
			jewel: timelessJewel(tlEternal, 1),
			want:  []build.Conquest{replaced(2), replaced(4, sv(tlLife, 10)), replaced(5, sv(tlEternalKeystoneV2, 1)), replaced(7)},
		},
		{
			name:  "Lethal Pride, revision without a keystone of its own",
			jewel: timelessJewel(tlKarui, 1),
			want: []build.Conquest{
				added(2, sv(tlKaruiStrength, 2)), added(4, sv(tlKaruiStrength, 20)), replaced(5, sv(tlKaruiKeystone, 1)), added(7, sv(tlKaruiStrength, 4)),
			},
		},
		{
			name:  "Militant Faith adding to the notable",
			jewel: timelessJewel(tlTemplar, 0),
			want: []build.Conquest{
				replaced(2, sv(tlDevotion, 10)), added(4, sv(tlDevotion, 5)), replaced(5, sv(tlTemplarKeystone, 1)), added(7, sv(tlDevotion, 5)),
			},
		},
		{
			name:  "Militant Faith replacing the notable",
			jewel: seededJewel(tlTemplar, 0, 103),
			want: []build.Conquest{
				replaced(2, sv(tlDevotion, 10)), replaced(4, sv(tlTemplarNotable, 1)), replaced(5, sv(tlTemplarKeystone, 1)), added(7, sv(tlDevotion, 5)),
			},
		},
		{
			name:  "Glorious Vanity",
			jewel: timelessJewel(tlVaal, 0),
			want: []build.Conquest{
				replaced(2, sv(tlVaalDamage, 10)), replaced(4, sv(tlVaalNotable, 21)),
				replaced(5, sv(tlVaalKeystone, 1)), replaced(7, sv(tlVaalDamage, 11)),
			},
		},
		{name: "socket not allocated", nodes: []uint32{1, 2}, jewel: timelessJewel(tlEternal, 0)},
		{name: "version not in the tree", jewel: timelessJewel(abyss, 0)},
	}
	byID := cmp.Comparer(func(a, b *dataset.Node) bool { return a.ID == b.ID })
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			nodes := testCase.nodes
			if nodes == nil {
				nodes = []uint32{1, 2, 3}
			}
			b := &build.Build{
				Format: build.Format, Game: "poe1", Variant: "standard", Class: marauder, Level: 90,
				Nodes: nodes, Jewels: build.Jewels{3: testCase.jewel},
			}
			c, _ := check(t, b, s)
			if diff := cmp.Diff(testCase.want, c.Conquests(), cmpopts.EquateEmpty(), byID); diff != "" {
				t.Errorf("Conquests (-want +got):\n%s", diff)
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
