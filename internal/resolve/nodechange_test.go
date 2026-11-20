package resolve_test

import (
	"slices"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/internal/resolve"
)

func TestChangeNode(t *testing.T) {
	t.Parallel()
	node := func() *dataset.Node {
		return &dataset.Node{
			ID: 7, Kind: dataset.Notable, Neighbours: []uint32{1, 9},
			Stats:          []dataset.StatValue{{Stat: 3, Value: 20}, {Stat: 4, Value: 8}},
			Buffs:          [][]dataset.StatValue{{{Stat: 5, Value: 5}}},
			MasteryEffects: []dataset.MasteryEffect{{ID: 1}},
		}
	}
	replace := resolve.NodeChange{Replace: true, Stats: []dataset.StatValue{{Stat: 4, Value: 5}}}
	add := resolve.NodeChange{Stats: []dataset.StatValue{{Stat: 4, Value: 2}, {Stat: 6, Value: 1}}}
	replaced, added, both := node(), node(), node()
	replaced.Stats, replaced.Buffs, replaced.MasteryEffects = []dataset.StatValue{{Stat: 4, Value: 5}}, nil, nil
	added.Stats = []dataset.StatValue{{Stat: 3, Value: 20}, {Stat: 4, Value: 10}, {Stat: 6, Value: 1}}
	both.Stats, both.Buffs, both.MasteryEffects = []dataset.StatValue{{Stat: 4, Value: 7}, {Stat: 6, Value: 1}}, nil, nil
	testCases := []struct {
		name    string
		changes []resolve.NodeChange
		want    *dataset.Node
	}{
		{"none", nil, node()},
		{"replacement", []resolve.NodeChange{replace}, replaced},
		{"addition", []resolve.NodeChange{add}, added},
		{"replacement then addition", []resolve.NodeChange{replace, add}, both},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			n := node()
			got := resolve.ChangeNode(n, testCase.changes...)
			if diff := cmp.Diff(testCase.want, got); diff != "" {
				t.Errorf("(-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(node(), n); diff != "" {
				t.Errorf("node changed (-want +got):\n%s", diff)
			}
		})
	}
}

// conquestDataset is testDataset with jewel socket 3 off the start, small node 4 giving +10
// strength as attribute nodes do, every node within a Large radius of 3, the tattoo Valako4 (+20 maximum life) for small attribute nodes, and an
// Eternal alternate tree version at row 1 that blanks small passives and replaces notables by
// 10% increased maximum life.
func conquestDataset() *dataset.Dataset {
	d := testDataset()
	tree := &d.Trees[0]
	tree.Nodes[0].Neighbours = []uint32{3, 4, 7}
	tree.Nodes[1].Stats = []dataset.StatValue{{Stat: 6, Value: 10}}
	tree.Nodes = slices.Insert(tree.Nodes, 1, dataset.Node{ID: 3, Kind: dataset.JewelSocket, Neighbours: []uint32{1}})
	tree.Overrides = []dataset.NodeOverride{
		{ID: "Valako4", Icon: "v.png", Target: dataset.SmallAttribute, Stats: []dataset.StatValue{{Stat: 9, Value: 20}}},
	}
	tree.JewelRadii = []dataset.JewelRadius{{ID: "Large", Radius: 1500}}
	tree.TimelessVersions = []dataset.TimelessVersion{{ID: "None"}, {
		ID: "Eternal", SmallAttributeReplaced: true, SmallNormalReplaced: true, NotableReplacementChance: 100,
		Replacements: []dataset.TimelessReplacement{
			{ID: "blank", Kinds: []dataset.TimelessKind{dataset.TimelessSmallAttribute, dataset.TimelessSmallNormal}, Weight: 100},
			{ID: "life", Kinds: []dataset.TimelessKind{dataset.TimelessNotable}, Stats: []dataset.StatRange{{Stat: 4, Min: 10, Max: 10}}, Weight: 100},
		},
	}}
	l := dataset.Layout{Variant: "standard", Orbits: []dataset.Orbit{{Slots: 1}}}
	for i, n := range tree.Nodes {
		id := uint32(i + 1)
		l.Groups = append(l.Groups, dataset.LayoutGroup{ID: id, X: 400 * float64(i)})
		l.Nodes = append(l.Nodes, dataset.LayoutNode{ID: n.ID, Group: id, Icon: "n.png"})
	}
	d.Layouts = []dataset.Layout{l}
	return d
}

