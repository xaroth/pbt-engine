package poe1

import (
	"github.com/xaroth/pbt-engine/internal/core"
	"github.com/xaroth/pbt-engine/output"
)

// statPoisonDPS, read in a hit's context, is the damage per second the enemy takes from the
// skill's poisons, by PoB's expected value (poisonDPS); the game shows no such number.
const statPoisonDPS = "poison_dps"

// poisonKind is poison: chaos damage over time of 30% of the physical and chaos damage of the
// hit that inflicted it per second, for 2 seconds. The share and duration are PoB's (Data.lua
// misc); the dataset holds no text that gives them.
func poisonKind() ailmentKind {
	const share, duration = 0.3, 2
	return ailmentKind{ailment: ailmentPoison, damage: Chaos, share: share, duration: duration, inherent: []DamageType{Physical}}
}

// mechanicPoison names the poison mechanic for those that come after it.
const mechanicPoison = "poison"

// formulaPoisonDPS is the formula name shown in explanations.
const formulaPoisonDPS = "poison damage per second"

func poisonMechanic() mechanic {
	return mechanic{
		name: mechanicPoison, after: []string{mechanicAilment, mechanicHit, mechanicAccuracy},
		stats: []string{statPoisonDPS}, def: poisonDef,
	}
}

func poisonDef(def core.StatDef, ids map[string]core.StatID) core.StatDef {
	if def.Name == statPoisonDPS {
		d := poisonDPS{kind: poisonKind(), reads: newAilmentReads(ids), masks: newAilmentMasks()}
		def.FormulaName, def.Formula, def.Attribute = formulaPoisonDPS, d.formula(), d.attribute()
	}
	return def
}

// poisonOutputs are the main skill's poison outputs.
func poisonOutputs() []Output {
	return []Output{
		{Key: PoisonDPS, Label: "Poison DPS", Better: output.Higher, Places: hitPlaces, stat: statPoisonDPS, needs: needsAilment},
	}
}

// poisonDPS is the damage per second the enemy takes from a skill's poisons, read in its hit's
// context. Every poison deals damage, without limit. PoB's expected value for it, which the
// engine takes as its definition:
//
//   - The poisons on the enemy at once are the hits per second that land, times the chance per
//     hit to poison, times the duration; fewer than one scales the damage down by that share.
//   - Each starts from the middle of the source damage's range.
//   - Each comes from a critical strike with the hit's chance to strike critically, and then
//     adds critical_ailment_damage_over_time_multiplier.
//   - One poison's damage per second, and all of them, are each at most dotDPSCap.
//
// The count and the weighing follow PoB (CalcOffence.lua); the game gives none.
type poisonDPS struct {
	kind  ailmentKind
	reads ailmentReads
	masks ailmentMasks
}

// poison is the parts of a skill's poison damage per second.
type poison struct {
	in inflicted
	// stacks is the poisons on the enemy at once and source the damage one starts from.
	stacks, source float64
	// multiplied is one poison's damage per second before the critical strike multiplier,
	// weighted with it and single after the enemy and the cap.
	multiplied, weighted, single float64
	dps                          float64
}

func (d poisonDPS) of(m core.Mask, stat func(core.StatID, core.Mask) float64) poison {
	in := d.kind.read(d.reads, d.masks, m, stat)
	p := poison{in: in, stacks: in.applied(), source: (in.minimum + in.maximum) * half}
	p.weighted = in.weighted(d.kind, p.source, in.crit)
	if p.weighted > 0 {
		p.multiplied = p.source * in.multiplier * d.kind.share * in.faster
	}
	p.single = min(p.weighted*in.enemy(), dotDPSCap)
	p.dps = min(p.single*p.stacks, dotDPSCap)
	return p
}

func (d poisonDPS) formula() core.Formula {
	return func(r core.Reader, _ core.Agg) float64 {
		return d.of(r.Mask(), r.StatAt).dps
	}
}

// attribute credits the source damage's minimum and maximum by half each, then what the Damage
// over Time Multiplier, critical strikes, the enemy's resistance and damage taken change to
// their sources; the count of poisons and the caps go to the hits per second's sources.
func (d poisonDPS) attribute() core.Attribute {
	return func(a *core.Attributor) core.Attribution {
		total := a.Explanation().Total
		m := a.Mask()
		read := func(s core.StatID, at core.Mask) float64 { return a.Read(a.RefAt(s, at)) }
		p := d.of(m, read)
		if total == 0 || p.weighted == 0 {
			return core.Attribution{Amount: total}
		}
		withAilment, ofType := d.kind.at(d.masks, m)
		base := d.kind.share * p.in.faster
		var parts []core.Part
		for _, src := range d.kind.sources(d.masks, withAilment, read, d.reads.canInflict) {
			for _, s := range []core.StatID{d.reads.minimum, d.reads.maximum} {
				ps, _ := a.Of(a.RefAt(s, src))
				parts = append(parts, scaled(ps, half*base)...)
			}
		}
		parts = append(parts, d.reads.enemySteps(a, ofType, p.in, p.source*base, p.multiplied, p.weighted)...)
		return core.Attribution{Parts: rest(a, parts, total, a.RefAt(d.reads.rate, m)), Amount: total}
	}
}
