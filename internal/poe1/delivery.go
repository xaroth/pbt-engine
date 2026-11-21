package poe1

import (
	"github.com/xaroth/pbt-engine/internal/core"
)

// Engine stats of a skill a totem, trap or mine uses, read in its context, beside its hit rate
// (hitRate).
//
// trap_throwing_speed and mine_laying_speed are the speed of throwing the skill's traps and
// mines in percent, 100 before modifiers; traps_per_throw and mines_per_throw how many one
// throw places, 1 before modifiers. throw_rate is the skill's uses per second when a trap or
// mine delivers it, 0 otherwise. totems_summoned is how many totems a totem skill has out at
// once, 0 for another skill, one more with ancestral_bond, which a second Ancestral Bond does
// not add to; totems_hit_dps is the hit damage per second of all of them.
const (
	statTrapThrowingSpeed = "trap_throwing_speed"
	statMineLayingSpeed   = "mine_laying_speed"
	statTrapsPerThrow     = "traps_per_throw"
	statMinesPerThrow     = "mines_per_throw"
	statThrowRate         = "throw_rate"
	statAncestralBond     = "ancestral_bond"
	statTotemsSummoned    = "totems_summoned"
	statTotemsHitDPS      = "totems_hit_dps"
)

// deliveryStats are the stats of delivery.go the hit mechanic registers before hits per second.
// totems_hit_dps follows hit DPS.
func deliveryStats() []string {
	return []string{
		statTrapThrowingSpeed, statMineLayingSpeed, statTrapsPerThrow, statMinesPerThrow, statThrowRate,
		statAncestralBond, statTotemsSummoned,
	}
}

// Throwing times before speed and what each mine past the first adds to a throw's time, as PoB
// has them (CalcSetup.lua TrapThrowingTime and MineLayingTime, CalcOffence.lua MineThrowCount);
// the game data read here gives none of them.
const (
	trapThrowingSeconds = 0.6
	mineLayingSeconds   = 0.3
	additionalMineTime  = 0.1
)

// Formula names shown in explanations.
const (
	formulaThrowRate      = "uses per second of traps and mines"
	formulaTotemsSummoned = "totems summoned"
	formulaTotemsHitDPS   = "hit damage per second of every totem"
)

// deliveryDef completes def when it is a stat of delivery.go.
//
// A trap is thrown at its throwing speed over 0.6 seconds, a mine at its laying speed over 0.3
// seconds, slowed by a tenth for each mine past the first a throw places; both scale with action
// speed, are capped at one throw per server tick and use the skill once per trap or mine
// placed, and a mine that throws traps once per trap of each. A totem skill counts the totems it
// can have out. These follow PoB (CalcOffence.lua TrapThrowingSpeed, MineLayingSpeed,
// timeOverride, ActiveTotemLimit).
func deliveryDef(def core.StatDef, ids map[string]core.StatID) core.StatDef {
	c := modelContexts()
	switch def.Name {
	case statThrowRate:
		t := throwRate{
			trapSpeed: mustID(ids, statTrapThrowingSpeed), mineSpeed: mustID(ids, statMineLayingSpeed),
			traps: mustID(ids, statTrapsPerThrow), mines: mustID(ids, statMinesPerThrow), action: mustID(ids, statActionSpeed),
			trap: typeMask(c, typeTrapped), mine: typeMask(c, typeRemoteMined),
		}
		def.FormulaName, def.Formula, def.Attribute = formulaThrowRate, t.formula(), t.attribute()
	case statAncestralBond:
		def.Flag = true
	case statTotemsSummoned:
		t := totemsSummoned{bond: mustID(ids, statAncestralBond), totem: typeMask(c, typeSummonsTotem)}
		def.FormulaName, def.Formula, def.Attribute = formulaTotemsSummoned, t.formula(), t.attribute()
	case statTotemsHitDPS:
		dps, totems := mustID(ids, statHitDPS), mustID(ids, statTotemsSummoned)
		def.FormulaName = formulaTotemsHitDPS
		def.Formula = func(r core.Reader, _ core.Agg) float64 { return r.Stat(dps) * r.Stat(totems) }
		def.Attribute = func(a *core.Attributor) core.Attribution {
			total := a.Explanation().Total
			if total == 0 {
				return core.Attribution{}
			}
			parts, _ := a.Of(core.SelfRef(dps))
			return core.Attribution{Parts: rest(a, parts, total, core.SelfRef(totems)), Amount: total}
		}
	}
	return def
}

