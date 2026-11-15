package poe1

import (
	"fmt"
	"math"
	"slices"

	"github.com/xaroth/pbt-engine/internal/core"
	"github.com/xaroth/pbt-engine/internal/resolve"
)

// Engine stats every damaging ailment shares. A hit's chance to inflict an ailment is read in the
// hit's context with the ailment; the ailment's damage, duration and multipliers in its context
// for the ailment (ailmentMasks). ailment_chance is the chance in percent that a hit that is not
// a critical strike inflicts it, and cannot_inflict_ailment keeps hits from inflicting it.
// damage_can_inflict_ailment, read for a damage type with the
// ailment, makes a hit's damage of that type count toward an ailment it does not count toward by
// itself. ailment_minimum_damage and ailment_maximum_damage, read in the hit's context for one
// damage type with the ailment, are the hit's damage of that type an ailment starts from: the
// hit's base damage and conversion, taken by the ailment's damage modifiers instead of the hit's;
// the _before_conversion stats are before what converts away. ailment_duration is the
// ailment's duration in percent of its base duration, 100 before modifiers;
// faster_ailment_damage how much faster in percent the ailment deals its damage, shortening it.
// critical_ailment_damage_over_time_multiplier is what an ailment from a critical strike adds to
// its Damage over Time Multiplier, and ailments_never_from_critical_strikes makes a critical
// strike inflict ailments as a hit that is not critical does. ailment_maximum_instances is how
// many instances of an ailment of which only the strongest deal damage do so at once, 1 before
// modifiers.
const (
	statAilmentChance                         = "ailment_chance"
	statCannotInflictAilment                  = "cannot_inflict_ailment"
	statDamageCanInflictAilment               = "damage_can_inflict_ailment"
	statAilmentMinimumDamageBeforeConversion  = "ailment_minimum_damage_before_conversion"
	statAilmentMaximumDamageBeforeConversion  = "ailment_maximum_damage_before_conversion"
	statAilmentMinimumDamage                  = "ailment_minimum_damage"
	statAilmentMaximumDamage                  = "ailment_maximum_damage"
	statAilmentDuration                       = "ailment_duration"
	statFasterAilmentDamage                   = "faster_ailment_damage"
	statCriticalAilmentDamageOverTimeMultiple = "critical_ailment_damage_over_time_multiplier"
	statAilmentsNeverFromCriticalStrikes      = "ailments_never_from_critical_strikes"
	statAilmentMaximumInstances               = "ailment_maximum_instances"
)

// mechanicAilment names the ailment mechanic for those that come after it.
const mechanicAilment = "ailment"

func ailmentMechanic() mechanic {
	return mechanic{
		name: mechanicAilment, after: []string{mechanicConversion, mechanicDot},
		stats: []string{
			statAilmentChance,
			statCannotInflictAilment,
			statDamageCanInflictAilment,
			statAilmentMinimumDamageBeforeConversion,
			statAilmentMaximumDamageBeforeConversion,
			statAilmentMinimumDamage,
			statAilmentMaximumDamage,
			statAilmentDuration,
			statFasterAilmentDamage,
			statCriticalAilmentDamageOverTimeMultiple,
			statAilmentsNeverFromCriticalStrikes,
			statAilmentMaximumInstances,
		},
		def:      ailmentDef,
		complete: func(b *core.ModelBuilder, r registered) { addAilmentSourceFormulas(b, r.ids) },
	}
}

// Formula names shown in explanations.
const (
	formulaAilmentSource                 = "hit damage an ailment starts from"
	formulaAilmentSourceBeforeConversion = "hit damage an ailment starts from, before conversion"
)

// ailmentDef completes def when it is an ailment stat.
func ailmentDef(def core.StatDef, ids map[string]core.StatID) core.StatDef {
	switch def.Name {
	case statCannotInflictAilment, statDamageCanInflictAilment, statAilmentsNeverFromCriticalStrikes:
		def.Flag = true
	case statAilmentChance:
		def.HasMin, def.HasMax, def.Max = true, true, percentScale
	case statAilmentMinimumDamageBeforeConversion, statAilmentMaximumDamageBeforeConversion:
		// The formula reads both stats, so addAilmentSourceFormulas sets it once they are
		// registered.
		def.HasMin = true
	case statAilmentMinimumDamage, statAilmentMaximumDamage:
		before := statAilmentMinimumDamageBeforeConversion
		if def.Name == statAilmentMaximumDamage {
			before = statAilmentMaximumDamageBeforeConversion
		}
		h := newConvertedAwayHit(ids, before)
		h.at = newAilmentMasks().parts()
		def.HasMin = true
		def.FormulaName, def.Formula, def.Attribute = formulaAilmentSource, h.formula(), h.attribute()
	}
	return def
}

