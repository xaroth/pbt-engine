package poe1

import (
	"fmt"
	"math"
	"slices"

	"github.com/xaroth/pbt-engine/internal/core"
	"github.com/xaroth/pbt-engine/internal/resolve"
	"github.com/xaroth/pbt-engine/internal/semantics"
)

// Skill types that decide whether a skill reserves.
const (
	typeHasReservation         = "HasReservation"
	typeReservationBecomesCost = "ReservationBecomesCost"
)

// Cost types of a skill's level row that the sheet gives.
const (
	costMana = "Mana"
	costLife = "Life"
)

const (
	// baseEfficiency is a reservation or cost efficiency before modifiers: 100%.
	baseEfficiency = 100
	// permyriadScale turns hundredths of a percent into a fraction.
	permyriadScale = 10000
	// multiplierPlaces is the decimal places the product of supports' cost and reservation
	// multipliers keeps, rounded down.
	multiplierPlaces = 4
)

// reservationPool is one pool's reservation stats: per skill, the base flat amount and
// percentage (with supports' multipliers), the efficiency and the result; on the sheet, the
// totals and what is left.
type reservationPool struct {
	base, basePermyriad, efficiency, flat, permyriad string
	reserved, reservedPermyriad, unreserved, pool    string
}

func reservationPools() (mana, life reservationPool) {
	mana = reservationPool{
		base: statManaReservationBase, basePermyriad: statManaReservationPermyriadBase,
		efficiency: statManaReservationEfficiency, flat: statManaReservation, permyriad: statManaReservationPermyriad,
		reserved: statReservedMana, reservedPermyriad: statReservedManaPermyriad, unreserved: statUnreservedMana,
		pool: statMaximumMana,
	}
	life = reservationPool{
		base: statLifeReservationBase, basePermyriad: statLifeReservationPermyriadBase,
		efficiency: statLifeReservationEfficiency, flat: statLifeReservation, permyriad: statLifeReservationPermyriad,
		reserved: statReservedLife, reservedPermyriad: statReservedLifePermyriad, unreserved: statUnreservedLife,
		pool: statMaximumLife,
	}
	return mana, life
}

// reservationDef completes def when it is a reservation stat. A skill's reservation is its base
// times its increases and more multipliers, divided by its efficiency, rounded half up: flat
// amounts to integers, percentages to hundredths. A skill that reserves life instead of mana
// adds its mana base to its life base. A pool's reservation is the flat amounts plus its
// percentages of the maximum, rounded up; what is left is the maximum minus that, 0 for an
// empty pool. All follow PoB (CalcPerform.lua); the game text gives none of the rounding.
func reservationDef(def core.StatDef, ids map[string]core.StatID) core.StatDef {
	mana, life := reservationPools()
	flags := func() (instead, none core.StatID) {
		return mustID(ids, statSkillReservesLifeInsteadOfMana), mustID(ids, statSkillHasNoReservation)
	}
	for _, p := range []reservationPool{mana, life} {
		isMana := p == mana
		switch def.Name {
		case p.efficiency:
			def.HasMin = true
			def.Includes = []core.StatID{mustID(ids, statReservationEfficiency)}
		case p.flat:
			def.Round, def.HasMin = core.RoundHalf, true
			def.Includes = []core.StatID{mustID(ids, statReservation)}
			instead, none := flags()
			amt := skillAmount{
				isMana: isMana, base: skillBases{own: mustID(ids, p.base), mana: mustID(ids, mana.base)},
				efficiency: mustID(ids, p.efficiency), instead: instead, zero: none, hasZero: true,
			}
			def.Formula, def.FormulaName, def.Attribute = amt.formula(), formulaSkillReservation, amt.attribute()
		case p.permyriad:
			def.Round, def.HasMin = core.RoundHalf, true
			// The percentage takes the flat amount's increases and more multipliers.
			def.Includes = []core.StatID{mustID(ids, statReservation), mustID(ids, p.flat)}
			instead, none := flags()
			amt := skillAmount{
				isMana: isMana, base: skillBases{own: mustID(ids, p.basePermyriad), mana: mustID(ids, mana.basePermyriad)},
				efficiency: mustID(ids, p.efficiency), instead: instead, zero: none, hasZero: true,
			}
			def.Formula, def.FormulaName, def.Attribute = amt.formula(), formulaSkillReservation, amt.attribute()
		case p.reserved:
			pool, pm := mustID(ids, p.pool), mustID(ids, p.reservedPermyriad)
			def.Formula = func(rd core.Reader, a core.Agg) float64 { return a.Base + percentReserved(rd.Stat(pool), rd.Stat(pm)) }
			def.FormulaName, def.Attribute = formulaReserved, reservedAttribute(pool, pm)
		case p.unreserved:
			pool, reserved := mustID(ids, p.pool), mustID(ids, p.reserved)
			def.Formula = func(rd core.Reader, _ core.Agg) float64 {
				if rd.Stat(pool) <= 0 {
					return 0
				}
				return rd.Stat(pool) - rd.Stat(reserved)
			}
			def.FormulaName, def.Attribute = formulaUnreserved, unreservedAttribute(pool, reserved)
		}
	}
	return def
}

