package build

import (
	"errors"
	"fmt"
	"slices"

	"github.com/xaroth/pbt-engine/dataset"
)

// MaxGemQuality is the highest gem quality: 20 from currency, 23 by corrupting.
const MaxGemQuality = 23

// Warning codes of Check for skills.
const (
	SkillsUnchecked      Code = "skills-unchecked"
	SlotUnknown          Code = "slot-unknown"
	GemUnknown           Code = "gem-unknown"
	GemEffectUnknown     Code = "gem-effect-unknown"
	GemRoleMismatch      Code = "gem-role-mismatch"
	GemLevelOutOfRange   Code = "gem-level-out-of-range"
	GemQualityOutOfRange Code = "gem-quality-out-of-range"
	GemChoiceOutOfRange  Code = "gem-choice-out-of-range"
	ActiveGemInvalid     Code = "active-gem-invalid"
	MainSkillOutOfRange  Code = "main-skill-out-of-range"
	GroupCountOutOfRange Code = "group-count-out-of-range"
)

// checkSkills checks the skill groups against the dataset's skills group: it matches the groups
// items and passives grant with the grants (checkGrants), drops unknown gems, clears unknown
// variants and slots without sockets, corrects which gems are supports, clamps gem levels,
// quality and group counts, moves a main skill or active gem that does not fit to the first,
// and checks each group's minion gear (checkMinionGear), each with a warning. Without the
// group, it warns once and keeps the groups as they are. The gems of the groups in the weapon
// swap are disabled.
func (c *Checked) checkSkills() error {
	defer c.disableSwap()
	sk := &c.build.Skills
	if len(c.skillGrants) == 0 && !slices.ContainsFunc(sk.Groups, func(g SkillGroup) bool { return len(g.Gems) > 0 || len(g.MinionItems) > 0 }) {
		return nil
	}
	data, err := c.store.SkillIndex()
	if _, missing := errors.AsType[*dataset.MissingGroupError](err); missing {
		c.warnf(SkillsUnchecked, "skills", "dataset %s has no skills group; gems kept unchecked", c.store.Manifest().Version)
		return nil
	}
	if err != nil {
		return fmt.Errorf("build: load skills: %w", err)
	}
	c.checkGrants(data)
	for i := range sk.Groups {
		c.checkGroup(i, data)
		if err := c.checkMinionGear(i, data); err != nil {
			return err
		}
	}
	if !sk.hasGroup(sk.MainGroup) {
		c.warnf(MainSkillOutOfRange, "skills", "main skill group %d of %d groups; group 1 used", sk.MainGroup+1, len(sk.Groups))
		sk.MainGroup = 0
	}
	return nil
}

// checkGroup checks group i: its slot, each gem, then its active gem among the gems kept.
func (c *Checked) checkGroup(i int, data *dataset.SkillIndex) {
	g := &c.build.Skills.Groups[i]
	subject := fmt.Sprintf("skill group %d", i+1)
	if g.Slot != "" && !slices.Contains(Slots(), g.Slot) {
		c.warnf(SlotUnknown, subject, "slot %q has no sockets; cleared", g.Slot)
		g.Slot = ""
	}
	if g.Count < 0 {
		c.warnf(GroupCountOutOfRange, subject, "count %d is below 1; 1 used", g.Count)
		g.Count = 0
	}
	active := -1
	var kept []Gem
	for j, gem := range g.Gems {
		if !c.checkGem(fmt.Sprintf("%s gem %d", subject, j+1), &gem, data) {
			continue
		}
		if j == g.Active {
			active = len(kept)
		}
		kept = append(kept, gem)
	}
	g.Gems = kept
	first := slices.IndexFunc(kept, func(x Gem) bool { return len(ActiveSkills(data, x)) > 0 })
	if first < 0 {
		// Only supports, such as for a skill an item grants: no active gem to choose.
		g.Active, g.ActiveSkill = 0, 0
		return
	}
	if active < 0 || len(ActiveSkills(data, kept[active])) == 0 {
		c.warnf(ActiveGemInvalid, subject, "active gem %d is not a gem with an active skill the group keeps; gem %d used", g.Active+1, first+1)
		active, g.ActiveSkill = first, ActiveSkills(data, kept[first])[0]
	}
	g.Active = active
	c.checkActiveSkill(subject, g, data)
}

// checkActiveSkill moves the group's active skill to the first active skill of its active gem
// when the gem has no such active skill.
func (c *Checked) checkActiveSkill(subject string, g *SkillGroup, data *dataset.SkillIndex) {
	skills := ActiveSkills(data, g.Gems[g.Active])
	if !slices.Contains(skills, g.ActiveSkill) {
		c.warnf(ActiveGemInvalid, subject, "skill %d of gem %d is not an active skill; skill %d used", g.ActiveSkill+1, g.Active+1, skills[0]+1)
		g.ActiveSkill = skills[0]
	}
}

