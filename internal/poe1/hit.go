package poe1

import (
	"fmt"
	"math"
	"slices"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/internal/core"
	"github.com/xaroth/pbt-engine/internal/resolve"
	"github.com/xaroth/pbt-engine/internal/semantics"
)

// Skill types that decide which of a skill's hit outputs the sheet gives.
const (
	typeSpell        = "Spell"
	typeAttack       = "Attack"
	typeDamage       = "Damage"
	typeMinion       = "Minion"
	typeChannel      = "Channel"
	typeBrand        = "Brand"
	typeTrapped      = "Trapped"
	typeRemoteMined  = "RemoteMined"
	typeSummonsTotem = "SummonsTotem"
	typeTriggered    = "Triggered"
	typeCooldown     = "Cooldown"
)

// Hit limits. The rate cap is the server tick: a skill used more often than once a tick still
// hits once a tick. The armour formula and the enemy's cap on physical damage reduction follow
// PoB (CalcDefence.lua armourReductionF, Data.lua ServerTickRate, Misc.lua monster
// maximum_physical_damage_reduction_%); the game data read here gives none of them.
const (
	serverTickSeconds         = 0.033
	maximumPhysicalReduction  = 75
	armourDamageFactor        = 5
	millisecondsPerSecond     = 1000
	critPermyriadToPercent    = 100
	effectivenessPermyriadPct = 100
	// half averages a minimum and a maximum, splits what critical strikes add between their
	// chance and multiplier, and rounds half up.
	half = 0.5
)

// SpellHit reports whether the sheet gives sk's hit as a spell's: a spell that deals damage, not
// an attack and not a minion's.
func SpellHit(sk *resolve.Skill) bool {
	return hasTypes(sk, typeSpell, typeDamage) && !hasAnyType(sk, typeAttack, typeMinion)
}

// attackHit reports whether the sheet gives sk's hit as an attack's with hands: an attack the
// character makes with one weapon in the main hand and none in the off hand, not a minion's
// and not made by a totem, trap or mine.
func attackHit(sk *resolve.Skill, h Hands) bool {
	return hasTypes(sk, typeAttack) && h.MainHand != nil && h.OffHand == nil &&
		!hasAnyType(sk, typeMinion, typeTrapped, typeRemoteMined, typeSummonsTotem)
}

// Hit reports whether the sheet gives sk's hit with hands: a spell hit, or an attack with one
// weapon.
func Hit(sk *resolve.Skill, h Hands) bool {
	return sk != nil && (SpellHit(sk) || attackHit(sk, h))
}

// selfUsedHit reports whether the sheet gives sk's rate of hits with hands: a hit the character
// uses itself, without a cooldown, channelling, brand, trap, mine, totem or trigger, whose
// rates these rules do not give.
func selfUsedHit(sk *resolve.Skill, h Hands) bool {
	return Hit(sk, h) &&
		!hasAnyType(sk, typeChannel, typeBrand, typeTrapped, typeRemoteMined, typeSummonsTotem, typeTriggered, typeCooldown)
}

// Hands is what the character holds: the weapon in each hand, nil for none. A shield or quiver
// is not a weapon.
type Hands struct {
	MainHand, OffHand *Weapon
}

// HandsOf returns what items put in the character's hands.
func HandsOf(items []Item) Hands {
	var h Hands
	for _, it := range items {
		switch {
		case it.Node != 0:
		case it.Slot == build.SlotWeapon:
			h.MainHand = it.Weapon
		case it.Slot == build.SlotOffhand:
			h.OffHand = it.Weapon
		}
	}
	return h
}

func hasTypes(sk *resolve.Skill, types ...string) bool {
	for _, t := range types {
		if !slices.Contains(sk.Types, t) {
			return false
		}
	}
	return true
}

func hasAnyType(sk *resolve.Skill, types ...string) bool {
	return slices.ContainsFunc(types, func(t string) bool { return slices.Contains(sk.Types, t) })
}

