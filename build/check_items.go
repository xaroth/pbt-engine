package build

import (
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/xaroth/pbt-engine/dataset"
)

// Warning codes of Check for items.
const (
	ItemsUnchecked     Code = "items-unchecked"
	ItemSlotUnknown    Code = "item-slot-unknown"
	ItemBaseUnknown    Code = "item-base-unknown"
	ItemSlotMismatch   Code = "item-slot-mismatch"
	ItemLineUnread     Code = "item-line-unread"
	ItemStatUnknown    Code = "item-stat-unknown"
	ItemOffhandBlocked Code = "item-offhand-blocked"
	ItemQuiverNoBow    Code = "item-quiver-no-bow"
)

// oneHandClasses are the ItemClass.IDs of weapons that can be held in either hand.
func oneHandClasses() []string {
	return []string{
		"Claw", "Dagger", "Rune Dagger", "Wand", "One Hand Sword", "Thrusting One Hand Sword", "One Hand Axe",
		"One Hand Mace", "Sceptre",
	}
}

// twoHandClasses are the ItemClass.IDs of weapons that take both hands.
func twoHandClasses() []string {
	return []string{"Bow", "Staff", "Warstaff", "Two Hand Sword", "Two Hand Axe", "Two Hand Mace", "FishingRod"}
}

// slotClasses returns the ItemClass.IDs an item in slot s can be of.
func slotClasses(s Slot) []string {
	switch s {
	case SlotWeapon, SlotWeaponSwap:
		return append(oneHandClasses(), twoHandClasses()...)
	case SlotOffhand, SlotOffhandSwap:
		return append(oneHandClasses(), "Shield", "Quiver")
	case SlotHelmet:
		return []string{"Helmet"}
	case SlotBodyArmour:
		return []string{"Body Armour"}
	case SlotGloves:
		return []string{"Gloves"}
	case SlotBoots:
		return []string{"Boots"}
	case SlotAmulet:
		return []string{"Amulet"}
	case SlotRing1, SlotRing2:
		return []string{"Ring"}
	case SlotBelt:
		return []string{"Belt"}
	}
	return nil
}

// checkItems checks the items against the dataset's items group. Without one, it warns once
// and keeps the items as they are.
func (c *Checked) checkItems() error {
	if len(c.build.Items) == 0 {
		return nil
	}
	data, err := c.store.Items()
	if _, missing := errors.AsType[*dataset.MissingGroupError](err); missing {
		c.warnf(ItemsUnchecked, "items", "dataset %s has no items group; items kept unchecked", c.store.Manifest().Version)
		return nil
	}
	if err != nil {
		return fmt.Errorf("build: load items: %w", err)
	}
	if _, err := c.store.Stats(); err != nil {
		return fmt.Errorf("build: load stats: %w", err)
	}
	for _, s := range slices.Sorted(maps.Keys(c.build.Items)) {
		it := c.build.Items[s]
		if !c.checkItem(s, &it, data) {
			delete(c.build.Items, s)
			continue
		}
		c.build.Items[s] = it
	}
	c.checkHands(data)
	return nil
}

// checkHands leaves out an offhand item the game refuses beside the weapon: anything beside a
// two-handed weapon other than a quiver beside a bow, and a quiver beside a weapon that is not
// a bow. A quiver with no weapon is kept, as the game keeps it when the bow comes off.
func (c *Checked) checkHands(data *dataset.Items) {
	off, ok := c.build.Items[SlotOffhand]
	if !ok {
		return
	}
	subject := "item " + string(SlotOffhand)
	offClass := baseClass(data, off.Base)
	weapon, armed := c.build.Items[SlotWeapon]
	weaponClass := baseClass(data, weapon.Base)
	switch {
	case armed && weaponClass == "Bow" && offClass == "Quiver":
	case armed && slices.Contains(twoHandClasses(), weaponClass):
		c.warnf(ItemOffhandBlocked, subject, "a %s beside a two-handed %s, which the game refuses; offhand left out", offClass, weaponClass)
		delete(c.build.Items, SlotOffhand)
	case offClass != "Quiver":
	case armed:
		c.warnf(ItemQuiverNoBow, subject, "a Quiver beside a %s, which the game refuses; left out", weaponClass)
		delete(c.build.Items, SlotOffhand)
	default:
		c.warnf(ItemQuiverNoBow, subject, "a Quiver with no bow equipped; kept")
	}
}

// baseClass returns the ItemClass.ID of base id, or "" when data has no such base.
func baseClass(data *dataset.Items, id string) string {
	if i := slices.IndexFunc(data.Bases, func(b dataset.ItemBase) bool { return b.ID == id }); i >= 0 {
		return data.Bases[i].Class
	}
	return ""
}

// checkItem fixes it as the dataset says, or reports false to unequip it.
func (c *Checked) checkItem(s Slot, it *Item, data *dataset.Items) bool {
	subject := "item " + string(s)
	if !slices.Contains(ItemSlots(), s) {
		c.warnf(ItemSlotUnknown, subject, "no item slot %q; item left out", s)
		return false
	}
	i := slices.IndexFunc(data.Bases, func(b dataset.ItemBase) bool { return b.ID == it.Base })
	if i < 0 {
		c.warnf(ItemBaseUnknown, subject, "base %s not in %s; item left out", it.Base, c.store.Manifest().Version)
		return false
	}
	if class := data.Bases[i].Class; !slices.Contains(slotClasses(s), class) {
		c.warnf(ItemSlotMismatch, subject, "a %s does not fit the slot; item left out", class)
		return false
	}
	for j := range it.Mods {
		m := &it.Mods[j]
		if len(m.Stats) == 0 {
			c.warnf(ItemLineUnread, subject, "line %q not read as stats; left out", m.Text)
			continue
		}
		m.Stats = slices.DeleteFunc(m.Stats, func(sv StatValue) bool {
			// The stats group loaded above, so HasStat cannot fail.
			if ok, _ := c.store.HasStat(sv.Stat); ok {
				return false
			}
			c.warnf(ItemStatUnknown, subject, "stat %s of line %q not in %s; left out", sv.Stat, m.Text, c.store.Manifest().Version)
			return true
		})
	}
	return true
}

// setAsideSwap leaves out the items in the weapon-swap slots and disables the gems socketed in
// them.
func (c *Checked) setAsideSwap() {
	for s := range c.build.Items {
		if s.Swap() {
			delete(c.build.Items, s)
		}
	}
	for i := range c.build.Skills.Groups {
		g := &c.build.Skills.Groups[i]
		if !g.Slot.Swap() {
			continue
		}
		for j := range g.Gems {
			g.Gems[j].Disabled = true
		}
	}
}
