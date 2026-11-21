package poe1

import (
	"fmt"
	"slices"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/internal/core"
	"github.com/xaroth/pbt-engine/internal/poe1/itemdefence"
	"github.com/xaroth/pbt-engine/internal/resolve"
)

// Item is an equipped item or a socketed jewel compiled for a sheet: its stats with the local
// ones resolved on the item, the rest the character's.
type Item struct {
	Slot build.Slot
	// Node is a jewel's socket node; 0 for an equipped item, which has a Slot instead.
	Node uint32
	Name string
	// Passives is, for a jewel whose effect grows with them, the allocated passives between its
	// socket and the class start; 0 for others.
	Passives int
	// Key identifies the item's content; a sheet keeps one source per slot and key.
	Key   string
	Stats []resolve.StatValue
	// Weapon is the item's own weapon values; nil for an item that is not a weapon.
	Weapon *Weapon
	// Shield is true for a shield.
	Shield bool
	// Armour is the item's own armour; 0 for an item without.
	Armour float64
	// LifeModifiers is true for an item with a Life Modifier (ModTags.LifeModifiers).
	LifeModifiers bool
	// MovementPenalty is its armour base's movement speed penalty as an increase, such as -5;
	// armour_movement_penalties_are_ignored lifts it.
	MovementPenalty float64
}

// CompiledItem is an item compiled for a sheet.
type CompiledItem struct {
	// Stats are the stats it gives the character.
	Stats []resolve.StatValue
	// Weapon is its own weapon values, nil for an item that is not a weapon.
	Weapon *Weapon
	// Shield is true for a shield.
	Shield bool
	// Armour is its own armour, 0 for an item without.
	Armour float64
	// Assumed lists the defences taken at the highest roll of a base whose roll varies.
	Assumed []string
	// MovementPenalty is as Item's.
	MovementPenalty float64
}

// shieldClass is the ItemClass id of shields.
const shieldClass = "Shield"

// defencelessUniques are the uniques on armour bases that have no defences, by name. The
// dataset does not say which items these are: their hidden mod (RemoveStatsFromTabulaRasa and
// the like) is not in their text. The list is PoB's (Item.lua, BuildModList).
func defencelessUniques() []string {
	return []string{"Tabula Rasa", "Skin of the Loyal", "Skin of the Lords", "The Apostate"}
}

// CompileItem returns what it gives the character on base: each defence of an armour base as one
// flat stat (compileDefences), a weapon's own values with its local stats applied
// (compileWeapon), the rest of its stats as they are. A defenceless unique (defencelessUniques)
// has no defences, whatever its text gives. A shield's chance to block is one flat stat
// (shieldBlock). An armour base's movement speed penalty is kept apart, as MovementPenalty.
func CompileItem(it *build.Item, base *dataset.ItemBase) CompiledItem {
	c := CompiledItem{Shield: base.Class == shieldClass}
	sums := statSums(it)
	local := map[string]bool{}
	if c.Weapon = compileWeapon(it, base, sums); c.Weapon != nil {
		local = localWeaponStats()
	}
	if base.Armour != nil {
		c.MovementPenalty = float64(base.Armour.MovementSpeed)
		if !defenceless(it) {
			c.Stats, c.Assumed = compileDefences(it, base.Armour)
			c.Armour = armourOf(c.Stats)
		}
	}
	if c.Shield {
		if b := shieldBlock(base.Block, sums); b != 0 {
			c.Stats = append(c.Stats, resolve.StatValue{Stat: statShieldBlock, Value: b})
		}
	}
	for _, m := range it.Mods {
		for _, sv := range m.Stats {
			if !local[sv.Stat] && !itemdefence.IsLocal(sv.Stat) && !(c.Shield && isLocalBlock(sv.Stat)) {
				c.Stats = append(c.Stats, resolve.StatValue{Stat: sv.Stat, Value: float64(sv.Value)})
			}
		}
	}
	return c
}

