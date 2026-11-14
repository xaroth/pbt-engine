package resolve

import (
	"slices"
	"strings"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/dataset"
)

// supportedTypes applies the supports of group g to skill ge of gem src: it sets sk.Supports
// and returns the skill's types with those the supports add, indexed as Skills.Types. A
// support applies when the skill can be supported, shares a weapon class with the support if
// the support restricts them, and has the types the support allows and none it excludes. Types
// a support adds can let another support apply, so supports are tried until none more apply.
// Every support is then checked again against the final types, so one that a later support's
// types exclude does not apply whatever the group's order; the types it added stay. The rest
// warn.
func (r *skillResolver) supportedTypes(src GemSource, g build.SkillGroup, ge *dataset.GrantedEffect, sk *Skill) []bool {
	active := r.data.ActiveSkill(ge.ActiveSkill)
	types := make([]bool, len(r.data.Types))
	minion := make([]bool, len(r.data.Types))
	for _, t := range active.Types {
		types[t] = true
	}
	for _, t := range active.MinionTypes {
		minion[t] = true
	}
	pending := r.supportGems(src, g)
	applied := make([]bool, len(pending))
	for progress := true; progress; {
		progress = false
		for k, sp := range pending {
			if applied[k] || r.notSupported(ge, active, sp.effect, types, minion) != "" {
				continue
			}
			applied[k], progress = true, true
			for _, t := range sp.effect.AddedTypes {
				types[t] = true
			}
		}
	}
	for _, sp := range pending {
		if why := r.notSupported(ge, active, sp.effect, types, minion); why != "" {
			r.warnf(SupportNotApplied, sp.src, "%s does not support %s: %s; left out", sp.effect.ID, ge.ID, why)
			continue
		}
		sk.Supports = append(sk.Supports, r.support(g.Slot, sp))
	}
	return types
}

// supportGem is an enabled support gem of a group with its support effect.
type supportGem struct {
	src    GemSource
	gem    build.Gem
	effect *dataset.GrantedEffect
}

// supportGems returns the enabled support effects of group g's gems other than the skill's gem
// src: those of support gems, and the support an active gem grants as its second effect, such
// as the one with which a gem's skill supports warcries in its group.
func (r *skillResolver) supportGems(src GemSource, g build.SkillGroup) []supportGem {
	var out []supportGem
	i := src.Group
	for j, gem := range g.Gems {
		if j == src.Gem || gem.Disabled {
			continue
		}
		e := r.gemEffect(gem)
		if e == nil {
			continue
		}
		id := e.GrantedEffect
		if !gem.Support {
			id = e.SecondaryGrantedEffect
		}
		if ge := r.data.GrantedEffect(id); ge != nil && ge.Support {
			out = append(out, supportGem{src: GemSource{Group: i, Gem: j, ID: gem.ID}, gem: gem, effect: ge})
		}
	}
	return out
}

// notSupported returns why support does not support skill ge, whose active skill is active and
// whose types and minion types are as given; empty when it does.
func (r *skillResolver) notSupported(ge *dataset.GrantedEffect, active *dataset.ActiveSkill, support *dataset.GrantedEffect, types, minion []bool) string {
	if ge.CannotBeSupported {
		return "the skill cannot be supported"
	}
	w := support.SupportWeaponRestrictions
	if len(w) > 0 && !slices.ContainsFunc(active.WeaponRestrictions, func(c string) bool { return slices.Contains(w, c) }) {
		return "the skill is not used with " + strings.Join(w, ", ")
	}
	if len(support.ExcludedTypes) > 0 && r.typesMatch(support.ExcludedTypes, func(t uint16) bool { return types[t] }) {
		return "the skill has a type the support excludes"
	}
	if len(support.AllowedTypes) > 0 && !r.typesMatch(support.AllowedTypes, func(t uint16) bool { return types[t] || minion[t] }) {
		return "the skill lacks the types the support needs"
	}
	return ""
}

// typesMatch evaluates a type expression in the game's postfix form, in which AND, OR and NOT
// are operators and any other type is true when has says so. Values left at the end are
// alternatives: it matches when any is true. The dataset checks the operators' arity.
func (r *skillResolver) typesMatch(expr []uint16, has func(uint16) bool) bool {
	var stack []bool
	pop := func() bool {
		v := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		return v
	}
	for _, t := range expr {
		switch r.data.Types[t] {
		case "AND":
			a, b := pop(), pop()
			stack = append(stack, a && b)
		case "OR":
			a, b := pop(), pop()
			stack = append(stack, a || b)
		case "NOT":
			stack = append(stack, !pop())
		default:
			stack = append(stack, has(t))
		}
	}
	return slices.Contains(stack, true)
}

// support returns the support sp socketed in slot as it applies: at its level and quality with
// what items add, with the stats of its primary stat set.
func (r *skillResolver) support(slot build.Slot, sp supportGem) Support {
	level, quality := r.itemBonus(slot, sp.gem, r.tags(sp.gem))
	out := Support{Source: sp.src, GrantedEffect: sp.effect.ID, Level: sp.gem.Level + level, Quality: sp.gem.Quality + quality}
	if len(sp.effect.Levels) > 0 {
		out.Level = r.clampLevel(sp.src, sp.effect, out.Level)
		row := sp.effect.Levels[rowAt(sp.effect.Levels, out.Level, effectLevel)]
		out.CostMultiplier, out.Reservation = row.CostMultiplier, rowReservation(row)
	}
	if len(sp.effect.StatSets) > 0 {
		out.Stats = r.statSet(r.data.StatSet(sp.effect.StatSets[0]), out.Level, out.Quality, sp.effect.QualityStats).Stats
	}
	return out
}

func (r *skillResolver) typeNames(types []bool) []string {
	var out []string
	for t, ok := range types {
		if ok {
			out = append(out, r.data.Types[t])
		}
	}
	return out
}
