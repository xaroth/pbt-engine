package poe1

import (
	"math"

	"github.com/xaroth/pbt-engine/internal/core"
	"github.com/xaroth/pbt-engine/internal/resolve"
)

// Engine stats of how often a skill hits, read in its context, beside its cast or attack rate
// (hitRate).
//
// hit_interval is the seconds between the hits of a skill that hits on its own at a fixed
// interval once used, such as a brand activating or Winter Orb firing; 0 for a skill that hits
// when used. activations_per_hit is how many of those intervals pass per hit, 1 when 0.
// hit_frequency is the speed of those hits in percent, 100 before modifiers; cast speed
// increases apply to it too where cast_speed_applies_to_hit_frequency is set, and
// cast_speed_increase is their sum.
//
// repeat_count is how many times a use repeats; the repeats of a melee attack can take more
// damage each by repeat_damage_first, _second and _third, and a spell's by
// repeat_damage_per_repeat for each repeat before; repeat_damage is the more damage the repeats
// average to.
//
// brands_per_enemy is how many of a brand skill's brands attach to one enemy, brands_allowed
// how many brands the character may have out, and brands_attached the brands attached to the
// enemy, each hitting it.
const (
	statHitInterval                    = "hit_interval"
	statActivationsPerHit              = "activations_per_hit"
	statHitFrequency                   = "hit_frequency"
	statCastSpeedAppliesToHitFrequency = "cast_speed_applies_to_hit_frequency"
	statCastSpeedIncrease              = "cast_speed_increase"
	statRepeatCount                    = "repeat_count"
	statRepeatDamageFirst              = "repeat_damage_first"
	statRepeatDamageSecond             = "repeat_damage_second"
	statRepeatDamageThird              = "repeat_damage_third"
	statRepeatDamagePerRepeat          = "repeat_damage_per_repeat"
	statRepeatDamage                   = "repeat_damage"
	statBrandsPerEnemy                 = "brands_per_enemy"
	statBrandsAllowed                  = "brands_allowed"
	statBrandsAttached                 = "brands_attached"
)

// rateStats are the stats of rate.go, in the order the hit mechanic registers them: after
// cast_speed, which cast_speed_increase includes.
func rateStats() []string {
	return []string{
		statHitInterval, statActivationsPerHit, statCastSpeedAppliesToHitFrequency, statCastSpeedIncrease,
		statHitFrequency, statRepeatCount, statRepeatDamageFirst, statRepeatDamageSecond, statRepeatDamageThird,
		statRepeatDamagePerRepeat, statRepeatDamage, statBrandsPerEnemy, statBrandsAllowed, statBrandsAttached,
	}
}

// Formula names shown in explanations.
const (
	formulaCastSpeedIncrease = "cast speed increases"
	formulaHitFrequency      = "hit frequency"
	formulaRepeatCount       = "repeats of a use"
	formulaRepeatDamage      = "average more damage of repeats"
	formulaBrandsAttached    = "brands attached to the enemy"
)

// activationsPerHit are the activations of a brand between its hits, for the brands that hit
// other than on each activation, by granted effect: Penance Brand explodes when the enemy it is
// attached to reaches 20 energy, one per activation. The game data gives the number only in
// the gem's description.
func activationsPerHit() map[string]float64 {
	const penanceBrandEnergy = 20
	return map[string]float64{"PenanceBrand": penanceBrandEnergy}
}

// noRepeatTypes are the skill types whose uses do not repeat, as PoB has it (CalcOffence.lua
// repeatSkillTypesCheck).
func noRepeatTypes() []string {
	return []string{typeInstant, typeChannel, typeTriggered, typeRetaliation}
}

