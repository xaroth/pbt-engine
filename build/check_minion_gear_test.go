package build_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/dataset"
)

func animateGuardian() build.GemID {
	return build.GemID{ID: "Metadata/Items/Gems/SkillGemAnimateArmour"}
}

// minionGearDataset is skillsDataset with itemsDataset's items and Animate Guardian.
func minionGearDataset() *dataset.Dataset {
	d := skillsDataset()
	d.Items = itemsDataset().Items
	d.Skills.Gems = append(d.Skills.Gems, dataset.Gem{ID: animateGuardian().ID, Colour: dataset.Red, Effects: []dataset.GemEffect{
		{ID: "AnimateGuardian", GrantedEffect: "AnimateGuardian", Style: dataset.DefaultStyle},
	}})
	d.Skills.GrantedEffects = append(d.Skills.GrantedEffects, dataset.GrantedEffect{
		ID: "AnimateGuardian", ActiveSkill: "animate_guardian", Levels: []dataset.EffectLevel{{Level: 1}, {Level: 20}},
	})
	d.Skills.ActiveSkills = append(d.Skills.ActiveSkills, dataset.ActiveSkill{ID: "animate_guardian", Types: []uint16{0}})
	return d
}

// TestCheckMinionGear: Check keeps the gear of a group's minion that fits as it keeps the
// character's, and leaves out with a warning what does not.
func TestCheckMinionGear(t *testing.T) {
	t.Parallel()
	s := openStore(t, minionGearDataset())
	item := func(base string) build.Item { return build.Item{Rarity: build.Normal, Base: base} }
	gear := build.Items{build.SlotWeapon: item(clawBase), build.SlotHelmet: helmet()}
	guardian := []build.Gem{{GemID: inspiration(), Level: 20, Support: true}, {GemID: animateGuardian(), Level: 20}}
	testCases := []struct {
		name  string
		group build.SkillGroup
		codes []build.Code
		want  build.SkillGroup
	}{
		{
			"gear that fits", build.SkillGroup{Gems: guardian, Active: 1, Count: 2, MinionItems: gear},
			nil, build.SkillGroup{Gems: guardian, Active: 1, Count: 2, MinionItems: gear},
		},
		{
			"a slot the minion does not use", build.SkillGroup{Gems: guardian, Active: 1, MinionItems: build.Items{build.SlotRing1: item(ringBase)}},
			[]build.Code{build.MinionSlotUnused}, build.SkillGroup{Gems: guardian, Active: 1},
		},
		{
			"an item the slot does not take",
			build.SkillGroup{Gems: guardian, Active: 1, MinionItems: build.Items{build.SlotHelmet: item(shieldBase), build.SlotWeapon: item(clawBase)}},
			[]build.Code{build.ItemSlotMismatch}, build.SkillGroup{Gems: guardian, Active: 1, MinionItems: build.Items{build.SlotWeapon: item(clawBase)}},
		},
		{
			"an unknown base", build.SkillGroup{Gems: guardian, Active: 1, MinionItems: build.Items{build.SlotHelmet: item("Metadata/Items/Nothing")}},
			[]build.Code{build.ItemBaseUnknown}, build.SkillGroup{Gems: guardian, Active: 1},
		},
		{
			"a skill without a minion that takes gear", build.SkillGroup{Gems: []build.Gem{{GemID: fireball(), Level: 20}}, MinionItems: gear},
			[]build.Code{build.MinionGearUnused}, build.SkillGroup{Gems: []build.Gem{{GemID: fireball(), Level: 20}}},
		},
		{
			"a count below 1", build.SkillGroup{Gems: guardian, Active: 1, Count: -2},
			[]build.Code{build.GroupCountOutOfRange}, build.SkillGroup{Gems: guardian, Active: 1},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			b := juggernaut()
			b.Skills.Groups = []build.SkillGroup{testCase.group}
			c, warnings, err := build.Check(b, s)
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(testCase.codes, codesOf(warnings)); diff != "" {
				t.Errorf("codes (-want +got):\n%s\n%v", diff, warnings)
			}
			if diff := cmp.Diff(testCase.want, c.Build().Skills.Groups[0]); diff != "" {
				t.Errorf("group (-want +got):\n%s", diff)
			}
		})
	}
}
