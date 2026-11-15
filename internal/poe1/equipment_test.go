package poe1_test

import (
	"testing"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/internal/poe1"
	"github.com/xaroth/pbt-engine/internal/resolve"
)

// setItems equips items on sh.
func setItems(t *testing.T, sh *poe1.Sheet, items ...poe1.Item) {
	t.Helper()
	if err := sh.SetItems(items); err != nil {
		t.Fatal(err)
	}
}

// armourPiece compiles a made-up armour item in slot on a base with armour, evasion or both,
// whose own armour the item keeps.
func armourPiece(slot build.Slot, armour, evasion int32) poe1.Item {
	base := &dataset.ItemBase{Armour: &dataset.ArmourValues{
		Armour: dataset.ValueRange{Min: armour, Max: armour}, Evasion: dataset.ValueRange{Min: evasion, Max: evasion},
	}}
	c := poe1.CompileItem(&build.Item{Name: string(slot)}, base)
	return poe1.Item{Slot: slot, Name: string(slot), Key: string(slot), Stats: c.Stats, Armour: c.Armour}
}

// "+1% to all maximum Elemental Resistances if Equipped Helmet, Body Armour, Gloves, and Boots
// all have Armour" raises the 75% maximum to 76% only while each of the four has armour of its
// own; an evasion item or an empty slot among them does not, and other slots do not count.
func TestArmourOnAllArmourSlots(t *testing.T) {
	t.Parallel()
	sh := open(t, resolved(endgame(
		resolve.StatValue{Stat: "additional_maximum_all_elemental_resistances_%_if_all_equipment_grants_armour", Value: 1},
	)...))
	helmet, body := armourPiece(build.SlotHelmet, 100, 0), armourPiece(build.SlotBodyArmour, 300, 300)
	gloves, boots := armourPiece(build.SlotGloves, 50, 0), armourPiece(build.SlotBoots, 50, 0)
	evasionBoots := armourPiece(build.SlotBoots, 0, 80)
	ring := poe1.Item{Slot: build.SlotRing1, Name: "Ring", Key: "ring"}
	for _, tc := range []struct {
		name  string
		items []poe1.Item
		want  float64
	}{
		{"all four with armour", []poe1.Item{helmet, body, gloves, boots, ring}, 76},
		{"evasion boots", []poe1.Item{helmet, body, gloves, evasionBoots}, 75},
		{"no boots", []poe1.Item{helmet, body, gloves}, 75},
		{"no items", nil, 75},
		{"all four again", []poe1.Item{helmet, body, gloves, boots}, 76},
	} {
		setItems(t, sh, tc.items...)
		if got := value(t, sh, "maximum_fire_resistance"); got != tc.want {
			t.Errorf("%s: maximum fire resistance %v, want %v", tc.name, got, tc.want)
		}
	}
}

// "15% increased maximum Life if there are no Life Modifiers on Equipped Body Armour" counts
// without a body armour and with one without Life Modifiers, not with one that has them; Life
// Modifiers on another item do not matter. 100 life.
func TestNoLifeModifiersOnBodyArmour(t *testing.T) {
	t.Parallel()
	sh := open(t, resolved(bundle(resolve.NodeSource{ID: 1}, resolve.Global,
		resolve.StatValue{Stat: "base_maximum_life", Value: 100},
		resolve.StatValue{Stat: "maximum_life_+%_if_no_life_tags_on_body_armour", Value: 15},
	)))
	plate := poe1.Item{Slot: build.SlotBodyArmour, Name: "Plate", Key: "plate"}
	lifePlate := poe1.Item{Slot: build.SlotBodyArmour, Name: "Plate", Key: "life plate", LifeModifiers: true}
	lifeHelmet := poe1.Item{Slot: build.SlotHelmet, Name: "Helmet", Key: "helmet", LifeModifiers: true}
	for _, tc := range []struct {
		name  string
		items []poe1.Item
		want  float64
	}{
		{"no body armour", nil, 115},
		{"body armour without", []poe1.Item{plate}, 115},
		{"body armour with", []poe1.Item{lifePlate}, 100},
		{"helmet with", []poe1.Item{lifeHelmet, plate}, 115},
		{"no items again", nil, 115},
	} {
		setItems(t, sh, tc.items...)
		if got := value(t, sh, "maximum_life"); got != tc.want {
			t.Errorf("%s: maximum life %v, want %v", tc.name, got, tc.want)
		}
	}
}
