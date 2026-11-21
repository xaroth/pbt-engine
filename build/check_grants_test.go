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
	return openStore(t, d)
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
