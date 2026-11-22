package pob

import (
	"fmt"
	"slices"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/pob/internal/pobxml"
)

// minionGear returns the gear of the minion of skill, the GrantedEffect id of a group's main
// skill, from pg, that skill's gem: the items of the item set pg names, or of the file's first
// set when the file has no set of that id, as PoB takes them (CalcActiveSkill.lua), in the
// slots the minion uses (build.MinionSlots). None for a skill whose minion takes no gear. An
// item that does not read warns; subject names the group.
func (im *importer) minionGear(subject, skill string, pg pobxml.Gem, read readItem) (build.Items, error) {
	var out build.Items
	uses := build.MinionSlots(skill)
	sets := im.file.ItemSets
	if uses == nil || len(sets) == 0 {
		return out, nil
	}
	set := sets[0]
	if i := slices.IndexFunc(sets, func(s pobxml.ItemSet) bool { return s.ID == pg.MinionItemSet }); i >= 0 {
		set = sets[i]
	}
	for _, e := range set.Slots {
		slot, ok := ItemSlot(e.Slot)
		j := slices.IndexFunc(im.file.Items, func(it pobxml.Item) bool { return it.ID == e.Item })
		if !ok || j < 0 || !slices.Contains(uses, slot) {
			continue
		}
		if err := im.readMinionItem(&out, slot, im.file.Items[j], read); err != nil {
			return nil, fmt.Errorf("pob: %s's item set %d: %w", subject, set.ID, err)
		}
	}
	return out, nil
}

// readMinionItem reads it into out at slot and marks it used, or warns.
func (im *importer) readMinionItem(out *build.Items, slot build.Slot, it pobxml.Item, read readItem) error {
	item, warnings, err := read(it)
	im.itemWarnings(it, warnings)
	switch {
	case err == nil:
		if *out == nil {
			*out = build.Items{}
		}
		(*out)[slot] = item
		if im.minionItems == nil {
			im.minionItems = map[int]bool{}
		}
		im.minionItems[it.ID] = true
	case ignorable(err):
		im.warnf(ItemIgnored, itemSubject(it), "a minion's item not read: %v; not imported", err)
	default:
		return err
	}
	return nil
}
