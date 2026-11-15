package poe1

import (
	"slices"

	"github.com/xaroth/pbt-engine/internal/core"
	"github.com/xaroth/pbt-engine/internal/resolve"
)

// Engine stats of a skill's damage over time, read in its context for damage over time, which
// has the skill's types but Attack, and but Area, Spell and Projectile unless
// area_damage_modifiers_apply_to_skill_dot, spell_damage_modifiers_apply_to_skill_dot or
// projectile_damage_modifiers_apply_to_skill_dot is set for it. base_damage_over_time, read for one damage type, is the skill's own damage of that
// type per second before modifiers; damage_over_time_multiplier is the Damage over Time
// Multiplier in percent. damage_over_time_enemy_resistance is the resistance damage over time of
// the type meets, and damage_over_time what the enemy takes of the type per second.
// damage_over_time_dps totals it over the types.
const (
	statAreaDamageModifiersApplyToSkillDot       = "area_damage_modifiers_apply_to_skill_dot"
	statSpellDamageModifiersApplyToSkillDot      = "spell_damage_modifiers_apply_to_skill_dot"
	statProjectileDamageModifiersApplyToSkillDot = "projectile_damage_modifiers_apply_to_skill_dot"
	statBaseDamageOverTime                       = "base_damage_over_time"
	statDamageOverTimeMultiplier                 = "damage_over_time_multiplier"
	statDamageOverTimeEnemyResistance            = "damage_over_time_enemy_resistance"
	statDamageOverTime                           = "damage_over_time"
	statDamageOverTimeDPS                        = "damage_over_time_dps"
)

const typeDamageOverTime = "DamageOverTime"

// dotDPSCap is the most damage over time per second PoB gives: the largest 32-bit integer over
// 60 seconds (Data.lua DotDpsCap).
const dotDPSCap = 35791394

// dotSkill reports whether the sheet gives sk's damage over time: a skill of the DamageOverTime
// type.
func dotSkill(sk *resolve.Skill) bool {
	return hasTypes(sk, typeDamageOverTime)
}

// dotBases are sk's own values of its damage over time: for a skill whose damage over time is
// area damage, a ground or aura effect such as Righteous Fire's burn and not a debuff an area
// skill applies, that modifiers to Area Damage reach it.
func dotBases(sk *resolve.Skill) []hitBase {
	if slices.Contains(areaDotSkills(), sk.GrantedEffect) {
		return []hitBase{{statAreaDamageModifiersApplyToSkillDot, core.OpFlag, 1}}
	}
	return nil
}

// dotMasks are the context bits a skill's context loses or gains for its damage over time.
type dotMasks struct {
	attack, forms, dot core.Mask
	// kept are the types the context keeps only while their flag is set.
	kept []keptType
}

// keptType is a skill type and the flag that keeps it in the context of damage over time.
type keptType struct {
	bit  core.Mask
	flag core.StatID
}

func newDotMasks(ids map[string]core.StatID) dotMasks {
	c := modelContexts()
	d := dotMasks{attack: typeMask(c, typeAttack), dot: core.MaskOf(c.form.Bit(int(formDot) - 1))}
	for _, f := range []damageForm{formHit, formDot, formAilment} {
		d.forms = d.forms.Set(c.form.Bit(int(f) - 1))
	}
	for t, flag := range map[string]string{
		typeArea:       statAreaDamageModifiersApplyToSkillDot,
		typeSpell:      statSpellDamageModifiersApplyToSkillDot,
		typeProjectile: statProjectileDamageModifiersApplyToSkillDot,
	} {
		d.kept = append(d.kept, keptType{bit: typeMask(c, t), flag: mustID(ids, flag)})
	}
	return d
}

// of returns the context of damage over time of a skill whose context is m, with flag reading a
// flag in the skill's context.
func (d dotMasks) of(m core.Mask, flag func(core.StatID) float64) core.Mask {
	out := m.AndNot(d.attack).AndNot(d.forms).Or(d.dot)
	for _, k := range d.kept {
		if flag(k.flag) == 0 {
			out = out.AndNot(k.bit)
		}
	}
	return out
}

// areaDotSkills are the granted effects whose damage over time is area damage. The game data
// does not mark them; these are edition values. Source: Path of Building f9f4f3b and b32759a,
// the same in both, Data/Skills (skill("dotIsArea", true)), without FrostBladesAltX, which
// sets it for one skill part only.
func areaDotSkills() []string {
	return []string{
		"AzmeriZombieCausticGroundWhenHit", "CausticArrow", "ColdSnap", "CreepingFrost", "CreepingFrostAltX", "DeathAura",
		"Desecrate", "FireTrap", "FlameDash", "FlameDashAltY", "FlameWall", "GuardianSentinelFireAura", "InfernalLegion",
		"ReplicaSiegebreakerBurningGround", "RighteousFire", "RighteousFireAltX", "SandstormChaosElementalSummoned",
		"SandstormChaosElementalSummonedEmpowered", "SiegebreakerCausticGround", "ToxicRain", "ToxicRainAltY",
		"VaalCausticArrow", "VaalColdSnap", "VaalFirestorm", "VaalReap", "VaalRighteousFire", "VolatileAnomaly",
		"Vortex", "VortexAltX",
	}
}

