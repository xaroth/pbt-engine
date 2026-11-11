package items

import (
	"maps"
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

// gameModeRuthless is Mods.GameMode of a mod that exists in Ruthless only.
const gameModeRuthless = 2

// influences maps an item's influence flag to the suffix of the tags it gives, as in
// "boots_shaper".
func influences() map[string]string {
	return map[string]string{
		"Shaper Item": "shaper", "Elder Item": "elder", "Crusader Item": "crusader",
		"Redeemer Item": "eyrie", "Hunter Item": "basilisk", "Warlord Item": "adjudicator",
	}
}

// target is the item a line is read for: its base, its tags with those its influences give, and
// whether it is unique.
type target struct {
	base   *dataset.ItemBase
	tags   []string
	unique bool
}

// target returns the target of item it of base b.
func (r *Reader) target(it *build.Item, b *dataset.ItemBase) target {
	t := target{base: b, tags: b.Tags, unique: it.Rarity == build.Unique}
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

const (
	defaultTag      = "default"
	localStatPrefix = "local_"
)

// choice is the reading chosen for a line.
type choice struct {
	statdesc.RangeReading

	// others are the stats of other readings that remain possible, which make the line ambiguous.
	others []string
	// outOfRange is set when no mod the item can have rolls the line's values.
	outOfRange bool
}

// test is one step of narrowing readings down: the readings it keeps, and whether keeping them
// means a value outside every mod's range.
type test struct {
	keep       func(statdesc.RangeReading) bool
	outOfRange bool
}

// choose returns the reading of a line of kind on item it of base b. Readings are narrowed in
// turn by the first test some reading passes:
//   - for an implicit line: an implicit mod of the base with the values in its range; else a
//     former implicit (a mod no base has, which spawns nowhere) with the values in its range and
//     other stats than the base's implicit, as on an older item; else the base's implicit with
//     any values;
//   - a mod the item can have, with the values in its range; else a retired mod of the item
//     (spawn weights all 0) with the values in its range; else, as a last resort, a mod the item
//     can have with any values.
//
// Then, on armour and weapons, readings with a local stat that a mod rolling on the base has,
// since a unique's local stat can read the same as a global one the unique has; then the first
// in the game's order.
func (r *Reader) choose(it *build.Item, b *dataset.ItemBase, kind build.ModKind, rs []statdesc.RangeReading) choice {
	t := r.target(it, b)
	onItem := func(m *dataset.Mod) bool { return r.canBeOn(m, t) }
	retired := func(m *dataset.Mod) bool { return r.retired(m, t) }
	var tests []test
	if kind == build.Implicit {
		tests = r.implicitTests(b)
	}
	tests = append(tests,
		test{keep: func(rd statdesc.RangeReading) bool { return r.fits(rd, true, onItem) }},
		test{keep: func(rd statdesc.RangeReading) bool { return r.fits(rd, true, retired) }},
		test{keep: func(rd statdesc.RangeReading) bool { return r.fits(rd, false, onItem) }, outOfRange: true})
	keep, outOfRange := narrow(rs, tests)
	if b.Armour != nil || b.Weapon != nil {
		rolls := func(m *dataset.Mod) bool { return len(m.SpawnWeights) > 0 && onItem(m) }
		keep, _ = narrow(keep, []test{{keep: func(rd statdesc.RangeReading) bool {
			return slices.ContainsFunc(rd.Stats, isLocal) && r.fits(rd, false, rolls)
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

// implicitTests are the tests of an implicit line on an item of base b. Implicits share their
// generation with uniques' mods, so a former implicit counts on items of any rarity.
func (r *Reader) implicitTests(b *dataset.ItemBase) []test {
	ofBase := func(m *dataset.Mod) bool { return slices.Contains(b.Implicits, m.ID) }
	former := func(m *dataset.Mod) bool {
		return m.Domain == classDomain(b.Class) && !r.implicit[m.ID] && !slices.ContainsFunc(m.SpawnWeights, positive)
	}
	return []test{
		{keep: func(rd statdesc.RangeReading) bool { return r.fits(rd, true, ofBase) }},
		{keep: func(rd statdesc.RangeReading) bool { return r.fits(rd, true, former) && !r.fits(rd, false, ofBase) }},
		{keep: func(rd statdesc.RangeReading) bool { return r.fits(rd, false, ofBase) }, outOfRange: true},
	}
}

func isLocal(stat string) bool { return strings.HasPrefix(stat, localStatPrefix) }

// narrow returns the readings that pass the first of tests any reading passes, and that test's
// outOfRange, or all of them when none does.
func narrow(rs []statdesc.RangeReading, tests []test) ([]statdesc.RangeReading, bool) {
	for _, t := range tests {
		var out []statdesc.RangeReading
		for _, rd := range rs {
			if t.keep(rd) {
				out = append(out, rd)
			}
		}
		if len(out) > 0 {
			return out, t.outOfRange
		}
	}
	return rs, false
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
	return inRange && r.sumFits(rd, shown, ok)
}

// sumFits reports whether two mods that pass ok, of no shared family, add up to rd's values:
// the game shows mods of one stat, such as a prefix and a suffix, as one line.
func (r *Reader) sumFits(rd statdesc.RangeReading, shown int, ok func(*dataset.Mod) bool) bool {
	var mods []*dataset.Mod
	for _, ms := range r.mods[rd.Stats[shown]] {
		if ok(ms.mod) && r.has(ms.mod, rd, false) && !slices.Contains(mods, ms.mod) {
			mods = append(mods, ms.mod)
		}
	}
	for i, a := range mods {
		for _, b := range mods[i+1:] {
			if !slices.ContainsFunc(a.Families, func(f string) bool { return slices.Contains(b.Families, f) }) && r.sums(a, b, rd) {
				return true
			}
		}
	}
	return false
}

// sums reports whether a and b add up to rd's values for each stat rd shows.
func (r *Reader) sums(a, b *dataset.Mod, rd statdesc.RangeReading) bool {
	for k, st := range rd.Stats {
		if rd.Min[k] == 0 && rd.Max[k] == 0 {
			continue
		}
		x, okA := r.rangeOf(a, st)
		y, okB := r.rangeOf(b, st)
		if !okA || !okB || rd.Min[k] < x.min+y.min || rd.Max[k] > x.max+y.max {
			return false
		}
	}
	return true
}

// rangeOf returns the range of stat st on mod m.
func (r *Reader) rangeOf(m *dataset.Mod, st string) (modStat, bool) {
	i := slices.IndexFunc(r.mods[st], func(ms modStat) bool { return ms.mod == m })
	if i < 0 {
		return modStat{}, false
	}
	return r.mods[st][i], true
}

// has reports whether m has each stat rd shows, with rd's values in its range when inRange is
// set. A value out of range still fits when the mod at its own values renders the same text, as
// for a stat the text shows as fixed words, such as "30%" written into the description.
func (r *Reader) has(m *dataset.Mod, rd statdesc.RangeReading, inRange bool) bool {
	own := map[string]int32{}
	out := false
	for k, st := range rd.Stats {
		if rd.Min[k] == 0 && rd.Max[k] == 0 {
			continue
		}
		ms, ok := r.rangeOf(m, st)
		if !ok {
			return false
		}
		own[st] = ms.min
		out = out || rd.Min[k] < ms.min || rd.Max[k] > ms.max
	}
	return !inRange || !out || r.sameText(rd, own)
}

// sameText reports whether rd's stats render the same text at their low values and with the
// values in own put in.
func (r *Reader) sameText(rd statdesc.RangeReading, own map[string]int32) bool {
	read := make(map[string]int32, len(rd.Stats))
	for k, st := range rd.Stats {
		read[st] = rd.Min[k]
	}
	put := maps.Clone(read)
	maps.Copy(put, own)
	a, errA := r.set.Describe(statScope, read)
	b, errB := r.set.Describe(statScope, put)
	return errA == nil && errB == nil && len(a) > 0 &&
		slices.EqualFunc(a, b, func(x, y statdesc.Line) bool { return x.Text == y.Text })
}

// canBeOn reports whether mod m can be on item t: one of its base's implicits, or a mod of the
// item's domain or a crafting domain that spawns on it. A unique's mods can be on a unique item
// only.
func (r *Reader) canBeOn(m *dataset.Mod, t target) bool {
	if slices.Contains(t.base.Implicits, m.ID) {
		return true
	}
	if !ofDomain(m, t) {
		return false
	}
	if len(m.SpawnWeights) == 0 {
		return t.unique || r.rollsStatOn(m, t)
	}
	return spawnsOn(m, t.tags)
}

// retired reports whether m is a mod of the item that no longer spawns: of the item's domain,
// not a crafting one (bench crafts have zero weights too), with spawn weights that are all 0 and
// name only tags of the item, and one other than "default" unless the domain is the item's own
// kind, such as flasks.
func (r *Reader) retired(m *dataset.Mod, t target) bool {
	if m.Domain != classDomain(t.base.Class) || !ofDomain(m, t) || len(m.SpawnWeights) == 0 || slices.ContainsFunc(m.SpawnWeights, positive) {
		return false
	}
	named := false
	for _, w := range m.SpawnWeights {
		if w.Tag == defaultTag {
			continue
		}
		if !slices.Contains(t.tags, w.Tag) {
			return false
		}
		named = true
	}
	// Weights on "default" alone say nothing of the item's kind unless the domain does.
	return named || m.Domain != domainItem
}

func positive(w dataset.TagWeight) bool { return w.Weight > 0 }

// ofDomain reports whether m is of the mod domain of items of t's class or a crafting domain,
// and not a unique's mod unless the item is unique.
func ofDomain(m *dataset.Mod, t target) bool {
	if m.Generation == generationUnique && m.Domain != domainCrafted && !t.unique {
		return false
	}
	return m.Domain == classDomain(t.base.Class) || m.Domain == domainCrafted || m.Domain == domainDelve || m.Domain == domainUnveiled
}

// rollsStatOn reports whether a mod with spawn weights that spawns on item t has m's first stat.
// The dataset does not say which items a crafted, veiled or enchanted mod goes on, so on items
// other than uniques such a mod counts only for stats the item can roll.
func (r *Reader) rollsStatOn(m *dataset.Mod, t target) bool {
	id, ok := r.firstStat[m]
	return ok && slices.ContainsFunc(r.mods[id], func(x modStat) bool { return len(x.mod.SpawnWeights) > 0 && spawnsOn(x.mod, t.tags) })
}

// spawnsOn reports whether m's spawn weights give it a positive weight on an item with tags.
func spawnsOn(m *dataset.Mod, tags []string) bool {
	for _, w := range m.SpawnWeights {
		if slices.Contains(tags, w.Tag) {
			return w.Weight > 0
		}
	}
	return false
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
