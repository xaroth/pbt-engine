package poe1

import (
	"math"

	"github.com/xaroth/pbt-engine/internal/core"
)

// Engine stats of accuracy. accuracy_rating is an attack's accuracy rating; hits_cannot_be_evaded
// is a flag that makes its hits always land. hit_chance is the chance to hit in percent, and
// effective_critical_strike_chance the chance that a use of the skill hits and is critical.
const (
	statAccuracyRating                = "accuracy_rating"
	statHitsCannotBeEvaded            = "hits_cannot_be_evaded"
	statHitChance                     = "hit_chance"
	statEffectiveCriticalStrikeChance = "effective_critical_strike_chance"
)

// Chance to hit limits and the formula's constants. They follow PoB (CalcDefence.lua
// calcs.hitChance); the game data read here gives none of them.
const (
	minimumHitChance = 5
	hitChanceScale   = 125
	evasionDivisor   = 5
	evasionExponent  = 0.9
	alwaysHitChance  = percentScale
)

// Formula names shown in explanations.
const (
	formulaHitChance               = "chance to hit"
	formulaEffectiveCriticalChance = "critical strike chance of a use that hits"
)

// mechanicAccuracy names the accuracy mechanic for those that come after it.
const mechanicAccuracy = "accuracy"

func accuracyMechanic() mechanic {
	return mechanic{name: mechanicAccuracy, after: []string{mechanicDamage, mechanicEnemy}, stats: []string{
		statAccuracyRating,
		statHitsCannotBeEvaded,
		statHitChance,
		statEffectiveCriticalStrikeChance,
	}, def: accuracyDef}
}

// accuracyDef completes def when it is a stat of accuracy or the chance to hit.
//
// Accuracy is rounded down and at least 0, the chance to hit from 0 to 100%. An attack's chance to hit is accuracy / (accuracy +
// (evasion / 5)^0.9) x 125%, rounded half up, from 5% to 100%, against the enemy's evasion; it
// always hits while hits_cannot_be_evaded is set. Other hits take their chance to hit from
// their modifiers. A critical strike counts only on a hit that lands, so the chance that a use
// strikes critically is the critical strike chance times the chance to hit. All follow PoB
// (CalcOffence.lua, the hit chance and the crit confirmation roll).
func accuracyDef(def core.StatDef, ids map[string]core.StatID) core.StatDef {
	switch def.Name {
	case statAccuracyRating:
		def.Round, def.HasMin = core.RoundFloor, true
	case statHitsCannotBeEvaded:
		def.Flag = true
	case statHitChance:
		def.HasMin, def.HasMax, def.Max = true, true, percentScale
		h := hitChance{
			accuracy: mustID(ids, statAccuracyRating), evasion: mustID(ids, statEnemyEvasion),
			cannotBeEvaded: mustID(ids, statHitsCannotBeEvaded), attack: attackMask(),
		}
		def.FormulaName, def.Formula, def.Attribute = formulaHitChance, h.formula(), h.attribute()
	case statEffectiveCriticalStrikeChance:
		h := effectiveCriticalChance{chance: mustID(ids, statCriticalStrikeChance), hit: mustID(ids, statHitChance)}
		def.FormulaName, def.Formula, def.Attribute = formulaEffectiveCriticalChance, h.formula(), h.attribute()
	}
	return def
}

// chanceToHit is the chance in percent that a hit with accuracy lands on an enemy with
// evasion. Against no evasion and no accuracy it lands.
func chanceToHit(accuracy, evasion float64) float64 {
	d := accuracy + math.Pow(max(evasion, 0)/evasionDivisor, evasionExponent)
	if d <= 0 {
		return alwaysHitChance
	}
	return min(max(roundHalf(max(accuracy, 0)/d*hitChanceScale), minimumHitChance), alwaysHitChance)
}

// hitChance is a skill's chance to hit, read in its context.
type hitChance struct {
	accuracy, evasion, cannotBeEvaded core.StatID
	// attack is the Attack type's context bit.
	attack core.Mask
}

// evaded reports whether a hit in context mask m can be evaded, with stat reading a stat.
func (h hitChance) evaded(m core.Mask, stat func(core.StatID) float64) bool {
	return m.Intersects(h.attack) && stat(h.cannotBeEvaded) == 0
}

func (h hitChance) formula() core.Formula {
	return func(r core.Reader, agg core.Agg) float64 {
		switch {
		case !r.Mask().Intersects(h.attack):
			return agg.Default()
		case !h.evaded(r.Mask(), r.Stat):
			return alwaysHitChance
		}
		return chanceToHit(r.Stat(h.accuracy), r.Stat(h.evasion))
	}
}

// attribute credits an attack's chance to hit to its accuracy's sources, or to the sources
// that make it always hit; another hit's by the default rule.
func (h hitChance) attribute() core.Attribute {
	return func(a *core.Attributor) core.Attribution {
		total := a.Explanation().Total
		switch {
		case !a.Mask().Intersects(h.attack):
			return core.Attribution{Parts: a.Default(), Amount: total}
		case !h.evaded(a.Mask(), a.Stat):
			return core.Attribution{Parts: a.Spread(core.SelfRef(h.cannotBeEvaded), total, core.OpBase), Amount: total}
		}
		return core.Attribution{Parts: a.Spread(core.SelfRef(h.accuracy), total, core.OpBase), Amount: total}
	}
}

// effectiveCriticalChance is the chance that a use of a skill hits and strikes critically.
type effectiveCriticalChance struct {
	chance, hit core.StatID
}

func (h effectiveCriticalChance) formula() core.Formula {
	return func(r core.Reader, _ core.Agg) float64 {
		return r.Stat(h.chance) * r.Stat(h.hit) / percentScale
	}
}

// attribute credits the critical strike chance's sources with their share of the hits that
// land, and the chance to hit's sources with what misses take off.
func (h effectiveCriticalChance) attribute() core.Attribute {
	return func(a *core.Attributor) core.Attribution {
		total := a.Explanation().Total
		parts, _ := a.Of(core.SelfRef(h.chance))
		return core.Attribution{Parts: rest(a, scaled(parts, a.Stat(h.hit)/percentScale), total, core.SelfRef(h.hit)), Amount: total}
	}
}
