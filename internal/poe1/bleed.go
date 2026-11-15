package poe1

import (
	"fmt"

	"github.com/xaroth/pbt-engine/internal/core"
	"github.com/xaroth/pbt-engine/internal/semantics"
	"github.com/xaroth/pbt-engine/output"
)

// Engine stats of bleeding. bleed_dps, read in a hit's context, is the damage per second the
// enemy takes from the skill's bleeding, by PoB's expected value (strongestDPS); the game shows
// no such number. no_extra_bleed_damage_while_moving keeps bleeding from dealing more damage
// while the enemy moves.
const (
	statBleedDPS                      = "bleed_dps"
	statNoExtraBleedDamageWhileMoving = "no_extra_bleed_damage_while_moving"
)

// bleedKind is bleeding: physical damage over time of 70% of the physical damage of the hit
// that inflicted it per second, for 5 seconds. The share and duration are PoB's (Data.lua
// misc); the dataset holds no text that gives them.
func bleedKind() ailmentKind {
	const share, duration = 0.7, 5
	return ailmentKind{ailment: ailmentBleed, damage: Physical, share: share, duration: duration}
}

// bleedMovingDamage is the more damage bleeding deals while the enemy moves: 200%, three times
// as much. Source: PoB b32759a, CalcSetup.lua; the dataset holds no text that gives it.
const bleedMovingDamage = 200

// mechanicBleed names the bleed mechanic for those that come after it.
const mechanicBleed = "bleed"

// formulaBleedDPS is the formula name shown in explanations.
const formulaBleedDPS = "bleeding damage per second"

func bleedMechanic() mechanic {
	return mechanic{
		name: mechanicBleed, after: []string{mechanicAilment, mechanicHit, mechanicAccuracy},
		stats: []string{statBleedDPS, statNoExtraBleedDamageWhileMoving}, def: bleedDef,
	}
}

func bleedDef(def core.StatDef, ids map[string]core.StatID) core.StatDef {
	switch def.Name {
	case statBleedDPS:
		d := newStrongestDPS(bleedKind(), ids)
		def.FormulaName, def.Formula, def.Attribute = formulaBleedDPS, d.formula(), d.attribute()
	case statNoExtraBleedDamageWhileMoving:
		def.Flag = true
	}
	return def
}

// bleedOutputs are the main skill's bleed outputs.
func bleedOutputs() []Output {
	return []Output{
		{Key: BleedDPS, Label: "Bleed DPS", Better: output.Higher, Places: hitPlaces, stat: statBleedDPS, needs: needsAilment},
	}
}

// addBleedMoving adds the more damage bleeding deals while the enemy moves, unless
// no_extra_bleed_damage_while_moving is set.
func (sh *Sheet) addBleedMoving() error {
	p, err := predicate(sh.session.Model(), semantics.ContextFilter{In: [][]string{{"ailment.bleed"}, {"form.ailment"}}}, sh.ctx, -1)
	if err != nil {
		return fmt.Errorf("sheet: bleeding while moving: %w", err)
	}
	src := sh.newSource(Source{RuleID: "bleed-moving", Rule: "bleeding against a moving enemy (200% more damage)"})
	if _, err := sh.session.AddModifier(core.Modifier{
		Stat: sh.stats[statDamage], Op: core.OpMore, Value: bleedMovingDamage, Source: src, Pred: p,
		Conds: []core.Cond{
			{Ref: core.SelfRef(sh.stats[statEnemyIsMoving]), Cmp: core.CmpNE},
			{Ref: core.SelfRef(sh.stats[statNoExtraBleedDamageWhileMoving]), Cmp: core.CmpEQ},
		},
	}); err != nil {
		return fmt.Errorf("sheet: bleeding while moving: %w", err)
	}
	return nil
}