// damageDimension is the damage dimension of a sheet model: addContexts registers it the same
// way on every builder, so its bits are the same in every model.
func damageDimension() core.Dimension {
	var b core.ModelBuilder
	return addContexts(&b).damage
}

// damageBit returns the context bit of damage type d.
func damageBit(dim core.Dimension, d DamageType) core.Mask {
	return core.MaskOf(dim.Bit(int(d) - 1))
}

// attackMask is the Attack type's context bit of a sheet model, the same in every model as
// damageDimension's.
func attackMask() core.Mask {
	var b core.ModelBuilder
	types := addContexts(&b).types
	return core.MaskOf(types.Bit(slices.Index(types.Values, typeAttack)))
}

// hitDef completes def when it is a hit stat.
//
// A hit's damage of one type, read in the skill's context for that type, is the skill's base
// damage of the type times its base damage multiplier, plus added damage times its damage
// effectiveness, times the type's damage modifier and the modifier of the smallest or largest
// damage alone, rounded half up. Against the enemy it is
// reduced by the enemy's resistance to the type, or for physical damage by its armour against
// a hit of that size. A critical strike multiplies the hit before the enemy reduces it. The
// average hit weighs the non-critical and critical averages of every type by the critical
// strike chance. Hits per second are the cast time divided by the cast speed, rounded to
// hundredths, at most one per server tick; hit DPS is the average hit times the chance to hit
// times hits per second. The order, roundings and cap follow PoB (CalcOffence.lua); the game
// text gives none of them.
func hitDef(def core.StatDef, ids map[string]core.StatID) core.StatDef {
	switch def.Name {
	case statMinimumHitDamage, statMaximumHitDamage:
		h := hitDamage{
			base: mustID(ids, statBaseMinimumDamage), added: mustID(ids, statAddedMinimumDamage),
			multiplier: mustID(ids, statBaseDamageMultiplier), effectiveness: mustID(ids, statDamageEffectiveness),
			damage: mustID(ids, statDamage), extreme: mustID(ids, statMinimumDamage),
		}
		if def.Name == statMaximumHitDamage {
			h.base, h.added = mustID(ids, statBaseMaximumDamage), mustID(ids, statAddedMaximumDamage)
			h.extreme = mustID(ids, statMaximumDamage)
		}
		def.HasMin = true
		def.FormulaName, def.Formula, def.Attribute = formulaHitDamage, h.formula(), h.attribute()
	case statEffectiveMinimumHitDamage, statEffectiveMaximumHitDamage:
		h := effectiveHit{
			hit: mustID(ids, statMinimumHitDamage), minimum: mustID(ids, statMinimumHitDamage), maximum: mustID(ids, statMaximumHitDamage),
			enemy: newEnemyReduction(ids),
		}
		if def.Name == statEffectiveMaximumHitDamage {
			h.hit = mustID(ids, statMaximumHitDamage)
		}
		def.FormulaName, def.Formula, def.Attribute = formulaHitAgainstEnemy, h.formula(), h.attribute()
	case statCriticalStrikeChance, statHitChance:
		def.HasMin, def.HasMax, def.Max = true, true, percentScale
	case statCriticalStrikeMultiplier:
		def.HasMin, def.Min = true, percentScale
	case statAverageHit:
		h := newAverageHit(ids)
		def.FormulaName, def.Formula, def.Attribute = formulaAverageHit, h.formula(), h.attribute()
	case statHitsPerSecond:
		h := hitRate{
			castTime: mustID(ids, statCastTime), castSpeed: mustID(ids, statCastSpeed),
			attackRate: mustID(ids, statAttackRate), attackSpeed: mustID(ids, statAttackSpeed), attack: attackMask(),
		}
		def.FormulaName, def.Formula, def.Attribute = formulaHitsPerSecond, h.formula(), h.attribute()
	case statHitDPS:
		h := hitDPS{average: mustID(ids, statAverageHit), chance: mustID(ids, statHitChance), rate: mustID(ids, statHitsPerSecond)}
		def.FormulaName, def.Formula, def.Attribute = formulaHitDPS, h.formula(), h.attribute()
	}
	return def
}

