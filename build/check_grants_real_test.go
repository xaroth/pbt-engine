package build_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/internal/realdata"
)

// The Foulborn Severed in Sleep of 3.30 reads "Grants Level 25 Envy" as a granted skill: its
// Envy group stays at level 25 without a warning, and a build without the group gets it.
func TestCheckGrantedEnvyInGame(t *testing.T) {
	t.Parallel()
	s := realdata.Open(t, realdata.InGame)
	const dagger = "Metadata/Items/Weapons/OneHandWeapons/Daggers/Dagger12"
	weapon := build.Item{Rarity: build.Unique, Name: "Foulborn Severed in Sleep", Base: dagger, Mods: []build.ModLine{{
		Kind: build.Explicit, Text: "Grants Level 25 Envy", Stats: []build.StatValue{{Stat: "local_display_grants_level_X_envy", Value: 25}},
	}}}
	envy := build.SkillGroup{Source: &build.SkillSource{Item: build.SlotWeapon}, Gems: []build.Gem{{Skill: "Envy", Level: 25}}}
	added := envy
	added.Slot = build.SlotWeapon
	for _, tc := range []struct {
		name         string
		groups, want []build.SkillGroup
	}{
		{"imported with its group", []build.SkillGroup{envy}, []build.SkillGroup{envy}},
		{"without its group", nil, []build.SkillGroup{added}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			b := juggernaut()
			b.Version = "3.30"
			b.Items, b.Skills.Groups = build.Items{build.SlotWeapon: weapon}, tc.groups
			c, warnings, err := build.Check(b, s)
			if err != nil {
				t.Fatal(err)
			}
			for _, w := range warnings {
				if w.Code == build.GrantedSkillRemoved || w.Code == build.GrantedSkillUnknown || w.Code == build.GrantedSkillLevel {
					t.Errorf("warning %v, want no granted skill warning", w)
				}
			}
			if diff := cmp.Diff(tc.want, c.Build().Skills.Groups); diff != "" {
				t.Errorf("groups (-want +got):\n%s", diff)
			}
		})
	}
}