// rateDef completes def when it is a stat of rate.go.
//
// A skill with a hit interval hits once per interval times its activations per hit, divided by
// its hit frequency: the base 100% with its own increases and, where the skill says so, cast
// speed increases, times its own more multipliers. Neither its cast speed's more multipliers
// nor action speed apply, and no server tick caps it. A brand's hit damage per second counts
// each brand attached to the enemy: as many as one enemy can have and the character can have
// out, whichever is fewer. Repeats average their more damage over the use and its repeats.
// These follow PoB (CalcOffence.lua hitTimeOverride, the brands' and Winter Orb's
// preDamageFunc, CalcPerform.lua attachedBrandCount, CalcOffence.lua repeats in its default
// Average mode).
func rateDef(def core.StatDef, ids map[string]core.StatID) core.StatDef {
	switch def.Name {
	case statCastSpeedAppliesToHitFrequency:
		def.Flag = true
	case statCastSpeedIncrease:
		def.Includes = []core.StatID{mustID(ids, statCastSpeed)}
		def.FormulaName, def.Formula, def.Attribute = formulaCastSpeedIncrease, castSpeedIncrease, increaseParts
	case statHitFrequency:
		h := hitFrequency{applies: mustID(ids, statCastSpeedAppliesToHitFrequency), increase: mustID(ids, statCastSpeedIncrease)}
		def.FormulaName, def.Formula, def.Attribute = formulaHitFrequency, h.formula(), h.attribute()
	case statRepeatCount:
		def.FormulaName, def.Formula, def.Attribute = formulaRepeatCount, repeatCount(), defaultUnlessZero
	case statRepeatDamage:
		h := repeatDamage{
			count: mustID(ids, statRepeatCount), perRepeat: mustID(ids, statRepeatDamagePerRepeat),
			nth: [3]core.StatID{mustID(ids, statRepeatDamageFirst), mustID(ids, statRepeatDamageSecond), mustID(ids, statRepeatDamageThird)},
		}
		def.FormulaName, def.Formula, def.Attribute = formulaRepeatDamage, h.formula(), h.attribute()
	case statBrandsAttached:
		b := brandsAttached{perEnemy: mustID(ids, statBrandsPerEnemy), allowed: mustID(ids, statBrandsAllowed)}
		def.FormulaName, def.Formula, def.Attribute = formulaBrandsAttached, b.formula(), b.attribute()
	}
	return def
}

// repeatCount is the repeats of a use, none for a skill of a type that does not repeat.
func repeatCount() core.Formula {
	var excluded core.Mask
	for _, t := range noRepeatTypes() {
		excluded = excluded.Or(typeMask(modelContexts(), t))
	}
	return func(r core.Reader, a core.Agg) float64 {
		if r.Mask().Intersects(excluded) {
			return 0
		}
		return a.Default()
	}
}

// defaultUnlessZero attributes a value by the default rule, and a value of 0 to nothing.
func defaultUnlessZero(a *core.Attributor) core.Attribution {
	total := a.Explanation().Total
	if total == 0 {
		return core.Attribution{}
	}
	return core.Attribution{Parts: a.Default(), Amount: total}
}

// brandsAttached is the brands of a skill attached to the enemy: as many as one enemy can have
// and the character can have out, whichever is fewer.
type brandsAttached struct {
	perEnemy, allowed core.StatID
}

// limit returns the stat that limits the brands, with stat reading a stat.
func (b brandsAttached) limit(stat func(core.StatID) float64) core.StatID {
	if stat(b.allowed) < stat(b.perEnemy) {
		return b.allowed
	}
	return b.perEnemy
}

func (b brandsAttached) formula() core.Formula {
	return func(r core.Reader, _ core.Agg) float64 { return max(r.Stat(b.limit(r.Stat)), 0) }
}

// attribute credits the sources of the stat that limits the brands.
func (b brandsAttached) attribute() core.Attribute {
	return func(a *core.Attributor) core.Attribution {
		total := a.Explanation().Total
		if total == 0 {
			return core.Attribution{}
		}
		return core.Attribution{Parts: a.Spread(core.SelfRef(b.limit(a.Stat)), total, core.OpBase), Amount: total}
	}
}

// castSpeedIncrease is the sum of the cast speed increases a node matches.
func castSpeedIncrease(_ core.Reader, a core.Agg) float64 { return a.Inc }

// increaseParts credits each increase with its value: of the default layers over the base, the
// increased terms, which over a base of 100 are their values.
func increaseParts(a *core.Attributor) core.Attribution {
	var parts []core.Part
	for _, p := range a.Layers(a.BaseTerms()) {
		if p.Op == core.OpInc {
			parts = append(parts, p)
		}
	}
	return core.Attribution{Parts: parts, Amount: a.Explanation().Total}
}

