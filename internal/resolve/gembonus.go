package resolve

import (
	"slices"

	"github.com/xaroth/pbt-engine/build"
)

// GemBonus is a stat that adds levels or quality to the gems it reaches and selects. The game
// defines which stats these are; each game's rules list them.
type GemBonus struct {
	Stat string
	// Quality makes the stat add quality; otherwise it adds levels.
	Quality bool
	Reach   GemReach
	Gems    GemFilter
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

// itemGemStat is a stat of an equipped item that adds levels or quality to gems, with its
// bonuses.
type itemGemStat struct {
	slot    build.Slot
	value   float64
	bonuses []GemBonus
}

// itemGemStats returns the stats of items that add levels or quality to gems. Items in the
// weapon-swap slots are not in use.
func itemGemStats(items build.Items, bonuses gemBonuses) []itemGemStat {
	var out []itemGemStat
	for s, it := range items {
		if s.Swap() {
			continue
		}
		for _, m := range it.Mods {
			for _, sv := range m.Stats {
				if bs := bonuses[sv.Stat]; len(bs) > 0 {
					out = append(out, itemGemStat{slot: s, value: float64(sv.Value), bonuses: bs})
				}
			}
		}
	}
	return out
}

// itemBonus returns the levels and quality the equipped items add to gem, with tags, socketed
// in slot.
func (r *skillResolver) itemBonus(slot build.Slot, gem build.Gem, tags []string) (level, quality int) {
	for _, st := range r.itemStats {
		l, q := gemBonus(st.bonuses, st.value, func(b GemBonus) bool {
			return (b.Reach == Equipped || b.Reach == Socketed && st.slot == slot) && b.Gems.matches(gem.Support, tags)
		})
		level, quality = level+l, quality+q
	}
	return level, quality
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
