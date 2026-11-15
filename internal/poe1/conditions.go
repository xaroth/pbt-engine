package poe1

import (
	"fmt"
	"slices"

	"github.com/xaroth/pbt-engine/config"
	"github.com/xaroth/pbt-engine/internal/core"
	"github.com/xaroth/pbt-engine/internal/semantics"
)

// Engine stats of conditions, each 1 while it holds and else 0. Table modifiers hold on them
// ("while"). enemy_shock_effect is the increased damage the enemy takes while Shocked, in
// percent.
const (
	statEnemyIsShocked             = "enemy_is_shocked"
	statEnemyShockEffect           = "enemy_shock_effect"
	statEnemyIsChilled             = "enemy_is_chilled"
	statEnemyIsFrozen              = "enemy_is_frozen"
	statEnemyIsIgnited             = "enemy_is_ignited"
	statEnemyIsBleeding            = "enemy_is_bleeding"
	statEnemyIsMoving              = "enemy_is_moving"
	statEnemyIsUnnerved            = "enemy_is_unnerved"
	statEnemyIsIntimidated         = "enemy_is_intimidated"
	statEnemyIsTaunted             = "enemy_is_taunted"
	statEnemyIsOnConsecratedGround = "enemy_is_on_consecrated_ground"
	statEnemyIsUnique              = "enemy_is_unique"
	statIsOnFullLife               = "is_on_full_life"
	statIsOnFullEnergyShield       = "is_on_full_energy_shield"
	statIsLeeching                 = "is_leeching"
	statIsFocused                  = "is_focused"
	statIsFortified                = "is_fortified"
)

// Condition config keys: whether a condition holds for the character or the enemy its damage
// is measured against. Each is off by default. A key at its default has no value from a call,
// sim or build, so a value derived from the build can take its place; an explicit value wins
// over a derived one.
const (
	EnemyShocked             config.Key[bool] = "enemy-shocked"
	EnemyChilled             config.Key[bool] = "enemy-chilled"
	EnemyFrozen              config.Key[bool] = "enemy-frozen"
	EnemyIgnited             config.Key[bool] = "enemy-ignited"
	EnemyBleeding            config.Key[bool] = "enemy-bleeding"
	EnemyMoving              config.Key[bool] = "enemy-moving"
	EnemyUnnerved            config.Key[bool] = "enemy-unnerved"
	EnemyIntimidated         config.Key[bool] = "enemy-intimidated"
	EnemyTaunted             config.Key[bool] = "enemy-taunted"
	EnemyOnConsecratedGround config.Key[bool] = "enemy-on-consecrated-ground"
	OnFullLife               config.Key[bool] = "full-life"
	OnFullEnergyShield       config.Key[bool] = "full-energy-shield"
	Leeching                 config.Key[bool] = "leeching"
	Focused                  config.Key[bool] = "focused"
	Fortified                config.Key[bool] = "fortified"
)

// EnemyShockEffect is the effect of the Shock on the enemy while EnemyShocked holds: the
// increased damage it takes, in percent, at most maximumShockEffect. Its default, 0, adds
// none, as Path of Building counts a Shock whose effect the config does not give.
const EnemyShockEffect config.Key[ShockEffect] = "enemy-shock-effect"

// ShockEffect is the effect of a Shock in percent, from 0.
type ShockEffect int

// Valid reports whether e is not negative.
func (e ShockEffect) Valid() error {
	if e >= 0 {
		return nil
	}
	return fmt.Errorf("%w: shock effect %d, want 0 or more", ErrConfigValue, e)
}

// maximumShockEffect caps a Shock's effect: 50%, the game's default maximum. Source: PoB
// b32759a, Data.lua nonDamagingAilment.Shock.max; stats that raise it are not modelled.
const maximumShockEffect = 50

// condition is a condition config key and the engine stat that holds it.
type condition struct {
	key  config.Key[bool]
	stat string
	// text names the condition in explanations.
	text string
}

func conditions() []condition {
	return []condition{
		{EnemyShocked, statEnemyIsShocked, "the enemy is Shocked"},
		{EnemyChilled, statEnemyIsChilled, "the enemy is Chilled"},
		{EnemyFrozen, statEnemyIsFrozen, "the enemy is Frozen"},
		{EnemyIgnited, statEnemyIsIgnited, "the enemy is Ignited"},
		{EnemyBleeding, statEnemyIsBleeding, "the enemy is Bleeding"},
		{EnemyMoving, statEnemyIsMoving, "the enemy is moving"},
		{EnemyUnnerved, statEnemyIsUnnerved, "the enemy is Unnerved"},
		{EnemyIntimidated, statEnemyIsIntimidated, "the enemy is Intimidated"},
		{EnemyTaunted, statEnemyIsTaunted, "the enemy is Taunted"},
		{EnemyOnConsecratedGround, statEnemyIsOnConsecratedGround, "the enemy is on Consecrated Ground"},
		{OnFullLife, statIsOnFullLife, "on Full Life"},
		{OnFullEnergyShield, statIsOnFullEnergyShield, "on Full Energy Shield"},
		{Leeching, statIsLeeching, "Leeching"},
		{Focused, statIsFocused, "Focused"},
		{Fortified, statIsFortified, "Fortified"},
	}
}

func conditionSpecs() []config.Spec {
	cs := conditions()
	out := make([]config.Spec, 0, len(cs)+1)
	for _, c := range cs {
		out = append(out, config.SpecOf(c.key, false))
	}
	return append(out, config.SpecOf(EnemyShockEffect, 0))
}