// hitDamage is a hit's smallest or largest damage of one type before the enemy.
type hitDamage struct {
	base, added, multiplier, effectiveness, damage core.StatID
	// extreme is the modifier of the smallest or the largest damage alone.
	extreme core.StatID
}

func (h hitDamage) formula() core.Formula {
	return func(r core.Reader, _ core.Agg) float64 {
		v := r.Stat(h.base)*r.Stat(h.multiplier)/percentScale + r.Stat(h.added)*r.Stat(h.effectiveness)/percentScale
		return roundHalf(v * r.Stat(h.damage) / percentScale * r.Stat(h.extreme) / percentScale)
	}
}

// attribute credits the base and added damage, then the damage modifier's and the smallest or
// largest damage's increases and more multipliers over them; what the base damage multiplier, damage effectiveness and rounding
// change goes to the damage effectiveness's sources.
func (h hitDamage) attribute() core.Attribute {
	return func(a *core.Attributor) core.Attribution {
		var parts []core.Part
		for _, s := range []core.StatID{h.base, h.added} {
			if v := a.Stat(s); v != 0 {
				parts = append(parts, a.Spread(core.SelfRef(s), v, core.OpBase)...)
			}
		}
		parts = a.LayersOver(core.SelfRef(h.extreme), a.LayersOver(core.SelfRef(h.damage), parts))
		total := a.Explanation().Total
		return core.Attribution{Parts: rest(a, parts, total, core.SelfRef(h.effectiveness)), Amount: total}
	}
}

// effectiveHit is what the enemy takes of a hit of one type that is not critical, read in the
// type's context: physical damage is reduced by armour against the average of the minimum and
// maximum hit, other damage by the resistance.
type effectiveHit struct {
	hit, minimum, maximum core.StatID
	enemy                 enemyReduction
}

func (h effectiveHit) formula() core.Formula {
	return func(r core.Reader, _ core.Agg) float64 {
		if r.Mask().Intersects(h.enemy.physical) {
			return r.Stat(h.hit) * armourMultiplier(r.Stat(h.enemy.armour), (r.Stat(h.minimum)+r.Stat(h.maximum))*half)
		}
		return r.Stat(h.hit) * resistanceMultiplier(r.Stat(h.enemy.resistance))
	}
}

// attribute credits the hit, and what the enemy takes off to the enemy's armour or resistance.
func (h effectiveHit) attribute() core.Attribute {
	return func(a *core.Attributor) core.Attribution {
		parts, _ := a.Of(core.SelfRef(h.hit))
		reducer := h.enemy.resistance
		if a.Mask().Intersects(h.enemy.physical) {
			reducer = h.enemy.armour
		}
		total := a.Explanation().Total
		return core.Attribution{Parts: rest(a, parts, total, core.SelfRef(reducer)), Amount: total}
	}
}

// enemyReduction is the enemy's stats that reduce hits.
type enemyReduction struct {
	resistance, armour core.StatID
	// physical is the physical damage bit, which armour reduces instead of a resistance.
	physical core.Mask
}

func newEnemyReduction(ids map[string]core.StatID) enemyReduction {
	return enemyReduction{
		resistance: mustID(ids, statEnemyResistance), armour: mustID(ids, statEnemyArmour),
		physical: damageBit(damageDimension(), Physical),
	}
}

// armourMultiplier is what armour leaves of a physical hit of size raw.
func armourMultiplier(armour, raw float64) float64 {
	armour = max(armour, 0)
	if armour == 0 && raw == 0 {
		return 1
	}
	return 1 - min(armour/(armour+raw*armourDamageFactor)*percentScale, maximumPhysicalReduction)/percentScale
}

func resistanceMultiplier(res float64) float64 {
	return 1 - max(res, resistanceFloor)/percentScale
}

