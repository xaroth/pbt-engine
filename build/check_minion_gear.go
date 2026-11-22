package build

import (
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/xaroth/pbt-engine/dataset"
)

// Warning codes of Check for the gear of a group's minion.
const (
	MinionGearUnused Code = "minion-gear-unused"
	MinionSlotUnused Code = "minion-slot-unused"
)

// checkMinionGear checks the gear of group i's minion as checkItems does the character's,
// after the group's gems: all of it is left out when the group's main active skill has no
// minion that takes gear, and an item in a slot its minion does not use. Each of those has a
// warning. Without the dataset's items group, it warns and keeps the gear as it is.
func (c *Checked) checkMinionGear(i int, skills *dataset.SkillIndex) error {
	g := &c.build.Skills.Groups[i]
	if len(g.MinionItems) == 0 {
		return nil
	}
	subject := fmt.Sprintf("skill group %d minion", i+1)
	skill := ""
	if len(g.Gems) > 0 && len(ActiveSkills(skills, g.Gems[g.Active])) > 0 {
		skill = GemSkill(skills, g.Gems[g.Active], g.ActiveSkill)
	}
	if MinionSlots(skill) == nil {
		c.warnf(MinionGearUnused, subject, "the group's main skill has no minion that takes gear; its %d items left out", len(g.MinionItems))
		g.MinionItems = nil
		return nil
	}
	data, err := c.store.Items()
	if _, missing := errors.AsType[*dataset.MissingGroupError](err); missing {
		c.warnf(ItemsUnchecked, subject, "dataset %s has no items group; items kept unchecked", c.store.Manifest().Version)
		return nil
	}
	if err != nil {
		return fmt.Errorf("build: load items: %w", err)
	}
	if _, err := c.store.Stats(); err != nil {
		return fmt.Errorf("build: load stats: %w", err)
	}
	c.checkMinionItems(subject, skill, g, data)
	return nil
}

// checkMinionItems checks each item of g's minion gear, in a slot the minion of the skill
// uses, as checkItems does the character's; subject names the gear.
func (c *Checked) checkMinionItems(subject, skill string, g *SkillGroup, data *dataset.Items) {
	uses := MinionSlots(skill)
	for _, s := range slices.Sorted(maps.Keys(g.MinionItems)) {
		it := g.MinionItems[s]
		about := Warning{Subject: subject + " " + string(s)}
		if !slices.Contains(uses, s) {
			c.warnAboutf(about, MinionSlotUnused, "the minion of %s uses no item in this slot; item left out", skill)
			delete(g.MinionItems, s)
			continue
		}
		fits := c.checkItemFits(about, s, &it, data, func(j int) Warning {
			return Warning{Subject: fmt.Sprintf("%s abyss jewel %s %d", subject, s, j)}
		})
		if !fits {
			delete(g.MinionItems, s)
			continue
		}
		g.MinionItems[s] = it
	}
	if len(g.MinionItems) == 0 {
		g.MinionItems = nil
	}
}
