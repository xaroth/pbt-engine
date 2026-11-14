package build_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/dataset"
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

// TestCheckThirdRing: the third ring slot holds a ring when an allocated node or a chosen
// mastery effect grants the stat that opens it; otherwise the ring is left out with a warning.
func TestCheckThirdRing(t *testing.T) {
	t.Parallel()
	d := itemsDataset()
	d.Stats = append(d.Stats, dataset.Stat{ID: build.SlotStat(build.SlotRing3)})
	opens := []dataset.StatValue{{Stat: uint32(len(d.Stats) - 1), Value: 1}}
	tree := &d.Trees[0]
	for i := range tree.Nodes {
		switch n := &tree.Nodes[i]; n.ID {
		case 52:
			n.Stats = opens
		case 4:
			n.MasteryEffects[1].Stats = opens
		}
	}
	s := openStore(t, d)
	ring := build.Item{Rarity: build.Normal, Base: ringBase}
	testCases := []struct {
		name      string
		nodes     []uint32
		masteries map[uint32]uint32
		codes     []build.Code
		want      build.Items
	}{
		{"bloodline node", []uint32{50, 52}, nil, nil, build.Items{build.SlotRing3: ring}},
		{"mastery effect", []uint32{4}, map[uint32]uint32{4: 8}, nil, build.Items{build.SlotRing3: ring}},
		{"other bloodline node", []uint32{50, 51}, nil, []build.Code{build.ItemSlotClosed}, build.Items{}},
		{"other mastery effect", []uint32{4}, map[uint32]uint32{4: 7}, []build.Code{build.ItemSlotClosed}, build.Items{}},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			b := juggernaut()
			b.Bloodline = "Oshabi"
			b.Nodes = append(b.Nodes, testCase.nodes...)
			b.Masteries = testCase.masteries
			b.Items = build.Items{build.SlotRing3: ring}
			c, warnings, err := build.Check(b, s)
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(testCase.codes, codesOf(warnings)); diff != "" {
				t.Errorf("codes (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(testCase.want, c.Build().Items); diff != "" {
				t.Errorf("items (-want +got):\n%s", diff)
			}
			if _, kept := testCase.want[build.SlotRing3]; c.SlotOpen(build.SlotRing3) != kept {
				t.Errorf("SlotOpen(ring-3) = %t, want %t", !kept, kept)
			}
		})
	}
}

// TestCheckClosedSlots: an allocated node that grants the stats closing the amulet and belt
// slots leaves the items in them out with a warning and marks the slots closed; without it they
// stay.
func TestCheckClosedSlots(t *testing.T) {
	t.Parallel()
	d := itemsDataset()
	closes := make([]dataset.StatValue, 0, 2)
	for _, s := range []build.Slot{build.SlotAmulet, build.SlotBelt} {
		d.Stats = append(d.Stats, dataset.Stat{ID: build.SlotDisableStat(s)})
		closes = append(closes, dataset.StatValue{Stat: uint32(len(d.Stats) - 1), Value: 1})
	}
	tree := &d.Trees[0]
	for i := range tree.Nodes {
		if n := &tree.Nodes[i]; n.ID == 51 {
			n.Stats = closes
		}
	}
	s := openStore(t, d)
	items := build.Items{
		build.SlotAmulet: {Rarity: build.Normal, Base: amuletBase},
		build.SlotBelt:   {Rarity: build.Normal, Base: beltBase},
	}
	for _, testCase := range []struct {
		name  string
		nodes []uint32
		codes []build.Code
		want  build.Items
	}{
		{"closing node", []uint32{50, 51}, []build.Code{build.ItemSlotClosed, build.ItemSlotClosed}, build.Items{}},
		{"other node", []uint32{50, 52}, nil, items},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			b := juggernaut()
			b.Bloodline = "Oshabi"
			b.Nodes = append(b.Nodes, testCase.nodes...)
			b.Items = items
			c, warnings, err := build.Check(b, s)
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(testCase.codes, codesOf(warnings)); diff != "" {
				t.Errorf("codes (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(testCase.want, c.Build().Items); diff != "" {
				t.Errorf("items (-want +got):\n%s", diff)
			}
			open := len(testCase.want) > 0
			for _, slot := range []build.Slot{build.SlotAmulet, build.SlotBelt} {
				if c.SlotOpen(slot) != open {
					t.Errorf("SlotOpen(%s) = %t, want %t", slot, !open, open)
				}
			}
			if !c.SlotOpen(build.SlotHelmet) {
				t.Error("SlotOpen(helmet) = false")
			}
		})
	}
}

// TestCheckAbyssJewels: an abyss jewel in a belt's abyssal socket is kept; one past the belt's
// abyssal sockets, or one that is not an abyss jewel, is left out with a warning.
func TestCheckAbyssJewels(t *testing.T) {
	t.Parallel()
	d := itemsDataset()
	d.Items.Classes = append(d.Items.Classes, dataset.ItemClass{ID: "Jewel"}, dataset.ItemClass{ID: "AbyssJewel"})
	d.Items.Bases = append(d.Items.Bases,
		dataset.ItemBase{ID: jewelBase, Class: "Jewel"}, dataset.ItemBase{ID: abyssBase, Class: "AbyssJewel"},
	)
	s := openStore(t, d)
	belt := func(jewels map[int]build.Item) build.Item {
		return build.Item{Rarity: build.Rare, Name: "Victory Bond", Base: beltBase, Sockets: "A", AbyssJewels: jewels}
	}
	testCases := []struct {
		name   string
		jewels map[int]build.Item
		want   []build.Warning
		kept   map[int]build.Item
	}{
		{
			name:   "abyss jewel in the socket",
			jewels: map[int]build.Item{1: jewel(abyssBase)},
			kept:   map[int]build.Item{1: jewel(abyssBase)},
		},
		{
			name:   "one more jewel than sockets",
			jewels: map[int]build.Item{1: jewel(abyssBase), 2: jewel(abyssBase)},
			want: []build.Warning{{
				Code: build.AbyssJewelNoSocket, Subject: "abyss jewel belt 2", Slot: build.SlotBelt,
				Message: "the item has 1 abyssal sockets, no socket 2; jewel left out",
			}},
			kept: map[int]build.Item{1: jewel(abyssBase)},
		},
		{
			name:   "not an abyss jewel",
			jewels: map[int]build.Item{1: jewel(jewelBase)},
			want: []build.Warning{{
				Code: build.JewelKindMismatch, Subject: "abyss jewel belt 1", Slot: build.SlotBelt,
				Message: "a Jewel does not fit an abyssal socket; left out",
			}},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			b := juggernaut()
			b.Items = build.Items{build.SlotBelt: belt(testCase.jewels)}
			c, warnings, err := build.Check(b, s)
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(testCase.want, warnings); diff != "" {
				t.Errorf("warnings (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(build.Items{build.SlotBelt: belt(testCase.kept)}, c.Build().Items); diff != "" {
				t.Errorf("items (-want +got):\n%s", diff)
			}
		})
	}
}
