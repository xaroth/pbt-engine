package build

import (
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/xaroth/pbt-engine/dataset"
)

// Warning codes of Check for granted skills.
const (
	// GrantedSkillUnknown is a granted skill the dataset does not have as an active skill; its
	// group is removed.
	GrantedSkillUnknown Code = "granted-skill-unknown"
	// GrantedSkillRemoved is a granted skill whose source does not grant it, such as an item
	// taken off; its group is removed. One in a group no item or passive grants is dropped.
	GrantedSkillRemoved Code = "granted-skill-removed"
	// GrantedSkillLevel is a granted skill at another level than its source grants; the grant's
	// level is used.
	GrantedSkillLevel Code = "granted-skill-level"
)

// ruthlessVariant is the tree variant whose items grant their Ruthless skills.
const ruthlessVariant = "ruthless"

// grant is a skill an item or passive grants.
type grant struct {
	source SkillSource
	skill  string
	level  int
}

// findGrants finds the skills the allocated passives and the items grant, the weapon swap's
// included, in node id and then slot order. It notes when the dataset does not say what the
// items or the tree grant, so the groups of that source are kept as they are.
func (c *Checked) findGrants() error {
	c.treeGrantsUnknown = !slices.ContainsFunc(c.tree.Nodes, func(n dataset.Node) bool { return n.GrantedSkill != nil })
	for _, n := range c.nodes {
		if g := n.GrantedSkill; g != nil {
			c.addGrant(SkillSource{Node: n.ID}, *g)
		}
	}
	if len(c.build.Items) == 0 {
		return nil
	}
	x, err := c.store.ItemGrants()
	if _, missing := errors.AsType[*dataset.MissingGroupError](err); missing {
		c.itemGrantsUnknown = true
		return nil
	}
	if err != nil {
		return fmt.Errorf("build: load item grants: %w", err)
	}
	c.itemGrantsUnknown = x.None()
	ruthless := c.build.Variant == ruthlessVariant
	for _, s := range slices.Sorted(maps.Keys(c.build.Items)) {
		for _, m := range c.build.Items[s].Mods {
			c.addLineGrants(SkillSource{Item: s}, m, x, ruthless)
		}
	}
	return nil
}

// addLineGrants adds the skills line m grants, each once: a mod's other stats, such as the
// trigger support of a skill it grants, name the skill again.
func (c *Checked) addLineGrants(src SkillSource, m ModLine, x *dataset.ItemGrants, ruthless bool) {
	var seen []string
	for _, sv := range m.Stats {
		for _, g := range x.Skills(sv.Stat, sv.Value, ruthless) {
			if !slices.Contains(seen, g.Effect) {
				seen = append(seen, g.Effect)
				c.addGrant(src, g)
			}
		}
	}
}

func (c *Checked) addGrant(src SkillSource, g dataset.GrantedSkill) {
	c.skillGrants = append(c.skillGrants, grant{source: src, skill: g.Effect, level: int(g.Level)})
}

// checkGrants matches the groups items and passives grant with the grants: a group keeps its
// grant, at the grant's level; one whose source does not grant its skill, or whose skill the
// dataset does not have, is removed; a grant without a group gets one, socketed in its item's
// slot when that has sockets. A granted skill in a group of the player's gems is dropped.
func (c *Checked) checkGrants(data *dataset.SkillIndex) {
	sk := &c.build.Skills
	claimed := make([]bool, len(c.skillGrants))
	var removed []int
	for i := range sk.Groups {
		if !c.matchGrant(i, data, claimed) {
			removed = append(removed, i)
		}
	}
	for _, i := range slices.Backward(removed) {
		_ = sk.apply(RemoveSkillGroup(i))
	}
	for k, gr := range c.skillGrants {
		if claimed[k] {
			continue
		}
		g := SkillGroup{Source: &SkillSource{Item: gr.source.Item, Node: gr.source.Node}, Gems: []Gem{{Skill: gr.skill, Level: gr.level}}}
		if slices.Contains(Slots(), gr.source.Item) {
			g.Slot = gr.source.Item
		}
		sk.Groups = append(sk.Groups, g)
	}
}

// matchGrant matches group i with an unclaimed grant of its source and skill and claims it, or
// reports false to remove the group. A group of the player's gems is kept, without its granted
// skills, and so is one whose source the dataset does not say the grants of.
func (c *Checked) matchGrant(i int, data *dataset.SkillIndex, claimed []bool) bool {
	g := &c.build.Skills.Groups[i]
	subject := fmt.Sprintf("skill group %d", i+1)
	first := slices.IndexFunc(g.Gems, func(x Gem) bool { return x.Skill != "" })
	c.dropGrantedGems(subject, g, first)
	switch {
	case g.Source == nil:
		return true
	case first < 0:
		c.warnf(GrantedSkillRemoved, subject, "granted by %s, it holds no granted skill; removed", g.Source)
		return false
	case !activeEffect(data, g.Gems[first].Skill):
		c.warnf(GrantedSkillUnknown, subject, "granted skill %s is not an active skill in %s; removed", g.Gems[first].Skill, c.store.Manifest().Version)
		return false
	case c.grantsUnknown(*g.Source):
		return true
	}
	gem := &g.Gems[first]
	k := c.unclaimedGrant(*g.Source, *gem, claimed)
	if k < 0 {
		c.warnf(GrantedSkillRemoved, subject, "%s does not grant %s; removed", g.Source, gem.Skill)
		return false
	}
	claimed[k] = true
	if l := c.skillGrants[k].level; gem.Level != l {
		c.warnf(GrantedSkillLevel, subject, "%s at level %d, %s grants level %d; %d used", gem.Skill, gem.Level, g.Source, l, l)
		gem.Level = l
	}
	return true
}

// unclaimedGrant returns the index of the unclaimed grant of gem's skill from src, at gem's
// level when there is one, such as for an item that grants a skill twice at two levels; -1 for
// none.
func (c *Checked) unclaimedGrant(src SkillSource, gem Gem, claimed []bool) int {
	k := -1
	for j, gr := range c.skillGrants {
		if claimed[j] || gr.source != src || gr.skill != gem.Skill {
			continue
		}
		if gr.level == gem.Level {
			return j
		}
		if k < 0 {
			k = j
		}
	}
	return k
}

// dropGrantedGems drops the granted skills of g but the one at keep, which is its granted skill
// when an item or passive grants g.
func (c *Checked) dropGrantedGems(subject string, g *SkillGroup, keep int) {
	if g.Source == nil {
		keep = -1
	}
	for j, gem := range slices.Backward(g.Gems) {
		if gem.Skill == "" || j == keep {
			continue
		}
		c.warnf(GrantedSkillRemoved, fmt.Sprintf("%s gem %d", subject, j+1), "granted skill %s is not the skill its group's source grants; dropped", gem.Skill)
		g.removeGem(j)
	}
}

// grantsUnknown reports whether the dataset does not say what src's kind of source grants.
func (c *Checked) grantsUnknown(src SkillSource) bool {
	if src.Item != "" {
		return c.itemGrantsUnknown
	}
	return c.treeGrantsUnknown
}