// A tattoo applies before the timeless jewel's conquest, so Elegant Hubris blanks a tattooed
// small passive; the conquered notable loses its buff.
func TestResolveConquest(t *testing.T) {
	t.Parallel()
	s := open(t, conquestDataset())
	jewel := build.Item{Rarity: build.Unique, Base: "Metadata/Items/Jewels/JewelTimeless", Mods: []build.ModLine{{
		Kind: build.Explicit, Text: "conquered", Stats: []build.StatValue{
			{Stat: "local_unique_jewel_alternate_tree_version", Value: 1},
			{Stat: "local_unique_jewel_alternate_tree_seed", Value: 100},
			{Stat: "local_unique_jewel_alternate_tree_keystone", Value: 1},
		},
	}}}
	global := func(id uint32, stats ...resolve.StatValue) resolve.Bundle {
		return resolve.Bundle{Actor: resolve.Player, Scope: resolve.Global, Source: resolve.NodeSource{ID: id}, Stats: stats}
	}
	testCases := []struct {
		name   string
		jewels build.Jewels
		want   []resolve.Bundle
	}{
		{
			name: "tattoo alone",
			want: []resolve.Bundle{
				global(4, resolve.StatValue{"base_maximum_life", 20}),
				global(7, resolve.StatValue{"additional_strength", 20}, resolve.StatValue{"maximum_life_+%", 8}),
				{Actor: resolve.Player, Scope: resolve.Buff, Source: resolve.NodeSource{ID: 7}, Stats: []resolve.StatValue{{"aura_damage_+%", 5}}},
			},
		},
		{
			name:   "tattoo then conquest",
			jewels: build.Jewels{3: jewel},
			want:   []resolve.Bundle{global(7, resolve.StatValue{"maximum_life_+%", 10})},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			b := testBuild(90, 1, 3, 4, 7)
			b.Overrides = map[uint32]string{4: "Valako4"}
			b.Jewels = testCase.jewels
			r, err := resolveBuild(t, s, b)
			if err != nil {
				t.Fatal(err)
			}
			got := slices.DeleteFunc(r.Bundles, func(bd resolve.Bundle) bool { _, ok := bd.Source.(resolve.NodeSource); return !ok })
			if diff := cmp.Diff(testCase.want, got); diff != "" {
				t.Errorf("node bundles (-want +got):\n%s", diff)
			}
		})
	}
}

// jewelWith is a unique jewel with stats, each of value 1.
func jewelWith(stats ...string) build.Item {
	svs := make([]build.StatValue, 0, len(stats))
	for _, st := range stats {
		svs = append(svs, build.StatValue{Stat: st, Value: 1})
	}
	return build.Item{Rarity: build.Unique, Base: "Metadata/Items/Jewels/JewelDex", Mods: []build.ModLine{{Kind: build.Explicit, Text: "instinct", Stats: svs}}}
}