// addAilmentSourceFormulas sets the formulas of the ailment's source damage before conversion,
// which read each other: a hit's damage before conversion (convertedHit), its base damage and
// conversion read in the hit's context, its damage modifiers in the ailment's. The modifiers
// apply again to what earlier types convert to it, as for the hit; PoB has it so
// (CalcOffence.lua calcAilmentSourceDamage).
func addAilmentSourceFormulas(b *core.ModelBuilder, ids map[string]core.StatID) {
	for _, name := range []string{statAilmentMinimumDamageBeforeConversion, statAilmentMaximumDamageBeforeConversion} {
		h := newConvertedHit(ids, name == statAilmentMaximumDamageBeforeConversion)
		h.minimum, h.maximum = mustID(ids, statAilmentMinimumDamageBeforeConversion), mustID(ids, statAilmentMaximumDamageBeforeConversion)
		h.at = newAilmentMasks().parts()
		id := mustID(ids, name)
		if err := b.SetFormula(id, h.formula()); err != nil {
			panic(err)
		}
		if err := b.SetAttribute(id, formulaAilmentSourceBeforeConversion, h.attribute()); err != nil {
			panic(err)
		}
	}
}

// ailmentMasks turns a hit's context with an ailment into the ailment's context and back. The
// ailment's context keeps the skill's types but Area and Projectile, whose damage modifiers
// apply to hits only, the hit's weapon and hand, the ailment and the damage types read; its
// form is the ailment's. Attack, Spell and Melee stay: damage with Attack or Spell Skills and
// with ailments from Melee Hits reaches ailments, while modifiers to Attack, Spell or Melee
// Damage do not (the table keeps those off the ailment form). This follows PoB
// (CalcOffence.lua, the ailments' configurations); the game text says only which modifiers
// apply.
type ailmentMasks struct {
	dropped, ailments, form, damage core.Mask
	// bits are each ailment's.
	bits [ailmentPoison + 1]core.Mask
	dim  core.Dimension
}

func newAilmentMasks() ailmentMasks {
	c := modelContexts()
	m := ailmentMasks{form: core.MaskOf(c.form.Bit(int(formAilment) - 1)), dim: c.damage}
	for _, t := range []string{typeArea, typeProjectile} {
		m.dropped = m.dropped.Or(typeMask(c, t))
	}
	for _, f := range []damageForm{formHit, formDot, formAilment} {
		m.dropped = m.dropped.Set(c.form.Bit(int(f) - 1))
	}
	for a := range m.bits {
		m.bits[a] = core.MaskOf(c.ailment.Bit(a))
		m.ailments = m.ailments.Or(m.bits[a])
	}
	for d := Physical; d <= Chaos; d++ {
		m.damage = m.damage.Or(damageBit(c.damage, d))
	}
	return m
}

// hit returns the hit's context of m, a hit's context with an ailment: m without the ailment.
func (m ailmentMasks) hit(mask core.Mask) core.Mask { return mask.AndNot(m.ailments) }

// of returns the ailment's context of m, a hit's context with the ailment.
func (m ailmentMasks) of(mask core.Mask) core.Mask { return mask.AndNot(m.dropped).Or(m.form) }

// parts reads a hit's base damage and conversion in the hit's context and its damage modifiers
// in the ailment's.
func (m ailmentMasks) parts() partMasks { return partMasks{hit: m.hit, scale: m.of} }

// ailmentKind is what a damaging ailment is: its type of damage, which its multipliers and the
// enemy's resistance are read for, its damage per second as a share of the damage it starts
// from, and its base duration, as PoB has them (Data.lua misc).
// onCrit is set for an ailment a critical strike always inflicts; inherent are the types whose
// damage counts toward it besides its own without damage_can_inflict_ailment.
type ailmentKind struct {
	ailment  ailment
	damage   DamageType
	share    float64
	duration float64
	onCrit   bool
	inherent []DamageType
}

