package resolve

import (
	"slices"
	"strings"

	"github.com/xaroth/pbt-engine/dataset"
)

// supportedTypes applies the supports that can support skill ge of gem src (see candidates):
// it sets sk.Supports and returns the skill's types with those the supports add, indexed as
// Skills.Types. A support applies when the skill can be supported, shares a weapon class with
// the support if the support restricts them, and has the types the support allows and none it
// excludes. Types a support adds can let another support apply, so supports are tried until
// none more apply. Every support is then checked again against the final types, so one that a
// later support's types exclude does not apply whatever the order; the types it added stay. A
// support gem of the skill's group that does not apply warns.
func (r *skillResolver) supportedTypes(src GemSource, ge *dataset.GrantedEffect, sk *Skill) []bool {
	active := r.data.ActiveSkill(ge.ActiveSkill)
	types, minion := r.typeSet(active.Types), r.typeSet(active.MinionTypes)
	pending := r.candidates(src)
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
			if sp.own {
				r.warnf(SupportNotApplied, sp.src, "%s does not support %s: %s; left out", sp.effect.ID, ge.ID, why)
			}
			continue
		}
		sk.Supports = append(sk.Supports, r.support(sp))
	}
	return types
}

// supportGem is a support that can support a skill, at its level and quality with what items
// add to a gem.
type supportGem struct {
	src            Source
	effect         *dataset.GrantedEffect
	level, quality int
	// own is a gem of the skill's group.
	own bool
}

// typeSet returns ts, indexes into Skills.Types, as a set indexed the same way.
func (r *skillResolver) typeSet(ts []uint16) []bool {
	out := make([]bool, len(r.data.Types))
	for _, t := range ts {
		out[t] = true
	}
	return out
}

// candidates returns the supports that can support gem src: those the item it is socketed in
// grants, its group's other gems', then those socketed in the slots linked to its own and those
// the items there grant.
func (r *skillResolver) candidates(src GemSource) []supportGem {
	slot := r.groups[src.Group].Slot
	l := supportList{r: r}
	if slot != "" {
		l.add(r.itemSupports[slot]...)
	}
	own := r.groupSupports(src.Group, src.Gem)
	for i := range own {
		own[i].own = true
	}
	l.add(own...)
	if slot == "" || slot.Swap() {
		return l.list
	}
	for _, from := range r.linked[slot] {
		l.add(r.itemSupports[from]...)
		for i, g := range r.groups {
			if g.Slot == from {
				l.add(r.groupSupports(i, -1)...)
			}
		}
	}
	return l.list
}

// supportList is a list of supports with one of each effect: the one at the highest level, then
// quality, in the place of the first, as in Path of Building. A gem of the skill's group left
// out warns.
type supportList struct {
	r    *skillResolver
	list []supportGem
}

func (l *supportList) add(sps ...supportGem) {
	for _, sp := range sps {
		k := slices.IndexFunc(l.list, func(o supportGem) bool { return o.effect == sp.effect })
		if k < 0 {
			l.list = append(l.list, sp)
			continue
		}
		keep, drop := l.list[k], sp
		if sp.level > keep.level || sp.level == keep.level && sp.quality > keep.quality {
			keep, drop = sp, keep
			l.list[k] = sp
		}
		if drop.own {
			l.r.warnf(SupportNotApplied, drop.src, "%s of %s at level %d quality %d applies instead; left out", drop.effect.ID, keep.src, keep.level, keep.quality)
		}
	}
}

// groupSupports returns the enabled support effects of group i's gems other than gem skip:
// those of support gems, and the support an active gem grants as its second effect, such as
// the one with which a gem's skill supports warcries in its group.
func (r *skillResolver) groupSupports(i, skip int) []supportGem {
	var out []supportGem
	for j, gem := range r.groups[i].Gems {
		if j == skip || gem.Disabled {
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
		ge := r.data.GrantedEffect(id)
		if ge == nil || !ge.Support {
			continue
		}
		src := GemSource{Group: i, Gem: j, ID: gem.ID}
		level, quality := r.characterBonus(src, gem, r.tags(gem))
		out = append(out, supportGem{src: src, effect: ge, level: gem.Level + level, quality: gem.Quality + quality})
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

// support returns the support sp as it applies, with the stats of its primary stat set.
func (r *skillResolver) support(sp supportGem) Support {
	out := Support{Source: sp.src, GrantedEffect: sp.effect.ID, Level: sp.level, Quality: sp.quality}
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
