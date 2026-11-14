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
	"github.com/xaroth/pbt-engine/pob"
)

// TestFromXMLItemsReal imports a 3.29 build from pbt-conformance with an item in every
// equipment slot, weapon swap included, and a jewel in the tree, on the pinned dataset.
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
	for _, w := range warnings {
		if w.Code == pob.ItemIgnored {
			t.Errorf("warning %v", w)
		}
	}
	_, checked, err := build.Check(b, reg.Stores("poe1")[0])
	if err != nil {
		t.Fatal(err)
	}
	leftOut := []build.Code{build.ItemSlotUnknown, build.ItemBaseUnknown, build.ItemSlotMismatch}
	for _, w := range checked {
		if slices.Contains(leftOut, w.Code) {
			t.Errorf("check: %v", w)
		}
	}
}

// FromXML keeps a ring in PoB's third ring slot, and Check keeps it for a build with The Unseen
// Hand of the King in the Mists bloodline, which opens the slot.
func TestFromXMLThirdRing(t *testing.T) {
	t.Parallel()
	reg := &dataset.Registry{}
	if _, err := reg.Add(realdata.Record(t, realdata.Current)); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join("testdata", "king-in-the-mists-ring3.xml"))
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
	c, checked, err := build.Check(b, reg.Stores("poe1")[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(checked) > 0 {
		t.Errorf("check: %v", checked)
	}
	if got := c.Build().Items[build.SlotRing3].Name; got != "Rune Loop" {
		t.Errorf("ring-3 holds %q, want Rune Loop; items %v", got, c.Build().Items)
	}
}
