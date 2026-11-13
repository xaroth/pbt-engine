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

// itemBonus returns the levels and quality the equipped items add to gem, with tags, socketed
// in slot. Items in the weapon-swap slots are not in use.
func (r *skillResolver) itemBonus(slot build.Slot, gem build.Gem, tags []string) (level, quality int) {
	for s, it := range r.items {
		if s.Swap() {
			continue
		}
		for _, m := range it.Mods {
			for _, sv := range m.Stats {
				l, q := gemBonus(r.bonuses, sv.Stat, float64(sv.Value), func(b GemBonus) bool {
					return (b.Reach == Equipped || b.Reach == Socketed && s == slot) && b.Gems.matches(gem.Support, tags)
				})
				level, quality = level+l, quality+q
			}
		}
	}
	return level, quality
}

// supportedBonus returns the levels and quality a support's stats add to the active skill gem
// it supports, which is a support or not and has tags.
func supportedBonus(bonuses []GemBonus, stats []StatValue, support bool, tags []string) (level, quality int) {
	for _, sv := range stats {
		l, q := gemBonus(bonuses, sv.Stat, sv.Value, func(b GemBonus) bool { return b.Reach == Supported && b.Gems.matches(support, tags) })
		level, quality = level+l, quality+q
	}
	return level, quality
}

// gemBonus returns the levels and quality stat with value adds through the bonuses that reach.
func gemBonus(bonuses []GemBonus, stat string, value float64, reach func(GemBonus) bool) (level, quality int) {
	for _, b := range bonuses {
		if b.Stat != stat || !reach(b) {
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
