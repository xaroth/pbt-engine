package poe1

import "github.com/xaroth/pbt-engine/internal/core"

// Engine stats of charges. power_charges, frenzy_charges and endurance_charges are the charges
// the character has: its minimum, at most its maximum, as PoB counts them by default (charges
// not in use). fortification is the character's Fortification: its maximum while
// has_maximum_fortification is set, else none, as PoB counts it by default (not Fortified);
// fortification_above_20 is the part above 20. The character also has its maximum while the
// config says it is Fortified (is_fortified). inspiration_charges are the Inspiration Charges
// the character has: its maximum_inspiration_charges, as PoB counts them by default.
const (
	statMaximumEnduranceChargesAreMaximumFrenzyCharges = "maximum_endurance_charges_are_maximum_frenzy_charges"
	statHasMaximumFortification                        = "has_maximum_fortification"
	statMaximumPowerCharges                            = "maximum_power_charges"
	statMaximumFrenzyCharges                           = "maximum_frenzy_charges"
	statMaximumEnduranceCharges                        = "maximum_endurance_charges"
	statMinimumPowerCharges                            = "minimum_power_charges"
	statMinimumFrenzyCharges                           = "minimum_frenzy_charges"
	statMinimumEnduranceCharges                        = "minimum_endurance_charges"
	statPowerCharges                                   = "power_charges"
	statFrenzyCharges                                  = "frenzy_charges"
	statEnduranceCharges                               = "endurance_charges"
	statMaximumFortification                           = "maximum_fortification"
	statFortification                                  = "fortification"
	statFortificationAbove20                           = "fortification_above_20"
	statMaximumInspirationCharges                      = "maximum_inspiration_charges"
	statInspirationCharges                             = "inspiration_charges"
)

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
	formulaDefaultFortification        = "maximum fortification while fortified"
	formulaFortificationAboveThreshold = "fortification above 20"
	formulaDefaultInspirationCharges   = "maximum inspiration charges"
)

// chargeDef completes def when it is one of a charge's stats. A charge's limits are at least
// 0, and the character has its minimum charges, at most the maximum: PoB's default, which
// counts no charges gained in combat (CalcPerform.lua doActorCharges).
func chargeDef(def core.StatDef, ids map[string]core.StatID) core.StatDef {
	if def.Name == statMaximumEnduranceChargesAreMaximumFrenzyCharges {
		def.Flag = true
		return def
	}
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
	switch def.Name {
	case statMaximumEnduranceCharges:
		def.Formula, def.Attribute = maximumEnduranceCharges(ids)
		def.FormulaName = formulaMaximumEnduranceCharges
	case statMaximumInspirationCharges:
		def.HasMin = true
	case statInspirationCharges:
		// PoB gives the player its maximum Inspiration Charges unless the config says otherwise
		// (CalcPerform.lua doActorCharges).
		maximum := mustID(ids, statMaximumInspirationCharges)
		def.Formula = func(rd core.Reader, _ core.Agg) float64 { return rd.Stat(maximum) }
		def.FormulaName = formulaDefaultInspirationCharges
		def.Attribute = func(a *core.Attributor) core.Attribution {
			parts, amount := a.Of(core.SelfRef(maximum))
			return core.Attribution{Parts: parts, Amount: amount}
		}
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
// maximum Fortification while "You have your maximum Fortification" is set or the config says
// it is Fortified, else none: PoB's default, which leaves Fortify gained in combat out
// (CalcPerform.lua, Fortify).
func fortificationDef(def core.StatDef, ids map[string]core.StatID) core.StatDef {
	switch def.Name {
	case statHasMaximumFortification:
		def.Flag = true
	case statMaximumFortification:
		def.HasMin = true
	case statFortification:
		has, fortified := mustID(ids, statHasMaximumFortification), mustID(ids, statIsFortified)
		maximum := mustID(ids, statMaximumFortification)
		def.Formula = func(rd core.Reader, _ core.Agg) float64 {
			if rd.Stat(has) == 0 && rd.Stat(fortified) == 0 {
				return 0
			}
			return rd.Stat(maximum)
		}
		def.FormulaName = formulaDefaultFortification
		def.Attribute = func(a *core.Attributor) core.Attribution {
			if a.Stat(has) == 0 && a.Stat(fortified) == 0 {
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

func chargeMechanic() mechanic {
	return mechanic{
		name: "charges", after: []string{mechanicConditions}, stats: []string{
			statMaximumEnduranceChargesAreMaximumFrenzyCharges,
			statHasMaximumFortification,
			statMaximumPowerCharges,
			statMaximumFrenzyCharges,
			statMaximumEnduranceCharges,
			statMinimumPowerCharges,
			statMinimumFrenzyCharges,
			statMinimumEnduranceCharges,
			statPowerCharges,
			statFrenzyCharges,
			statEnduranceCharges,
			statMaximumFortification,
			statFortification,
			statFortificationAbove20,
			statMaximumInspirationCharges,
			statInspirationCharges,
		},
		def: func(def core.StatDef, ids map[string]core.StatID) core.StatDef {
			return fortificationDef(chargeDef(def, ids), ids)
		},
	}
}