// On conquestDataset with a Small radius around socket 3 that reaches small passive 4 and
// notable 7 but not mastery 9, allocated small passives in it grant nothing and the jewel grants
// unallocated ones; notables keep what they give.
func TestResolveRadiusPassives(t *testing.T) {
	t.Parallel()
	d := conquestDataset()
	d.Trees[0].JewelRadii = append(d.Trees[0].JewelRadii, dataset.JewelRadius{ID: "Small", Radius: 700})
	s := open(t, d)
	const (
		nothing = "local_jewel_allocated_non_notable_passives_in_radius_grant_nothing"
		granted = "local_jewel_copy_stats_from_unallocated_non_notable_passives_in_radius"
	)
	small := []resolve.StatValue{{"base_strength", 10}}
	notable := []resolve.StatValue{{"additional_strength", 20}, {"maximum_life_+%", 8}}
	node := func(id uint32, stats []resolve.StatValue) resolve.Bundle {
		return resolve.Bundle{Actor: resolve.Player, Scope: resolve.Global, Source: resolve.NodeSource{ID: id}, Stats: stats}
	}
	fromJewel := resolve.Bundle{Actor: resolve.Player, Scope: resolve.Global, Source: resolve.RadiusSource{Socket: 3, Node: 4}, Stats: small}
	testCases := []struct {
		name  string
		nodes []uint32
		jewel build.Item
		want  []resolve.Bundle
	}{
		{"allocated small passive grants nothing", []uint32{1, 3, 4, 7}, jewelWith(nothing), []resolve.Bundle{node(7, notable)}},
		{"allocated small passive with only the grant", []uint32{1, 3, 4, 7}, jewelWith(granted), []resolve.Bundle{node(4, small), node(7, notable)}},
		{"unallocated small passive granted", []uint32{1, 3, 7}, jewelWith(granted), []resolve.Bundle{node(7, notable), fromJewel}},
		{"unallocated small passive with only grant nothing", []uint32{1, 3, 7}, jewelWith(nothing), []resolve.Bundle{node(7, notable)}},
		{"Unnatural Instinct", []uint32{1, 3, 7}, jewelWith(nothing, granted), []resolve.Bundle{node(7, notable), fromJewel}},
		{"socket not allocated", []uint32{1, 4, 7}, jewelWith(nothing, granted), []resolve.Bundle{node(4, small), node(7, notable)}},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			b := testBuild(90, testCase.nodes...)
			b.Jewels = build.Jewels{3: testCase.jewel}
			r, err := resolveBuild(t, s, b)
			if err != nil {
				t.Fatal(err)
			}
			got := slices.DeleteFunc(r.Bundles, func(bd resolve.Bundle) bool {
				switch bd.Source.(type) {
				case resolve.NodeSource, resolve.RadiusSource:
					return bd.Scope != resolve.Global
				}
				return true
			})
			if diff := cmp.Diff(testCase.want, got); diff != "" {
				t.Errorf("node bundles (-want +got):\n%s", diff)
			}
		})
	}
}

