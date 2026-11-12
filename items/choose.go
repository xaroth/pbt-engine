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
	genUnique          = 3
	genCorrupted       = 5
	genEnchant         = 10
	genAnoint          = 19
	genFlaskEnch1      = 21
	genFlaskEnch2      = 22
	genScourgeUpside   = 24
	genScourgeDownside = 25
	genExarch          = 28
	genEater           = 29
	genCrucible        = 31
	genCrucibleUnique  = 32
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
	// tags are the base's tags with those its influences give; influenced holds the latter.
	tags       []string
	influenced []string
	rarity     build.Rarity
	corrupted  bool
	domain     int32
}

func (r *Reader) newTarget(it *build.Item, b *dataset.ItemBase) target {
	t := target{base: b, tags: b.Tags, rarity: it.Rarity, corrupted: slices.Contains(it.Flags, corruptedFlag), domain: b.ModDomain}
	if !r.crafting.known {
		t.domain = classDomain(b.Class)
	}
	for _, f := range it.Flags {
		suffix, ok := influences()[f]
		if !ok {
			continue
		}
		for _, tag := range b.Tags {
			t.influenced = append(t.influenced, tag+"_"+suffix)
		}
	}
	t.tags = slices.Concat(b.Tags, t.influenced)
	return t
}

// choice is the reading chosen for a line, with the values it stores.
type choice struct {
	statdesc.RangeReading

	// others are the stats of other readings that remain possible, which make the line ambiguous.
	others []string
	// warnings are the codes of the step that chose the reading and of its values, with their
	// messages.
	warnings []build.Warning
}

// step is one step of narrowing readings down: the mods that give a reading it keeps, with the
// values in range or not, an extra test of the reading, and the warning keeping it gives.
type step struct {
	mods    func(*dataset.Mod) bool
	inRange bool
	sums    bool
	also    func(statdesc.RangeReading) bool
	warn    build.Code
}

func (r *Reader) keeps(s step, rd statdesc.RangeReading) bool {
	if s.also != nil && !s.also(rd) {
		return false
	}
	return r.gives(rd, s.inRange, s.mods) || s.sums && r.givesSum(rd, s.mods)
}

// choose returns the reading of a line of kind on item it of base b. The line's kind and the
// item give the mods the line can be (see sources). Readings are narrowed by the first of these
// steps some reading passes:
//  1. a current mod with the values in range, alone or as two mods of one stat;
//  2. a bench craft with the values in range, on a line without a marker;
//  3. a given mod with the values in range;
//  4. for an implicit line, a corruption implicit on an item not marked Corrupted;
//  5. a retired mod with the values in range;
//  6. a current mod or bench craft with any values, which is out of range.
//
// Then on armour and weapons the readings with a local stat, then the first in the game's order.
// A value the text does not show is the value of the mods that give the reading.
func (r *Reader) choose(it *build.Item, b *dataset.ItemBase, kind build.ModKind, rs []statdesc.RangeReading) choice {
	t := r.newTarget(it, b)
	src := r.sources(t, kind)
	ofBase := func(m *dataset.Mod) bool { return slices.Contains(b.Implicits, m.ID) }
	anyCurrent := func(m *dataset.Mod) bool { return src.current(m) || src.craft(m) }
	steps := []step{{mods: src.current, inRange: true, sums: true}, {mods: src.craft, inRange: true}, {mods: src.given, inRange: true}}
	if kind == build.Implicit {
		steps = append(steps, step{mods: func(m *dataset.Mod) bool { return corruption(m, t) }, inRange: true, warn: NotCorrupted})
	}
	steps = append(steps,
		step{mods: src.retired, inRange: true, also: func(rd statdesc.RangeReading) bool {
			// A former implicit has other stats than the base's implicit; else it is the base's
			// implicit out of range.
			return kind != build.Implicit || !r.gives(rd, false, ofBase)
		}},
		step{mods: anyCurrent, warn: RollOutOfRange})
	keep, won, ok := r.narrow(rs, steps)
	if b.Armour != nil || b.Weapon != nil {
		keep = r.preferLocal(keep, t)
	}
	c := choice{RangeReading: keep[0]}
	for _, rd := range keep[1:] {
		if !slices.Equal(rd.Stats, keep[0].Stats) {
			c.others = append(c.others, strings.Join(rd.Stats, ", "))
		}
	}
	if ok {
		c.RangeReading, c.warnings = r.finish(keep[0], won, kind)
	}
	return c
}

