package poe1

import (
	"github.com/xaroth/pbt-engine/internal/core"
)

// strongestDPS is the damage per second the enemy takes from an ailment of which only the
// strongest instances deal damage, as many as ailment_maximum_instances (ignite and bleed),
// read in the skill's hit's context. PoB's expected value for it, which the engine takes as
// its definition since the game shows none:
//
//   - The instances applied over one duration are the hits per second that land, times the
//     chance per hit to inflict it, times the duration.
//   - With more applied than the maximum, the strongest roll higher: of a source damage between
//     its minimum and maximum, they sit on average at (applied - (maximum-1)/2) / (applied+1)
//     of the way; else half way.
//   - The chance that the strongest came from a critical strike is 1-(1-crit)^(applied/maximum),
//     at least crit; an instance from a critical strike adds
//     critical_ailment_damage_over_time_multiplier.
//   - The instances that deal damage are the applied ones, at most the maximum; fewer than one
//     scales the damage down by that share.
//
// The roll, the weighing and the count follow PoB (CalcOffence.lua); the game gives none.
type strongestDPS struct {
	kind    ailmentKind
	reads   ailmentReads
	masks   ailmentMasks
	maximum core.StatID
}

func newStrongestDPS(kind ailmentKind, ids map[string]core.StatID) strongestDPS {
	return strongestDPS{kind: kind, reads: newAilmentReads(ids), masks: newAilmentMasks(), maximum: mustID(ids, statAilmentMaximumInstances)}
}

// strongest is the parts of a skill's damage per second from an ailment of which only the
// strongest instances deal damage.
type strongest struct {
	in inflicted
	// applied is the instances applied over a duration, active those that deal damage, roll
	// where the strongest sit in the source damage's range and source its damage at that roll.
	applied, active, roll, source float64
	// multiplied is the damage per second of one instance before the critical strike
	// multiplier, and weighted with it.
	multiplied, weighted float64
	dps                  float64
}

func (d strongestDPS) of(m core.Mask, stat func(core.StatID, core.Mask) float64) strongest {
	in := d.kind.read(d.reads, d.masks, m, stat)
	withAilment, _ := d.kind.at(d.masks, m)
	limit := max(stat(d.maximum, withAilment), 1)
	g := strongest{in: in, applied: in.applied(), roll: half}
	g.active = min(g.applied, limit)
	potential := g.applied / limit
	if potential > 1 {
		g.roll = (g.applied - (limit-1)/2) / (g.applied + 1)
	}
	g.source = in.minimum + (in.maximum-in.minimum)*g.roll
	crit := critOfStrongest(in.crit, potential)
	g.weighted = in.weighted(d.kind, g.source, crit)
	if g.weighted > 0 {
		g.multiplied = g.source * in.multiplier * d.kind.share * in.faster
	}
	g.dps = min(g.weighted*g.active*in.enemy(), dotDPSCap)
	return g
}

func (d strongestDPS) formula() core.Formula {
	return func(r core.Reader, _ core.Agg) float64 {
		return d.of(r.Mask(), r.StatAt).dps
	}
}

// attribute credits the source damage's minimum and maximum by the roll, then what the Damage
// over Time Multiplier, critical strikes, the enemy's resistance and damage taken change to
// their sources; the count of instances and the cap go to the hits per second's sources.
func (d strongestDPS) attribute() core.Attribute {
	return func(a *core.Attributor) core.Attribution {
		total := a.Explanation().Total
		m := a.Mask()
		read := func(s core.StatID, at core.Mask) float64 { return a.Read(a.RefAt(s, at)) }
		g := d.of(m, read)
		if total == 0 || g.weighted == 0 {
			return core.Attribution{Amount: total}
		}
		withAilment, ofType := d.kind.at(d.masks, m)
		base := d.kind.share * g.in.faster
		var parts []core.Part
		for _, src := range d.kind.sources(d.masks, withAilment, read, d.reads.canInflict) {
			lo, _ := a.Of(a.RefAt(d.reads.minimum, src))
			hi, _ := a.Of(a.RefAt(d.reads.maximum, src))
			parts = append(parts, scaled(lo, (1-g.roll)*base)...)
			parts = append(parts, scaled(hi, g.roll*base)...)
		}
		v := g.source * base
		parts = append(parts, d.reads.enemySteps(a, ofType, g.in, v, g.multiplied, g.weighted)...)
		return core.Attribution{Parts: rest(a, parts, total, a.RefAt(d.reads.rate, m)), Amount: total}
	}
}
