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
// of its gem. 3.28's dataset has no granted skills on mods.
func TestItemSupports(t *testing.T) {
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
		{"local_display_socketed_gems_get_spell_totem_level", 32, []dataset.GrantedSkill{{Effect: "SupportSpellTotem", Level: 32}}},
		{"local_display_supported_by_level_10_intensify", 2, nil},
	}
	x, err := realdata.Open(t, realdata.Current).ItemSupports()
	if err != nil {
		t.Fatal(err)
	}
	for _, testCase := range testCases {
		if got := x.Supports(testCase.stat, testCase.value); !slices.Equal(got, testCase.want) {
			t.Errorf("%s %d: got %+v, want %+v", testCase.stat, testCase.value, got, testCase.want)
		}
	}
}
