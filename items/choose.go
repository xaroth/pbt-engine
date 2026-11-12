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

// Mods.GenerationType values as the game numbers them.
const (
	genPrefix = 1
	genSuffix = 2
	// genUnique is a unique's mod or a base's implicit.
	genUnique     = 3
	genCorrupted  = 5
	genEnchant    = 10
	genAnoint     = 19
	genFlaskEnch1 = 21
	genFlaskEnch2 = 22
	genExarch     = 28
	genEater      = 29
)

const (
	defaultTag      = "default"
	localStatPrefix = "local_"
	corruptedFlag   = "Corrupted"
	anointClass     = "Amulet"
)

// influences maps an item's influence flag to the suffix of the tags it gives, as in
// "boots_shaper".
func influences() map[string]string {
	return map[string]string{
		"Shaper Item": "shaper", "Elder Item": "elder", "Crusader Item": "crusader",
		"Redeemer Item": "eyrie", "Hunter Item": "basilisk", "Warlord Item": "adjudicator",
	}
}

// target is the item a line is read for.
type target struct {
	base *dataset.ItemBase
	// tags are the base's tags with those its influences give.
	tags      []string
	rarity    build.Rarity
	corrupted bool
	domain    int32
}

func (r *Reader) target(it *build.Item, b *dataset.ItemBase) target {
	t := target{base: b, tags: b.Tags, rarity: it.Rarity, corrupted: slices.Contains(it.Flags, corruptedFlag), domain: classDomain(b.Class)}
	for _, f := range it.Flags {
		suffix, ok := influences()[f]
		if !ok {
			continue
		}
		for _, tag := range b.Tags {
			if inf := tag + "_" + suffix; r.tags[inf] && !slices.Contains(t.tags, inf) {
				t.tags = append(slices.Clip(t.tags), inf)
			}
		}
	}
	return t
}

// choice is the reading chosen for a line.
type choice struct {
	statdesc.RangeReading

	// others are the stats of other readings that remain possible, which make the line ambiguous.
	others []string
	// outOfRange is set when no mod the item can have rolls the line's values.
	outOfRange bool
}

// step is one step of narrowing readings down: the readings it keeps, and whether keeping them
// means a value outside every mod's range.
type step struct {
	keep       func(statdesc.RangeReading) bool
	outOfRange bool
}

// choose returns the reading of a line of kind on item it of base b. The line's kind and the
// item give the mods the line can be (see sources): current ones and retired ones. Readings are
// narrowed by the first of these steps some reading passes: a current mod with the values in
// range, alone or as two mods of one stat; else a retired mod with the values in range; else a
// current mod with any values, which is out of range. Then on armour and weapons the readings
// with a local stat, then the first in the game's order.
func (r *Reader) choose(it *build.Item, b *dataset.ItemBase, kind build.ModKind, rs []statdesc.RangeReading) choice {
	t := r.target(it, b)
	cur, ret := r.sources(t, kind)
	ofBase := func(m *dataset.Mod) bool { return slices.Contains(b.Implicits, m.ID) }
	keep, outOfRange := narrow(rs, []step{
		{keep: func(rd statdesc.RangeReading) bool { return r.gives(rd, true, cur) || r.givesSum(rd, cur) }},
		{keep: func(rd statdesc.RangeReading) bool {
			// A former implicit has other stats than the base's implicit; else it is the
			// base's implicit out of range.
			return r.gives(rd, true, ret) && (kind != build.Implicit || !r.gives(rd, false, ofBase))
		}},
		{keep: func(rd statdesc.RangeReading) bool { return r.gives(rd, false, cur) }, outOfRange: true},
	})
	if b.Armour != nil || b.Weapon != nil {
		// A unique's local stat can read as a global stat the unique has, as with a sword's
		// "+190 to Armour", so only a local stat that rolls on the base counts.
		rolls := func(m *dataset.Mod) bool { return ofBase(m) || spawnsOn(m, t.tags) }
		keep, _ = narrow(keep, []step{{keep: func(rd statdesc.RangeReading) bool {
			return slices.ContainsFunc(rd.Stats, isLocal) && r.gives(rd, false, rolls)
		}}})
	}
	c := choice{RangeReading: keep[0], outOfRange: outOfRange}
	for _, rd := range keep[1:] {
		if !slices.Equal(rd.Stats, keep[0].Stats) {
			c.others = append(c.others, strings.Join(rd.Stats, ", "))
		}
	}
	return c
}

// narrow returns the readings that pass the first of steps any reading passes, and that step's
// outOfRange, or all of them when none does.
func narrow(rs []statdesc.RangeReading, steps []step) ([]statdesc.RangeReading, bool) {
	for _, s := range steps {
		var out []statdesc.RangeReading
		for _, rd := range rs {
			if s.keep(rd) {
				out = append(out, rd)
			}
		}
		if len(out) > 0 {
			return out, s.outOfRange
		}
	}
	return rs, false
}

