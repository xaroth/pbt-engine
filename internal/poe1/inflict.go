package poe1

import (
	"github.com/xaroth/pbt-engine/internal/core"
)

// Engine stats of whether a skill's hit inflicts an ailment that is a condition on the enemy,
// read in the hit's context: 1 when it can, else 0. A hit can when it has a chance to inflict
// the ailment, with or without a critical strike, and damage the ailment starts from.
const (
	statInflictsIgnite = "inflicts_ignite"
	statInflictsShock  = "inflicts_shock"
	statInflictsChill  = "inflicts_chill"
)

// shockKind is shock: a hit's lightning damage inflicts it by its chance, and a critical
// strike always does, as for ignite (PoB b32759a CalcOffence.lua ShockChanceOnCrit).
func shockKind() ailmentKind {
	return ailmentKind{ailment: ailmentShock, damage: Lightning, onCrit: true}
}

// chillKind is chill: every hit with cold damage inflicts it ("Chill: Cold Damage from Hits
// Chills enemies", the game's reminder text).
func chillKind() ailmentKind {
	return ailmentKind{ailment: ailmentChill, damage: Cold, always: true}
}

// mechanicInflict names the inflict mechanic.
const mechanicInflict = "inflict"

const formulaInflicts = "whether a hit inflicts the ailment"

func inflictMechanic() mechanic {
	return mechanic{
		name: mechanicInflict, after: []string{mechanicAilment, mechanicHit, mechanicAccuracy},
		stats: []string{statInflictsIgnite, statInflictsShock, statInflictsChill}, def: inflictDef,
	}
}

func inflictDef(def core.StatDef, ids map[string]core.StatID) core.StatDef {
	kinds := map[string]ailmentKind{statInflictsIgnite: igniteKind(), statInflictsShock: shockKind(), statInflictsChill: chillKind()}
	k, ok := kinds[def.Name]
	if !ok {
		return def
	}
	reads, masks := newAilmentReads(ids), newAilmentMasks()
	def.FormulaName = formulaInflicts
	def.Formula = func(r core.Reader, _ core.Agg) float64 {
		if k.inflicts(reads, masks, r.Mask(), r.StatAt) {
			return 1
		}
		return 0
	}
	return def
}

// inflicts reports whether a hit in context m can inflict ailment k: it has a chance to, with
// or without a critical strike, and damage the ailment starts from. The damage is read last,
// as it costs the most.
func (k ailmentKind) inflicts(r ailmentReads, masks ailmentMasks, m core.Mask, stat func(core.StatID, core.Mask) float64) bool {
	withAilment, _ := k.at(masks, m)
	onHit, onCrit, _ := k.chances(r, m, withAilment, stat)
	if onHit <= 0 && (onCrit <= 0 || stat(r.critChance, m) <= 0) {
		return false
	}
	for _, src := range k.sources(masks, withAilment, stat, r.canInflict) {
		if stat(r.maximum, src) > 0 {
			return true
		}
	}
	return false
}