// On conquestDataset with notable 7 also giving Strength and Dexterity, a tattoo giving
// Strength, and a second socket 10, a jewel in socket 3 that transforms an attribute changes it
// on the passives in its Large radius, 4 and 7, and leaves their other stats and buffs as they
// are. Passives a tattoo replaces or a timeless jewel conquers keep their attributes.
func TestResolveAttributeTransforms(t *testing.T) {
	t.Parallel()
	d := conquestDataset()
	hybrid := uint32(len(d.Stats))
	d.Stats = append(d.Stats, dataset.Stat{ID: "base_strength_and_dexterity"})
	tree := &d.Trees[0]
	tree.Nodes[0].Neighbours = append(tree.Nodes[0].Neighbours, 10)
	tree.Nodes[3].Stats = []dataset.StatValue{{Stat: 3, Value: 20}, {Stat: 4, Value: 8}, {Stat: hybrid, Value: 5}}
	tree.Nodes = append(tree.Nodes, dataset.Node{ID: 10, Kind: dataset.JewelSocket, Neighbours: []uint32{1}})
	tree.Overrides[0].Stats = []dataset.StatValue{{Stat: 6, Value: 20}}
	l := &d.Layouts[0]
	l.Groups = append(l.Groups, dataset.LayoutGroup{ID: 10, X: 2000})
	l.Nodes = append(l.Nodes, dataset.LayoutNode{ID: 10, Group: 10, Icon: "n.png"})
	s := open(t, d)
	timeless := build.Item{Rarity: build.Unique, Base: "Metadata/Items/Jewels/JewelTimeless", Mods: []build.ModLine{{
		Kind: build.Explicit, Text: "conquered", Stats: []build.StatValue{
			{Stat: "local_unique_jewel_alternate_tree_version", Value: 1},
			{Stat: "local_unique_jewel_alternate_tree_seed", Value: 100},
			{Stat: "local_unique_jewel_alternate_tree_keystone", Value: 1},
		},
	}}}
	const (
		strToInt = "local_jewel_nearby_passives_str_to_int"
		dexToStr = "local_jewel_nearby_passives_dex_to_str"
		intToDex = "local_jewel_nearby_passives_int_to_dex"
	)
	global := func(id uint32, stats ...resolve.StatValue) resolve.Bundle {
		return resolve.Bundle{Actor: resolve.Player, Scope: resolve.Global, Source: resolve.NodeSource{ID: id}, Stats: stats}
	}
	buff := resolve.Bundle{Actor: resolve.Player, Scope: resolve.Buff, Source: resolve.NodeSource{ID: 7}, Stats: []resolve.StatValue{{"aura_damage_+%", 5}}}
	small := global(4, resolve.StatValue{"base_strength", 10})
	notable := global(7, resolve.StatValue{"additional_strength", 20}, resolve.StatValue{"maximum_life_+%", 8},
		resolve.StatValue{"base_strength_and_dexterity", 5})
	notableToInt := global(7, resolve.StatValue{"maximum_life_+%", 8}, resolve.StatValue{"base_intelligence", 25}, resolve.StatValue{"base_dexterity", 5})
	testCases := []struct {
		name      string
		nodes     []uint32
		stat      string
		overrides map[uint32]string
		timeless  bool
		want      []resolve.Bundle
	}{
		{name: "Strength to Intelligence", nodes: []uint32{1, 3, 4, 7}, stat: strToInt, want: []resolve.Bundle{
			global(4, resolve.StatValue{"base_intelligence", 10}), notableToInt, buff,
		}},
		{name: "Dexterity to Strength", nodes: []uint32{1, 3, 4, 7}, stat: dexToStr, want: []resolve.Bundle{
			small, global(7, resolve.StatValue{"additional_strength", 20}, resolve.StatValue{"maximum_life_+%", 8}, resolve.StatValue{"base_strength", 10}), buff,
		}},
		{name: "no attribute to transform", nodes: []uint32{1, 3, 4, 7}, stat: intToDex, want: []resolve.Bundle{small, notable, buff}},
		{name: "socket not allocated", nodes: []uint32{1, 4, 7}, stat: strToInt, want: []resolve.Bundle{small, notable, buff}},
		{name: "tattoo", nodes: []uint32{1, 3, 4, 7}, stat: strToInt, overrides: map[uint32]string{4: "Valako4"}, want: []resolve.Bundle{
			global(4, resolve.StatValue{"base_strength", 20}), notableToInt, buff,
		}},
		{name: "conquered", nodes: []uint32{1, 3, 4, 7, 10}, stat: strToInt, timeless: true, want: []resolve.Bundle{
			global(7, resolve.StatValue{"maximum_life_+%", 10}),
		}},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			b := testBuild(90, testCase.nodes...)
			b.Overrides = testCase.overrides
			b.Jewels = build.Jewels{3: jewelWith(testCase.stat)}
			if testCase.timeless {
				b.Jewels[10] = timeless
			}
			r, err := resolveBuild(t, s, b)
			if err != nil {
				t.Fatal(err)
			}
			got := slices.DeleteFunc(r.Bundles, func(bd resolve.Bundle) bool {
				_, ok := bd.Source.(resolve.NodeSource)
				return !ok
			})
			if diff := cmp.Diff(testCase.want, got); diff != "" {
				t.Errorf("node bundles (-want +got):\n%s", diff)
			}
		})
	}
}