// preferLocal returns the readings of rs with a local stat that rolls on t, or rs when there are
// none. A unique's local stat can read as a global stat the unique has, as with a sword's
// "+190 to Armour", so only a local stat that rolls on the base counts.
func (r *Reader) preferLocal(rs []statdesc.RangeReading, t target) []statdesc.RangeReading {
	rolls := func(m *dataset.Mod) bool { return slices.Contains(t.base.Implicits, m.ID) || spawnsOn(m, t.tags) }
	local, _, _ := r.narrow(rs, []step{{mods: rolls, also: func(rd statdesc.RangeReading) bool {
		return slices.ContainsFunc(rd.Stats, isLocal)
	}}})
	return local
}

// finish returns rd with the values the mods of step s store, and the warnings of s and of the
// values.
func (r *Reader) finish(rd statdesc.RangeReading, s step, kind build.ModKind) (statdesc.RangeReading, []build.Warning) {
	var warnings []build.Warning
	if s.warn != "" {
		warnings = append(warnings, build.Warning{Code: s.warn, Message: warnMessage(s.warn, kind)})
	}
	rd, w := r.storedValues(rd, s.mods, s.inRange)
	if w != nil {
		warnings = append(warnings, *w)
	}
	return rd, warnings
}

func warnMessage(code build.Code, kind build.ModKind) string {
	if code == NotCorrupted {
		return "read as a corruption implicit; the item is not marked Corrupted"
	}
	return "no " + string(kind) + " mod the item can have rolls this value"
}

// narrow returns the readings that pass the first of steps any reading passes, and that step, or
// all of them and false when none does.
func (r *Reader) narrow(rs []statdesc.RangeReading, steps []step) ([]statdesc.RangeReading, step, bool) {
	for _, s := range steps {
		var out []statdesc.RangeReading
		for _, rd := range rs {
			if r.keeps(s, rd) {
				out = append(out, rd)
			}
		}
		if len(out) > 0 {
			return out, s, true
		}
	}
	return rs, step{}, false
}

// lineSources are the mods a line can be: current mods, which the item can roll now; bench
// crafts of a line without a marker, which count after current mods; given mods, which a source
// other than rolling gives, such as an essence, and which count after those and only with the
// values in range; and retired ones, which it may still have from an older version, and which
// count only with the values in range.
type lineSources struct {
	current, craft, given, retired func(*dataset.Mod) bool
}

func none(*dataset.Mod) bool { return false }

// sources returns the mods a line of kind on item t can be.
func (r *Reader) sources(t target, kind build.ModKind) lineSources {
	craft := r.benchCraft(t)
	former := func(m *dataset.Mod) bool { return r.formerCraft(m, t) }
	given := func(m *dataset.Mod) bool { return r.essence(m, t) || elevated(m, t) }
	switch kind {
	case build.Implicit:
		return lineSources{
			current: func(m *dataset.Mod) bool { return implicitMod(m, t) },
			craft:   none,
			given:   none,
			retired: func(m *dataset.Mod) bool { return formerImplicit(m, t) },
		}
	case build.Crafted:
		return lineSources{current: craft, craft: none, given: none, retired: former}
	case build.Enchant:
		return lineSources{
			current: func(m *dataset.Mod) bool { return enchant(m, t) || r.enchantCraft(m, t) },
			craft:   none,
			given:   none,
			retired: func(m *dataset.Mod) bool { return retiredEnchant(m, t) },
		}
	case build.Fractured:
		// A fractured mod is any explicit mod but a bench craft.
		return lineSources{
			current: func(m *dataset.Mod) bool { return explicitMod(m, t) },
			craft:   none,
			given:   given,
			retired: func(m *dataset.Mod) bool { return r.retiredAffix(m, t) },
		}
	default:
		// A line without a marker may be a bench craft that lost its marker.
		return lineSources{
			current: func(m *dataset.Mod) bool { return explicitMod(m, t) },
			craft:   craft,
			given:   given,
			retired: func(m *dataset.Mod) bool { return r.retiredAffix(m, t) || passiveTreeMod(m, t) || former(m) },
		}
	}
}

