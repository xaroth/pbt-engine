package poe1

import (
	"github.com/xaroth/pbt-engine/internal/core"
	"github.com/xaroth/pbt-engine/internal/resolve"
	"github.com/xaroth/pbt-engine/output"
)

// statIgniteDPS, read in a hit's context, is the damage per second the enemy takes from the
// skill's ignite, by PoB's expected value (igniteDPS); the game shows no such number.
const statIgniteDPS = "ignite_dps"

// igniteKind is ignite: fire damage over time of 90% of the fire damage of the hit that
// inflicted it per second, for 4 seconds, as the game text says ("Ignited enemies take 90% of
// the Fire Damage of the Hit that Ignited them as Fire Damage over Time, over 4 seconds").
func igniteKind() ailmentKind {
	const share, duration = 0.9, 4
	return ailmentKind{ailment: ailmentIgnite, damage: Fire, share: share, duration: duration}
}

// igniteSkill reports whether the sheet gives sk's ignite with hands: a hit the character uses
// itself, with one hand, since its rate of hits decides how many ignites it applies.
func igniteSkill(sk *resolve.Skill, h Hands) bool {
	return selfUsedHit(sk, h) && !bothHandsHit(sk, h)
}

// mechanicIgnite names the ignite mechanic for those that come after it.
const mechanicIgnite = "ignite"

// formulaIgniteDPS is the formula name shown in explanations.
const formulaIgniteDPS = "ignite damage per second"

func igniteMechanic() mechanic {
	return mechanic{
		name: mechanicIgnite, after: []string{mechanicAilment, mechanicHit, mechanicAccuracy},
		stats: []string{statIgniteDPS}, def: igniteDef,
	}
}

func igniteDef(def core.StatDef, ids map[string]core.StatID) core.StatDef {
	if def.Name == statIgniteDPS {
		d := igniteDPS{kind: igniteKind(), reads: newAilmentReads(ids), masks: newAilmentMasks()}
		def.FormulaName, def.Formula, def.Attribute = formulaIgniteDPS, d.formula(), d.attribute()
	}
	return def
}

// igniteOutputs are the main skill's ignite outputs.
func igniteOutputs() []Output {
	return []Output{
		{Key: IgniteDPS, Label: "Ignite DPS", Better: output.Higher, Places: hitPlaces, stat: statIgniteDPS, needs: needsIgnite},
	}
}

// igniteDPS is the damage per second the enemy takes from a skill's ignite, read in its hit's
// context. Only the strongest ignite deals damage. PoB's expected value for it, which the
// engine takes as its definition:
//
//   - The ignites applied over one duration are the hits per second that land, times the
//     chance per hit to ignite (a critical strike always ignites), times the duration.
//   - With more than one, the strongest rolls higher: of a source damage between its minimum
//     and maximum, it sits at applied/(applied+1) of the way; with one or fewer, half way.
//   - The chance that the strongest came from a critical strike is 1-(1-crit)^applied; an
//     ignite from a critical strike adds critical_ailment_damage_over_time_multiplier.
//   - Fewer than one ignite applied per duration scales the damage down by that share.
//
// The roll, the weighing and the scaling follow PoB (CalcOffence.lua); the game gives none.
type igniteDPS struct {
	kind  ailmentKind
	reads ailmentReads
	masks ailmentMasks
}

// ignite is the parts of a skill's ignite damage per second.
type ignite struct {
	in inflicted
	// applied is the ignites applied over a duration, roll where the strongest sits in the
	// source damage's range and source its damage at that roll.
	applied, roll, source float64
	// multiplied is the damage per second before the critical strike multiplier, and weighted
	// with it.
	multiplied, weighted float64
	dps                  float64
}

func (d igniteDPS) of(m core.Mask, stat func(core.StatID, core.Mask) float64) ignite {
	in := d.kind.read(d.reads, d.masks, m, stat)
	g := ignite{in: in, applied: in.applied(), roll: half}
	if g.applied > 1 {
		g.roll = g.applied / (g.applied + 1)
	}
	g.source = in.minimum + (in.maximum-in.minimum)*g.roll
	crit := critOfStrongest(in.crit, g.applied)
	g.weighted = in.weighted(d.kind, g.source, crit)
	if g.weighted > 0 {
		g.multiplied = g.source * in.multiplier * d.kind.share * in.faster
	}
	g.dps = min(g.weighted*min(g.applied, 1)*in.enemy(), dotDPSCap)
	return g
}

func (d igniteDPS) formula() core.Formula {
	return func(r core.Reader, _ core.Agg) float64 {
		return d.of(r.Mask(), r.StatAt).dps
	}
}

// attribute credits the source damage's minimum and maximum by the roll, then what the Damage
// over Time Multiplier, critical strikes, the enemy's resistance and damage taken change to
// their sources; the share of an ignite applied, when below one, and the cap go to the hits per
// second's sources.
func (d igniteDPS) attribute() core.Attribute {
	return func(a *core.Attributor) core.Attribution {
		total := a.Explanation().Total
		m := a.Mask()
		g := d.of(m, func(s core.StatID, at core.Mask) float64 { return a.Read(a.RefAt(s, at)) })
		if total == 0 || g.weighted == 0 {
			return core.Attribution{Amount: total}
		}
		withAilment, ofType := d.kind.at(d.masks, m)
		base := d.kind.share * g.in.faster
		var parts []core.Part
		for _, src := range d.kind.sources(d.masks, withAilment, func(s core.StatID, at core.Mask) float64 { return a.Read(a.RefAt(s, at)) }, d.reads.canInflict) {
			lo, _ := a.Of(a.RefAt(d.reads.minimum, src))
			hi, _ := a.Of(a.RefAt(d.reads.maximum, src))
			parts = append(parts, scaled(lo, (1-g.roll)*base)...)
			parts = append(parts, scaled(hi, g.roll*base)...)
		}
		v := g.source * base
		steps := []struct {
			ref  core.Ref
			next float64
		}{
			{a.RefAt(d.reads.multiplier, ofType), g.multiplied},
			{a.RefAt(d.reads.criticalMultiplier, ofType), g.weighted},
			{a.RefAt(d.reads.resistance, ofType), g.weighted * resistanceMultiplier(g.in.resistance)},
			{a.RefAt(d.reads.taken, ofType), g.weighted * g.in.enemy()},
		}
		for _, s := range steps {
			if delta := s.next - v; delta != 0 {
				parts = append(parts, a.Spread(s.ref, delta, core.OpMore)...)
			}
			v = s.next
		}
		return core.Attribution{Parts: rest(a, parts, total, a.RefAt(d.reads.rate, m)), Amount: total}
	}
}