// ailmentSkill reports whether the sheet gives sk's damaging ailments with hands: a hit the
// character uses itself, with one hand, since its rate of hits decides how many it applies.
func ailmentSkill(sk *resolve.Skill, h Hands) bool {
	return selfUsedHit(sk, h) && !bothHandsHit(sk, h)
}

// ailmentReads are the stats an ailment's damage reads.
type ailmentReads struct {
	chance, cannot, canInflict, minimum, maximum, duration, faster core.StatID
	multiplier, criticalMultiplier, neverFromCrit                  core.StatID
	critChance, hitChance, rate, resistance, taken                 core.StatID
}

func newAilmentReads(ids map[string]core.StatID) ailmentReads {
	return ailmentReads{
		chance: mustID(ids, statAilmentChance), cannot: mustID(ids, statCannotInflictAilment), canInflict: mustID(ids, statDamageCanInflictAilment),
		minimum: mustID(ids, statAilmentMinimumDamage), maximum: mustID(ids, statAilmentMaximumDamage),
		duration: mustID(ids, statAilmentDuration), faster: mustID(ids, statFasterAilmentDamage),
		multiplier: mustID(ids, statDamageOverTimeMultiplier), criticalMultiplier: mustID(ids, statCriticalAilmentDamageOverTimeMultiple),
		neverFromCrit: mustID(ids, statAilmentsNeverFromCriticalStrikes),
		critChance:    mustID(ids, statEffectiveCriticalStrikeChance), hitChance: mustID(ids, statHitChance),
		rate: mustID(ids, statHitsPerSecond), resistance: mustID(ids, statDamageOverTimeEnemyResistance),
		taken: mustID(ids, statEnemyDamageTaken),
	}
}

// inflicted is what a skill's hits do toward one ailment, read in the hit's context: the hit's
// chance to inflict it without and with a critical strike, the chance that a hit strikes
// critically, the hit damage it starts from, the multipliers it takes without and with a
// critical strike, how much faster it deals its damage and its duration.
type inflicted struct {
	onHit, onCrit, crit               float64
	minimum, maximum                  float64
	multiplier, criticalMultiplier    float64
	faster, duration, hitChance, rate float64
	resistance, taken                 float64
}

// at returns the masks inflicted reads at, from m, a hit's context: the hit's with the ailment,
// and the ailment's for its type of damage.
func (k ailmentKind) at(masks ailmentMasks, m core.Mask) (withAilment, ofType core.Mask) {
	withAilment = masks.hit(m).AndNot(masks.damage).Or(masks.bits[k.ailment])
	return withAilment, masks.of(withAilment).Or(damageBit(masks.dim, k.damage))
}

// sources returns the damage chains of the hit damage the ailment starts from, with flag
// reading a flag at a mask: its own type, and each inherent type or type with
// damage_can_inflict_ailment that comes before it in conversion order, read with the
// ailment's type in its chain so modifiers to either apply once. A type after it cannot be
// read so, as a chain is read as its earliest type; PoB lets chaos damage ignite, which this
// leaves out.
func (k ailmentKind) sources(masks ailmentMasks, withAilment core.Mask, flag func(core.StatID, core.Mask) float64, canInflict core.StatID) []core.Mask {
	own := damageBit(masks.dim, k.damage)
	out := []core.Mask{withAilment.Or(own)}
	for _, d := range conversionOrder() {
		if d == k.damage {
			break
		}
		if bit := damageBit(masks.dim, d); slices.Contains(k.inherent, d) || flag(canInflict, withAilment.Or(bit)) != 0 {
			out = append(out, withAilment.Or(bit).Or(own))
		}
	}
	return out
}

// read returns what a skill's hits do toward ailment k, with stat reading a stat at a mask and
// m the hit's context.
func (k ailmentKind) read(r ailmentReads, masks ailmentMasks, m core.Mask, stat func(core.StatID, core.Mask) float64) inflicted {
	withAilment, ofType := k.at(masks, m)
	in := inflicted{
		onHit:      min(max(stat(r.chance, withAilment), 0), percentScale) / percentScale,
		crit:       stat(r.critChance, m) / percentScale,
		multiplier: 1 + stat(r.multiplier, ofType)/percentScale,
		faster:     1 + stat(r.faster, ofType)/percentScale,
		hitChance:  stat(r.hitChance, m) / percentScale,
		rate:       stat(r.rate, m),
		resistance: stat(r.resistance, ofType),
		taken:      stat(r.taken, ofType) / percentScale,
	}
	in.criticalMultiplier = in.multiplier + stat(r.criticalMultiplier, ofType)/percentScale
	in.onCrit = in.onHit
	if k.onCrit {
		in.onCrit = 1
	}
	if stat(r.neverFromCrit, m) != 0 {
		in.onCrit, in.criticalMultiplier = in.onHit, in.multiplier
	}
	if stat(r.cannot, withAilment) != 0 {
		in.onHit, in.onCrit = 0, 0
	}
	if in.faster > 0 {
		in.duration = k.duration * max(stat(r.duration, ofType), 0) / percentScale / in.faster
	}
	for _, src := range k.sources(masks, withAilment, stat, r.canInflict) {
		in.minimum += stat(r.minimum, src)
		in.maximum += stat(r.maximum, src)
	}
	return in
}

