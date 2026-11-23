package poe1

import (
	"fmt"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/internal/core"
	"github.com/xaroth/pbt-engine/internal/resolve"
)

// MinionCountAboveLimit is a skill group counted as more Animated Weapons than the build's
// skills allow: the count is used, as PoB uses it.
const MinionCountAboveLimit build.Code = "minion-count-above-limit"

// statAnimatedWeaponsAllowed is the number of Animated Weapons the character's skills allow,
// read on the character in any context. statBaseAnimatedWeaponsAllowed is a skill's own.
const (
	statAnimatedWeaponsAllowed     = "animated_weapons_allowed"
	statBaseAnimatedWeaponsAllowed = "base_number_of_animated_weapons_allowed"
)

func minionLimitMechanic() mechanic {
	return mechanic{name: "minion-limits", stats: []string{statAnimatedWeaponsAllowed}}
}

// animatedWeaponsAllowed returns the number of Animated Weapons the skills of all allow: the
// most any one allows, as PoB's ActiveAnimatedWeaponLimit is (CalcPerform.lua minionCounts).
func animatedWeaponsAllowed(all []resolve.SkillWarnings) int {
	n := 0
	for _, sw := range all {
		if sw.Skill != nil && len(sw.Skill.StatSets) > 0 {
			n = max(n, int(statOf(sw.Skill.Set().Stats, statBaseAnimatedWeaponsAllowed)))
		}
	}
	return n
}

// countWarnings warns once about each group of groups whose main active skill, in all, summons
// Animated Weapons and whose count is above allowed.
func countWarnings(all []resolve.SkillWarnings, groups []build.SkillGroup, allowed int) []build.Warning {
	var out []build.Warning
	for _, sw := range all {
		sk := sw.Skill
		if sk == nil || len(sk.StatSets) == 0 || statOf(sk.Set().Stats, statBaseAnimatedWeaponsAllowed) == 0 {
			continue
		}
		i := sk.Source.Group
		if i < 0 || i >= len(groups) {
			continue
		}
		g := &groups[i]
		if sk.Source.Gem != g.Active || sk.Which != g.ActiveSkill || g.ActiveCount() <= allowed {
			continue
		}
		out = append(out, build.Warning{
			Code: MinionCountAboveLimit, Subject: fmt.Sprintf("skill group %d", i+1),
			Message: fmt.Sprintf("count %d is above the %d Animated Weapons allowed; the count is used", g.ActiveCount(), allowed),
		})
	}
	return out
}

// addMinionLimits adds the number of Animated Weapons allowed, 0 until SetAnimatedWeaponsAllowed.
func (sh *Sheet) addMinionLimits() error {
	src := sh.newSource(Source{RuleID: "animated-weapons-allowed", Rule: "Animated Weapons the skills allow"})
	id, err := sh.session.AddModifier(core.Modifier{Stat: sh.stats[statAnimatedWeaponsAllowed], Op: core.OpBase, Source: src})
	if err != nil {
		return fmt.Errorf("sheet: animated weapons allowed: %w", err)
	}
	sh.animatedWeapons = id
	return nil
}

// SetAnimatedWeaponsAllowed sets the number of Animated Weapons the build's skills allow
// (SheetSkills.AnimatedWeapons).
func (sh *Sheet) SetAnimatedWeaponsAllowed(n int) {
	// The modifier is this session's, so this cannot fail.
	_ = sh.session.SetModifierValue(sh.animatedWeapons, float64(n))
}