// Debuffs that make the enemy take increased damage of a kind of skill, 10% each. Source: the
// game's reminder texts ("Unnerved enemies take 10% increased Spell Damage", "Intimidated
// enemies take 10% increased Attack Damage"), as PoB b32759a CalcSetup.lua applies them.
const debuffDamageTaken = 10

// conditionMods are the modifiers and sources that hold the conditions on a sheet.
type conditionMods struct {
	// sources holds each condition stat's source, on while it holds.
	sources     map[string]core.SourceID
	shockEffect core.ModID
}

// addConditions adds each condition as a source that sets its stat while on, all off, and
// what the conditions do themselves: a Shocked enemy takes increased damage by the Shock's
// effect, an Unnerved one increased spell damage and an Intimidated one increased attack
// damage.
func (sh *Sheet) addConditions() error {
	sh.conditions.sources = map[string]core.SourceID{}
	cs := conditions()
	stats := make([]string, 0, len(cs)+1)
	stats = append(stats, statEnemyIsUnique)
	texts := map[string]string{statEnemyIsUnique: "the enemy is Unique"}
	for _, c := range cs {
		stats = append(stats, c.stat)
		texts[c.stat] = c.text
	}
	for _, stat := range stats {
		src := sh.newSource(Source{RuleID: "condition-" + stat, Rule: texts[stat]})
		if _, err := sh.session.AddModifier(core.Modifier{Stat: sh.stats[stat], Op: core.OpFlag, Value: 1, Source: src}); err != nil {
			return fmt.Errorf("sheet: condition %s: %w", stat, err)
		}
		// src is this session's, so this cannot fail.
		_ = sh.session.SetSourceEnabled(src, false)
		sh.conditions.sources[stat] = src
	}
	shock := sh.newSource(Source{RuleID: "shock", Rule: "Shock (configured effect)"})
	var err error
	if sh.conditions.shockEffect, err = sh.session.AddModifier(core.Modifier{Stat: sh.stats[statEnemyShockEffect], Op: core.OpBase, Source: shock}); err != nil {
		return fmt.Errorf("sheet: shock: %w", err)
	}
	if _, err := sh.session.AddModifier(core.Modifier{
		Stat: sh.stats[statEnemyDamageTaken], Op: core.OpInc, Value: 1, Source: shock,
		Conds:  []core.Cond{{Ref: core.SelfRef(sh.stats[statEnemyIsShocked]), Cmp: core.CmpNE}},
		Scales: []core.Scale{{Ref: core.SelfRef(sh.stats[statEnemyShockEffect]), Step: 1}},
	}); err != nil {
		return fmt.Errorf("sheet: shock: %w", err)
	}
	return sh.addDebuffs()
}

func (sh *Sheet) addDebuffs() error {
	for _, d := range []struct{ stat, rule, skillType string }{
		{statEnemyIsUnnerved, "Unnerve (10% increased spell damage taken)", "type.Spell"},
		{statEnemyIsIntimidated, "Intimidate (10% increased attack damage taken)", "type.Attack"},
	} {
		p, err := predicate(sh.session.Model(), semantics.ContextFilter{In: [][]string{{d.skillType}}}, sh.ctx, -1)
		if err != nil {
			return fmt.Errorf("sheet: %s: %w", d.stat, err)
		}
		src := sh.newSource(Source{RuleID: d.stat, Rule: d.rule})
		if _, err := sh.session.AddModifier(core.Modifier{
			Stat: sh.stats[statEnemyDamageTaken], Op: core.OpInc, Value: debuffDamageTaken, Source: src, Pred: p,
			Conds: []core.Cond{{Ref: core.SelfRef(sh.stats[d.stat]), Cmp: core.CmpNE}},
		}); err != nil {
			return fmt.Errorf("sheet: %s: %w", d.stat, err)
		}
	}
	return nil
}

// setConditions switches each condition on or off by cfg, and sets the Shock's effect.
func (sh *Sheet) setConditions(cfg config.Resolved) {
	for _, c := range conditions() {
		on, _ := config.Lookup(cfg, c.key)
		// The sources are this session's, so these cannot fail.
		_ = sh.session.SetSourceEnabled(sh.conditions.sources[c.stat], on)
	}
	effect, _ := config.Lookup(cfg, EnemyShockEffect)
	_ = sh.session.SetModifierValue(sh.conditions.shockEffect, float64(min(effect, maximumShockEffect)))
}

// setEnemyUnique switches enemy_is_unique on for every enemy kind but a normal monster: map,
// pinnacle and uber bosses are unique monsters.
func (sh *Sheet) setEnemyUnique(kind EnemyKind) {
	_ = sh.session.SetSourceEnabled(sh.conditions.sources[statEnemyIsUnique], kind != EnemyNone)
}

func conditionMechanic() mechanic {
	cs := conditions()
	stats := make([]string, 0, len(cs))
	for _, c := range cs {
		stats = append(stats, c.stat)
	}
	stats = append(stats, statEnemyIsUnique, statEnemyShockEffect)
	slices.Sort(stats)
	return mechanic{name: mechanicConditions, stats: stats, def: func(def core.StatDef, _ map[string]core.StatID) core.StatDef {
		if def.Name != statEnemyShockEffect && slices.Contains(stats, def.Name) {
			def.Flag = true
		}
		return def
	}}
}

// mechanicConditions names the conditions mechanic for those that come after it.
const mechanicConditions = "conditions"