// skillBases are the base stats of a skill's cost or reservation of one pool: its own and the
// mana pool's.
type skillBases struct {
	own, mana core.StatID
}

// skillAmount is a skill's cost or reservation of one pool, read in the skill's context: its
// own base, plus the mana base for life while instead is set, times the stat's increases and
// more multipliers, divided by efficiency. The mana pool's is 0 while instead is set, and both
// are while zero is.
type skillAmount struct {
	isMana     bool
	base       skillBases
	efficiency core.StatID
	instead    core.StatID
	zero       core.StatID
	hasZero    bool
	// cost rounds after the increases and after the more multipliers, and adds the stat's flat
	// terms after efficiency.
	cost bool
}

// off returns the flag that makes the amount 0, if one is set.
func (s skillAmount) off(get func(core.StatID) float64) (core.StatID, bool) {
	switch {
	case s.hasZero && get(s.zero) != 0:
		return s.zero, true
	case s.isMana && get(s.instead) != 0:
		return s.instead, true
	}
	return 0, false
}

// moved reports whether the mana base adds to this pool's.
func (s skillAmount) moved(get func(core.StatID) float64) bool {
	return !s.isMana && get(s.instead) != 0
}

// scaled is the amount before efficiency: the bases with the increases and more multipliers.
func (s skillAmount) scaled(get func(core.StatID) float64, a core.Agg) float64 {
	v := get(s.base.own)
	if s.moved(get) {
		v += get(s.base.mana)
	}
	if !s.cost {
		return v * (1 + a.Inc/percentScale) * a.More
	}
	v = roundCost(v*(1+a.Inc/percentScale), a.Inc < 0)
	return roundCost(v*a.More, a.More < 1)
}

func (s skillAmount) value(get func(core.StatID) float64, a core.Agg) float64 {
	if _, ok := s.off(get); ok {
		return 0
	}
	v := s.scaled(get, a)
	// No efficiency is 0 or below in the game; PoB divides by it all the same.
	if eff := get(s.efficiency); eff > 0 {
		v = v * baseEfficiency / eff
	}
	if s.cost {
		v = max(v+a.Base, 0)
	}
	return v
}

func (s skillAmount) formula() core.Formula {
	return func(rd core.Reader, a core.Agg) float64 { return s.value(rd.Stat, a) }
}

// attribute splits the amount: the bases, then the increases and more multipliers over them,
// then what efficiency and rounding change, credited to the efficiency's sources, then flat
// terms. While a flag makes it 0, the flag's sources are listed at 0.
func (s skillAmount) attribute() core.Attribute {
	return func(a *core.Attributor) core.Attribution {
		if f, ok := s.off(a.Stat); ok {
			return core.Attribution{Parts: a.Spread(core.SelfRef(f), 0, core.OpFlag)}
		}
		var base []core.Part
		if v := a.Stat(s.base.own); v != 0 {
			base = append(base, a.Spread(core.SelfRef(s.base.own), v, core.OpBase)...)
		}
		if v := a.Stat(s.base.mana); v != 0 && s.moved(a.Stat) {
			base = append(base, a.Spread(core.SelfRef(s.base.mana), v, core.OpBase)...)
		}
		parts := a.Layers(base)
		e := a.Explanation()
		flat := 0.0
		if s.cost {
			flat = e.Agg.Base
		}
		sum := 0.0
		for _, p := range parts {
			sum += p.Value
		}
		if rest := e.Total - flat - sum; rest != 0 {
			parts = append(parts, a.Spread(core.SelfRef(s.efficiency), rest, core.OpMore)...)
		}
		if s.cost {
			parts = append(parts, a.BaseTerms()...)
		}
		return core.Attribution{Parts: parts, Amount: e.Total}
	}
}

// percentReserved is the part of pool permyriad hundredths of a percent reserve, rounded up.
func percentReserved(pool, permyriad float64) float64 {
	if permyriad == 0 {
		return 0
	}
	return math.Ceil(pool * permyriad / permyriadScale)
}

// reservedAttribute credits the flat terms to their skills and the percentage part to the
// skills reserving it.
func reservedAttribute(pool, permyriad core.StatID) core.Attribute {
	return func(a *core.Attributor) core.Attribution {
		parts := a.BaseTerms()
		if v := percentReserved(a.Stat(pool), a.Stat(permyriad)); v != 0 {
			parts = append(parts, a.Spread(core.SelfRef(permyriad), v, core.OpBase)...)
		}
		return core.Attribution{Parts: parts, Amount: a.Explanation().Total}
	}
}

// unreservedAttribute is the pool's parts less the reservation's.
func unreservedAttribute(pool, reserved core.StatID) core.Attribute {
	return func(a *core.Attributor) core.Attribution {
		if a.Stat(pool) <= 0 {
			return core.Attribution{}
		}
		parts, amount := a.Of(core.SelfRef(pool))
		if r := a.Stat(reserved); r != 0 {
			parts = append(parts, a.Spread(core.SelfRef(reserved), -r, core.OpBase)...)
			amount -= r
		}
		return core.Attribution{Parts: parts, Amount: amount}
	}
}