// On conquestDataset with a Medium and a Small radius, the tattoo giving Strength, the conquered
// notable also giving Strength and a second socket 10 whose Large radius reaches 4 and 7 but not
// 3, radius jewels change what passives give: The Light of Meaning adds its stat to small and
// notable passives, tattooed ones too, and to those Unnatural Instinct grants, but not to the
// class start, masteries or conquered passives; Healthy Mind moves increases to maximum life to
// maximum mana at twice their value, but not on conquered passives; Transcendent Flesh gives its
// value of Strength per Strength on each passive, conquered or tattooed, each jewel counting the
// passive as the others leave it.
func TestResolveRadiusChanges(t *testing.T) {
	t.Parallel()
	d := conquestDataset()
	d.Stats = append(d.Stats, dataset.Stat{ID: "maximum_mana_+%"})
	tree := &d.Trees[0]
	tree.JewelRadii = append(tree.JewelRadii, dataset.JewelRadius{ID: "Medium", Radius: 1500}, dataset.JewelRadius{ID: "Small", Radius: 700})
	tree.Nodes[0].Neighbours = append(tree.Nodes[0].Neighbours, 10)
	tree.Nodes = append(tree.Nodes, dataset.Node{ID: 10, Kind: dataset.JewelSocket, Neighbours: []uint32{1}})
	tree.Overrides[0].Stats = []dataset.StatValue{{Stat: 6, Value: 20}}
	tree.TimelessVersions[1].Replacements[1].Stats = append(tree.TimelessVersions[1].Replacements[1].Stats, dataset.StatRange{Stat: 3, Min: 5, Max: 5})
	l := &d.Layouts[0]
	l.Groups = append(l.Groups, dataset.LayoutGroup{ID: 10, X: 2000})
	l.Nodes = append(l.Nodes, dataset.LayoutNode{ID: 10, Group: 10, Icon: "n.png"})
	s := open(t, d)
	jewel := func(stat string, value int32) build.Item {
		return build.Item{Rarity: build.Unique, Base: "Metadata/Items/Jewels/JewelDex", Mods: []build.ModLine{{
			Kind: build.Explicit, Text: "radius", Stats: []build.StatValue{{Stat: stat, Value: value}},
		}}}
	}
	timeless := build.Item{Rarity: build.Unique, Base: "Metadata/Items/Jewels/JewelTimeless", Mods: []build.ModLine{{
		Kind: build.Explicit, Text: "conquered", Stats: []build.StatValue{
			{Stat: "local_unique_jewel_alternate_tree_version", Value: 1},
			{Stat: "local_unique_jewel_alternate_tree_seed", Value: 100},
			{Stat: "local_unique_jewel_alternate_tree_keystone", Value: 1},
		},
	}}}
	chaos := jewel("local_unique_jewel_passives_in_radius_grant_base_chaos_damage_resistance_%", 4)
	lifeToMana := jewel("local_unique_jewel_life_increases_applies_to_mana_doubled", 1)
	flesh := jewel("local_unique_jewel_X_strength_per_1_strength_allocated_in_radius", -1)
	instinct := jewelWith("local_jewel_allocated_non_notable_passives_in_radius_grant_nothing",
		"local_jewel_copy_stats_from_unallocated_non_notable_passives_in_radius")
	global := func(src resolve.Source, stats ...resolve.StatValue) resolve.Bundle {
		return resolve.Bundle{Actor: resolve.Player, Scope: resolve.Global, Source: src, Stats: stats}
	}
	n4, n7 := resolve.NodeSource{ID: 4}, resolve.NodeSource{ID: 7}
	str, addStr, life, mana, chaosRes := "base_strength", "additional_strength", "maximum_life_+%", "maximum_mana_+%", "base_chaos_damage_resistance_%"
	conquered := global(n7, resolve.StatValue{life, 10}, resolve.StatValue{addStr, 5})
	tattoo := map[uint32]string{4: "Valako4"}
	testCases := []struct {
		name      string
		nodes     []uint32
		jewels    build.Jewels
		overrides map[uint32]string
		want      []resolve.Bundle
	}{
		{name: "The Light of Meaning", nodes: []uint32{1, 3, 4, 7}, jewels: build.Jewels{3: chaos}, want: []resolve.Bundle{
			global(n4, resolve.StatValue{str, 10}, resolve.StatValue{chaosRes, 4}),
			global(n7, resolve.StatValue{addStr, 20}, resolve.StatValue{life, 8}, resolve.StatValue{chaosRes, 4}),
		}},
		{name: "The Light of Meaning on a tattoo", nodes: []uint32{1, 3, 4}, jewels: build.Jewels{3: chaos}, overrides: tattoo, want: []resolve.Bundle{
			global(n4, resolve.StatValue{str, 20}, resolve.StatValue{chaosRes, 4}),
		}},
		{name: "The Light of Meaning beside a timeless jewel", nodes: []uint32{1, 3, 7, 10}, jewels: build.Jewels{3: chaos, 10: timeless},
			want: []resolve.Bundle{conquered}},
		{name: "The Light of Meaning on a passive Unnatural Instinct grants", nodes: []uint32{1, 3, 10}, jewels: build.Jewels{3: instinct, 10: chaos},
			want: []resolve.Bundle{global(resolve.RadiusSource{Socket: 3, Node: 4}, resolve.StatValue{str, 10}, resolve.StatValue{chaosRes, 4})}},
		{name: "Healthy Mind", nodes: []uint32{1, 3, 7}, jewels: build.Jewels{3: lifeToMana}, want: []resolve.Bundle{
			global(n7, resolve.StatValue{addStr, 20}, resolve.StatValue{mana, 16}),
		}},
		{name: "Healthy Mind beside a timeless jewel", nodes: []uint32{1, 3, 7, 10}, jewels: build.Jewels{3: lifeToMana, 10: timeless},
			want: []resolve.Bundle{conquered}},
		{name: "Transcendent Flesh", nodes: []uint32{1, 3, 4, 7}, jewels: build.Jewels{3: flesh}, want: []resolve.Bundle{
			global(n7, resolve.StatValue{addStr, 20}, resolve.StatValue{life, 8}, resolve.StatValue{str, -20}),
		}},
		{name: "Transcendent Flesh on a tattoo", nodes: []uint32{1, 3, 4}, jewels: build.Jewels{3: flesh}, overrides: tattoo, want: []resolve.Bundle{}},
		{name: "Transcendent Flesh beside a timeless jewel", nodes: []uint32{1, 3, 7, 10}, jewels: build.Jewels{3: flesh, 10: timeless}, want: []resolve.Bundle{
			global(n7, resolve.StatValue{life, 10}, resolve.StatValue{addStr, 5}, resolve.StatValue{str, -5}),
		}},
		{name: "two Transcendent Flesh", nodes: []uint32{1, 3, 4, 7, 10}, jewels: build.Jewels{3: flesh, 10: flesh}, want: []resolve.Bundle{
			global(n4, resolve.StatValue{str, -10}),
			global(n7, resolve.StatValue{addStr, 20}, resolve.StatValue{life, 8}, resolve.StatValue{str, -40}),
		}},
		{name: "socket not allocated", nodes: []uint32{1, 4}, jewels: build.Jewels{3: chaos}, want: []resolve.Bundle{global(n4, resolve.StatValue{str, 10})}},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			b := testBuild(90, testCase.nodes...)
			b.Overrides = testCase.overrides
			b.Jewels = testCase.jewels
			r, err := resolveBuild(t, s, b)
			if err != nil {
				t.Fatal(err)
			}
			got := slices.DeleteFunc(r.Bundles, func(bd resolve.Bundle) bool {
				switch bd.Source.(type) {
				case resolve.NodeSource, resolve.RadiusSource:
					return bd.Scope != resolve.Global
				}
				return true
			})
			if diff := cmp.Diff(testCase.want, got); diff != "" {
				t.Errorf("node bundles (-want +got):\n%s", diff)
			}
		})
	}
}
