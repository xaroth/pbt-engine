package items

import (
	"slices"
	"strings"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/statdesc"
)

// Mods.Domain values as the game numbers them.
const (
	domainItem       = 1
	domainFlask      = 2
	domainCrafted    = 9
	domainJewel      = 10
	domainAbyssJewel = 13
	domainDelve      = 16
	domainUnveiled   = 28
	domainTincture   = 34
)

// generationUnique is Mods.GenerationType of a unique item's mods, and of crafted mods.
const generationUnique = 3

const (
	defaultTag      = "default"
	localStatPrefix = "local_"
	oneHandTag      = "one_hand_weapon"
	twoHandTag      = "two_hand_weapon"
	weaponTag       = "weapon"
)

// choose returns the reading of a line of kind on item it of base b, and the stats of other
// readings that remain possible, which make the line ambiguous. Readings are kept in turn while
// any is left: for an implicit line, those of an implicit mod of the base; those whose stats
// are in a mod that can be on the item with the values in its range, else with any values, as
// for an older roll; on armour and weapons, those with a local stat that a mod rolling on the
// base has, since a unique's local stat can read the same as a global one the unique has; then
// the first in the game's order.
func (r *Reader) choose(it *build.Item, b *dataset.ItemBase, kind build.ModKind, rs []statdesc.RangeReading) (statdesc.RangeReading, []string) {
	keep := rs
	if kind == build.Implicit {
		keep = narrow(keep, func(rd statdesc.RangeReading) bool {
			return r.fits(rd, true, func(m *dataset.Mod) bool { return slices.Contains(b.Implicits, m.ID) })
		})
	}
	unique := it.Rarity == build.Unique
	onItem := func(m *dataset.Mod) bool { return r.canBeOn(m, b, unique) }
	keep = narrow(keep,
		func(rd statdesc.RangeReading) bool { return r.fits(rd, true, onItem) },
		func(rd statdesc.RangeReading) bool { return r.fits(rd, false, onItem) })
	if b.Armour != nil || b.Weapon != nil {
		rolls := func(m *dataset.Mod) bool { return len(m.SpawnWeights) > 0 && onItem(m) }
		keep = narrow(keep, func(rd statdesc.RangeReading) bool {
			return slices.ContainsFunc(rd.Stats, func(s string) bool { return strings.HasPrefix(s, localStatPrefix) }) && r.fits(rd, false, rolls)
		})
	}
	var others []string
	for _, rd := range keep[1:] {
		if !slices.Equal(rd.Stats, keep[0].Stats) {
			others = append(others, strings.Join(rd.Stats, ", "))
		}
	}
	return keep[0], others
}

