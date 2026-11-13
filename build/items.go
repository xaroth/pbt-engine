package build

import (
	"cmp"
	"maps"
	"reflect"
	"slices"
)

// Item slots that have no sockets, beside Slots.
const (
	SlotAmulet Slot = "amulet"
	SlotRing1  Slot = "ring-1"
	SlotRing2  Slot = "ring-2"
	SlotBelt   Slot = "belt"
)

// ItemSlots returns the slots an item can be equipped in.
func ItemSlots() []Slot {
	return append(Slots(), SlotAmulet, SlotRing1, SlotRing2, SlotBelt)
}

// Rarity is an item's rarity.
type Rarity string

// Rarities.
const (
	Normal Rarity = "normal"
	Magic  Rarity = "magic"
	Rare   Rarity = "rare"
	Unique Rarity = "unique"
)

// ModKind says where on an item a mod line is.
type ModKind string

// Mod kinds.
const (
	Implicit  ModKind = "implicit"
	Explicit  ModKind = "explicit"
	Enchant   ModKind = "enchant"
	Crafted   ModKind = "crafted"
	Fractured ModKind = "fractured"
)

// Item is an item as a build holds it: its base by id and its mod lines with the stats read
// from them. Item text is read into an Item where a dataset is at hand; a build never reads
// text.
type Item struct {
	Rarity Rarity `json:"rarity"`
	// Name is a rare or unique item's name, or a magic item's full name.
	Name string `json:"name,omitzero"`
	// Base is the dataset ItemBase.ID.
	Base      string `json:"base"`
	ItemLevel int    `json:"itemLevel,omitzero"`
	Quality   int    `json:"quality,omitzero"`
	// Sockets as the text writes them, such as "R-R-B G".
	Sockets string `json:"sockets,omitzero"`
	// Armour, Evasion, EnergyShield and Ward are the item's own defences where its text gives
	// them, local modifiers and quality included; 0 when it gives none.
	Armour       int `json:"armour,omitzero"`
	Evasion      int `json:"evasion,omitzero"`
	EnergyShield int `json:"energyShield,omitzero"`
	Ward         int `json:"ward,omitzero"`
	// Flags are the item's other properties as the text names them, such as "Corrupted" or
	// "Shaper Item".
	Flags []string `json:"flags,omitzero"`
	// Mods are the mod lines in the text's order.
	Mods []ModLine `json:"mods,omitzero"`
}

// ModLine is one mod line of an item, or the lines of one description that spans several.
type ModLine struct {
	Kind ModKind `json:"kind"`
	// Text is the line as written; lines of one description are joined with "\n".
	Text string `json:"text"`
	// Mod is the dataset Mod.ID the line is a roll of; empty when not known.
	Mod string `json:"mod,omitzero"`
	// Stats are the stats the line was read as; none when it could not be read.
	Stats []StatValue `json:"stats,omitzero"`
}

// StatValue is a stat id with its value.
type StatValue struct {
	Stat  string `json:"stat"`
	Value int32  `json:"value"`
}

// Clone returns a deep copy of it.
func (it *Item) Clone() *Item {
	c := *it
	c.Flags = slices.Clone(it.Flags)
	c.Mods = slices.Clone(it.Mods)
	for i := range c.Mods {
		c.Mods[i].Stats = slices.Clone(c.Mods[i].Stats)
	}
	return &c
}

// Items are a build's equipped items by slot.
type Items map[Slot]Item

// cloneItems returns a deep copy of items by slot or jewels by socket.
func cloneItems[M ~map[K]Item, K comparable](m M) M {
	if m == nil {
		return nil
	}
	c := make(M, len(m))
	for k, it := range m {
		c[k] = *it.Clone()
	}
	return c
}

// Edit ops of items.
const (
	OpEquip   EditOp = "equip"
	OpUnequip EditOp = "unequip"
)

// Equip puts it in slot s, replacing what is there. Whether it fits the slot is not checked
// here; Check warns.
func Equip(s Slot, it Item) Edit { return Edit{Op: OpEquip, Slot: s, Item: it.Clone()} }

// Unequip empties slot s.
func Unequip(s Slot) Edit { return Edit{Op: OpUnequip, Slot: s} }

func (b *Build) applyItem(e Edit) {
	if e.Op == OpUnequip {
		delete(b.Items, e.Slot)
		return
	}
	if b.Items == nil {
		b.Items = Items{}
	}
	var it Item
	if e.Item != nil {
		it = *e.Item.Clone()
	}
	b.Items[e.Slot] = it
}

// itemsDiff returns the edits that turn a into b, items by slot or jewels by socket: places b
// leaves empty, then items b changes or adds, each in key order.
func itemsDiff[K cmp.Ordered](a, b map[K]Item, remove func(K) Edit, put func(K, Item) Edit) []Edit {
	var out []Edit
	for _, k := range slices.Sorted(maps.Keys(a)) {
		if _, ok := b[k]; !ok {
			out = append(out, remove(k))
		}
	}
	for _, k := range slices.Sorted(maps.Keys(b)) {
		if x, ok := a[k]; !ok || !reflect.DeepEqual(x, b[k]) {
			out = append(out, put(k, b[k]))
		}
	}
	return out
}