// ActiveSkills returns which skills of gem are active skills, by index: 0 the gem's first
// skill, 1 its second. An active gem's first skill is one, and so is a Vaal gem's second (its
// regular skill); a support's second is one when it grants an active skill, such as Predator's
// Signal Prey. A skill an item or passive grants is its first. Nil for a gem data does not
// have.
func ActiveSkills(data *dataset.SkillIndex, gem Gem) []int {
	if gem.Skill != "" {
		if activeEffect(data, gem.Skill) {
			return []int{0}
		}
		return nil
	}
	dg := data.Gem(gem.ID)
	if dg == nil || len(dg.Effects) == 0 {
		return nil
	}
	e := &dg.Effects[0]
	if x := dg.Effect(gem.Effect); x != nil {
		e = x
	}
	var out []int
	if !dg.Support {
		out = append(out, 0)
	}
	if e.SecondaryGrantedEffect == "" {
		return out
	}
	if ge := data.GrantedEffect(e.SecondaryGrantedEffect); ge != nil && !ge.Support {
		out = append(out, 1)
	}
	return out
}

// GemSkill returns the GrantedEffect id of skill which of gem: 0 its first, 1 its second. A
// skill an item or passive grants is its first. Empty for a gem data does not have.
func GemSkill(data *dataset.SkillIndex, gem Gem, which int) string {
	if gem.Skill != "" {
		if which == 0 {
			return gem.Skill
		}
		return ""
	}
	dg := data.Gem(gem.ID)
	if dg == nil || len(dg.Effects) == 0 {
		return ""
	}
	e := &dg.Effects[0]
	if x := dg.Effect(gem.Effect); x != nil {
		e = x
	}
	if which == 1 {
		return e.SecondaryGrantedEffect
	}
	return e.GrantedEffect
}

// checkGem fixes gem as the dataset says, or reports false to drop it. A granted skill is
// checkGrants's.
func (c *Checked) checkGem(subject string, gem *Gem, data *dataset.SkillIndex) bool {
	if gem.Skill != "" {
		return true
	}
	dg := data.Gem(gem.ID)
	if dg == nil || len(dg.Effects) == 0 {
		c.warnf(GemUnknown, subject, "gem %s not in %s; dropped", gem.ID, c.store.Manifest().Version)
		return false
	}
	effect := c.gemEffect(subject, gem, dg)
	if gem.Support != dg.Support {
		c.warnf(GemRoleMismatch, subject, "%s marked support %t, the gem is %t; corrected", gem.ID, gem.Support, dg.Support)
		gem.Support = dg.Support
	}
	c.clampGem(subject, gem, maxGemLevel(data.GrantedEffect(effect.GrantedEffect)))
	return true
}

// gemEffect returns the variant of dg gem names, clearing a name that is not one of dg's or
// that is its first.
func (c *Checked) gemEffect(subject string, gem *Gem, dg *dataset.Gem) *dataset.GemEffect {
	if gem.Effect == "" {
		return &dg.Effects[0]
	}
	e := dg.Effect(gem.Effect)
	if e == nil {
		c.warnf(GemEffectUnknown, subject, "%s is not a variant of %s; its first used", gem.Effect, gem.ID)
		e = &dg.Effects[0]
	}
	if e == &dg.Effects[0] {
		gem.Effect = ""
	}
	return e
}

// clampGem clamps the gem's level to 1 to top, unless top is 0, its quality, and its stat set
// and stages to at least 0.
func (c *Checked) clampGem(subject string, gem *Gem, top int) {
	if l := gem.Level; top > 0 && (l < 1 || l > top) {
		gem.Level = min(max(l, 1), top)
		c.warnf(GemLevelOutOfRange, subject, "level %d is outside 1 to %d; %d used", l, top, gem.Level)
	}
	if q := gem.Quality; q < 0 || q > MaxGemQuality {
		gem.Quality = min(max(q, 0), MaxGemQuality)
		c.warnf(GemQualityOutOfRange, subject, "quality %d is outside 0 to %d; %d used", q, MaxGemQuality, gem.Quality)
	}
	if gem.StatSet < 0 {
		c.warnf(GemChoiceOutOfRange, subject, "stat set %d is below 0; the primary set used", gem.StatSet)
		gem.StatSet = 0
	}
	if gem.Stages < 0 {
		c.warnf(GemChoiceOutOfRange, subject, "%d stages is below 0; none used", gem.Stages)
		gem.Stages = 0
	}
}

// maxGemLevel returns the highest level of ge, 0 when it has no levels.
func maxGemLevel(ge *dataset.GrantedEffect) int {
	if ge == nil || len(ge.Levels) == 0 {
		return 0
	}
	return int(ge.Levels[len(ge.Levels)-1].Level)
}

// activeEffect reports whether the granted effect id is an active skill.
func activeEffect(data *dataset.SkillIndex, id string) bool {
	ge := data.GrantedEffect(id)
	return ge != nil && !ge.Support && data.ActiveSkill(ge.ActiveSkill) != nil
}
