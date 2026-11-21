package poe1

import "github.com/xaroth/pbt-engine/internal/core"

// hitRate is a skill's hits per second: an attack's from its attack rate and speed, another
// skill's from its cast time and speed, either times the action speed.
type hitRate struct {
	castTime, castSpeed, attackRate, attackSpeed, action core.StatID
	// attack is the Attack type's context bit.
	attack core.Mask
}

// base returns the skill's uses per second before its speed, and the speed stat that scales
// them, with stat reading a stat.
func (h hitRate) base(isAttack bool, stat func(core.StatID) float64) (float64, core.StatID) {
	if isAttack {
		return stat(h.attackRate), h.attackSpeed
	}
	if t := stat(h.castTime); t > 0 {
		return 1 / t, h.castSpeed
	}
	return 0, h.castSpeed
}

func (h hitRate) formula() core.Formula {
	return func(r core.Reader, _ core.Agg) float64 {
		action := r.Stat(h.action) / percentScale
		if r.Mask().Intersects(h.attack) {
			rate := r.Stat(h.attackRate) * roundHundredths(r.Stat(h.attackSpeed)/percentScale) * action
			return min(max(rate, 0), 1/serverTickSeconds)
		}
		t := r.Stat(h.castTime)
		if t <= 0 {
			return 0
		}
		return min(roundHundredths(r.Stat(h.castSpeed)/percentScale)/t*action, 1/serverTickSeconds)
	}
}

// attribute credits the uses per second before speed to the sources of the attack rate or cast
// time, then the speed's increases and more multipliers over them, then the action speed's over
// those; what rounding and the tick cap change goes to the speed's sources.
func (h hitRate) attribute() core.Attribute {
	return func(a *core.Attributor) core.Attribution {
		total := a.Explanation().Total
		isAttack := a.Mask().Intersects(h.attack)
		base, speed := h.base(isAttack, a.Stat)
		if base <= 0 {
			return core.Attribution{Amount: total}
		}
		from := h.castTime
		if isAttack {
			from = h.attackRate
		}
		parts := a.LayersOver(core.SelfRef(h.action), a.LayersOver(core.SelfRef(speed), a.Spread(core.SelfRef(from), base, core.OpBase)))
		return core.Attribution{Parts: rest(a, parts, total, core.SelfRef(speed)), Amount: total}
	}
}

// hitDPS is a skill's hit damage per second.
type hitDPS struct {
	average, chance, rate core.StatID
}

func (h hitDPS) formula() core.Formula {
	return func(r core.Reader, _ core.Agg) float64 {
		return r.Stat(h.average) * r.Stat(h.chance) / percentScale * r.Stat(h.rate)
	}
}

// attribute credits the average hit's sources with the hits that land in one second's worth of
// one hit per second, and the hits per second's sources with the rest.
func (h hitDPS) attribute() core.Attribute {
	return func(a *core.Attributor) core.Attribution {
		total := a.Explanation().Total
		avgParts, _ := a.Of(core.SelfRef(h.average))
		parts := scaled(avgParts, a.Stat(h.chance)/percentScale)
		return core.Attribution{Parts: rest(a, parts, total, core.SelfRef(h.rate)), Amount: total}
	}
}