// perHit is the chance that a hit inflicts the ailment.
func (in inflicted) perHit() float64 { return in.onHit*(1-in.crit) + in.onCrit*in.crit }

// applied is how many of the ailment a skill applies over its duration: per hit that lands, its
// chance to inflict it, times the hits over the duration.
func (in inflicted) applied() float64 { return in.hitChance * in.perHit() * in.duration * in.rate }

// weighted is the damage per second, before the enemy, of an instance from source damage
// source with crit the chance that it came from a critical strike: the instance from a hit and
// from a critical strike weighed by how often each inflicts it, with the multiplier of each. It
// is 0 when neither inflicts it.
func (in inflicted) weighted(k ailmentKind, source, crit float64) float64 {
	fromHit, fromCrit := in.onHit*(1-crit), in.onCrit*crit
	if fromHit+fromCrit <= 0 {
		return 0
	}
	return source * (in.multiplier*fromHit + in.criticalMultiplier*fromCrit) / (fromHit + fromCrit) * k.share * in.faster
}

// enemy is what the enemy leaves of the ailment's damage: its resistance, which no penetration
// lowers, and the damage it takes.
func (in inflicted) enemy() float64 { return resistanceMultiplier(in.resistance) * in.taken }

// enemySteps credits, from the damage per second v of one instance at its source damage, what
// the Damage over Time Multiplier (to multiplied), critical strikes (to weighted), the enemy's
// resistance and the damage it takes change, each to its sources.
func (r ailmentReads) enemySteps(a *core.Attributor, ofType core.Mask, in inflicted, v, multiplied, weighted float64) []core.Part {
	steps := []struct {
		ref  core.Ref
		next float64
	}{
		{a.RefAt(r.multiplier, ofType), multiplied},
		{a.RefAt(r.criticalMultiplier, ofType), weighted},
		{a.RefAt(r.resistance, ofType), weighted * resistanceMultiplier(in.resistance)},
		{a.RefAt(r.taken, ofType), weighted * in.enemy()},
	}
	var parts []core.Part
	for _, s := range steps {
		if delta := s.next - v; delta != 0 {
			parts = append(parts, a.Spread(s.ref, delta, core.OpMore)...)
		}
		v = s.next
	}
	return parts
}

// critOfStrongest is the chance that the strongest of n instances came from a critical strike,
// each with chance crit: PoB's expected value for an ailment of which only the strongest
// applies (CalcOffence.lua).
func critOfStrongest(crit, n float64) float64 { return 1 - math.Pow(1-crit, max(1, n)) }

// baseAilmentDuration is an ailment's duration before modifiers: all of its base duration.
const baseAilmentDuration = 100

// addBaseAilmentValues adds the base ailment duration and the one instance of an ailment that
// deals damage at once as rules' sources, so modifiers apply to them by the default rule.
func (sh *Sheet) addBaseAilmentValues() error {
	for _, b := range []struct {
		stat, id, rule string
		value          float64
	}{
		{statAilmentDuration, "base-ailment-duration", "base ailment duration (100%)", baseAilmentDuration},
		{statAilmentMaximumInstances, "base-ailment-instances", "one instance of an ailment deals damage at once", 1},
	} {
		src := sh.newSource(Source{RuleID: b.id, Rule: b.rule})
		mod := core.Modifier{Stat: sh.stats[b.stat], Op: core.OpBase, Value: b.value, Source: src}
		if _, err := sh.session.AddModifier(mod); err != nil {
			return fmt.Errorf("sheet: %s: %w", b.rule, err)
		}
	}
	return nil
}
