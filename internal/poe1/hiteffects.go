package poe1

import (
	"github.com/xaroth/pbt-engine/internal/core"
)

// Engine stats of what scales a hit's damage as a whole, read in the skill's context.
// double_damage_chance and triple_damage_chance are the chances in percent that a hit deals
// double or triple damage. hits_per_ruthless_blow is how many hits make one Ruthless Blow, 0
// for none, and ruthless_blow_damage the more damage in percent a Ruthless Blow's hit deals.
const (
	statDoubleDamageChance  = "double_damage_chance"
	statTripleDamageChance  = "triple_damage_chance"
	statHitsPerRuthlessBlow = "hits_per_ruthless_blow"
	statRuthlessBlowDamage  = "ruthless_blow_damage"
)

// tripleDamageAdded is what a hit that deals triple damage adds, in hits.
const tripleDamageAdded = 2

// mechanicHitEffects names the hit effects mechanic for those that come after it.
const mechanicHitEffects = "hit effects"

func hitEffectsMechanic() mechanic {
	return mechanic{name: mechanicHitEffects, stats: []string{
		statDoubleDamageChance,
		statTripleDamageChance,
		statHitsPerRuthlessBlow,
		statRuthlessBlowDamage,
	}, def: hitEffectsDef}
}

// hitEffectsDef completes def when it is a hit effect stat: the chances are from 0 to 100%,
// the hits per Ruthless Blow whole and at least 0.
func hitEffectsDef(def core.StatDef, _ map[string]core.StatID) core.StatDef {
	switch def.Name {
	case statDoubleDamageChance, statTripleDamageChance:
		def.HasMin, def.HasMax, def.Max = true, true, percentScale
	case statHitsPerRuthlessBlow:
		def.Round, def.HasMin = core.RoundFloor, true
	}
	return def
}

// hitEffects reads what scales a hit's damage as a whole, before the enemy reduces it and
// after its damage of each type is rounded.
//
// A hit deals triple damage at its chance, else double damage at its chance: the double
// damage chance counts only on hits that do not deal triple damage. Every Nth hit is a
// Ruthless Blow, counted as a chance of 1/N in whole percent, rounded half up, that a hit
// deals the Ruthless Blow's more damage. Both are averaged into every hit. This follows PoB
// (CalcOffence.lua ScaledDamageEffect and RuthlessBlowHitEffect); the game text does not say
// how they combine.
type hitEffects struct {
	double, triple, ruthlessHits, ruthlessDamage core.StatID
}

func newHitEffects(ids map[string]core.StatID) hitEffects {
	return hitEffects{
		double: mustID(ids, statDoubleDamageChance), triple: mustID(ids, statTripleDamageChance),
		ruthlessHits: mustID(ids, statHitsPerRuthlessBlow), ruthlessDamage: mustID(ids, statRuthlessBlowDamage),
	}
}

// hitScale is what the effects multiply a hit by, as fractions above 1: from double damage,
// from triple damage and from Ruthless Blows.
type hitScale struct {
	double, triple, ruthless float64
}

// read returns the effects' scale, with stat reading a stat in the hit's context.
func (h hitEffects) read(stat func(core.StatID) float64) hitScale {
	triple := stat(h.triple) / percentScale
	s := hitScale{double: stat(h.double) / percentScale * (1 - triple), triple: tripleDamageAdded * triple}
	if n := stat(h.ruthlessHits); n > 0 {
		chance := roundHalf(percentScale/n) / percentScale
		s.ruthless = chance * stat(h.ruthlessDamage) / percentScale
	}
	return s
}

// multiplier is the factor s multiplies a hit by.
func (s hitScale) multiplier() float64 { return (1 + s.double + s.triple) * (1 + s.ruthless) }

// credit returns the parts of what s adds to a hit of amount: double and triple damage first,
// then Ruthless Blows over both, each to the sources of its stat.
func (h hitEffects) credit(a *core.Attributor, s hitScale, amount float64) []core.Part {
	var parts []core.Part
	for _, e := range []struct {
		stat core.StatID
		v    float64
	}{
		{h.double, amount * s.double},
		{h.triple, amount * s.triple},
		{h.ruthlessDamage, amount * (1 + s.double + s.triple) * s.ruthless},
	} {
		if e.v != 0 {
			parts = append(parts, a.Spread(core.SelfRef(e.stat), e.v, core.OpMore)...)
		}
	}
	return parts
}
