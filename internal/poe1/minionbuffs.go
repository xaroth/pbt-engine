package poe1

import (
	"fmt"

	"github.com/xaroth/pbt-engine/config"
	"github.com/xaroth/pbt-engine/internal/core"
	"github.com/xaroth/pbt-engine/internal/resolve"
	"github.com/xaroth/pbt-engine/internal/semantics"
)

// statHasFeedingFrenzy is set while you have Feeding Frenzy.
const statHasFeedingFrenzy = "has_feeding_frenzy"

// FeedingFrenzy is whether you have Feeding Frenzy, the buff the minions of a skill Feeding
// Frenzy supports grant you on hit; it reaches all your minions. Off by default.
const FeedingFrenzy config.Key[bool] = "feeding-frenzy"

// Feeding Frenzy's effect on your minions: 10% more damage and 10% increased attack, cast and
// movement speed. Source: the buff's text, as PoB b32759a ConfigOptions.lua
// (feedingFrenzyFeedingFrenzyActive) applies it.
const (
	feedingFrenzyMoreDamage = 10
	feedingFrenzySpeed      = 10
)

// Minion charge config keys. MinionFrenzyCharges is whether your minions have frenzy charges,
// off by default. While it is on they have MinionFrenzyChargeCount of them, or their maximum
// when it is 0, its default; it is not capped by the maximum, as PoB does not cap it.
const (
	MinionFrenzyCharges     config.Key[bool]        = "minion-frenzy-charges"
	MinionFrenzyChargeCount config.Key[ChargeCount] = "minion-frenzy-charge-count"
)

// ChargeCount is a number of charges, from 0.
type ChargeCount int

// Valid reports whether n is not negative.
func (n ChargeCount) Valid() error {
	if n >= 0 {
		return nil
	}
	return fmt.Errorf("%w: charges %d, want 0 or more", ErrConfigValue, n)
}

func minionBuffSpecs() []config.Spec {
	return []config.Spec{config.SpecOf(MinionFrenzyCharges, false), config.SpecOf(MinionFrenzyChargeCount, 0)}
}

// minionChargeMods are the modifiers that hold the frenzy charges the config gives minions.
type minionChargeMods struct {
	uses  core.SourceID
	count core.ModID
}

// addMinionBuffs adds Feeding Frenzy's effect on minions, held while you have it, and the frenzy
// charges minions use, none until the config gives them.
func (sh *Sheet) addMinionBuffs() error {
	src := sh.newSource(Source{
		RuleID: "feeding-frenzy", Rule: "Feeding Frenzy (10% more minion damage, 10% increased minion attack, cast and movement speed)",
	})
	has := core.SelfRef(sh.stats[statHasFeedingFrenzy])
	has.Actor = int32(playerActor)
	cond := []core.Cond{{Ref: has, Cmp: core.CmpNE}}
	for _, m := range []struct {
		stat  string
		op    core.Op
		value float64
	}{
		{statDamage, core.OpMore, feedingFrenzyMoreDamage},
		{statAttackSpeed, core.OpInc, feedingFrenzySpeed},
		{statCastSpeed, core.OpInc, feedingFrenzySpeed},
		{statMovementSpeed, core.OpInc, feedingFrenzySpeed},
	} {
		mod := core.Modifier{Actor: minionActor, Stat: sh.stats[m.stat], Op: m.op, Value: m.value, Source: src, Conds: cond}
		if _, err := sh.session.AddModifier(mod); err != nil {
			return fmt.Errorf("sheet: feeding frenzy: %w", err)
		}
	}
	return sh.addMinionCharges()
}

func (sh *Sheet) addMinionCharges() error {
	uses := sh.newSource(Source{RuleID: "minion-frenzy-charges", Rule: "minions use frenzy charges (configured)"})
	flag := core.Modifier{Actor: minionActor, Stat: sh.stats[statUsesFrenzyCharges], Op: core.OpFlag, Value: 1, Source: uses}
	if _, err := sh.session.AddModifier(flag); err != nil {
		return fmt.Errorf("sheet: minion frenzy charges: %w", err)
	}
	// uses is this session's, so this cannot fail.
	_ = sh.session.SetSourceEnabled(uses, false)
	count := sh.newSource(Source{RuleID: "minion-frenzy-charge-count", Rule: "frenzy charges of minions (configured)"})
	id, err := sh.session.AddModifier(core.Modifier{Actor: minionActor, Stat: sh.stats[statConfiguredFrenzyCharges], Op: core.OpBase, Source: count})
	if err != nil {
		return fmt.Errorf("sheet: minion frenzy charges: %w", err)
	}
	sh.minionCharges = minionChargeMods{uses: uses, count: id}
	return nil
}

// setMinionCharges gives minions the frenzy charges cfg says.
func (sh *Sheet) setMinionCharges(cfg config.Resolved) {
	on, _ := config.Lookup(cfg, MinionFrenzyCharges)
	n, _ := config.Lookup(cfg, MinionFrenzyChargeCount)
	// The source and modifier are this session's, so these cannot fail.
	_ = sh.session.SetSourceEnabled(sh.minionCharges.uses, on)
	_ = sh.session.SetModifierValue(sh.minionCharges.count, float64(n))
}

// addMinionBuff adds the buff of sk that reaches your minions only, from its buff stats, and
// returns its sources: the stats reach every minion, scaled by the buff's effect on the minion,
// buff_effect on the minion actor in sk's context ctx, 100% before increases in slot (PoB
// CalcPerform.lua, a buff with buffMinions and buffNotPlayer).
func (sh *Sheet) addMinionBuff(sk *resolve.Skill, stats []resolve.StatValue, slot int, ctx core.CtxID) ([]core.SourceID, error) {
	effect := core.Ref{Actor: int32(minionActor), Stat: sh.stats[statBuffEffect], Ctx: int64(ctx)}
	bd := minionBundle(sk.Source, stats)
	buff, err := sh.addBundle(&bd, sh.table, sh.gameVersion, -1, &carried{effect: effect})
	if err != nil {
		return nil, err
	}
	src := sh.newSource(Source{RuleID: "skill-buff", Rule: "buff of " + sk.Source.String() + " on minions"})
	pred, err := predicate(sh.session.Model(), semantics.ContextFilter{}, sh.ctx, slot)
	if err != nil {
		return nil, err
	}
	mod := core.Modifier{Actor: minionActor, Stat: sh.stats[statBuffEffect], Op: core.OpBase, Value: baseBuffEffect, Source: src, Pred: pred}
	if _, err := sh.session.AddModifier(mod); err != nil {
		return nil, fmt.Errorf("sheet: buff of %s: %w", sk.Source, err)
	}
	return []core.SourceID{buff, src}, nil
}
