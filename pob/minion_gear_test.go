package pob_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/internal/realdata"
	"github.com/xaroth/pbt-engine/pob"
)

// gearOf names the items of a group's minion gear by slot.
type gearOf struct {
	count   int
	fullDPS bool
	gear    map[build.Slot]string
}

// TestFromXMLMinionGear imports a file with two item sets: Animate Guardian's gem names set 2,
// and so does the Animate Guardian's Weapon that the body armour grants, counted 14 times. Each
// group takes the items of set 2 in the slots its minion uses, its count and its full DPS flag;
// the ring in set 2 no minion uses is left out with a warning. Check keeps it all.
func TestFromXMLMinionGear(t *testing.T) {
	t.Parallel()
	reg := &dataset.Registry{}
	if _, err := reg.Add(realdata.Record(t, realdata.Current)); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join("testdata", "animate-guardian-gear.xml"))
	if err != nil {
		t.Fatal(err)
	}
	b, warnings, err := pob.FromXML(data, reg)
	if err != nil {
		t.Fatal(err)
	}
	ring := build.Warning{Code: pob.ItemIgnored, Subject: "Iron Ring", Message: "not equipped in the active item set; not imported"}
	if want := []build.Warning{ring}; !cmp.Equal(want, warnings) {
		t.Errorf("warnings %v, want %v", warnings, want)
	}
	want := []gearOf{
		{fullDPS: true, gear: map[build.Slot]string{build.SlotWeapon: "Gale Edge", build.SlotHelmet: "Grim Visor"}},
		{count: 14, fullDPS: true, gear: map[build.Slot]string{build.SlotWeapon: "Gale Edge"}},
	}
	if diff := cmp.Diff(want, groupGear(b), cmp.AllowUnexported(gearOf{})); diff != "" {
		t.Errorf("groups (-want +got):\n%s", diff)
	}
	if b.Items[build.SlotHelmet].Name != "Doom Crown" {
		t.Errorf("character's helmet %q, want Doom Crown", b.Items[build.SlotHelmet].Name)
	}
	_, checked, err := build.Check(b, reg.Stores("poe1")[0])
	if err != nil || len(checked) != 0 {
		t.Errorf("check: %v, %v", checked, err)
	}
}

func groupGear(b *build.Build) []gearOf {
	out := make([]gearOf, 0, len(b.Skills.Groups))
	for _, g := range b.Skills.Groups {
		got := gearOf{count: g.Count, fullDPS: g.FullDPS}
		for s, it := range g.MinionItems {
			if got.gear == nil {
				got.gear = map[build.Slot]string{}
			}
			got.gear[s] = it.Name
		}
		out = append(out, got)
	}
	return out
}