// hitFrequency is a skill's hit frequency: its own base, increases and more multipliers, with
// the cast speed increases added to its own where the skill says they apply.
type hitFrequency struct {
	applies, increase core.StatID
}

func (h hitFrequency) castIncrease(stat func(core.StatID) float64) float64 {
	if stat(h.applies) == 0 {
		return 0
	}
	return stat(h.increase)
}

func (h hitFrequency) formula() core.Formula {
	return func(r core.Reader, a core.Agg) float64 {
		return a.Base * (1 + (a.Inc+h.castIncrease(r.Stat))/percentScale) * a.More
	}
}

// attribute credits the frequency's own terms by the default rule and what cast speed
// increases add to their sources.
func (h hitFrequency) attribute() core.Attribute {
	return func(a *core.Attributor) core.Attribution {
		total := a.Explanation().Total
		return core.Attribution{Parts: rest(a, a.Default(), total, core.SelfRef(h.increase)), Amount: total}
	}
}

// repeatDamage is the more damage, in percent, a use's repeats average to: per repeat damage
// grows by its value with each repeat, so it averages half of it per repeat; the first, second
// and third repeat's own more damage average over the use and its repeats. The two multiply.
type repeatDamage struct {
	count, perRepeat core.StatID
	nth              [3]core.StatID
}

func (h repeatDamage) value(stat func(core.StatID) float64) float64 {
	n := stat(h.count)
	if n <= 0 {
		return 0
	}
	sum := 0.0
	for i, s := range h.nth {
		if float64(i) < n {
			sum += stat(s)
		}
	}
	return ((1+stat(h.perRepeat)*n*half/percentScale)*(1+sum/(n+1)/percentScale) - 1) * percentScale
}

func (h repeatDamage) formula() core.Formula {
	return func(r core.Reader, _ core.Agg) float64 { return h.value(r.Stat) }
}

// attribute credits the sources of the repeat damage stats with the whole, in proportion to
// their parts.
func (h repeatDamage) attribute() core.Attribute {
	return func(a *core.Attributor) core.Attribution {
		total := a.Explanation().Total
		if total == 0 {
			return core.Attribution{}
		}
		var parts []core.Part
		sum := 0.0
		for _, s := range append([]core.StatID{h.perRepeat}, h.nth[:]...) {
			p, v := a.Of(core.SelfRef(s))
			if v != 0 {
				parts, sum = append(parts, p...), sum+v
			}
		}
		return core.Attribution{Parts: scaled(parts, total/sum), Amount: total}
	}
}

// rateBases are sk's hit rate bases its granted effect gives beside its stats.
func rateBases(sk *resolve.Skill) []hitBase {
	if n, ok := activationsPerHit()[sk.GrantedEffect]; ok {
		return []hitBase{{statActivationsPerHit, core.OpBase, n}}
	}
	return nil
}

// hitRate is a skill's hits per second: one with a hit interval hits at that, scaled by its hit
// frequency; else an attack's from its attack rate and speed, another skill's from its cast
// time and speed, either times the action speed and capped by the server tick, times the uses
// and their repeats, unless channelled.
type hitRate struct {
	castTime, castSpeed, attackRate, attackSpeed, action core.StatID
	interval, activations, frequency, repeats            core.StatID
	// attack and channel are the Attack and Channel types' context bits.
	attack, channel core.Mask
}

func newHitRate(ids map[string]core.StatID) hitRate {
	c := modelContexts()
	return hitRate{
		castTime: mustID(ids, statCastTime), castSpeed: mustID(ids, statCastSpeed),
		attackRate: mustID(ids, statAttackRate), attackSpeed: mustID(ids, statAttackSpeed),
		action: mustID(ids, statActionSpeed), interval: mustID(ids, statHitInterval),
		activations: mustID(ids, statActivationsPerHit), frequency: mustID(ids, statHitFrequency),
		repeats: mustID(ids, statRepeatCount), attack: typeMask(c, typeAttack), channel: typeMask(c, typeChannel),
	}
}

// period returns the seconds between the skill's hits at its hit interval before its hit
// frequency, 0 for a skill without one, with stat reading a stat.
func (h hitRate) period(stat func(core.StatID) float64) float64 {
	return stat(h.interval) * max(stat(h.activations), 1)
}

