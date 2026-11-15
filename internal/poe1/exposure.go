package poe1

import (
	"fmt"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/config"
	"github.com/xaroth/pbt-engine/internal/core"
)

// Exposure config keys: whether the enemy has Exposure of an element. Each is off by default
// and derived from the build where no layer sets it (DeriveConditions).
const (
	EnemyFireExposure      config.Key[bool] = "enemy-fire-exposure"
	EnemyColdExposure      config.Key[bool] = "enemy-cold-exposure"
	EnemyLightningExposure config.Key[bool] = "enemy-lightning-exposure"
)

// Engine stats of the Exposure a skill's hit inflicts, read in the hit's context with the
// element's damage type. inflicted_exposure is the strongest Exposure every hit inflicts, as
// what it applies to the resistance (negative); exposure_chance the chance in percent to
// inflict Exposure of baseExposure.
const (
	statInflictedExposure = "inflicted_exposure"
	statExposureChance    = "exposure_chance"
)

// baseExposure is what Exposure applies to its resistance where its source gives no value.
// Source: the game's text ("Minions inflict Exposure on Hit, applying -10% to the Elemental
// Resistance ..."), as PoB b32759a ConfigOptions.lua applies it for the enemy exposure
// conditions.
const baseExposure = -10

// certainChance is a chance to inflict Exposure that inflicts it on every hit.
const certainChance = 100

// exposure is an element's Exposure: its config key and damage type, and its text in
// explanations.
type exposure struct {
	key    config.Key[bool]
	damage DamageType
	text   string
}

func exposures() []exposure {
	return []exposure{
		{EnemyFireExposure, Fire, "Fire Exposure on the enemy"},
		{EnemyColdExposure, Cold, "Cold Exposure on the enemy"},
		{EnemyLightningExposure, Lightning, "Lightning Exposure on the enemy"},
	}
}

func exposureSpecs() []config.Spec {
	es := exposures()
	out := make([]config.Spec, 0, len(es))
	for _, e := range es {
		out = append(out, config.SpecOf(e.key, false))
	}
	return out
}

func exposureMechanic() mechanic {
	def := func(def core.StatDef, _ map[string]core.StatID) core.StatDef {
		if def.Name == statInflictedExposure {
			def.LowestBase = true
		}
		return def
	}
	return mechanic{name: "exposure", stats: []string{statInflictedExposure, statExposureChance}, def: def}
}

// addExposure adds the enemy's Exposure of each element, in the context of its damage type,
// as a rule's source, none until set (setExposure).
func (sh *Sheet) addExposure() error {
	sh.exposure = map[DamageType]core.ModID{}
	for _, e := range exposures() {
		src := sh.newSource(Source{RuleID: "exposure-" + e.key.Name(), Rule: e.text})
		pred := core.Predicate{Req: []core.Mask{core.MaskOf(sh.ctx.damage.Bit(int(e.damage) - 1))}}
		id, err := sh.session.AddModifier(core.Modifier{Stat: sh.stats[statEnemyExposure], Op: core.OpBase, Source: src, Pred: pred})
		if err != nil {
			return fmt.Errorf("sheet: exposure: %w", err)
		}
		sh.exposure[e.damage] = id
	}
	return nil
}

// setExposure sets the enemy's Exposure of each element by cfg alone: baseExposure while its
// key is on. DeriveConditions sets what the build inflicts.
func (sh *Sheet) setExposure(cfg config.Resolved) {
	for _, e := range exposures() {
		on, _ := config.Lookup(cfg, e.key)
		sh.setExposureValue(e.damage, exposureValue(on, inflictedExposure{}))
	}
}

func (sh *Sheet) setExposureValue(d DamageType, v float64) {
	// The id is this session's, so this cannot fail.
	_ = sh.session.SetModifierValue(sh.exposure[d], v)
}

// inflictedExposure is the Exposure of an element the build's hits inflict: the strongest one
// a hit inflicts every time (0 for none), and whether a hit inflicts some by chance.
type inflictedExposure struct {
	strongest float64
	chance    bool
}

// certain reports whether a hit of the build inflicts the Exposure every time.
func (x inflictedExposure) certain() bool { return x.strongest < 0 }

// exposureValue is what the enemy's Exposure applies to its resistance, 0 while off: the
// strongest of what the build's hits inflict every time and, when a hit inflicts it only by
// chance or none does, baseExposure.
func exposureValue(on bool, x inflictedExposure) float64 {
	if !on {
		return 0
	}
	if x.chance || !x.certain() {
		return min(x.strongest, baseExposure)
	}
	return x.strongest
}

// deriveExposure derives each element's Exposure key into out from the skills on the sheet, as
// DeriveConditions does a condition, with this difference: Exposure a hit inflicts only by a
// chance below certainChance is no source to derive it from, though a key set on applies it
// without a warning. It sets the enemy's Exposure to the result and returns the warnings.
func (sh *Sheet) deriveExposure(cfg, out config.Resolved) ([]build.Warning, error) {
	var warnings []build.Warning
	for _, e := range exposures() {
		x, err := sh.exposureOf(e.damage)
		if err != nil {
			return nil, err
		}
		on, src := config.Lookup(cfg, e.key)
		switch {
		case !src.Explicit() && x.certain():
			out[e.key.Name()] = config.Entry{Value: true, Source: config.Derived}
		case !src.Explicit():
			out[e.key.Name()] = config.Entry{Value: false, Source: config.Default}
		case on && !x.certain() && !x.chance:
			warnings = append(warnings, noConditionSource(e.key, src))
		}
		v, _ := config.Lookup(out, e.key)
		sh.setExposureValue(e.damage, exposureValue(v, x))
	}
	return warnings, nil
}

// exposureOf returns the Exposure of damage type d that the hits of the skills on the
// sheet, but the default attack, inflict, with either hand.
func (sh *Sheet) exposureOf(d DamageType) (inflictedExposure, error) {
	var x inflictedExposure
	err := sh.eachHit(func(i int, read handRead) (bool, error) {
		strongest, err := sh.hitValue(i, statInflictedExposure, d, read)
		if err != nil {
			return false, err
		}
		chance, err := sh.hitValue(i, statExposureChance, d, read)
		if err != nil {
			return false, err
		}
		if chance >= certainChance {
			strongest = min(strongest, baseExposure)
		}
		x.strongest = min(x.strongest, strongest)
		x.chance = x.chance || chance > 0 && chance < certainChance
		return false, nil
	})
	return x, err
}