// averageHit is the average hit of a skill, read in its context: per damage type, the average of
// its minimum and maximum hit without and with a critical strike, each reduced by the enemy,
// weighed by the critical strike chance.
type averageHit struct {
	minimum, maximum, chance, multiplier core.StatID
	enemy                                enemyReduction
	dim                                  core.Dimension
}

func newAverageHit(ids map[string]core.StatID) averageHit {
	return averageHit{
		minimum: mustID(ids, statMinimumHitDamage), maximum: mustID(ids, statMaximumHitDamage),
		chance: mustID(ids, statCriticalStrikeChance), multiplier: mustID(ids, statCriticalStrikeMultiplier),
		enemy: newEnemyReduction(ids), dim: damageDimension(),
	}
}

// typeHit is one damage type's part of the average hit: the average of its minimum and
// maximum, what the enemy takes of it without a critical strike, and its part of the average
// hit.
type typeHit struct {
	raw, normal, total float64
}

// of returns damage type d's part, with read reading a stat in d's context.
func (h averageHit) of(d DamageType, crit, critMultiplier float64, read func(core.StatID) float64) typeHit {
	raw := (read(h.minimum) + read(h.maximum)) * half
	if raw == 0 {
		return typeHit{}
	}
	reduce := func(v float64) float64 {
		if d == Physical {
			return v * armourMultiplier(read(h.enemy.armour), v)
		}
		return v * resistanceMultiplier(read(h.enemy.resistance))
	}
	normal := reduce(raw)
	return typeHit{raw: raw, normal: normal, total: normal*(1-crit) + reduce(raw*critMultiplier)*crit}
}

func (h averageHit) formula() core.Formula {
	return func(r core.Reader, _ core.Agg) float64 {
		crit, critMultiplier := r.Stat(h.chance)/percentScale, r.Stat(h.multiplier)/percentScale
		total := 0.0
		for d := Physical; d <= Chaos; d++ {
			bit := damageBit(h.dim, d)
			total += h.of(d, crit, critMultiplier, func(s core.StatID) float64 { return r.StatWith(s, bit) }).total
		}
		return total
	}
}

// attribute credits, per damage type, the hit's sources with the average of its minimum and
// maximum, the enemy's resistance or armour with what it takes off, and the critical strike
// chance and multiplier with what critical strikes add, split equally between them.
func (h averageHit) attribute() core.Attribute {
	return func(a *core.Attributor) core.Attribution {
		crit, critMultiplier := a.Stat(h.chance)/percentScale, a.Stat(h.multiplier)/percentScale
		var parts []core.Part
		for d := Physical; d <= Chaos; d++ {
			bit := damageBit(h.dim, d)
			t := h.of(d, crit, critMultiplier, func(s core.StatID) float64 { return a.Read(a.RefWith(s, bit)) })
			if t.raw == 0 {
				continue
			}
			minParts, _ := a.Of(a.RefWith(h.minimum, bit))
			maxParts, _ := a.Of(a.RefWith(h.maximum, bit))
			parts = append(parts, scaled(append(minParts, maxParts...), half)...)
			if t.normal != t.raw {
				reducer := core.SelfRef(h.enemy.armour)
				if d != Physical {
					reducer = a.RefWith(h.enemy.resistance, bit)
				}
				parts = append(parts, a.Spread(reducer, t.normal-t.raw, core.OpMore)...)
			}
			if c := t.total - t.normal; c != 0 {
				parts = append(parts, a.Spread(core.SelfRef(h.chance), c*half, core.OpMore)...)
				parts = append(parts, a.Spread(core.SelfRef(h.multiplier), c*half, core.OpMore)...)
			}
		}
		return core.Attribution{Parts: parts, Amount: a.Explanation().Total}
	}
}

