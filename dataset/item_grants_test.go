package dataset_test

import (
	"slices"
	"testing"

	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/internal/realdata"
)

// The supports item stats grant on the current dataset. The level is the stat's value when the
// stat carries it, also beyond the levels mods roll, and the mod's when not, as for Intensify,
// whose value must be one a mod rolls. A stat of mods granting no support, or one that does not
// tell the supports of its mods apart, grants none. Cast On Critical Strike grants both effects
// of its gem. A stat that Ruthless-only mods share, as Arrogance's, still grants its support. 3.28's dataset has no granted skills on mods.
func TestItemGrantsSupports(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		stat  string
		value int32
		want  []dataset.GrantedSkill
	}{
		{"local_display_socketed_gems_get_spell_totem_level", 20, []dataset.GrantedSkill{{Effect: "SupportSpellTotem", Level: 20}}},
		{"local_display_socketed_gems_get_spell_totem_level", 22, []dataset.GrantedSkill{{Effect: "SupportSpellTotem", Level: 22}}},
		{"local_display_supported_by_level_10_intensify", 1, []dataset.GrantedSkill{{Effect: "SupportIntensify", Level: 10}}},
		{
			"local_display_socketed_gems_get_cast_on_crit_level", 20,
			[]dataset.GrantedSkill{{Effect: "SupportCastOnCriticalStrike", Level: 20}, {Effect: "SupportCastOnCritTriggered", Level: 20}},
		},
		{"local_unique_hungry_loop_has_consumed_gem", 1, nil},
		{"local_physical_damage_+%", 120, nil},
		{"local_display_socketed_gems_get_blood_magic_level", 25, []dataset.GrantedSkill{{Effect: "SupportArrogance", Level: 25}}},
		{"local_display_socketed_gems_get_spell_totem_level", 32, []dataset.GrantedSkill{{Effect: "SupportSpellTotem", Level: 32}}},
		{"local_display_supported_by_level_10_intensify", 2, nil},
	}
	x, err := realdata.Open(t, realdata.Current).ItemGrants()
	if err != nil {
		t.Fatal(err)
	}
	for _, testCase := range testCases {
		if got := x.Supports(testCase.stat, testCase.value); !slices.Equal(got, testCase.want) {
			t.Errorf("%s %d: got %+v, want %+v", testCase.stat, testCase.value, got, testCase.want)
		}
	}
}

// The skills item stats grant on the current dataset: at the stat's value when it carries the
// level, and at the mod's when the stat is a chance. In Ruthless a mod grants its own skill for
// Ruthless where it has one. A support or a plain stat grants no skill.
func TestItemGrantsSkills(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		stat     string
		value    int32
		ruthless bool
		want     []dataset.GrantedSkill
	}{
		{"local_display_grants_skill_herald_of_the_breach_level", 30, false, []dataset.GrantedSkill{{Effect: "HeraldOfTheBreach", Level: 30}}},
		{"local_display_trigger_level_1_blood_rage_on_kill_chance_%", 15, false, []dataset.GrantedSkill{{Effect: "JunTriggeredBloodRage", Level: 1}}},
		{"local_display_grants_skill_determination_level", 16, false, []dataset.GrantedSkill{{Effect: "Determination", Level: 16}}},
		{"local_display_grants_skill_determination_level", 16, true, []dataset.GrantedSkill{{Effect: "DeterminationHardMode", Level: 16}}},
		{"local_display_socketed_gems_get_spell_totem_level", 20, false, nil},
		{"local_physical_damage_+%", 120, false, nil},
	}
	x, err := realdata.Open(t, realdata.Current).ItemGrants()
	if err != nil {
		t.Fatal(err)
	}
	if x.None() {
		t.Error("None: true on a dataset whose mods grant skills")
	}
	for _, testCase := range testCases {
		if got := x.Skills(testCase.stat, testCase.value, testCase.ruthless); !slices.Equal(got, testCase.want) {
			t.Errorf("%s %d ruthless %t: got %+v, want %+v", testCase.stat, testCase.value, testCase.ruthless, got, testCase.want)
		}
	}
}