// throwRate is the uses per second of a skill a trap or mine delivers.
type throwRate struct {
	trapSpeed, mineSpeed, traps, mines, action core.StatID
	// trap and mine are the Trapped and RemoteMined types' context bits.
	trap, mine core.Mask
}

// throw is how a trap or mine delivers a skill: throws per second before action speed and
// the cap, the stats they scale with and that count what one throw places, and the uses per
// throw.
type throw struct {
	perSecond     float64
	speed, placed core.StatID
	uses          float64
}

// of returns how a trap or mine delivers a skill used in context m, with stat reading a stat;
// no throws for a skill neither delivers.
func (t throwRate) of(m core.Mask, stat func(core.StatID) float64) throw {
	switch {
	case m.Intersects(t.mine):
		n := stat(t.mines)
		th := throw{
			perSecond: stat(t.mineSpeed) / percentScale / mineLayingSeconds / (1 + (n-1)*additionalMineTime),
			speed:     t.mineSpeed, placed: t.mines, uses: n,
		}
		if m.Intersects(t.trap) {
			th.uses *= stat(t.traps)
		}
		return th
	case m.Intersects(t.trap):
		return throw{perSecond: stat(t.trapSpeed) / percentScale / trapThrowingSeconds, speed: t.trapSpeed, placed: t.traps, uses: stat(t.traps)}
	}
	return throw{}
}

func (t throwRate) formula() core.Formula {
	return func(r core.Reader, _ core.Agg) float64 {
		th := t.of(r.Mask(), r.Stat)
		if th.perSecond <= 0 {
			return 0
		}
		return min(th.perSecond*r.Stat(t.action)/percentScale, 1/serverTickSeconds) * th.uses
	}
}

// attribute credits the throwing speed's sources with the throws per second before action
// speed, the action speed's with what it and the cap add, and the count per throw's with the rest.
func (t throwRate) attribute() core.Attribute {
	return func(a *core.Attributor) core.Attribution {
		total := a.Explanation().Total
		th := t.of(a.Mask(), a.Stat)
		if total == 0 || th.perSecond <= 0 {
			return core.Attribution{}
		}
		parts := a.Spread(core.SelfRef(th.speed), th.perSecond, core.OpBase)
		parts = rest(a, parts, total/th.uses, core.SelfRef(t.action))
		return core.Attribution{Parts: rest(a, parts, total, core.SelfRef(th.placed)), Amount: total}
	}
}

// totemsSummoned is how many totems a totem skill has out: its own count and one for Ancestral
// Bond.
type totemsSummoned struct {
	bond  core.StatID
	totem core.Mask
}

// bonded returns the totem Ancestral Bond adds, with stat reading a stat.
func (t totemsSummoned) bonded(stat func(core.StatID) float64) float64 {
	if stat(t.bond) != 0 {
		return 1
	}
	return 0
}

func (t totemsSummoned) formula() core.Formula {
	return func(r core.Reader, a core.Agg) float64 {
		if !r.Mask().Intersects(t.totem) {
			return 0
		}
		return a.Default() + t.bonded(r.Stat)
	}
}

// attribute credits the count by the default rule and Ancestral Bond's totem to its sources.
func (t totemsSummoned) attribute() core.Attribute {
	return func(a *core.Attributor) core.Attribution {
		total := a.Explanation().Total
		if total == 0 {
			return core.Attribution{}
		}
		parts := a.Default()
		if b := t.bonded(a.Stat); b != 0 {
			parts = append(parts, a.Spread(core.SelfRef(t.bond), b, core.OpBase)...)
		}
		return core.Attribution{Parts: parts, Amount: total}
	}
}