// sources returns which mods a line of kind on item t can be: current mods, which the item can
// roll or be given now, and retired ones, which it may still have from an older version or a
// source the dataset does not name.
func (r *Reader) sources(t target, kind build.ModKind) (current, retired func(*dataset.Mod) bool) {
	switch kind {
	case build.Implicit:
		return r.implicitMod(t), func(m *dataset.Mod) bool { return formerImplicit(m, t) }
	case build.Crafted:
		return r.benchCraft(t), func(m *dataset.Mod) bool { return explicitMod(m, t) || retiredAffix(m, t) }
	case build.Enchant:
		return func(m *dataset.Mod) bool { return enchant(m, t) }, func(m *dataset.Mod) bool { return m.Generation == genEnchant && !spawns(m) }
	case build.Fractured:
		return func(m *dataset.Mod) bool { return explicitMod(m, t) }, func(m *dataset.Mod) bool { return retiredAffix(m, t) }
	default:
		// A line without a marker may be a bench craft that lost its marker.
		craft := r.benchCraft(t)
		return func(m *dataset.Mod) bool { return explicitMod(m, t) },
			func(m *dataset.Mod) bool { return retiredAffix(m, t) || craft(m) }
	}
}

// implicitMod is a current implicit of t: the base's own, an eldritch implicit on an item other
// than a unique, or a corrupted implicit on a corrupted item.
func (r *Reader) implicitMod(t target) func(*dataset.Mod) bool {
	return func(m *dataset.Mod) bool {
		if slices.Contains(t.base.Implicits, m.ID) {
			return true
		}
		if m.Domain != t.domain || !spawnsOn(m, t.tags) {
			return false
		}
		switch m.Generation {
		case genExarch, genEater:
			return t.rarity != build.Unique
		case genCorrupted:
			return t.corrupted
		}
		return false
	}
}

// formerImplicit is an implicit that spawns nowhere now: a base's former implicit, a unique's
// own implicit, or a retired eldritch or corrupted implicit. The game files do not tell a former
// implicit from a unique's mod.
func formerImplicit(m *dataset.Mod, t target) bool {
	if m.Domain != t.domain || spawns(m) {
		return false
	}
	switch m.Generation {
	case genUnique, genCorrupted, genExarch, genEater:
		return true
	}
	return false
}

// explicitMod is a current explicit of t: on a unique, a unique's mod; on a magic or rare item,
// a prefix or suffix that spawns on it. A normal item has no explicit mods.
func explicitMod(m *dataset.Mod, t target) bool {
	switch t.rarity {
	case build.Unique:
		return (m.Domain == t.domain || m.Domain == domainItem) && m.Generation == genUnique
	case build.Magic, build.Rare:
		return affixOf(m, t) && spawnsOn(m, t.tags)
	default:
		return false
	}
}

// retiredAffix is a prefix or suffix of a magic or rare item like t that spawns nowhere now:
// its weights are all 0 and name one of t's tags, or name only "default" in a domain of t's own
// kind, such as flasks.
func retiredAffix(m *dataset.Mod, t target) bool {
	if t.rarity != build.Magic && t.rarity != build.Rare || !affixOf(m, t) || len(m.SpawnWeights) == 0 || spawns(m) {
		return false
	}
	for _, w := range m.SpawnWeights {
		if w.Tag != defaultTag && slices.Contains(t.tags, w.Tag) {
			return true
		}
	}
	return m.Domain == t.domain && t.domain != domainItem
}

// affixOf reports whether m is a prefix or suffix of t's domain, or of the delve (fossil) or
// unveiled domains.
func affixOf(m *dataset.Mod, t target) bool {
	return (m.Generation == genPrefix || m.Generation == genSuffix) &&
		(m.Domain == t.domain || m.Domain == domainDelve || m.Domain == domainUnveiled)
}

// benchCraft is a bench craft that can be on t. The dataset does not say which item classes a
// craft applies to; until it does, a craft with a local stat counts on armour and weapons only.
func (r *Reader) benchCraft(t target) func(*dataset.Mod) bool {
	defences := t.base.Armour != nil || t.base.Weapon != nil
	return func(m *dataset.Mod) bool {
		return m.Domain == domainCrafted && (defences || !slices.ContainsFunc(r.mods.stats[m], isLocal))
	}
}

// enchant is an enchantment of t: a labyrinth or flask enchantment that spawns on it, or an
// anointment on an amulet.
func enchant(m *dataset.Mod, t target) bool {
	switch m.Generation {
	case genEnchant, genFlaskEnch1, genFlaskEnch2:
		return spawnsOn(m, t.tags)
	case genAnoint:
		return t.base.Class == anointClass
	}
	return false
}

func isLocal(stat string) bool { return strings.HasPrefix(stat, localStatPrefix) }

// spawns reports whether m has a positive spawn weight on any tag.
func spawns(m *dataset.Mod) bool {
	return slices.ContainsFunc(m.SpawnWeights, func(w dataset.TagWeight) bool { return w.Weight > 0 })
}

// spawnsOn reports whether m's spawn weights give it a positive weight on an item with tags: the
// first weight whose tag the item has decides.
func spawnsOn(m *dataset.Mod, tags []string) bool {
	for _, w := range m.SpawnWeights {
		if slices.Contains(tags, w.Tag) {
			return w.Weight > 0
		}
	}
	return false
}

// classDomain returns the mod domain of items of class, an ItemClass.ID. The dataset does not
// hold a base's mod domain yet.
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