// implicitMod is a current implicit of t: the base's own, an eldritch implicit on an item other
// than a unique, or a corruption implicit on a corrupted item.
func implicitMod(m *dataset.Mod, t target) bool {
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

// corruption is a corruption implicit that spawns on t.
func corruption(m *dataset.Mod, t target) bool {
	return m.Domain == t.domain && m.Generation == genCorrupted && spawnsOn(m, t.tags)
}

// formerImplicit is an implicit that spawns nowhere now: a base's former implicit, a unique's
// own implicit, or a retired eldritch or corruption implicit. The game files do not tell a former
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
// a prefix or suffix that spawns on it, rolled or added by a fossil. A normal item has no
// explicit mods.
func explicitMod(m *dataset.Mod, t target) bool {
	switch t.rarity {
	case build.Unique:
		return (m.Domain == t.domain || m.Domain == domainItem) && m.Generation == genUnique
	case build.Magic, build.Rare:
		return affix(m) && affixDomain(m, t) && spawnsOn(m, t.tags)
	default:
		return false
	}
}

// affixDomain reports whether m is of t's domain, or of the unveiled or Delve (fossil) domain,
// whose mods go on items of the item domain.
func affixDomain(m *dataset.Mod, t target) bool {
	return m.Domain == t.domain || m.Domain == domainUnveiled || m.Domain == domainDelve
}

func magicOrRare(t target) bool { return t.rarity == build.Magic || t.rarity == build.Rare }

// elevated is an elevated influence mod of a magic or rare item like t: a prefix or suffix of
// t's domain whose weights are all 0 and name a tag t's influences give.
func elevated(m *dataset.Mod, t target) bool {
	return magicOrRare(t) && affix(m) && m.Domain == t.domain && !spawns(m) &&
		slices.ContainsFunc(m.SpawnWeights, func(w dataset.TagWeight) bool { return slices.Contains(t.influenced, w.Tag) })
}

// essence is a mod an essence adds to a magic or rare item like t: one the dataset lists for t's
// class. A dataset without that list stands in: a prefix or suffix of the item domain whose
// weights are all 0 and name no item kind, with a local stat only on armour and weapons.
func (r *Reader) essence(m *dataset.Mod, t target) bool {
	if !magicOrRare(t) || !affix(m) {
		return false
	}
	if r.crafting.known {
		return slices.Contains(r.crafting.essence[m.ID], t.base.Class)
	}
	return t.domain == domainItem && m.Domain == domainItem && !spawns(m) && namesNoKind(m) && r.fitsStandIn(m, t)
}

// benchCraft is a bench craft that can be on a magic or rare item like t: one the dataset lists
// for t's class. A dataset without that list stands in: any craft whose weights name t's kind
// or no kind, with a local stat only on armour and weapons.
func (r *Reader) benchCraft(t target) func(*dataset.Mod) bool {
	return func(m *dataset.Mod) bool {
		if !magicOrRare(t) || m.Domain != domainCrafted {
			return false
		}
		if r.crafting.known {
			return slices.Contains(r.crafting.bench[m.ID], t.base.Class)
		}
		return r.fitsStandIn(m, t) && namesBaseOrNone(m, t)
	}
}

// namesBaseOrNone reports whether m's weights name a tag of t's base or none but "default": a
// craft whose weights name item kinds, as {shield 0} {ring 0}, was offered for those only.
func namesBaseOrNone(m *dataset.Mod, t target) bool {
	return namesNoKind(m) || slices.ContainsFunc(m.SpawnWeights, func(w dataset.TagWeight) bool {
		return w.Tag != defaultTag && slices.Contains(t.base.Tags, w.Tag)
	})
}

// listedEssence reports whether the dataset lists m as an essence mod, which counts only where
// it lists it (see essence).
func (r *Reader) listedEssence(m *dataset.Mod) bool {
	return r.crafting.known && r.crafting.essence[m.ID] != nil
}

// formerCraft is a crafted mod of a magic or rare item like t that the bench no longer offers:
// one the dataset's bench crafts do not list. The dataset does not say which items it went on:
// one whose weights name item kinds counts on those only, and one with a local stat on armour
// and weapons only. Without the list every craft is current.
func (r *Reader) formerCraft(m *dataset.Mod, t target) bool {
	return r.crafting.known && magicOrRare(t) && m.Domain == domainCrafted && r.crafting.bench[m.ID] == nil &&
		r.fitsStandIn(m, t) && namesBaseOrNone(m, t)
}

// enchantCraft is an enchantment the crafting bench adds to items of t's class.
func (r *Reader) enchantCraft(m *dataset.Mod, t target) bool {
	return m.Domain != domainCrafted && slices.Contains(r.crafting.bench[m.ID], t.base.Class)
}

// fitsStandIn reports whether t can take m where nothing says which items m goes on: a mod with
// a local stat only on armour and weapons, or in the item's own domain other than the item
// domain, such as a flask's mods, whose local stats are the flask's.
func (r *Reader) fitsStandIn(m *dataset.Mod, t target) bool {
	return t.base.Armour != nil || t.base.Weapon != nil || m.Domain == t.domain && t.domain != domainItem ||
		!slices.ContainsFunc(r.mods.stats[m], isLocal)
}

// namesNoKind reports whether m's weights name no tag but "default".
func namesNoKind(m *dataset.Mod) bool {
	return !slices.ContainsFunc(m.SpawnWeights, func(w dataset.TagWeight) bool { return w.Tag != defaultTag })
}

// retiredAffix is a prefix or suffix of a magic or rare item like t that spawns nowhere now:
// its weights are all 0 and name one of its base's tags or no item kind, such as older versions
// of mods and mods of retired crafting, but not an essence mod (see essence). Weights on a tag
// an influence gives are elevated mods (see elevated).
func (r *Reader) retiredAffix(m *dataset.Mod, t target) bool {
	if !retiredAffixOf(m, t) || r.listedEssence(m) {
		return false
	}
	if namesNoKind(m) {
		// Nothing says which items it went on: one with a local stat counts on armour and
		// weapons only. Without the dataset's essence mods, item mods like this stand in for
		// essence mods instead (see essence).
		return (r.crafting.known || m.Domain != domainItem) && r.fitsStandIn(m, t)
	}
	return slices.ContainsFunc(m.SpawnWeights, func(w dataset.TagWeight) bool {
		return w.Tag != defaultTag && slices.Contains(t.base.Tags, w.Tag)
	})
}

// retiredAffixOf reports whether m is a prefix or suffix of a magic or rare item like t, of its
// domains, with spawn weights that are all 0.
func retiredAffixOf(m *dataset.Mod, t target) bool {
	return magicOrRare(t) && affix(m) && affixDomain(m, t) && len(m.SpawnWeights) > 0 && !spawns(m)
}

// passiveTreeMod is a Crucible or Scourge mod of t's domain. Neither can be added today, so they
// count as retired; one with spawn weights only on the items they name.
func passiveTreeMod(m *dataset.Mod, t target) bool {
	if m.Domain != t.domain || spawns(m) && !spawnsOn(m, t.tags) {
		return false
	}
	switch m.Generation {
	case genCrucible, genCrucibleUnique, genScourgeUpside, genScourgeDownside:
		return true
	}
	return false
}

func affix(m *dataset.Mod) bool { return m.Generation == genPrefix || m.Generation == genSuffix }

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

// retiredEnchant is a labyrinth enchantment that spawns nowhere now.
func retiredEnchant(m *dataset.Mod, _ target) bool {
	return m.Generation == genEnchant && !spawns(m)
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

// classDomain returns the mod domain of items of class, an ItemClass.ID, for a dataset that does
// not hold the bases' mod domains.
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
