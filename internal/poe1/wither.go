package poe1

import (
	"fmt"
	"math"

	"github.com/xaroth/pbt-engine/config"
	"github.com/xaroth/pbt-engine/internal/core"
)

// Engine stats of Withered. can_wither is a flag that a skill's hits, or its minion's, inflict
// Withered, read in the skill's context. withered_effect is the increased chaos damage taken
// one Withered stack applies, in percent, read for the actor that inflicts it.
// enemy_withered_stacks is the number of Withered stacks on the enemy.
const (
	statCanWither           = "can_wither"
	statWitheredEffect      = "withered_effect"
	statEnemyWitheredStacks = "enemy_withered_stacks"
)

// EnemyWitheredStacks is the number of Withered stacks on the enemy, at most
// maximumWitheredStacks. The build's hits do not derive it: the config says it. Its default, 0,
// is Path of Building's. The stacks make the enemy take more chaos damage only when a skill of
// the build, or its minion, inflicts Withered; stats per Withered stack read them regardless.
const EnemyWitheredStacks config.Key[WitheredStacks] = "enemy-withered-stacks"

// WitheredStacks is a count of Withered stacks, from 0.
type WitheredStacks int

// Valid reports whether n is not negative.
func (n WitheredStacks) Valid() error {
	if n >= 0 {
		return nil
	}
	return fmt.Errorf("%w: withered stacks %d, want 0 or more", ErrConfigValue, n)
}

// Withered values. Source: the game's text ("Withered applies 6% increased Chaos Damage Taken,
// up to 15 stacks"), as PoB b32759a CalcPerform.lua (WitherEffectStack) and ConfigOptions.lua
// (multiplierWitheredStackCount) apply them.
const (
	maximumWitheredStacks = 15
	baseWitheredEffect    = 6
)

func witherSpecs() []config.Spec {
	return []config.Spec{config.SpecOf(EnemyWitheredStacks, 0)}
}

func witherMechanic() mechanic {
	def := func(def core.StatDef, _ map[string]core.StatID) core.StatDef {
		if def.Name == statCanWither {
			def.Flag = true
		}
		return def
	}
	return mechanic{name: "wither", stats: []string{statCanWither, statWitheredEffect, statEnemyWitheredStacks}, def: def}
}

// witherMods are the modifiers that hold the Withered stacks on the enemy and the chaos damage
// they make it take.
type witherMods struct {
	stacks, damageTaken core.ModID
}

// addWither adds the enemy's Withered stacks, 0 until the config sets them; the base effect of
// a stack for the character and its minions; and the increased chaos damage the enemy takes
// per stack, none until DeriveConditions finds a skill that inflicts Withered.
func (sh *Sheet) addWither() error {
	src := sh.newSource(Source{RuleID: "withered-stacks", Rule: "Withered stacks on the enemy (configured)"})
	var err error
	if sh.wither.stacks, err = sh.session.AddModifier(core.Modifier{Stat: sh.stats[statEnemyWitheredStacks], Op: core.OpBase, Source: src}); err != nil {
		return fmt.Errorf("sheet: withered: %w", err)
	}
	base := sh.newSource(Source{RuleID: "withered-effect", Rule: "Withered (6% increased chaos damage taken per stack)"})
	for _, actor := range []core.ActorID{playerActor, minionActor} {
		mod := core.Modifier{Stat: sh.stats[statWitheredEffect], Actor: actor, Op: core.OpBase, Value: baseWitheredEffect, Source: base}
		if _, err := sh.session.AddModifier(mod); err != nil {
			return fmt.Errorf("sheet: withered: %w", err)
		}
	}
	taken := sh.newSource(Source{RuleID: "withered", Rule: "Withered (chaos damage taken per stack)"})
	sh.wither.damageTaken, err = sh.session.AddModifier(core.Modifier{
		Stat: sh.stats[statEnemyDamageTaken], Op: core.OpInc, Source: taken,
		Pred:   core.Predicate{Req: []core.Mask{damageBit(sh.ctx.damage, Chaos)}},
		Scales: []core.Scale{{Ref: core.SelfRef(sh.stats[statEnemyWitheredStacks]), Step: 1}},
	})
	if err != nil {
		return fmt.Errorf("sheet: withered: %w", err)
	}
	return nil
}

// setWither sets the Withered stacks from cfg, and no effect until DeriveConditions sets it.
func (sh *Sheet) setWither(cfg config.Resolved) {
	stacks, _ := config.Lookup(cfg, EnemyWitheredStacks)
	// The modifiers are this session's, so these cannot fail.
	_ = sh.session.SetModifierValue(sh.wither.stacks, float64(min(stacks, maximumWitheredStacks)))
	_ = sh.session.SetModifierValue(sh.wither.damageTaken, 0)
}

// deriveWither sets the increased chaos damage a Withered stack makes the enemy take: the
// largest effect, rounded down, of the skills on the sheet that inflict Withered, but the
// default attack. A skill with a minion inflicts it when it or its minion can, at the minion's
// effect, as PoB does (CalcPerform.lua WitherEffectStack).
func (sh *Sheet) deriveWither() error {
	var effect float64
	// Most builds inflict no Withered; this spares them reading every skill for it.
	if !sh.session.Live(sh.stats[statCanWither], playerActor) && !sh.session.Live(sh.stats[statCanWither], minionActor) {
		sh.setWitherEffect(0)
		return nil
	}
	for i, sl := range sh.skills {
		if sl.source.Default() {
			continue
		}
		e, err := sh.witherEffect(i)
		if err != nil {
			return err
		}
		effect = max(effect, e)
	}
	sh.setWitherEffect(effect)
	return nil
}

// witherEffect returns the effect of a Withered stack skill i inflicts, 0 for none.
func (sh *Sheet) witherEffect(i int) (float64, error) {
	can, err := sh.witherValue(i, playerActor, statCanWither)
	if err != nil {
		return 0, err
	}
	actor := playerActor
	if i < len(sh.slotMinions) && sh.slotMinions[i] != nil {
		actor = minionActor
		minion, err := sh.witherValue(i, minionActor, statCanWither)
		if err != nil {
			return 0, err
		}
		can += minion
	}
	if can == 0 {
		return 0, nil
	}
	e, err := sh.witherValue(i, actor, statWitheredEffect)
	return math.Floor(e), err
}

// witherValue returns engine stat name of actor in skill i's context, the minion's with its
// skill's types.
func (sh *Sheet) witherValue(i int, actor core.ActorID, name string) (float64, error) {
	node := sh.skillNode
	if actor == minionActor {
		node = sh.minionNode
	}
	n, err := node(i, name, nil, formNone)
	if err != nil {
		return 0, err
	}
	v, err := sh.session.Get(n)
	if err != nil {
		return 0, fmt.Errorf("sheet: skill %d %s: %w", i, name, err)
	}
	return v, nil
}

func (sh *Sheet) setWitherEffect(v float64) {
	// The modifier is this session's, so this cannot fail.
	_ = sh.session.SetModifierValue(sh.wither.damageTaken, v)
}
