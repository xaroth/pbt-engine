package pob_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/internal/realdata"
	"github.com/xaroth/pbt-engine/items"
	"github.com/xaroth/pbt-engine/pob"
)

// TestFromXMLItemsReal imports a 3.29 build from pbt-conformance with an item in every
// equipment slot, weapon swap included, and a jewel in the tree, on the pinned dataset. The
// jewel is kept in its socket.
func TestFromXMLItemsReal(t *testing.T) {
	t.Parallel()
	reg := &dataset.Registry{}
	if _, err := reg.Add(realdata.Record(t, realdata.Current)); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join("testdata", "winter-orb-items.xml"))
	if err != nil {
		t.Fatal(err)
	}
	b, warnings, err := pob.FromXML(data, reg)
	if err != nil {
		t.Fatal(err)
	}
	names := map[build.Slot]string{}
	for s, it := range b.Items {
		names[s] = it.Name
	}
	want := map[build.Slot]string{
		build.SlotWeapon: "Blight Call", build.SlotOffhand: "Cataclysm Mark",
		build.SlotWeaponSwap: "Vortex Nock", build.SlotOffhandSwap: "Voidfletcher",
		build.SlotHelmet: "Victory Veil", build.SlotBodyArmour: "Tabula Rasa", build.SlotGloves: "Rage Grip",
		build.SlotBoots: "Brood March", build.SlotAmulet: "Torment Collar", build.SlotRing1: "Morbid Knot",
		build.SlotRing2: "Rune Turn", build.SlotBelt: "Behemoth Harness",
	}
	if diff := cmp.Diff(want, names); diff != "" {
		t.Errorf("items by slot (-want +got):\n%s", diff)
	}
	if got := b.Jewels[41263].Name; len(b.Jewels) != 1 || got != "Rapture Essence" {
		t.Errorf("jewels %v, want Rapture Essence in 41263", b.Jewels)
	}
	for _, w := range warnings {
		if w.Code == pob.ItemIgnored || w.Code == pob.JewelIgnored {
			t.Errorf("warning %v", w)
		}
	}
	_, checked, err := build.Check(b, reg.Stores("poe1")[0])
	if err != nil {
		t.Fatal(err)
	}
	leftOut := []build.Code{
		build.ItemSlotUnknown, build.ItemBaseUnknown, build.ItemSlotMismatch, build.JewelNotSocket, build.JewelNotAllocated, build.JewelKindMismatch,
	}
	for _, w := range checked {
		if slices.Contains(leftOut, w.Code) {
			t.Errorf("check: %v", w)
		}
	}
}

