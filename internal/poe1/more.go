package poe1

import (
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/xaroth/pbt-engine/internal/core"
	"github.com/xaroth/pbt-engine/internal/semantics"
)

// PoB multiplies the more multipliers of one mod name exactly and rounds the product, then
// multiplies the rounded products of every name it is asked for, and does the same for a
// skill's own mod list and for the character's, apart (ModList.lua and ModDB.lua
// MoreInternal). The game's own rule is not known; these rules follow PoB's.
//
// An engine stat stands for the PoB names that differ only in the damage type they are
// limited to: damage holds Damage, PhysicalDamage, ElementalDamage and the like. So a more
// modifier's group is its moreName, from its damage type context, and whether it comes from
// the skill's own stats (skillStore). Skill types, forms and weapons are flags in PoB, not
// part of the name.

// moreName is the part of a more modifier's PoB mod name its engine stat does not give.
type moreName core.MoreGroup

// More names: moreGeneric and moreElemental, or a DamageType in between.
const (
	moreGeneric   moreName = 0
	moreElemental moreName = moreName(Chaos) + 1
	// moreSupportMultiplier is PoB's SupportManaMultiplier: the supports' cost and reservation
	// multipliers.
	moreSupportMultiplier moreName = moreElemental + 1
)

// skillStore marks the group of a more modifier from a skill's own stats.
const skillStore core.MoreGroup = 1 << 8

// supportMultiplierPlaces is the decimal places PoB keeps of SupportManaMultiplier's product,
// rounded down (Data.lua highPrecisionMods).
const supportMultiplierPlaces = 4

// roundMore rounds the product of the more multipliers of one PoB mod name as PoB does: half
// up to hundredths, but down to supportMultiplierPlaces for moreSupportMultiplier, the one
// more name of PoB's highPrecisionMods. Every product of more multipliers goes through it.
func roundMore(name moreName, product float64) float64 {
	if name == moreSupportMultiplier {
		scale := math.Pow10(supportMultiplierPlaces)
		return math.Floor(product*scale) / scale
	}
	return roundHundredths(product)
}

// pobMore is the sheet's core.MoreRule: each group's product rounded, the skill's and the
// character's multiplied apart and then together.
func pobMore(products []core.MoreProduct) float64 {
	skill, character := 1.0, 1.0
	for _, p := range products {
		r := roundMore(moreName(p.Group&^skillStore), p.Value)
		if p.Group&skillStore != 0 {
			skill *= r
		} else {
			character *= r
		}
	}
	return skill * character
}

// moreGroup returns the group of table modifier m, from a skill's own stats when skill is set;
// 0 for a modifier that is not a more multiplier.
func moreGroup(m *semantics.Mod, skill bool) (core.MoreGroup, error) {
	if m.Op != core.OpMore {
		return 0, nil
	}
	name, ok := moreNameOf(m.Context)
	if !ok {
		return 0, fmt.Errorf("%w: no PoB more name for %v", ErrDamageType, m.Context.In)
	}
	g := core.MoreGroup(name)
	if skill {
		g |= skillStore
	}
	return g, nil
}

// moreNameOf returns the more name of a modifier limited to contexts c: the damage type its
// damage term requires, moreElemental for fire, cold and lightning together, moreGeneric
// without a damage term. It is false for other damage terms.
func moreNameOf(c semantics.ContextFilter) (moreName, bool) {
	for _, term := range c.In {
		if !slices.ContainsFunc(term, isDamageContext) {
			continue
		}
		var types []DamageType
		for _, n := range term {
			d, ok := damageContext(n)
			if !ok {
				return 0, false
			}
			types = append(types, d)
		}
		slices.Sort(types)
		switch {
		case len(types) == 1:
			return moreName(types[0]), true
		case slices.Equal(types, []DamageType{Fire, Cold, Lightning}):
			return moreElemental, true
		}
		return 0, false
	}
	return moreGeneric, true
}

func isDamageContext(name string) bool { return strings.HasPrefix(name, dimDamage+".") }

// damageContext returns the damage type of context value name, such as damage.fire.
func damageContext(name string) (DamageType, bool) {
	value, ok := strings.CutPrefix(name, dimDamage+".")
	for d := Physical; ok && d <= Chaos; d++ {
		if d.String() == value {
			return d, true
		}
	}
	return 0, false
}
