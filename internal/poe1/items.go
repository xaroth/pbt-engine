package poe1

import (
	"fmt"
	"maps"
	"math"
	"slices"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/internal/core"
	"github.com/xaroth/pbt-engine/internal/resolve"
)

// Item is an equipped item or a socketed jewel compiled for a sheet: its stats with the local
// ones resolved on the item, the rest the character's.
type Item struct {
	Slot build.Slot
	// Node is a jewel's socket node; 0 for an equipped item, which has a Slot instead.
	Node uint32
	Name string
	// Key identifies the item's content; a sheet keeps one source per slot and key.
	Key   string
	Stats []resolve.StatValue
	// Weapon is the item's own weapon values; nil for an item that is not a weapon.
	Weapon *Weapon
	// MovementPenalty is its armour base's movement speed penalty as an increase, such as -5;
	// armour_movement_penalties_are_ignored lifts it.
	MovementPenalty float64
}

// defence is one of an armour base's defences and the local stats that change it.
type defence struct {
	name string
	// given is the item's own value as its text gives it, local modifiers and quality included.
	given func(*build.Item) int
	base  func(*dataset.ArmourValues) dataset.ValueRange
	// stat is the character's stat the item's value adds to.
	stat      string
	flat, inc []string
}

// localDefencesInc is the local increase to all three defences.
const localDefencesInc = "local_armour_and_evasion_and_energy_shield_+%"

func defences() []defence {
	return []defence{
		{
			name: "armour", given: func(it *build.Item) int { return it.Armour },
			base: func(a *dataset.ArmourValues) dataset.ValueRange { return a.Armour },
			stat: "base_physical_damage_reduction_rating",
			flat: []string{"local_base_physical_damage_reduction_rating"},
			inc: []string{
				"local_physical_damage_reduction_rating_+%", "local_armour_and_evasion_+%", "local_armour_and_energy_shield_+%",
				localDefencesInc,
			},
		},
		{
			name: "evasion", given: func(it *build.Item) int { return it.Evasion },
			base: func(a *dataset.ArmourValues) dataset.ValueRange { return a.Evasion },
			stat: "base_evasion_rating",
			flat: []string{"local_base_evasion_rating"},
			inc: []string{
				"local_evasion_rating_+%", "local_armour_and_evasion_+%", "local_evasion_and_energy_shield_+%",
				localDefencesInc,
			},
		},
		{
			name: "energy shield", given: func(it *build.Item) int { return it.EnergyShield },
			base: func(a *dataset.ArmourValues) dataset.ValueRange { return a.EnergyShield },
			stat: "base_maximum_energy_shield",
			flat: []string{"local_energy_shield"},
			inc: []string{
				"local_energy_shield_+%", "local_armour_and_energy_shield_+%", "local_evasion_and_energy_shield_+%",
				localDefencesInc,
			},
		},
	}
}

// CompiledItem is an item compiled for a sheet.
type CompiledItem struct {
	// Stats are the stats it gives the character.
	Stats []resolve.StatValue
	// Weapon is its own weapon values, nil for an item that is not a weapon.
	Weapon *Weapon
	// Assumed lists the defences taken at the highest roll of a base whose roll varies.
	Assumed []string
	// MovementPenalty is as Item's.
	MovementPenalty float64
}

// defencelessUniques are the uniques on armour bases that have no defences, by name. The
// dataset does not say which items these are: their hidden mod (RemoveStatsFromTabulaRasa and
// the like) is not in their text. The list is PoB's (Item.lua, BuildModList).
func defencelessUniques() []string {
	return []string{"Tabula Rasa", "Skin of the Loyal", "Skin of the Lords", "The Apostate"}
}

// CompileItem returns what it gives the character on base: each defence of an armour base as one
// flat stat, a weapon's own values with its local stats applied (compileWeapon), the rest of its
// stats as they are. A defence the item's text gives is taken as it is; otherwise it is the
// base's highest roll plus the local flat stats, times the local increases and, separately, the
// quality, rounded half away from zero. Quality multiplying apart from the increases and the
// rounding follow PoB (Item.lua, BuildModListForSlotNum); unvalidated. A defenceless unique
// (defencelessUniques) has none, whatever its text gives. An armour base's movement speed
// penalty is kept apart, as MovementPenalty.
func CompileItem(it *build.Item, base *dataset.ItemBase) CompiledItem {
	var c CompiledItem
	sums := statSums(it)
	local := localDefenceStats()
	if c.Weapon = compileWeapon(it, base, sums); c.Weapon != nil {
		maps.Copy(local, localWeaponStats())
	}
	if base.Armour != nil {
		c.MovementPenalty = float64(base.Armour.MovementSpeed)
		if !defenceless(it) {
			c.Stats, c.Assumed = compileDefences(it, base.Armour, sums)
		}
	}
	for _, m := range it.Mods {
		for _, sv := range m.Stats {
			if !local[sv.Stat] {
				c.Stats = append(c.Stats, resolve.StatValue{Stat: sv.Stat, Value: float64(sv.Value)})
			}
		}
	}
	return c
}

// compileDefences returns each defence of it on a base with values a, given sums of its stats,
// as one flat stat, and the defences taken at the highest roll.
func compileDefences(it *build.Item, a *dataset.ArmourValues, sums map[string]float64) (stats []resolve.StatValue, assumed []string) {
	for _, d := range defences() {
		v, rolled := d.value(it, a, sums)
		if rolled {
			assumed = append(assumed, d.name)
		}
		if v != 0 {
			stats = append(stats, resolve.StatValue{Stat: d.stat, Value: v})
		}
	}
	return stats, assumed
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

// localDefenceStats are the stats that change an item's own defences.
func localDefenceStats() map[string]bool {
	local := map[string]bool{}
	for _, d := range defences() {
		for _, id := range slices.Concat(d.flat, d.inc) {
			local[id] = true
		}
	}
	return local
}

// value returns the defence of it on a base with values a, given sums of its stats; assumed
// reports a roll taken at its highest.
func (d defence) value(it *build.Item, a *dataset.ArmourValues, sums map[string]float64) (v float64, assumed bool) {
	if g := d.given(it); g != 0 {
		return float64(g), false
	}
	r := d.base(a)
	if r.Max == 0 {
		return 0, false
	}
	flat, inc := float64(r.Max), 0.0
	for _, id := range d.flat {
		flat += sums[id]
	}
	for _, id := range d.inc {
		inc += sums[id]
	}
	return math.Round(flat * (1 + inc/percentScale) * (1 + float64(it.Quality)/percentScale)), r.Min != r.Max
}

type itemKey struct {
	slot build.Slot
	node uint32
	key  string
}

// SetItems equips items and jewels: their sources go on, other items' off; what they put in the
// character's hands is set apart (SetHands). An item not seen yet in its slot or socket gets
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
	for _, it := range items {
		k := itemKey{it.Slot, it.Node, it.Key}
		if _, ok := sh.items[k]; ok {
			continue
		}
		var source resolve.Source = resolve.ItemSource{Slot: it.Slot, Name: it.Name}
		if it.Node != 0 {
			source = resolve.JewelSource{Node: it.Node, Name: it.Name}
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