// narrow returns the readings that pass the first of tests any reading passes, or all of them
// when none does.
func narrow(rs []statdesc.RangeReading, tests ...func(statdesc.RangeReading) bool) []statdesc.RangeReading {
	for _, test := range tests {
		var out []statdesc.RangeReading
		for _, rd := range rs {
			if test(rd) {
				out = append(out, rd)
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	return rs
}

// fits reports whether a mod that passes ok has every stat rd shows, with rd's values in the
// mod's range when inRange is set.
func (r *Reader) fits(rd statdesc.RangeReading, inRange bool, ok func(*dataset.Mod) bool) bool {
	shown := -1
	for k := range rd.Stats {
		if rd.Max[k] != 0 || rd.Min[k] != 0 {
			shown = k
			break
		}
	}
	if shown < 0 {
		return false
	}
	for _, ms := range r.mods[rd.Stats[shown]] {
		if ok(ms.mod) && r.has(ms.mod, rd, inRange) {
			return true
		}
	}
	return false
}

// has reports whether m has each stat rd shows, with rd's values in its range when inRange is
// set.
func (r *Reader) has(m *dataset.Mod, rd statdesc.RangeReading, inRange bool) bool {
	for k, st := range rd.Stats {
		if rd.Min[k] == 0 && rd.Max[k] == 0 {
			continue
		}
		i := slices.IndexFunc(r.mods[st], func(ms modStat) bool { return ms.mod == m })
		if i < 0 {
			return false
		}
		if ms := r.mods[st][i]; inRange && (rd.Min[k] < ms.min || rd.Max[k] > ms.max) {
			return false
		}
	}
	return true
}

// canBeOn reports whether mod m can be on an item of base b: one of its implicits, or a mod of
// the item's domain or a crafting domain that spawns on it. A unique's mods can be on a unique
// item only.
func (r *Reader) canBeOn(m *dataset.Mod, b *dataset.ItemBase, unique bool) bool {
	if slices.Contains(b.Implicits, m.ID) {
		return true
	}
	if !ofDomain(m, b, unique) {
		return false
	}
	if len(m.SpawnWeights) == 0 {
		return unique || r.rollsStatOn(m, b)
	}
	return spawnsOn(m, b)
}

// ofDomain reports whether m is of the mod domain of items of b's class or a crafting domain,
// and not a unique's mod unless the item is unique.
func ofDomain(m *dataset.Mod, b *dataset.ItemBase, unique bool) bool {
	if m.Generation == generationUnique && m.Domain != domainCrafted && !unique {
		return false
	}
	return m.Domain == classDomain(b.Class) || m.Domain == domainCrafted || m.Domain == domainDelve || m.Domain == domainUnveiled
}

// rollsStatOn reports whether a mod with spawn weights that spawns on base b has m's first stat.
// The dataset does not say which items a crafted, veiled or enchanted mod goes on, so on items
// other than uniques such a mod counts only for stats the base can roll.
func (r *Reader) rollsStatOn(m *dataset.Mod, b *dataset.ItemBase) bool {
	id, ok := r.firstStat[m]
	return ok && slices.ContainsFunc(r.mods[id], func(x modStat) bool { return len(x.mod.SpawnWeights) > 0 && spawnsOn(x.mod, b) })
}

// spawnsOn reports whether m's spawn weights give it a positive weight on base b.
func spawnsOn(m *dataset.Mod, b *dataset.ItemBase) bool {
	inherited := classTags(b.Class)
	for _, w := range m.SpawnWeights {
		if w.Tag == defaultTag || slices.Contains(b.Tags, w.Tag) || slices.Contains(inherited, w.Tag) {
			return w.Weight > 0
		}
	}
	return false
}

// classTags returns the tags items of class, an ItemClass.ID, inherit from it. The dataset's
// base tags lack them, and mods' spawn weights name them.
func classTags(class string) []string {
	switch class {
	case "Helmet", "Body Armour", "Gloves", "Boots", "Shield":
		return []string{strings.ReplaceAll(strings.ToLower(class), " ", "_"), "armour"}
	case "Amulet", "Ring", "Belt", "Quiver":
		return []string{strings.ToLower(class)}
	case "Claw", "Dagger", "Wand":
		return []string{strings.ToLower(class), oneHandTag, weaponTag}
	case "Rune Dagger":
		return []string{"dagger", oneHandTag, weaponTag}
	case "One Hand Sword", "Thrusting One Hand Sword", "One Hand Axe", "One Hand Mace", "Sceptre":
		return []string{weaponKind(class), oneHandTag, weaponTag}
	case "Two Hand Sword", "Two Hand Axe", "Two Hand Mace", "Bow", "Staff", "Warstaff":
		return []string{weaponKind(class), twoHandTag, weaponTag}
	case "LifeFlask", "ManaFlask", "HybridFlask", "UtilityFlask":
		return []string{"flask", strings.ToLower(strings.TrimSuffix(class, "Flask")) + "_flask"}
	case "FishingRod":
		return []string{"fishing_rod"}
	}
	return nil
}

// weaponKind returns the tag of a weapon class's kind, such as "sword" for "Two Hand Sword".
func weaponKind(class string) string {
	switch {
	case strings.HasSuffix(class, "Sword"):
		return "sword"
	case strings.HasSuffix(class, "Axe"):
		return "axe"
	case strings.HasSuffix(class, "Mace"), class == "Sceptre":
		return "mace"
	case class == "Warstaff":
		return "staff"
	}
	return strings.ToLower(class)
}

// classDomain returns the mod domain of items of class, an ItemClass.ID.
func classDomain(class string) int32 {
	switch class {
	case "LifeFlask", "ManaFlask", "HybridFlask", "UtilityFlask":
		return domainFlask
	case "Jewel":
		return domainJewel
	case "AbyssJewel":
		return domainAbyssJewel
	case "Tincture":
		return domainTincture
	}
	return domainItem
}
