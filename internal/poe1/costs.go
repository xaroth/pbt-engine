package poe1

import (
	"math"

	"github.com/xaroth/pbt-engine/internal/core"
)

// costDef completes def when it is a cost stat. A skill's cost, read in its context, is its
// base (with supports' multipliers, rounded down), plus the mana base for life while it costs
// life instead of mana; then its increases, rounded up when they reduce it and down otherwise;
// then its more multipliers, rounded the same way; then divided by its efficiency; then its flat
// terms added, at least 0. The order and rounding follow PoB (CalcOffence.lua), which rounds
// the more multipliers of the pool and of all costs apart; here they are one product. The game
// text gives none of it.
func costDef(def core.StatDef, ids map[string]core.StatID) core.StatDef {
	switch def.Name {
	case statManaCostEfficiency, statLifeCostEfficiency:
		def.HasMin = true
		def.Includes = []core.StatID{mustID(ids, statCostEfficiency)}
	case statManaCost, statLifeCost:
		def.Includes = []core.StatID{mustID(ids, statCost)}
		amt := skillAmount{
			isMana: def.Name == statManaCost, base: skillBases{own: mustID(ids, statLifeCostBase), mana: mustID(ids, statManaCostBase)},
			efficiency: mustID(ids, statLifeCostEfficiency), instead: mustID(ids, statSkillCostsLifeInsteadOfMana), cost: true,
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
