package poe1

import (
	"slices"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/dataset"
)

// lifeTag is the mod tag of Life Modifiers.
const lifeTag = "life"

// ModTags gives the tags of an item's mod lines from the dataset's mods. A line does not name
// its mod: its mods are those that give every stat of the line and no stat the item lacks, as a
// mod of several stats shows each on a line of its own. The tiers of one mod and the uniques'
// copies of it share their tags.
type ModTags struct {
	// byStat holds, per stat id, the stat ids and tags of each mod that gives it.
	byStat map[string][]modTags
}

type modTags struct {
	stats, tags []string
}

// NewModTags indexes the tags of mods, whose stats are handles into stats.
func NewModTags(mods []dataset.Mod, stats []dataset.Stat) *ModTags {
	t := &ModTags{byStat: map[string][]modTags{}}
	for i := range mods {
		m := &mods[i]
		ids := make([]string, 0, len(m.Stats))
		for _, st := range m.Stats {
			if int(st.Stat) < len(stats) && !slices.Contains(ids, stats[st.Stat].ID) {
				ids = append(ids, stats[st.Stat].ID)
			}
		}
		for _, id := range ids {
			t.byStat[id] = append(t.byStat[id], modTags{ids, m.Tags})
		}
	}
	return t
}

// LifeModifiers reports whether it has a Life Modifier: a mod line one of whose mods has the
// life tag.
func (t *ModTags) LifeModifiers(it *build.Item) bool { return t.has(it, lifeTag) }

func (t *ModTags) has(it *build.Item, tag string) bool {
	var all []string
	for _, l := range it.Mods {
		for _, sv := range l.Stats {
			all = append(all, sv.Stat)
		}
	}
	for _, l := range it.Mods {
		if len(l.Stats) == 0 {
			continue
		}
		line := make([]string, len(l.Stats))
		for i, sv := range l.Stats {
			line[i] = sv.Stat
		}
		for _, m := range t.byStat[line[0]] {
			if containsAll(m.stats, line) && containsAll(all, m.stats) && slices.Contains(m.tags, tag) {
				return true
			}
		}
	}
	return false
}

func containsAll(set, of []string) bool {
	for _, s := range of {
		if !slices.Contains(set, s) {
			return false
		}
	}
	return true
}
