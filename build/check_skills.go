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
	ActiveGemInvalid     Code = "active-gem-invalid"
	MainSkillOutOfRange  Code = "main-skill-out-of-range"
)

// checkSkills checks the skill groups against the dataset's skills group. Without one, it
// warns once and keeps the groups as they are.
func (c *Checked) checkSkills() error {
	sk := &c.build.Skills
	if !slices.ContainsFunc(sk.Groups, func(g SkillGroup) bool { return len(g.Gems) > 0 }) {
		return nil
	}
	data, err := c.store.Skills()
	if _, missing := errors.AsType[*dataset.MissingGroupError](err); missing {
		c.warnf(SkillsUnchecked, "skills", "dataset %s has no skills group; gems kept unchecked", c.store.Manifest().Version)
		return nil
	}
	if err != nil {
		return fmt.Errorf("build: load skills: %w", err)
	}
	effects := grantedEffects(data)
	for i := range sk.Groups {
		c.checkGroup(i, data, effects)
	}
	if !sk.hasGroup(sk.MainGroup) {
		c.warnf(MainSkillOutOfRange, "skills", "main skill group %d of %d groups; group 1 used", sk.MainGroup+1, len(sk.Groups))
		sk.MainGroup = 0
	}
	return nil
}

func grantedEffects(s *dataset.Skills) map[string]*dataset.GrantedEffect {
	out := make(map[string]*dataset.GrantedEffect, len(s.GrantedEffects))
	for i := range s.GrantedEffects {
		out[s.GrantedEffects[i].ID] = &s.GrantedEffects[i]
	}
	return out
}

// checkGroup checks group i: its slot, each gem, then its active gem among the gems kept.
func (c *Checked) checkGroup(i int, data *dataset.Skills, effects map[string]*dataset.GrantedEffect) {
	g := &c.build.Skills.Groups[i]
	subject := fmt.Sprintf("skill group %d", i+1)
	if g.Slot != "" && !slices.Contains(Slots(), g.Slot) {
		c.warnf(SlotUnknown, subject, "slot %q has no sockets; cleared", g.Slot)
		g.Slot = ""
	}
	active := -1
	var kept []Gem
	for j, gem := range g.Gems {
		if !c.checkGem(fmt.Sprintf("%s gem %d", subject, j+1), &gem, data, effects) {
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
func (c *Checked) checkActiveSkill(subject string, g *SkillGroup, data *dataset.Skills) {
	skills := ActiveSkills(data, g.Gems[g.Active])
	if !slices.Contains(skills, g.ActiveSkill) {
		c.warnf(ActiveGemInvalid, subject, "skill %d of gem %d is not an active skill; skill %d used", g.ActiveSkill+1, g.Active+1, skills[0]+1)
		g.ActiveSkill = skills[0]
	}
}

// ActiveSkills returns which skills of gem are active skills, by index: 0 the gem's first
// skill, 1 its second. An active gem's first skill is one, and so is a Vaal gem's second (its
// regular skill); a support's second is one when it grants an active skill, such as Predator's
// Signal Prey. Nil for a gem data does not have.
func ActiveSkills(data *dataset.Skills, gem Gem) []int {
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
	if i := slices.IndexFunc(data.GrantedEffects, func(ge dataset.GrantedEffect) bool { return ge.ID == e.SecondaryGrantedEffect }); i >= 0 &&
		!data.GrantedEffects[i].Support {
		out = append(out, 1)
	}
	return out
}

// checkGem fixes gem as the dataset says, or reports false to drop it.
func (c *Checked) checkGem(subject string, gem *Gem, data *dataset.Skills, effects map[string]*dataset.GrantedEffect) bool {
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
	c.clampGem(subject, gem, maxGemLevel(effects[effect.GrantedEffect]))
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

// clampGem clamps the gem's level to 1 to top, unless top is 0, and its quality.
func (c *Checked) clampGem(subject string, gem *Gem, top int) {
	if l := gem.Level; top > 0 && (l < 1 || l > top) {
		gem.Level = min(max(l, 1), top)
		c.warnf(GemLevelOutOfRange, subject, "level %d is outside 1 to %d; %d used", l, top, gem.Level)
	}
	if q := gem.Quality; q < 0 || q > MaxGemQuality {
		gem.Quality = min(max(q, 0), MaxGemQuality)
		c.warnf(GemQualityOutOfRange, subject, "quality %d is outside 0 to %d; %d used", q, MaxGemQuality, gem.Quality)
	}
}

// maxGemLevel returns the highest level of ge, 0 when it has no levels.
func maxGemLevel(ge *dataset.GrantedEffect) int {
	if ge == nil || len(ge.Levels) == 0 {
		return 0
	}
	return int(ge.Levels[len(ge.Levels)-1].Level)
}
