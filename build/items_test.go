package build_test

import (
	"encoding/json"
	"math/rand/v2"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/dataset"
)

const (
	helmetBase = "Metadata/Items/Armours/Helmets/HelmetStr10"
	ringBase   = "Metadata/Items/Rings/Ring1"
	shieldBase = "Metadata/Items/Armours/Shields/ShieldStr1"
	bowBase    = "Metadata/Items/Weapons/TwoHandWeapons/Bows/Bow1"
	swordBase  = "Metadata/Items/Weapons/TwoHandWeapons/TwoHandSwords/TwoHandSword1"
	clawBase   = "Metadata/Items/Weapons/OneHandWeapons/Claws/Claw1"
	quiverBase = "Metadata/Items/Quivers/Quiver1"
)

// itemsStore is testStore with an items group of one helmet, ring, shield, bow, two-handed
// sword, claw and quiver base.
func itemsStore(t *testing.T) *dataset.Store {
	t.Helper()
	d := testDataset()
	d.Items = &dataset.Items{
		Classes: []dataset.ItemClass{
			{ID: "Helmet"}, {ID: "Ring"}, {ID: "Shield"}, {ID: "Bow"}, {ID: "Two Hand Sword"}, {ID: "Claw"}, {ID: "Quiver"},
		},
		Bases: []dataset.ItemBase{
			{ID: helmetBase, Class: "Helmet", Armour: &dataset.ArmourValues{Armour: dataset.ValueRange{Min: 300, Max: 400}}},
			{ID: ringBase, Class: "Ring"},
			{ID: shieldBase, Class: "Shield"},
			{ID: bowBase, Class: "Bow"},
			{ID: swordBase, Class: "Two Hand Sword"},
			{ID: clawBase, Class: "Claw"},
			{ID: quiverBase, Class: "Quiver"},
		},
	}
	return openStore(t, d)
}

// helmet is a rare helmet whose ids all fit itemsStore.
func helmet() build.Item {
	return build.Item{
		Rarity: build.Rare, Name: "Foe Horn", Base: helmetBase, ItemLevel: 85, Quality: 20, Sockets: "R-R-R-R",
		Armour: 767, Mods: []build.ModLine{
			{Kind: build.Explicit, Text: "+101 to maximum Life", Stats: []build.StatValue{{Stat: "base_maximum_life", Value: 101}}},
		},
	}
}

func TestItemRoundTrip(t *testing.T) {
	t.Parallel()
	b := juggernaut()
	b.Nodes = []uint32{1, 3, 10}
	if err := b.Apply(build.Equip(build.SlotHelmet, helmet())); err != nil {
		t.Fatal(err)
	}
	data, err := b.Encode()
	if err != nil {
		t.Fatal(err)
	}
	got, err := build.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(b, got); diff != "" {
		t.Errorf("Parse(Encode) (-want +got):\n%s\n%s", diff, data)
	}

	e := build.Equip(build.SlotRing1, helmet())
	data, err = json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	var back build.Edit
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(e, back); diff != "" {
		t.Errorf("edit round trip (-want +got):\n%s", diff)
	}
}

// Equip copies the item, Clone copies the items, and Unequip empties the slot.
func TestEquipCopies(t *testing.T) {
	t.Parallel()
	it := helmet()
	e := build.Equip(build.SlotHelmet, it)
	it.Mods[0].Stats[0].Value = 1
	b := juggernaut()
	if err := b.Apply(e); err != nil {
		t.Fatal(err)
	}
	c := b.Clone()
	c.Items[build.SlotHelmet].Mods[0].Stats[0].Value = 2
	if diff := cmp.Diff(helmet(), b.Items[build.SlotHelmet]); diff != "" {
		t.Errorf("item changed (-want +got):\n%s", diff)
	}
	if err := b.Apply(build.Unequip(build.SlotHelmet)); err != nil || len(b.Items) != 0 {
		t.Errorf("unequip: %v, %v", b.Items, err)
	}
}

func randItems(r *rand.Rand) build.Items {
	var out build.Items
	for _, s := range []build.Slot{build.SlotHelmet, build.SlotRing1, build.SlotBelt} {
		if r.IntN(2) == 0 {
			continue
		}
		if out == nil {
			out = build.Items{}
		}
		it := helmet()
		it.Quality = r.IntN(3)
		it.Mods[0].Stats[0].Value = int32(r.IntN(3))
		out[s] = it
	}
	return out
}