// Reserves reports whether sk reserves life or mana while active.
func Reserves(sk *resolve.Skill) bool { return !sk.Reservation.IsZero() && reserves(sk.Types) }

func reserves(types []string) bool {
	return slices.Contains(types, typeHasReservation) && !slices.Contains(types, typeReservationBecomesCost)
}

// costMultiplier is the product of the supports' cost and reservation multipliers as roundMore
// rounds it, then rounded down to four decimal places again with PoB's allowance for float
// error (Common.lua floor), as PoB does (CalcOffence.lua, CalcPerform.lua).
func costMultiplier(supports []resolve.Support) float64 {
	m := 1.0
	for _, sp := range supports {
		// 0 is a support without levels.
		if sp.CostMultiplier > 0 {
			m *= 1 + float64(sp.CostMultiplier-percentScale)/percentScale
		}
	}
	scale := math.Pow10(multiplierPlaces)
	return math.Floor(roundMore(moreSupportMultiplier, m)*scale+floorAllowance) / scale
}

// floorAllowance is what PoB adds before rounding down to some decimal places.
const floorAllowance = 0.0001

// addSkillRules adds the base values of sk's costs and reservation in slot, ctx its context,
// and, when it reserves, the global terms that add its reservation to the pools'.
func (sh *Sheet) addSkillRules(sk *resolve.Skill, slot int, ctx core.CtxID) (core.SourceID, error) {
	src := sh.newSource(Source{RuleID: "skill-costs", Rule: "costs and reservation of " + sk.Source.String()})
	pred, err := predicate(sh.session.Model(), semantics.ContextFilter{}, sh.ctx, slot)
	if err != nil {
		return 0, err
	}
	for _, b := range skillRuleBases(sk) {
		if b.value == 0 {
			continue
		}
		if err := sh.addRuleMod(core.Modifier{Stat: sh.stats[b.stat], Op: core.OpBase, Value: b.value, Source: src, Pred: pred}); err != nil {
			return 0, err
		}
	}
	if !Reserves(sk) {
		return src, nil
	}
	mana, life := reservationPools()
	for _, p := range []reservationPool{mana, life} {
		for _, sum := range [][2]string{{p.reserved, p.flat}, {p.reservedPermyriad, p.permyriad}} {
			ref := core.Ref{Actor: 0, Stat: sh.stats[sum[1]], Ctx: int64(ctx)}
			mod := core.Modifier{Stat: sh.stats[sum[0]], Op: core.OpBase, Value: 1, Source: src, Scales: []core.Scale{{Ref: ref, Step: 1}}}
			if err := sh.addRuleMod(mod); err != nil {
				return 0, err
			}
		}
	}
	return src, nil
}

// ruleBase is a base value a rule gives an engine stat.
type ruleBase struct {
	stat  string
	value float64
}

// skillRuleBases are sk's reservation and costs with its supports' multipliers, flat amounts
// rounded down.
func skillRuleBases(sk *resolve.Skill) []ruleBase {
	mult := costMultiplier(sk.Supports)
	r := sk.Reservation
	out := []ruleBase{
		{statManaReservationBase, math.Floor(float64(r.ManaFlat) * mult)},
		{statManaReservationPermyriadBase, float64(r.ManaPercent) * mult},
		{statLifeReservationBase, math.Floor(float64(r.LifeFlat) * mult)},
		{statLifeReservationPermyriadBase, float64(r.LifePercent) * mult},
	}
	for _, c := range sk.Costs {
		switch c.Type {
		case costMana:
			out = append(out, ruleBase{statManaCostBase, math.Floor(float64(c.Amount) * mult)})
		case costLife:
			out = append(out, ruleBase{statLifeCostBase, math.Floor(float64(c.Amount) * mult)})
		}
	}
	return out
}

func (sh *Sheet) addRuleMod(mod core.Modifier) error {
	if _, err := sh.session.AddModifier(mod); err != nil {
		return fmt.Errorf("sheet: skill costs: %w", err)
	}
	return nil
}

// addBaseEfficiencies adds the base reservation and cost efficiencies as a rule's source, so
// their increases and more multipliers apply by the default rule.
func (sh *Sheet) addBaseEfficiencies() error {
	src := sh.newSource(Source{RuleID: "base-efficiency", Rule: "base reservation and cost efficiency (100%)"})
	for _, stat := range []string{statManaReservationEfficiency, statLifeReservationEfficiency, statManaCostEfficiency, statLifeCostEfficiency} {
		mod := core.Modifier{Stat: sh.stats[stat], Op: core.OpBase, Value: baseEfficiency, Source: src}
		if _, err := sh.session.AddModifier(mod); err != nil {
			return fmt.Errorf("sheet: base efficiency: %w", err)
		}
	}
	return nil
}