// base returns the skill's uses per second before its speed, and the speed stat that scales
// them, with stat reading a stat.
func (h hitRate) base(isAttack bool, stat func(core.StatID) float64) (float64, core.StatID) {
	if isAttack {
		return stat(h.attackRate), h.attackSpeed
	}
	if t := stat(h.castTime); t > 0 {
		return 1 / t, h.castSpeed
	}
	return 0, h.castSpeed
}

// limit returns the cap on the hits per second of a skill used in context m: one per server
// tick for the use and each repeat, none for a channelled skill.
func (h hitRate) limit(m core.Mask, stat func(core.StatID) float64) float64 {
	if m.Intersects(h.channel) {
		return math.Inf(1)
	}
	return (1 + stat(h.repeats)) / serverTickSeconds
}

func (h hitRate) formula() core.Formula {
	return func(r core.Reader, _ core.Agg) float64 {
		if p := h.period(r.Stat); p > 0 {
			return r.Stat(h.frequency) / percentScale / p
		}
		action := r.Stat(h.action) / percentScale
		limit := h.limit(r.Mask(), r.Stat)
		if r.Mask().Intersects(h.attack) {
			rate := r.Stat(h.attackRate) * roundHundredths(r.Stat(h.attackSpeed)/percentScale) * action
			return min(max(rate, 0), limit)
		}
		t := r.Stat(h.castTime)
		if t <= 0 {
			return 0
		}
		return min(roundHundredths(r.Stat(h.castSpeed)/percentScale)/t*action, limit)
	}
}

// attribute credits the uses per second before speed to the sources of the hit interval, or the
// attack rate or cast time, then the speed's increases and more multipliers over them, then the
// action speed's over those; what rounding and the tick cap change goes to the speed's sources.
func (h hitRate) attribute() core.Attribute {
	return func(a *core.Attributor) core.Attribution {
		total := a.Explanation().Total
		if p := h.period(a.Stat); p > 0 {
			parts := a.Spread(core.SelfRef(h.interval), 1/p, core.OpBase)
			return core.Attribution{Parts: rest(a, parts, total, core.SelfRef(h.frequency)), Amount: total}
		}
		isAttack := a.Mask().Intersects(h.attack)
		base, speed := h.base(isAttack, a.Stat)
		if base <= 0 {
			return core.Attribution{Amount: total}
		}
		from := h.castTime
		if isAttack {
			from = h.attackRate
		}
		parts := a.LayersOver(core.SelfRef(h.action), a.LayersOver(core.SelfRef(speed), a.Spread(core.SelfRef(from), base, core.OpBase)))
		return core.Attribution{Parts: rest(a, parts, total, core.SelfRef(speed)), Amount: total}
	}
}

// hitDPS is a skill's hit damage per second, times the brands attached to the enemy for a
// brand.
type hitDPS struct {
	average, chance, rate, brands core.StatID
	// brand is the Brand type's context bit.
	brand core.Mask
}

// times returns how many of the skill hit the enemy at once in context m, with stat reading a
// stat.
func (h hitDPS) times(m core.Mask, stat func(core.StatID) float64) float64 {
	if m.Intersects(h.brand) {
		return stat(h.brands)
	}
	return 1
}

func (h hitDPS) formula() core.Formula {
	return func(r core.Reader, _ core.Agg) float64 {
		return r.Stat(h.average) * r.Stat(h.chance) / percentScale * r.Stat(h.rate) * h.times(r.Mask(), r.Stat)
	}
}

// attribute credits the average hit's sources with the hits that land in one second's worth of
// one hit per second, the hits per second's sources with what the rate adds, and the brands'
// sources with the rest.
func (h hitDPS) attribute() core.Attribute {
	return func(a *core.Attributor) core.Attribution {
		total := a.Explanation().Total
		avgParts, _ := a.Of(core.SelfRef(h.average))
		parts := scaled(avgParts, a.Stat(h.chance)/percentScale)
		if n := h.times(a.Mask(), a.Stat); n != 1 {
			parts = rest(a, parts, total/n, core.SelfRef(h.rate))
			return core.Attribution{Parts: rest(a, parts, total, core.SelfRef(h.brands)), Amount: total}
		}
		return core.Attribution{Parts: rest(a, parts, total, core.SelfRef(h.rate)), Amount: total}
	}
}
