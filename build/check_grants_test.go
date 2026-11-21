package build_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/dataset"
)

// grantsStore is skillsStore with itemsStore's items, a mod whose stat grants Fireball at the
// stat's level and FireballAltX in Ruthless, one whose chance stat grants Fireball at level 1,
// node 3 granting Fireball at level 1, and a Ruthless tree like the standard one.
func grantsStore(t *testing.T) *dataset.Store {
	t.Helper()
	return openStore(t, grantsDataset())
}

func grantsDataset() *dataset.Dataset {
	d := skillsDataset()
	d.Items = itemsDataset().Items
	d.Stats = append(d.Stats, dataset.Stat{ID: "local_display_grants_skill_fireball_level"}, dataset.Stat{ID: "trigger_fireball_on_kill_%"})
	d.Mods = []dataset.Mod{{
		ID: "GrantsFireball", Type: "GrantsFireball", Stats: []dataset.ModStat{{Stat: 1, Min: 40, Max: 40, RuthlessMin: 40, RuthlessMax: 40}},
		GrantedSkills: []dataset.GrantedSkill{{Effect: "Fireball", Level: 40}}, RuthlessGrantedSkills: []dataset.GrantedSkill{{Effect: "FireballAltX", Level: 1}},
	}, {
		ID: "TriggerFireball", Type: "TriggerFireball", Stats: []dataset.ModStat{{Stat: 2, Min: 10, Max: 10, RuthlessMin: 10, RuthlessMax: 10}},
		GrantedSkills: []dataset.GrantedSkill{{Effect: "Fireball", Level: 1}},
	}}
	d.Trees[0].Nodes[2].GrantedSkill = &dataset.GrantedSkill{Effect: "Fireball", Level: 1}
	ruthless := d.Trees[0]
	ruthless.Variant = "ruthless"
	d.Trees = append(d.Trees, ruthless)
	return d
}

// TestCheckGrants: an item or passive that grants a skill adds its group; a group whose source
// no longer grants its skill, or whose skill the dataset lacks, is removed with a warning, and
// the build still checks.
func TestCheckGrants(t *testing.T) {
	t.Parallel()
	s := grantsStore(t)
	ring := build.Item{Rarity: build.Unique, Name: "Lost Unity", Base: ringBase, Mods: []build.ModLine{{
		Kind: build.Explicit, Text: "Grants Level 40 Fireball Skill", Stats: []build.StatValue{{Stat: "local_display_grants_skill_fireball_level", Value: 40}},
	}}}
	fromNode := build.SkillGroup{Source: &build.SkillSource{Node: 3}, Gems: []build.Gem{{Skill: "Fireball", Level: 1}}}
	fromRing := func(skill string, level int) build.SkillGroup {
		return build.SkillGroup{Source: &build.SkillSource{Item: build.SlotRing2}, Gems: []build.Gem{{Skill: skill, Level: level}}}
	}
	own := build.SkillGroup{Gems: []build.Gem{{GemID: fireball(), Level: 20}}}
	twice := ring
	twice.Mods = append(twice.Mods, build.ModLine{
		Kind: build.Explicit, Text: "10% chance to Trigger Level 1 Fireball on Kill", Stats: []build.StatValue{{Stat: "trigger_fireball_on_kill_%", Value: 10}},
	})
	testCases := []struct {
		name    string
		variant string
		items   build.Items
		groups  []build.SkillGroup
		codes   []build.Code
		want    []build.SkillGroup
	}{
		{name: "the passive's skill added", want: []build.SkillGroup{fromNode}},
		{
			name: "the item's skill added", items: build.Items{build.SlotRing2: ring}, groups: []build.SkillGroup{own},
			want: []build.SkillGroup{own, fromNode, fromRing("Fireball", 40)},
		},
		{
			name: "the item's skill in Ruthless", variant: "ruthless", items: build.Items{build.SlotRing2: ring},
			want: []build.SkillGroup{fromNode, fromRing("FireballAltX", 1)},
		},
		{
			name: "the item taken off", groups: []build.SkillGroup{fromNode, fromRing("Fireball", 40)},
			codes: []build.Code{build.GrantedSkillRemoved}, want: []build.SkillGroup{fromNode},
		},
		{
			name: "a skill the dataset lacks", items: build.Items{build.SlotRing2: ring}, groups: []build.SkillGroup{fromRing("Nothing", 40), fromNode},
			codes: []build.Code{build.GrantedSkillUnknown}, want: []build.SkillGroup{fromNode, fromRing("Fireball", 40)},
		},
		{
			name: "a disabled skill at another level", items: build.Items{build.SlotRing2: ring},
			groups: []build.SkillGroup{fromNode, {Source: &build.SkillSource{Item: build.SlotRing2}, Gems: []build.Gem{{Skill: "Fireball", Level: 5, Disabled: true}}}},
			codes:  []build.Code{build.GrantedSkillLevel},
			want:   []build.SkillGroup{fromNode, {Source: &build.SkillSource{Item: build.SlotRing2}, Gems: []build.Gem{{Skill: "Fireball", Level: 40, Disabled: true}}}},
		},
		{
			name: "one skill at two levels", items: build.Items{build.SlotRing2: twice},
			groups: []build.SkillGroup{fromNode, fromRing("Fireball", 1), fromRing("Fireball", 40)},
			want:   []build.SkillGroup{fromNode, fromRing("Fireball", 1), fromRing("Fireball", 40)},
		},
		{
			name:   "a granted skill among the player's gems",
			groups: []build.SkillGroup{{Gems: []build.Gem{{Skill: "Fireball", Level: 1}, {GemID: fireball(), Level: 20}}}},
			codes:  []build.Code{build.GrantedSkillRemoved}, want: []build.SkillGroup{own, fromNode},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			b := juggernaut()
			if testCase.variant != "" {
				b.Variant = testCase.variant
			}
			b.Items, b.Skills.Groups = testCase.items, testCase.groups
			c, warnings, err := build.Check(b, s)
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(testCase.codes, codesOf(warnings)); diff != "" {
				t.Errorf("codes (-want +got):\n%s\n%v", diff, warnings)
			}
			if diff := cmp.Diff(testCase.want, c.Build().Skills.Groups); diff != "" {
				t.Errorf("groups (-want +got):\n%s", diff)
			}
		})
	}
}