// speedDef completes def when it is attack speed, whose more multipliers round
// (roundedMoreFormula).
func speedDef(def core.StatDef) core.StatDef {
	if def.Name == statAttackSpeed {
		def.FormulaName, def.Formula, def.Attribute = formulaRoundedMore, roundedMoreFormula, roundedMoreAttribute
	}
	return def
}

// roundedMoreFormula is the default aggregation with the product of the more multipliers
// rounded half up to hundredths, as PoB rounds the more multipliers of one stat name
// (ModList.lua MoreInternal) before the increases apply.
func roundedMoreFormula(_ core.Reader, a core.Agg) float64 {
	return a.Base * (1 + a.Inc/percentScale) * roundHundredths(a.More)
}

// roundedMoreAttribute splits the value by the default layers, scaled so the rounding is shared
// out in proportion.
func roundedMoreAttribute(a *core.Attributor) core.Attribution {
	total := a.Explanation().Total
	parts := a.Layers(a.BaseTerms())
	sum := 0.0
	for _, p := range parts {
		sum += p.Value
	}
	if sum != 0 {
		parts = scaled(parts, total/sum)
	}
	return core.Attribution{Parts: parts, Amount: total}
}

// hitRate is a skill's hits per second: an attack's from its attack rate and speed, another
// skill's from its cast time and speed.
type hitRate struct {
	castTime, castSpeed, attackRate, attackSpeed core.StatID
	// attack is the Attack type's context bit.
	attack core.Mask
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

func (h hitRate) formula() core.Formula {
	return func(r core.Reader, _ core.Agg) float64 {
		if r.Mask().Intersects(h.attack) {
			rate := r.Stat(h.attackRate) * roundHundredths(r.Stat(h.attackSpeed)/percentScale)
			return min(max(rate, 0), 1/serverTickSeconds)
		}
		t := r.Stat(h.castTime)
		if t <= 0 {
			return 0
		}
		return min(roundHundredths(r.Stat(h.castSpeed)/percentScale)/t, 1/serverTickSeconds)
	}
}

// attribute credits the uses per second before speed to the sources of the attack rate or cast
// time, then the speed's increases and more multipliers over them; what rounding and the tick
// cap change goes to the speed's sources.
func (h hitRate) attribute() core.Attribute {
	return func(a *core.Attributor) core.Attribution {
		total := a.Explanation().Total
		isAttack := a.Mask().Intersects(h.attack)
		base, speed := h.base(isAttack, a.Stat)
		if base <= 0 {
			return core.Attribution{Amount: total}
		}
		from := h.castTime
		if isAttack {
			from = h.attackRate
		}
		parts := a.LayersOver(core.SelfRef(speed), a.Spread(core.SelfRef(from), base, core.OpBase))
		return core.Attribution{Parts: rest(a, parts, total, core.SelfRef(speed)), Amount: total}
	}
}

// hitDPS is a skill's hit damage per second.
type hitDPS struct {
	average, chance, rate core.StatID
}

func (h hitDPS) formula() core.Formula {
	return func(r core.Reader, _ core.Agg) float64 {
		return r.Stat(h.average) * r.Stat(h.chance) / percentScale * r.Stat(h.rate)
	}
}

// attribute credits the average hit's sources with the hits that land in one second's worth of
// one hit per second, and the hits per second's sources with the rest.
func (h hitDPS) attribute() core.Attribute {
	return func(a *core.Attributor) core.Attribution {
		total := a.Explanation().Total
		avgParts, _ := a.Of(core.SelfRef(h.average))
		parts := scaled(avgParts, a.Stat(h.chance)/percentScale)
		return core.Attribution{Parts: rest(a, parts, total, core.SelfRef(h.rate)), Amount: total}
	}
}

// scaled returns parts with their values times f.
func scaled(parts []core.Part, f float64) []core.Part {
	out := make([]core.Part, len(parts))
	for i, p := range parts {
		p.Value *= f
		out[i] = p
	}
	return out
}

// rest returns parts with what they leave of total credited to the sources of ref as a more
// multiplier.
func rest(a *core.Attributor, parts []core.Part, total float64, ref core.Ref) []core.Part {
	sum := 0.0
	for _, p := range parts {
		sum += p.Value
	}
	if r := total - sum; r != 0 {
		parts = append(parts, a.Spread(ref, r, core.OpMore)...)
	}
	return parts
}

// roundHalf rounds half up, as PoB's round does.
func roundHalf(v float64) float64 { return math.Floor(v + half) }

// roundHundredths rounds half up to hundredths, as PoB rounds a speed modifier, a weapon's
// attacks per second and its critical strike chance.
func roundHundredths(v float64) float64 {
	const scale = 100
	return math.Floor(v*scale+half) / scale
}

// hitBase is a value of a skill's hit stats from its granted effect and primary stat set.
type hitBase struct {
	stat  string
	op    core.Op
	value float64
}

// hitBases are sk's cast time in seconds, the damage effectiveness, base damage multiplier and
// critical strike chance of its primary stat set in percent, for a spell its chance to hit
// (spells always hit), and for an attack its level row's attack speed multiplier, a more
// multiplier to the weapon's attacks per second as PoB applies it (CalcOffence.lua
// attackSpeedMultiplier).
func hitBases(sk *resolve.Skill) []hitBase {
	out := []hitBase{{statCastTime, core.OpBase, float64(sk.CastTime) / millisecondsPerSecond}}
	if len(sk.StatSets) > 0 {
		set := sk.StatSets[0]
		out = append(out,
			hitBase{statDamageEffectiveness, core.OpBase, percentScale + float64(set.DamageEffectiveness)/effectivenessPermyriadPct},
			hitBase{statBaseDamageMultiplier, core.OpBase, percentScale + float64(set.BaseMultiplier)/effectivenessPermyriadPct},
			hitBase{statCriticalStrikeChance, core.OpBase, float64(set.SpellCritChance) / critPermyriadToPercent},
		)
	}
	switch {
	case hasTypes(sk, typeAttack):
		out = append(out, hitBase{statAttackRate, core.OpMore, float64(sk.AttackSpeedMultiplier)})
	case hasTypes(sk, typeSpell):
		out = append(out, hitBase{statHitChance, core.OpBase, percentScale})
	}
	return out
}

// addHitRules adds sk's hit bases in slot.
func (sh *Sheet) addHitRules(sk *resolve.Skill, slot int) (core.SourceID, error) {
	src := sh.newSource(Source{RuleID: "skill-hit", Rule: "hit of " + sk.Source.String()})
	pred, err := predicate(sh.session.Model(), semantics.ContextFilter{}, sh.ctx, slot)
	if err != nil {
		return 0, err
	}
	for _, b := range hitBases(sk) {
		if b.value == 0 {
			continue
		}
		mod := core.Modifier{Stat: sh.stats[b.stat], Op: b.op, Value: b.value, Source: src, Pred: pred}
		if _, err := sh.session.AddModifier(mod); err != nil {
			return 0, fmt.Errorf("sheet: skill hit: %w", err)
		}
	}
	return src, nil
}

// baseSpeed is cast and attack speed before modifiers: 100%.
const baseSpeed = 100

// addBaseSpeeds adds the base cast and attack speed as a rule's source each, so increases and
// more multipliers apply to them by the default rule.
func (sh *Sheet) addBaseSpeeds() error {
	for _, b := range []struct{ stat, id, rule string }{
		{statCastSpeed, "base-cast-speed", "base cast speed (100%)"},
		{statAttackSpeed, "base-attack-speed", "base attack speed (100%)"},
	} {
		src := sh.newSource(Source{RuleID: b.id, Rule: b.rule})
		mod := core.Modifier{Stat: sh.stats[b.stat], Op: core.OpBase, Value: baseSpeed, Source: src}
		if _, err := sh.session.AddModifier(mod); err != nil {
			return fmt.Errorf("sheet: %s: %w", b.rule, err)
		}
	}
	return nil
}
