package resolve

import (
	"maps"
	"slices"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/dataset"
)

// GemBonus is a stat that adds levels or quality to the gems it reaches and selects. The game
// defines which stats these are; each game's rules list them.
type GemBonus struct {
	Stat string
	// Quality makes the stat add quality; otherwise it adds levels.
	Quality bool
	Reach   GemReach
	// Socket, when set, limits a Socketed stat to gems in sockets of that colour.
	Socket dataset.GemColour
	Gems   GemFilter
}

// GemReach says which gems a GemBonus can reach.
type GemReach uint8

// Reaches.
const (
	// Socketed is an item stat that reaches the gems socketed in the item.
	Socketed GemReach = iota
	// Equipped is an item stat that reaches gems wherever they are.
	Equipped
	// Supported is a support's stat that reaches the active skill gem it supports.
	Supported
)

// GemKind selects gems by whether they are supports.
type GemKind uint8

// Gem kinds.
const (
	AnyGem GemKind = iota
	// SkillGem is a gem that is not a support.
	SkillGem
	SupportGem
)

// GemFilter selects gems by kind and GemTags.
type GemFilter struct {
	Kind GemKind
	// Tags must all be on the gem, and one of AnyTags if set; none of NotTags may be.
	Tags, AnyTags, NotTags []string
}

// matches reports whether f selects a gem with tags that is a support or not.
func (f GemFilter) matches(support bool, tags []string) bool {
	if f.Kind == SkillGem && support || f.Kind == SupportGem && !support {
		return false
	}
	has := func(t string) bool { return slices.Contains(tags, t) }
	return !slices.ContainsFunc(f.Tags, func(t string) bool { return !has(t) }) &&
		(len(f.AnyTags) == 0 || slices.ContainsFunc(f.AnyTags, has)) &&
		!slices.ContainsFunc(f.NotTags, has)
}

// gemBonuses are GemBonus by stat.
type gemBonuses map[string][]GemBonus

func bonusesByStat(bonuses []GemBonus) gemBonuses {
	out := gemBonuses{}
	for _, b := range bonuses {
		out[b.Stat] = append(out[b.Stat], b)
	}
	return out
}

// gemStat is a stat of the character that adds levels or quality to gems, with its bonuses:
// one of an equipped item, in slot, or of a jewel or passive, in no slot.
type gemStat struct {
	slot    build.Slot
	value   float64
	bonuses []GemBonus
}

// gemStats returns the stats that add levels or quality to gems in build b, which c checked:
// those of its equipped items outside the weapon swap, its jewels in allocated sockets, its
// allocated passives and its chosen mastery effects.
func gemStats(c *build.Checked, b *build.Build, stats []dataset.Stat, bonuses gemBonuses) []gemStat {
	out := itemGemStats(b, bonuses)
	for _, bd := range allocatedBundles(stats, c) {
		if bd.Scope != Global {
			continue
		}
		for _, sv := range bd.Stats {
			out = bonuses.add(out, "", sv.Stat, sv.Value)
		}
	}
	return out
}

// itemGemStats returns the stats of b's equipped items outside the weapon swap and its jewels in
// allocated sockets that add levels or quality to gems.
func itemGemStats(b *build.Build, bonuses gemBonuses) []gemStat {
	var out []gemStat
	addItem := func(slot build.Slot, it build.Item) {
		for _, m := range it.Mods {
			for _, sv := range m.Stats {
				out = bonuses.add(out, slot, sv.Stat, float64(sv.Value))
			}
		}
	}
	for _, s := range slices.Sorted(maps.Keys(b.Items)) {
		if !s.Swap() {
			addItem(s, b.Items[s])
		}
	}
	for _, id := range slices.Sorted(maps.Keys(b.Jewels)) {
		if slices.Contains(b.Nodes, id) {
			addItem("", b.Jewels[id])
		}
	}
	return out
}

// add appends stat with value, from slot, to out when it has bonuses.
func (b gemBonuses) add(out []gemStat, slot build.Slot, stat string, value float64) []gemStat {
	if bs := b[stat]; len(bs) > 0 {
		out = append(out, gemStat{slot: slot, value: value, bonuses: bs})
	}
	return out
}

// characterBonus returns the levels and quality the character's stats and its socket's colour add
// to gem src, with tags.
func (r *skillResolver) characterBonus(src GemSource, gem build.Gem, tags []string) (level, quality int) {
	slot, colour := r.groups[src.Group].Slot, r.sockets[src.Group][src.Gem]
	socketed := func(b GemBonus) bool {
		return b.Reach == Socketed && (b.Socket == 0 || b.Socket == colour)
	}
	for _, st := range r.gemStats {
		l, q := gemBonus(st.bonuses, st.value, func(b GemBonus) bool {
			return (b.Reach == Equipped || st.slot != "" && st.slot == slot && socketed(b)) && b.Gems.matches(gem.Support, tags)
		})
		level, quality = level+l, quality+q
	}
	return level, quality + r.socketQuality(src, gem)
}

// supportedBonus returns the levels and quality a support's stats add to the active skill gem
// it supports, which is a support or not and has tags.
func (b gemBonuses) supportedBonus(stats []StatValue, support bool, tags []string) (level, quality int) {
	for _, sv := range stats {
		l, q := gemBonus(b[sv.Stat], sv.Value, func(b GemBonus) bool { return b.Reach == Supported && b.Gems.matches(support, tags) })
		level, quality = level+l, quality+q
	}
	return level, quality
}

// gemBonus returns the levels and quality a stat with value adds through those of its bonuses
// that reach.
func gemBonus(bonuses []GemBonus, value float64, reach func(GemBonus) bool) (level, quality int) {
	for _, b := range bonuses {
		if !reach(b) {
			continue
		}
		if b.Quality {
			quality += int(value)
		} else {
			level += int(value)
		}
	}
	return level, quality
}
