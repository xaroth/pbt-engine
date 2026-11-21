package poe1

import (
	"math"
	"slices"

	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/internal/core"
	"github.com/xaroth/pbt-engine/internal/resolve"
)

// Engine stats of a skill a totem, trap or mine uses, read in its context, beside its hit rate
// (hitRate).
//
// trap_throwing_speed and mine_laying_speed are the speed of throwing the skill's traps and
// mines in percent, 100 before modifiers; traps_per_throw and mines_per_throw how many one
// throw places, 1 before modifiers. throw_rate is the skill's uses per second when a trap or
// mine delivers it, 0 otherwise. totems_summoned is how many totems a totem skill has out at
// once, 0 for another skill, one more with ancestral_bond, which a second Ancestral Bond does
// not add to; totems_hit_dps is the hit damage per second of all of them. totem_life is the
// life of one of them.
const (
	statTrapThrowingSpeed = "trap_throwing_speed"
	statMineLayingSpeed   = "mine_laying_speed"
	statTrapsPerThrow     = "traps_per_throw"
	statMinesPerThrow     = "mines_per_throw"
	statThrowRate         = "throw_rate"
	statAncestralBond     = "ancestral_bond"
	statTotemsSummoned    = "totems_summoned"
	statTotemsHitDPS      = "totems_hit_dps"
	statTotemLife         = "totem_life"
)

// deliveryStats are the stats of delivery.go the hit mechanic registers before hits per second.
// totems_hit_dps follows hit DPS.
func deliveryStats() []string {
	return []string{
		statTrapThrowingSpeed, statMineLayingSpeed, statTrapsPerThrow, statMinesPerThrow, statThrowRate,
		statAncestralBond, statTotemsSummoned, statTotemLife,
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
	formulaTotemLife      = "totem life"
)

// deliveryDef completes def when it is a stat of delivery.go.
//
// A trap is thrown at its throwing speed over 0.6 seconds, a mine at its laying speed over 0.3
// seconds, slowed by a tenth for each mine past the first a throw places; both scale with action
// speed, are capped at one throw per server tick and use the skill once per trap or mine
// placed, and a mine that throws traps once per trap of each. A totem skill counts the totems it
// can have out. A totem's life is its base life (TotemLives) times its increases and more
// multipliers, rounded half up. These follow PoB (CalcOffence.lua TrapThrowingSpeed,
// MineLayingSpeed, timeOverride, ActiveTotemLimit, calcTotemLife).
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
	case statTotemLife:
		def.FormulaName = formulaTotemLife
		def.Formula = func(_ core.Reader, a core.Agg) float64 {
			return roundHalf(a.Base * (1 + a.Inc/percentScale) * a.More)
		}
		def.Attribute = func(a *core.Attributor) core.Attribution {
			total := a.Explanation().Total
			parts := a.Default()
			if s := sum(parts); total != 0 && s != 0 {
				return core.Attribution{Parts: scaled(parts, total/s), Amount: total}
			}
			return core.Attribution{Amount: total}
		}
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

// TotemLives gives the base life of totems: an allied monster's life at the totem's level
// (totemLevel), times the life multiplier of the totem's first variety, rounded down, as PoB
// has it (CalcOffence.lua calcTotemLife, Misc.lua totemLifeMult). A totem without varieties,
// such as one a support makes, has a multiplier of 1, as PoB's totems by gem colour do.
type TotemLives struct {
	// allyLife is by level; multiplier in percent by SkillTotemId.
	allyLife   map[int]int32
	multiplier map[int32]int32
}

// TotemLivesOf returns the base life of the totems of the monsters group m.
func TotemLivesOf(m *dataset.Monsters) TotemLives {
	t := TotemLives{allyLife: make(map[int]int32, len(m.Levels)), multiplier: make(map[int32]int32, len(m.Totems))}
	for _, l := range m.Levels {
		t.allyLife[int(l.Level)] = l.AllyLife
	}
	varieties := make(map[string]int32, len(m.Varieties))
	for _, v := range m.Varieties {
		varieties[v.ID] = v.LifeMultiplier
	}
	for _, tt := range m.Totems {
		if len(tt.Skins) > 0 {
			if mult, ok := varieties[tt.Skins[0].Variety]; ok {
				t.multiplier[tt.Key] = mult
			}
		}
	}
	return t
}

// Stats that set a totem's level: a totem support's and a totem skill's own.
const (
	statTotemSupportGemLevel      = "totem_support_gem_level"
	statBaseActiveSkillTotemLevel = "base_active_skill_totem_level"
)

// totemLevel returns the level of sk's totem: the level its totem support or, without one, the
// skill itself sets, else the level its level row requires, as PoB has it (CalcActiveSkill.lua
// totemLevel, overridden by the stats' skill data).
func totemLevel(sk *resolve.Skill) int {
	for _, sp := range sk.Supports {
		if i := slices.IndexFunc(sp.Stats, func(sv resolve.StatValue) bool { return sv.Stat == statTotemSupportGemLevel }); i >= 0 {
			return int(sp.Stats[i].Value)
		}
	}
	stats := sk.Set().Stats
	if i := slices.IndexFunc(stats, func(sv resolve.StatValue) bool { return sv.Stat == statBaseActiveSkillTotemLevel }); i >= 0 {
		return int(stats[i].Value)
	}
	return sk.LevelRequirement
}

// base returns the base life of sk's totem, 0 when the levels do not hold its totem's level.
func (t TotemLives) base(sk *resolve.Skill) float64 {
	life, ok := t.allyLife[totemLevel(sk)]
	if !ok {
		return 0
	}
	mult, ok := t.multiplier[sk.Totem]
	if !ok {
		mult = percentScale
	}
	return math.Floor(float64(life) * float64(mult) / percentScale)
}

// SetTotemLives sets the base life of the totems of skills set after it.
func (sh *Sheet) SetTotemLives(t TotemLives) { sh.totems = t }

// totemBases is the base life of sk's totem, for a skill a totem uses.
func (sh *Sheet) totemBases(sk *resolve.Skill) []hitBase {
	if !hasTypes(sk, typeSummonsTotem) {
		return nil
	}
	return []hitBase{{statTotemLife, core.OpBase, sh.totems.base(sk)}}
}