// TestCheckAllocatedGrants: a passive an item or a pair of jewels allocates grants its skill as
// one on the tree does, and the group goes with the item or jewel. Each case starts with the
// groups of node 3, the anointed notable 41 and the jewels' ascendancy passive 11.
func TestCheckAllocatedGrants(t *testing.T) {
	t.Parallel()
	const (
		anointStat = "mod_granted_passive_hash"
		flameStat  = "unique_jewel_grants_notable_hash_part_1"
		fleshStat  = "unique_jewel_grants_notable_hash_part_2"
	)
	d := grantsDataset()
	d.Stats = append(d.Stats, dataset.Stat{ID: anointStat}, dataset.Stat{ID: flameStat}, dataset.Stat{ID: fleshStat})
	d.Items.Classes = append(d.Items.Classes, dataset.ItemClass{ID: "Jewel"})
	d.Items.Bases = append(d.Items.Bases, dataset.ItemBase{ID: jewelBase, Class: "Jewel"})
	d.Trees = d.Trees[:1]
	tree := &d.Trees[0]
	tree.Nodes[2].Neighbours = append(tree.Nodes[2].Neighbours, 70, 71)
	tree.Nodes = append(tree.Nodes,
		dataset.Node{ID: 70, Kind: dataset.JewelSocket, Neighbours: []uint32{3}},
		dataset.Node{ID: 71, Kind: dataset.JewelSocket, Neighbours: []uint32{3}},
	)
	tree.JewelSlots = []uint32{70, 71}
	for i, n := range tree.Nodes {
		if n.ID == 11 || n.ID == 41 {
			tree.Nodes[i].GrantedSkill = &dataset.GrantedSkill{Effect: "Fireball", Level: 40}
		}
	}
	s := openStore(t, d)
	line := func(stat string, node int32) build.ModLine {
		return build.ModLine{Kind: build.Explicit, Text: "Allocates", Stats: []build.StatValue{{Stat: stat, Value: node}}}
	}
	amulet := build.Item{Rarity: build.Rare, Name: "Amulet", Base: amuletBase, Mods: []build.ModLine{line(anointStat, 41)}}
	flame := build.Item{Rarity: build.Unique, Name: "Forbidden Flame", Base: jewelBase, Mods: []build.ModLine{line(flameStat, 11)}}
	flesh := build.Item{Rarity: build.Unique, Name: "Forbidden Flesh", Base: jewelBase, Mods: []build.ModLine{line(fleshStat, 11)}}
	fromNode := func(id uint32, level int) build.SkillGroup {
		return build.SkillGroup{Source: &build.SkillSource{Node: id}, Gems: []build.Gem{{Skill: "Fireball", Level: level}}}
	}
	testCases := []struct {
		name   string
		items  build.Items
		jewels build.Jewels
		codes  []build.Code
		want   []build.SkillGroup
	}{
		{
			name: "an anoint", items: build.Items{build.SlotAmulet: amulet},
			codes: []build.Code{build.GrantedSkillRemoved}, want: []build.SkillGroup{fromNode(3, 1), fromNode(41, 40)},
		},
		{name: "the anoint taken off", codes: []build.Code{build.GrantedSkillRemoved, build.GrantedSkillRemoved}, want: []build.SkillGroup{fromNode(3, 1)}},
		{
			name: "a jewel pair", jewels: build.Jewels{70: flame, 71: flesh},
			codes: []build.Code{build.GrantedSkillRemoved}, want: []build.SkillGroup{fromNode(3, 1), fromNode(11, 40)},
		},
		{
			name: "a jewel without its pair", jewels: build.Jewels{70: flame},
			codes: []build.Code{build.JewelNotableUnmatched, build.GrantedSkillRemoved, build.GrantedSkillRemoved}, want: []build.SkillGroup{fromNode(3, 1)},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			b := juggernaut()
			b.Nodes = append(b.Nodes, 70, 71)
			b.Items, b.Jewels = testCase.items, testCase.jewels
			b.Skills.Groups = []build.SkillGroup{fromNode(3, 1), fromNode(41, 40), fromNode(11, 40)}
			c, warnings, err := build.Check(b, s)
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(testCase.codes, codesOf(warnings)); diff != "" {
				t.Errorf("codes (-want +got):\n%s\n%v", diff, warnings)
			}
			if diff := cmp.Diff(testCase.want, c.Build().Skills.Groups); diff != "" {
				t.Errorf("groups (-want +got):\n%s", diff)
			}
		})
	}
}