// compileDefences returns each sheet defence of it on a base with values a as one flat stat, and
// the defences taken at the highest roll. A defence with a roll is computed from it
// (itemdefence); one without is taken as its text prints it, or else computed at the highest
// roll, as Path of Building does.
func compileDefences(it *build.Item, a *dataset.ArmourValues) (stats []resolve.StatValue, assumed []string) {
	l := itemdefence.LocalOf(it)
	for _, d := range itemdefence.All() {
		if d.Stat == "" {
			continue
		}
		roll, rolled := it.DefenceRolls[d.Key]
		var v float64
		switch p := d.Printed(it); {
		case rolled:
			v = d.Value(a, l, roll)
		case p != 0:
			v = float64(p)
		default:
			v = d.Value(a, l, 1)
			if v != 0 && d.Varies(a) {
				assumed = append(assumed, d.Name)
			}
		}
		if v != 0 {
			stats = append(stats, resolve.StatValue{Stat: d.Stat, Value: v})
		}
	}
	return stats, assumed
}

// armourOf returns the armour of an item's compiled defences.
func armourOf(defences []resolve.StatValue) float64 {
	var armour string
	for _, d := range itemdefence.All() {
		if d.Key == build.DefenceArmour {
			armour = d.Stat
		}
	}
	for _, sv := range defences {
		if sv.Stat == armour {
			return sv.Value
		}
	}
	return 0
}

func defenceless(it *build.Item) bool {
	return it.Rarity == build.Unique && slices.Contains(defencelessUniques(), it.Name)
}

func statSums(it *build.Item) map[string]float64 {
	sums := map[string]float64{}
	for _, m := range it.Mods {
		for _, sv := range m.Stats {
			sums[sv.Stat] += float64(sv.Value)
		}
	}
	return sums
}

type itemKey struct {
	slot build.Slot
	node uint32
	key  string
}

// SetItems equips items and jewels: their sources go on, other items' off, and the engine stats
// of the equipped items follow them (setEquipment); what they put in the character's hands is
// set apart (SetHands). An item not seen yet in its slot or socket gets
// its source added once; adding is refused while a checkpoint is open
// (ErrCheckpointOpen).
func (sh *Sheet) SetItems(items []Item) error {
	want := make(map[itemKey]bool, len(items))
	for _, it := range items {
		want[itemKey{it.Slot, it.Node, it.Key}] = true
	}
	for k, src := range sh.items {
		sh.setSources([]core.SourceID{src}, want[k])
	}
	sh.setEquipment(items)
	for _, it := range items {
		k := itemKey{it.Slot, it.Node, it.Key}
		if _, ok := sh.items[k]; ok {
			continue
		}
		var source resolve.Source = resolve.ItemSource{Slot: it.Slot, Name: it.Name}
		if it.Node != 0 {
			source = resolve.JewelSource{Node: it.Node, Name: it.Name, Passives: it.Passives}
		}
		if sh.marked {
			return fmt.Errorf("%w: %s", ErrCheckpointOpen, source)
		}
		bd := resolve.Bundle{Actor: resolve.Player, Scope: resolve.Global, Source: source, Stats: it.Stats}
		src, err := sh.add(&bd, sh.table, sh.gameVersion, -1)
		if err != nil {
			return err
		}
		if err := sh.addMovementPenalty(it.MovementPenalty, src); err != nil {
			return err
		}
		if sh.items == nil {
			sh.items = map[itemKey]core.SourceID{}
		}
		sh.items[k] = src
	}
	return nil
}

// addMovementPenalty adds an armour base's movement speed penalty to src, unless armour
// movement penalties are ignored.
func (sh *Sheet) addMovementPenalty(penalty float64, src core.SourceID) error {
	if penalty == 0 {
		return nil
	}
	mod := core.Modifier{
		Stat: sh.stats[statMovementSpeed], Op: core.OpInc, Value: penalty, Source: src,
		Conds: []core.Cond{{Ref: core.SelfRef(sh.stats[statArmourMovementPenaltiesAreIgnored]), Cmp: core.CmpEQ}},
	}
	if _, err := sh.session.AddModifier(mod); err != nil {
		return fmt.Errorf("sheet: movement penalty: %w", err)
	}
	return nil
}
