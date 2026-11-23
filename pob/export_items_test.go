package pob_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/internal/realdata"
	"github.com/xaroth/pbt-engine/pob"
)

// TestToXMLItemsReal imports builds with items on the pinned 3.29 dataset, exports them and
// imports them again unchanged, with no item left out. export-items.xml has a rare, a unique
// and a magic item, an abyss jewel in a belt, a jewel in the tree, a group the body armour
// grants and an Animate Guardian group with gear of its own; the others are pbt-conformance
// builds.
func TestToXMLItemsReal(t *testing.T) {
	t.Parallel()
	reg := &dataset.Registry{}
	if _, err := reg.Add(realdata.Record(t, realdata.Current)); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"export-items.xml", "winter-orb-items.xml", "winter-orb-abyss-jewel.xml"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			data, err := os.ReadFile(filepath.Join("testdata", name))
			if err != nil {
				t.Fatal(err)
			}
			b, _, err := pob.FromXML(data, reg)
			if err != nil {
				t.Fatal(err)
			}
			if name == "export-items.xml" {
				checkExportItems(t, b)
			}
			file, warnings, err := pob.ToXML(b, reg)
			if err != nil {
				t.Fatal(err)
			}
			for _, w := range warnings {
				if w.Code == pob.ItemNotExported {
					t.Errorf("export warning %v", w)
				}
			}
			got, _, err := pob.FromXML(file, reg)
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(b, got, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("build (-want +got):\n%s", diff)
			}
		})
	}
}

// checkExportItems checks that b, imported from export-items.xml, holds what the test is to
// export.
func checkExportItems(t *testing.T, b *build.Build) {
	t.Helper()
	type summary struct {
		Rarities                         map[build.Rarity]bool
		AbyssJewels, Jewels, MinionItems int
		Source                           *build.SkillSource
	}
	groups := b.Skills.Groups
	if len(groups) != 2 {
		t.Fatalf("groups %v, want Animate Guardian and the one the body armour grants", groups)
	}
	got := summary{
		Rarities: map[build.Rarity]bool{}, AbyssJewels: len(b.Items[build.SlotBelt].AbyssJewels), Jewels: len(b.Jewels),
		MinionItems: len(groups[0].MinionItems), Source: groups[1].Source,
	}
	for _, it := range b.Items {
		got.Rarities[it.Rarity] = true
	}
	want := summary{
		Rarities:    map[build.Rarity]bool{build.Rare: true, build.Unique: true, build.Magic: true},
		AbyssJewels: 1, Jewels: 1, MinionItems: 2, Source: &build.SkillSource{Item: build.SlotBodyArmour},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("fixture (-want +got):\n%s", diff)
	}
}
