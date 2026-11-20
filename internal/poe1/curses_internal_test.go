package poe1

import (
	"testing"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/internal/resolve"
)

// Of curses beyond the limit, the item slot of a curse's group decides first, then the gem's
// place in it, then the curse; a weapon swap slot counts as its weapon.
func TestCursePrecedence(t *testing.T) {
	t.Parallel()
	groups := []build.SkillGroup{{Slot: build.SlotGloves}, {Slot: build.SlotBoots}, {Slot: build.SlotWeaponSwap}, {Slot: build.SlotWeapon}}
	curse := func(group, gem int, id string) *resolve.Skill {
		return &resolve.Skill{Source: resolve.GemSource{Group: group, Gem: gem}, GrantedEffect: id}
	}
	for _, c := range []struct {
		name          string
		higher, lower *resolve.Skill
	}{
		{"boots over gloves", curse(1, 0, "ElementalWeakness"), curse(0, 3, "Flammability")},
		{"later gem in the group", curse(0, 2, "ElementalWeakness"), curse(0, 1, "Flammability")},
		{"by curse in the same place", curse(0, 1, "Flammability"), curse(0, 1, "ElementalWeakness")},
		{"swap as its weapon", curse(2, 1, "Flammability"), curse(3, 1, "ElementalWeakness")},
	} {
		if h, l := cursePrecedence(c.higher, groups), cursePrecedence(c.lower, groups); h <= l {
			t.Errorf("%s: got %d over %d", c.name, h, l)
		}
	}
}