// FromXML keeps a ring in PoB's third ring slot and an amulet, and Check, for a build with The
// Unseen Hand of the King in the Mists bloodline, which opens the third ring slot, and the node
// before it, which closes the amulet slot, keeps the ring and leaves out the amulet with a
// warning.
func TestFromXMLKingInTheMistsSlots(t *testing.T) {
	t.Parallel()
	reg := &dataset.Registry{}
	if _, err := reg.Add(realdata.Record(t, realdata.Current)); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join("testdata", "king-in-the-mists-slots.xml"))
	if err != nil {
		t.Fatal(err)
	}
	b, warnings, err := pob.FromXML(data, reg)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) > 0 {
		t.Errorf("warnings %v", warnings)
	}
	if got := b.Items[build.SlotAmulet].Name; got != "Ghoul Choker" {
		t.Errorf("amulet %q, want Ghoul Choker; items %v", got, b.Items)
	}
	c, checked, err := build.Check(b, reg.Stores("poe1")[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(checked) != 1 || checked[0].Code != build.ItemSlotClosed || checked[0].Slot != build.SlotAmulet {
		t.Errorf("check: %v, want the amulet slot closed", checked)
	}
	if got := c.Build().Items[build.SlotRing3].Name; got != "Rune Loop" {
		t.Errorf("ring-3 holds %q, want Rune Loop; items %v", got, c.Build().Items)
	}
	if _, ok := c.Build().Items[build.SlotAmulet]; ok {
		t.Error("amulet kept")
	}
}

// FromXML puts the jewel in PoB's first abyssal socket of the belt into the belt, for a 3.29
// build from pbt-conformance, and Check keeps it.
func TestFromXMLAbyssJewel(t *testing.T) {
	t.Parallel()
	reg := &dataset.Registry{}
	if _, err := reg.Add(realdata.Record(t, realdata.Current)); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join("testdata", "winter-orb-abyss-jewel.xml"))
	if err != nil {
		t.Fatal(err)
	}
	b, warnings, err := pob.FromXML(data, reg)
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range warnings {
		if w.Subject == "Grim Lens" {
			t.Errorf("warning %v", w)
		}
	}
	belt := b.Items[build.SlotBelt]
	if got := belt.AbyssJewels[1].Name; len(belt.AbyssJewels) != 1 || got != "Grim Lens" {
		t.Errorf("belt's abyss jewels %v, want Grim Lens in 1", belt.AbyssJewels)
	}
	c, checked, err := build.Check(b, reg.Stores("poe1")[0])
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range checked {
		if w.Slot == build.SlotBelt {
			t.Errorf("check: %v", w)
		}
	}
	if got := c.Build().Items[build.SlotBelt].AbyssJewels[1].Name; got != "Grim Lens" {
		t.Errorf("checked belt's abyss jewel %q, want Grim Lens", got)
	}
}

// FromXML reads a unique's selected variant at its rolls and passes on the reader's warnings
// about an item, such as a range with no roll, with the item as their subject.
func TestFromXMLItemRolls(t *testing.T) {
	t.Parallel()
	reg := &dataset.Registry{}
	if _, err := reg.Add(realdata.Record(t, realdata.Current)); err != nil {
		t.Fatal(err)
	}
	data := `<PathOfBuilding>
	<Build level="90" className="Witch" ascendClassName="None"/>
	<Tree activeSpec="1"><Spec treeVersion="3_29" classId="3" ascendClassId="0" nodes=""/></Tree>
	<Items activeItemSet="1">
		<Item id="1">Rarity: UNIQUE
Wake of Destruction
Mesh Boots
Variant: Pre 2.6.0
Variant: Pre 3.11.0
Variant: Current
Selected Variant: 3
Implicits: 0
Adds 1 to 120 Lightning Damage to Attacks
{range:0.5}(20-60)% increased Armour and Energy Shield
Gain (10-20) Life per Enemy Killed
{variant:2,3}15% increased Movement Speed
{variant:1,2}10% chance to Cause Monsters to Flee
{variant:3}Drops Shocked Ground while moving, lasting 2 seconds</Item>
		<ItemSet id="1"><Slot name="Boots" itemId="1"/></ItemSet>
	</Items>
</PathOfBuilding>`
	b, warnings, err := pob.FromXML([]byte(data), reg)
	if err != nil {
		t.Fatal(err)
	}
	mods := b.Items[build.SlotBoots].Mods
	lines := make([]string, 0, len(mods))
	for _, m := range mods {
		lines = append(lines, m.Text)
	}
	wantLines := []string{
		"Adds 1 to 120 Lightning Damage to Attacks", "40% increased Armour and Energy Shield",
		"Gain (10-20) Life per Enemy Killed", "15% increased Movement Speed",
		"Drops Shocked Ground while moving, lasting 2 seconds",
	}
	if diff := cmp.Diff(wantLines, lines); diff != "" {
		t.Errorf("boots' lines (-want +got):\n%s", diff)
	}
	want := []build.Warning{{
		Code: items.RollAssumed, Subject: "Wake of Destruction",
		Message: `"Gain (10-20) Life per Enemy Killed": a range of values; the highest used`,
	}}
	if diff := cmp.Diff(want, warnings); diff != "" {
		t.Errorf("warnings (-want +got):\n%s", diff)
	}
}
