package build_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/xaroth/pbt-engine/build"
)

// TestCheckHands: an offhand item the game refuses beside the weapon is left out with a
// warning; a quiver with no weapon is kept with one.
func TestCheckHands(t *testing.T) {
	t.Parallel()
	s := itemsStore(t)
	item := func(base string) build.Item { return build.Item{Rarity: build.Normal, Base: base} }
	bow, sword, claw := item(bowBase), item(swordBase), item(clawBase)
	shield, quiver := item(shieldBase), item(quiverBase)
	testCases := []struct {
		name  string
		items build.Items
		codes []build.Code
		want  build.Items
	}{
		{
			"bow and quiver", build.Items{build.SlotWeapon: bow, build.SlotOffhand: quiver},
			nil, build.Items{build.SlotWeapon: bow, build.SlotOffhand: quiver},
		},
		{
			"claw and shield", build.Items{build.SlotWeapon: claw, build.SlotOffhand: shield},
			nil, build.Items{build.SlotWeapon: claw, build.SlotOffhand: shield},
		},
		{
			"two-handed sword and shield", build.Items{build.SlotWeapon: sword, build.SlotOffhand: shield},
			[]build.Code{build.ItemOffhandBlocked}, build.Items{build.SlotWeapon: sword},
		},
		{
			"two-handed sword and claw", build.Items{build.SlotWeapon: sword, build.SlotOffhand: claw},
			[]build.Code{build.ItemOffhandBlocked}, build.Items{build.SlotWeapon: sword},
		},
		{
			"bow and shield", build.Items{build.SlotWeapon: bow, build.SlotOffhand: shield},
			[]build.Code{build.ItemOffhandBlocked}, build.Items{build.SlotWeapon: bow},
		},
		{
			"two-handed sword and quiver", build.Items{build.SlotWeapon: sword, build.SlotOffhand: quiver},
			[]build.Code{build.ItemOffhandBlocked}, build.Items{build.SlotWeapon: sword},
		},
		{
			"claw and quiver", build.Items{build.SlotWeapon: claw, build.SlotOffhand: quiver},
			[]build.Code{build.ItemQuiverNoBow}, build.Items{build.SlotWeapon: claw},
		},
		{
			"quiver alone", build.Items{build.SlotOffhand: quiver},
			[]build.Code{build.ItemQuiverNoBow}, build.Items{build.SlotOffhand: quiver},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			b := juggernaut()
			b.Items = testCase.items
			c, warnings, err := build.Check(b, s)
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(testCase.codes, codesOf(warnings)); diff != "" {
				t.Errorf("codes (-want +got):\n%s\n%v", diff, warnings)
			}
			if diff := cmp.Diff(testCase.want, c.Build().Items); diff != "" {
				t.Errorf("items (-want +got):\n%s", diff)
			}
		})
	}
}