// Formula names shown in explanations.
const (
	formulaDotEnemyResistance = "enemy resistance damage over time meets"
	formulaDamageOverTime     = "damage over time of a damage type"
	formulaDamageOverTimeDPS  = "damage over time per second"
)

// mechanicDot names the damage over time mechanic for those that come after it.
const mechanicDot = "damage over time"

func dotMechanic() mechanic {
	return mechanic{name: mechanicDot, after: []string{mechanicDamage, mechanicEnemy}, stats: []string{
		statAreaDamageModifiersApplyToSkillDot,
		statSpellDamageModifiersApplyToSkillDot,
		statProjectileDamageModifiersApplyToSkillDot,
		statBaseDamageOverTime,
		statDamageOverTimeMultiplier,
		statDamageOverTimeEnemyResistance,
		statDamageOverTime,
		statDamageOverTimeDPS,
	}, def: dotDef}
}

// dotDef completes def when it is a damage over time stat.
//
// A skill's damage over time of one type, read in its context for damage over time of that
// type, is its base damage over time times the type's damage modifier and the Damage over Time
// Multiplier, reduced by the resistance it meets (physical damage meets none) and scaled by the
// enemy's damage taken. Its damage over time per second totals the types, at most dotDPSCap.
// The order follows PoB (CalcOffence.lua, the skill's damage over time); the game text gives
// none of it.
func dotDef(def core.StatDef, ids map[string]core.StatID) core.StatDef {
	switch def.Name {
	case statAreaDamageModifiersApplyToSkillDot, statSpellDamageModifiersApplyToSkillDot, statProjectileDamageModifiersApplyToSkillDot:
		def.Flag = true
	case statDamageOverTimeEnemyResistance:
		h := newMetResistance(ids, false)
		def.FormulaName, def.Formula, def.Attribute = formulaDotEnemyResistance, h.formula(), h.attribute()
	case statDamageOverTime:
		d := damageOverTime{
			base: mustID(ids, statBaseDamageOverTime), damage: mustID(ids, statDamage),
			multiplier: mustID(ids, statDamageOverTimeMultiplier), resistance: mustID(ids, statDamageOverTimeEnemyResistance),
			taken: mustID(ids, statEnemyDamageTaken),
		}
		def.FormulaName, def.Formula, def.Attribute = formulaDamageOverTime, d.formula(), d.attribute()
	case statDamageOverTimeDPS:
		d := dotDPS{dot: mustID(ids, statDamageOverTime), dim: damageDimension()}
		def.FormulaName, def.Formula, def.Attribute = formulaDamageOverTimeDPS, d.formula(), d.attribute()
	}
	return def
}

// damageOverTime is what the enemy takes of a skill's damage over time of one type per second.
type damageOverTime struct {
	base, damage, multiplier, resistance, taken core.StatID
}

func (d damageOverTime) formula() core.Formula {
	return func(r core.Reader, _ core.Agg) float64 {
		v := r.Stat(d.base) * r.Stat(d.damage) / percentScale * (1 + r.Stat(d.multiplier)/percentScale)
		return v * resistanceMultiplier(r.Stat(d.resistance)) * r.Stat(d.taken) / percentScale
	}
}

// attribute credits the base, then the damage modifier's increases and more multipliers over
// it, what the Damage over Time Multiplier and the resistance add or take off to their
// sources, and what damage taken changes to its sources.
func (d damageOverTime) attribute() core.Attribute {
	return func(a *core.Attributor) core.Attribution {
		total := a.Explanation().Total
		base := a.Stat(d.base)
		if base == 0 {
			return core.Attribution{Amount: total}
		}
		parts := a.LayersOver(core.SelfRef(d.damage), a.Spread(core.SelfRef(d.base), base, core.OpBase))
		v := base * a.Stat(d.damage) / percentScale
		if m := v * a.Stat(d.multiplier) / percentScale; m != 0 {
			parts = append(parts, a.Spread(core.SelfRef(d.multiplier), m, core.OpMore)...)
			v += m
		}
		if r := v*resistanceMultiplier(a.Stat(d.resistance)) - v; r != 0 {
			parts = append(parts, a.Spread(core.SelfRef(d.resistance), r, core.OpMore)...)
		}
		return core.Attribution{Parts: rest(a, parts, total, core.SelfRef(d.taken)), Amount: total}
	}
}

// dotDPS is a skill's damage over time per second of every type.
type dotDPS struct {
	dot core.StatID
	dim core.Dimension
}

func (d dotDPS) formula() core.Formula {
	return func(r core.Reader, _ core.Agg) float64 {
		total := 0.0
		for t := Physical; t <= Chaos; t++ {
			total += r.StatWith(d.dot, damageBit(d.dim, t))
		}
		return min(total, dotDPSCap)
	}
}

// attribute credits each type's damage over time to its sources, scaled down when the cap
// applies.
func (d dotDPS) attribute() core.Attribute {
	return func(a *core.Attributor) core.Attribution {
		var parts []core.Part
		sum := 0.0
		for t := Physical; t <= Chaos; t++ {
			p, v := a.Of(a.RefWith(d.dot, damageBit(d.dim, t)))
			parts, sum = append(parts, p...), sum+v
		}
		total := a.Explanation().Total
		if sum != 0 && sum != total {
			parts = scaled(parts, total/sum)
		}
		return core.Attribution{Parts: parts, Amount: total}
	}
}