// TestCheckItems: each item check warns and fixes as its message says.
func TestCheckItems(t *testing.T) {
	t.Parallel()
	s := itemsStore(t)
	valid := func() *build.Build {
		b := juggernaut()
		b.Items = build.Items{build.SlotHelmet: helmet()}
		return b
	}
	if _, warnings, err := build.Check(valid(), s); err != nil || len(warnings) != 0 {
		t.Fatalf("valid items: %v, %v", warnings, err)
	}
	unread := helmet()
	unread.Mods = append(unread.Mods,
		build.ModLine{Kind: build.Explicit, Text: "Grants a wish"},
		build.ModLine{Kind: build.Crafted, Text: "+1 to Gone", Stats: []build.StatValue{{Stat: "gone", Value: 1}}})
	otherBase := helmet()
	otherBase.Base = "Metadata/Items/Nothing"
	leftOut := func(slot build.Slot, code build.Code, msg string) []build.Warning {
		return []build.Warning{{Code: code, Subject: "item " + string(slot), Message: msg, Slot: slot}}
	}
	testCases := []struct {
		name     string
		items    build.Items
		warnings []build.Warning
		want     build.Items
	}{
		{
			"no such slot", build.Items{"flask-1": helmet()},
			leftOut("flask-1", build.ItemSlotUnknown, `no item slot "flask-1"; item left out`), build.Items{},
		},
		{
			"unknown base", build.Items{build.SlotHelmet: otherBase},
			leftOut(build.SlotHelmet, build.ItemBaseUnknown, "base Metadata/Items/Nothing not in 3.29.3.3; item left out"), build.Items{},
		},
		{
			"helmet as a ring", build.Items{build.SlotRing2: helmet()},
			leftOut(build.SlotRing2, build.ItemSlotMismatch, "a Helmet does not fit the slot; item left out"), build.Items{},
		},
		{
			"unread line and unknown stat", build.Items{build.SlotHelmet: unread},
			[]build.Warning{
				{Code: build.ItemLineUnread, Subject: "item helmet", Message: `line "Grants a wish" not read as stats; left out`, Slot: build.SlotHelmet},
				{
					Code: build.ItemStatUnknown, Subject: "item helmet", Message: `stat gone of line "+1 to Gone" not in 3.29.3.3; left out`,
					Slot: build.SlotHelmet, Stat: "gone",
				},
			},
			build.Items{build.SlotHelmet: func() build.Item {
				it := unread.Clone()
				it.Mods[2].Stats = []build.StatValue{}
				return *it
			}()},
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
			if diff := cmp.Diff(testCase.warnings, warnings); diff != "" {
				t.Errorf("warnings (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(testCase.want, c.Build().Items); diff != "" {
				t.Errorf("items (-want +got):\n%s", diff)
			}
		})
	}

	_, warnings, err := build.Check(valid(), testStore(t))
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff([]build.Code{build.ItemsUnchecked}, codesOf(warnings)); diff != "" {
		t.Errorf("no items group (-want +got):\n%s", diff)
	}
}

// The weapon-swap slots are not in use: the checked build leaves their items out and disables
// the gems socketed in them, without a warning; the build itself keeps them.
func TestCheckSwapSlots(t *testing.T) {
	t.Parallel()
	shield := build.Item{Rarity: build.Rare, Base: shieldBase, Mods: helmet().Mods}
	gems := func() []build.Gem {
		return []build.Gem{{ID: "Metadata/Items/Gems/SkillGemFireball", Level: 20}}
	}
	b := juggernaut()
	b.Items = build.Items{build.SlotOffhand: shield, build.SlotOffhandSwap: shield}
	b.Skills.Groups = []build.SkillGroup{
		{Slot: build.SlotOffhand, Gems: gems()}, {Slot: build.SlotWeaponSwap, Gems: gems()}, {Slot: build.SlotOffhandSwap, Gems: gems()},
	}
	before := b.Clone()
	c, warnings, err := build.Check(b, itemsStore(t))
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff([]build.Code{build.SkillsUnchecked}, codesOf(warnings)); diff != "" {
		t.Errorf("codes (-want +got):\n%s", diff)
	}
	cb := c.Build()
	if diff := cmp.Diff(build.Items{build.SlotOffhand: shield}, cb.Items); diff != "" {
		t.Errorf("items (-want +got):\n%s", diff)
	}
	for i, want := range []bool{false, true, true} {
		if got := cb.Skills.Groups[i].Gems[0].Disabled; got != want {
			t.Errorf("group %d in %s: disabled %v, want %v", i, cb.Skills.Groups[i].Slot, got, want)
		}
	}
	if diff := cmp.Diff(before, b); diff != "" {
		t.Errorf("build changed (-before +after):\n%s", diff)
	}
}
