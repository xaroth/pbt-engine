package poe1

import (
	"math"
	"slices"

	"github.com/xaroth/pbt-engine/internal/core"
	"github.com/xaroth/pbt-engine/internal/resolve"
)

// Engine stats of costs, read in a skill's context. mana_cost_base is the skill's level row mana
// cost with its supports' multipliers, mana_cost the cost after its modifiers and
// mana_cost_efficiency; cost and cost_efficiency hold those to every cost. The skill_ flags
// are its "Costs Life instead of Mana", "Cost Energy Shield instead of Mana or Life", which
// leaves it no mana or life cost (the sheet gives no energy shield cost), and that something
// else pays its cost, such as Mirage Warriors that use it. mana_cost_%_paid_with_life is the
// share of its mana cost paid with life instead, and mana_cost_%_gained_as_life_cost the share of
// its mana base it also costs as life.
const (
	statCostEfficiency                  = "cost_efficiency"
	statManaCostEfficiency              = "mana_cost_efficiency"
	statLifeCostEfficiency              = "life_cost_efficiency"
	statSkillCostsLifeInsteadOfMana     = "skill_costs_life_instead_of_mana"
	statSkillCostsEnergyShieldInsteadOf = "skill_costs_energy_shield_instead_of_mana_and_life"
	statSkillHasNoCost                  = "skill_has_no_cost"
	statManaCostPaidWithLife            = "mana_cost_%_paid_with_life"
	statManaCostGainedAsLifeCost        = "mana_cost_%_gained_as_life_cost"
	statManaCostBase                    = "mana_cost_base"
	statLifeCostBase                    = "life_cost_base"
	statCost                            = "cost"
	statManaCost                        = "mana_cost"
	statLifeCost                        = "life_cost"
)

// statTriggeredBySpiritualCry is General's Cry's support stat: Mirage Warriors use the skill.
const statTriggeredBySpiritualCry = "triggered_by_spiritual_cry"

// paysCost reports whether the character pays sk's cost: not while Mirage Warriors summoned by
// General's Cry use it, which PoB gives no cost (CalcMirages.lua). The table sets
// skill_has_no_cost for it too, so its cost read in its context is 0.
func paysCost(sk *resolve.Skill) bool {
	return !slices.ContainsFunc(sk.Supports, func(sp resolve.Support) bool { return statOf(sp.Stats, statTriggeredBySpiritualCry) != 0 })
}

// formulaSkillCost is the formula name shown in explanations.
const formulaSkillCost = "cost of a skill"

func costMechanic() mechanic {
	return mechanic{name: "costs", stats: []string{
		statCostEfficiency,
		statManaCostEfficiency,
		statLifeCostEfficiency,
		statSkillCostsLifeInsteadOfMana,
		statSkillCostsEnergyShieldInsteadOf,
		statSkillHasNoCost,
		statManaCostPaidWithLife,
		statManaCostGainedAsLifeCost,
		statManaCostBase,
		statLifeCostBase,
		statCost,
		statManaCost,
		statLifeCost,
	}, def: costDef}
}

// costDef completes def when it is a cost stat. A skill's cost, read in its context, is its
// base (with supports' multipliers, rounded down), plus for life the mana base while it costs
// life instead of mana, else the shares of the mana base it pays or also costs as life,
// rounded; then its increases, rounded up when they reduce it and down otherwise;
// then its more multipliers, rounded the same way; then divided by its efficiency; then its flat
// terms added, at least 0; the mana cost then keeps what the share paid with life leaves,
// rounded down. The order and rounding follow PoB (CalcOffence.lua), which rounds
// the more multipliers of the pool and of all costs apart; here they are one product. The game
// text gives none of it.
func costDef(def core.StatDef, ids map[string]core.StatID) core.StatDef {
	switch def.Name {
	case statSkillCostsLifeInsteadOfMana, statSkillCostsEnergyShieldInsteadOf, statSkillHasNoCost:
		def.Flag = true
	case statManaCostEfficiency, statLifeCostEfficiency:
		def.HasMin = true
		def.Includes = []core.StatID{mustID(ids, statCostEfficiency)}
	case statManaCost, statLifeCost:
		def.Includes = []core.StatID{mustID(ids, statCost)}
		amt := skillAmount{
			isMana: def.Name == statManaCost, base: skillBases{own: mustID(ids, statLifeCostBase), mana: mustID(ids, statManaCostBase)},
			efficiency: mustID(ids, statLifeCostEfficiency), instead: mustID(ids, statSkillCostsLifeInsteadOfMana), cost: true,
			zeros: []core.StatID{mustID(ids, statSkillHasNoCost), mustID(ids, statSkillCostsEnergyShieldInsteadOf)},
			share: mustID(ids, statManaCostPaidWithLife), gain: mustID(ids, statManaCostGainedAsLifeCost),
		}
		if amt.isMana {
			amt.base.own, amt.efficiency = amt.base.mana, mustID(ids, statManaCostEfficiency)
		}
		def.Formula, def.FormulaName, def.Attribute = amt.formula(), formulaSkillCost, amt.attribute()
	}
	return def
}

// costPlaces is the decimal places a cost shows: efficiency leaves fractions.
const costPlaces = 2

// roundCost rounds a cost up when a modifier reduced it, else down.
func roundCost(v float64, reduced bool) float64 {
	if reduced {
		return math.Ceil(v)
	}
	return math.Floor(v)
}
