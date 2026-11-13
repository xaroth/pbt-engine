package poe1

import "github.com/xaroth/pbt-engine/internal/core"

// charge is one kind of charge's stats: the charges the character has and their limits.
type charge struct {
	current, minimum, maximum string
}

func charges() []charge {
	return []charge{
		{current: statPowerCharges, minimum: statMinimumPowerCharges, maximum: statMaximumPowerCharges},
		{current: statFrenzyCharges, minimum: statMinimumFrenzyCharges, maximum: statMaximumFrenzyCharges},
		{current: statEnduranceCharges, minimum: statMinimumEnduranceCharges, maximum: statMaximumEnduranceCharges},
	}
}

// fortificationThreshold is the 20 of "per Fortification above 20", from the stats' text.
const fortificationThreshold = 20

// Formula names shown in explanations.
const (
	formulaDefaultCharges              = "minimum charges, capped by the maximum"
	formulaMaximumEnduranceCharges     = "maximum endurance charges"
	formulaDefaultFortification        = "maximum fortification while you have it"
	formulaFortificationAboveThreshold = "fortification above 20"
)

// chargeDef completes def when it is one of a charge's stats. A charge's limits are at least
// 0, and the character has its minimum charges, at most the maximum: PoB's default, which
// counts no charges gained in combat (CalcPerform.lua doActorCharges).
func chargeDef(def core.StatDef, ids map[string]core.StatID) core.StatDef {
	for _, c := range charges() {
		switch def.Name {
		case c.minimum, c.maximum:
			def.HasMin = true
		case c.current:
			minimum, maximum := mustID(ids, c.minimum), mustID(ids, c.maximum)
			def.HasMin = true
			def.Formula = func(rd core.Reader, _ core.Agg) float64 { return min(rd.Stat(minimum), rd.Stat(maximum)) }
			def.FormulaName, def.Attribute = formulaDefaultCharges, cappedAttribute(minimum, maximum)
		}
	}
	if def.Name == statMaximumEnduranceCharges {
		def.Formula, def.Attribute = maximumEnduranceCharges(ids)
		def.FormulaName = formulaMaximumEnduranceCharges
	}
	return def
}

// maximumEnduranceCharges is the maximum frenzy charges while "Your Maximum Endurance Charges
// is equal to your Maximum Frenzy Charges" is set, else the default rule.
func maximumEnduranceCharges(ids map[string]core.StatID) (core.Formula, core.Attribute) {
	equal, frenzy := mustID(ids, statMaximumEnduranceChargesAreMaximumFrenzyCharges), mustID(ids, statMaximumFrenzyCharges)
	formula := func(rd core.Reader, a core.Agg) float64 {
		if rd.Stat(equal) != 0 {
			return rd.Stat(frenzy)
		}
		return a.Default()
	}
	attribute := func(a *core.Attributor) core.Attribution {
		if a.Stat(equal) != 0 {
			parts, amount := a.Of(core.SelfRef(frenzy))
			return core.Attribution{Parts: parts, Amount: amount}
		}
		return core.Attribution{Parts: a.Default(), Amount: a.Explanation().Total}
	}
	return formula, attribute
}

// fortificationDef completes def when it is a Fortification stat. The character has its
// maximum Fortification while "You have your maximum Fortification" is set, else none: PoB's
// default, which leaves Fortify gained in combat out (CalcPerform.lua, Fortify).
func fortificationDef(def core.StatDef, ids map[string]core.StatID) core.StatDef {
	switch def.Name {
	case statMaximumFortification:
		def.HasMin = true
	case statFortification:
		has, maximum := mustID(ids, statHasMaximumFortification), mustID(ids, statMaximumFortification)
		def.Formula = func(rd core.Reader, _ core.Agg) float64 {
			if rd.Stat(has) == 0 {
				return 0
			}
			return rd.Stat(maximum)
		}
		def.FormulaName = formulaDefaultFortification
		def.Attribute = func(a *core.Attributor) core.Attribution {
			if a.Stat(has) == 0 {
				return core.Attribution{}
			}
			parts, amount := a.Of(core.SelfRef(maximum))
			return core.Attribution{Parts: parts, Amount: amount}
		}
	case statFortificationAbove20:
		fortification := mustID(ids, statFortification)
		def.Formula = func(rd core.Reader, _ core.Agg) float64 {
			return max(rd.Stat(fortification)-fortificationThreshold, 0)
		}
		def.FormulaName = formulaFortificationAboveThreshold
		def.Attribute = func(a *core.Attributor) core.Attribution {
			v := max(a.Stat(fortification)-fortificationThreshold, 0)
			if v == 0 {
				return core.Attribution{}
			}
			return core.Attribution{Parts: a.Spread(core.SelfRef(fortification), v, core.OpBase), Amount: v}
		}
	}
	return def
}
