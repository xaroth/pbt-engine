package build_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/dataset"
)

// Items allocate the notables their lines name, each once; a line naming a node that is no
// notable of the tree (a small passive, a bloodline's notable, an unknown node) warns and is
// left out.
func TestCheckItemPassives(t *testing.T) {
	t.Parallel()
	const stat = "mod_granted_passive_hash"
	d := itemsDataset()
	d.Stats = append(d.Stats, dataset.Stat{ID: stat})
	s := openStore(t, d)
	allocates := func(name, base string, nodes ...int32) build.Item {
		it := build.Item{Rarity: build.Rare, Name: name, Base: base}
		for _, n := range nodes {
			it.Mods = append(it.Mods, build.ModLine{Kind: build.Crafted, Text: "Allocates", Stats: []build.StatValue{{Stat: stat, Value: n}}})
		}
		return it
	}
	b := juggernaut()
	b.Items = map[build.Slot]build.Item{
		build.SlotAmulet: allocates("Amulet", amuletBase, 41, 3, 52, 999),
		build.SlotRing1:  allocates("Ring", ringBase, 41),
	}
	c, warnings, err := build.Check(b, s)
	if err != nil {
		t.Fatal(err)
	}
	type passive struct {
		Slot build.Slot
		Name string
		Node uint32
	}
	got := make([]passive, 0, len(c.ItemPassives()))
	for _, p := range c.ItemPassives() {
		got = append(got, passive{p.Slot, p.Name, p.Node.ID})
	}
	if diff := cmp.Diff([]passive{{build.SlotAmulet, "Amulet", 41}}, got); diff != "" {
		t.Errorf("item passives (-want +got):\n%s", diff)
	}
	codes := make([]build.Code, 0, len(warnings))
	for _, w := range warnings {
		codes = append(codes, w.Code)
	}
	want := []build.Code{build.ItemNotableUnknown, build.ItemNotableUnknown, build.ItemNotableUnknown}
	if diff := cmp.Diff(want, codes); diff != "" {
		t.Errorf("warnings (-want +got):\n%s\n%v", diff, warnings)
	}
}
