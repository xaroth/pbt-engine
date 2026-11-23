package poe1

import (
	"github.com/xaroth/pbt-engine/config"
	"github.com/xaroth/pbt-engine/internal/core"
)

// Engine stats of charges. power_charges, frenzy_charges and endurance_charges are the charges
// the character has: its maximum while the config says it uses that kind (uses_power_charges and
// the like), else its minimum, at most its maximum, as PoB counts charges not in use. While it
// uses a kind, its configured_<kind>_charges, when above 0, takes the maximum's place, at
// least the minimum: the count the config gives a minion.
// fortification is the character's Fortification: its maximum while
// has_maximum_fortification is set, else none, as PoB counts it by default (not Fortified);
// fortification_above_20 is the part above 20. The character also has its maximum while the
// config says it is Fortified (is_fortified). inspiration_charges are the Inspiration Charges
// the character has: its maximum_inspiration_charges, as PoB counts them by default; the same
// for blood_charges, Reap's Blood Charges.
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
	statMaximumBloodCharges                            = "maximum_blood_charges"
	statBloodCharges                                   = "blood_charges"
	statUsesPowerCharges                               = "uses_power_charges"
	statUsesFrenzyCharges                              = "uses_frenzy_charges"
	statUsesEnduranceCharges                           = "uses_endurance_charges"
	statConfiguredPowerCharges                         = "configured_power_charges"
	statConfiguredFrenzyCharges                        = "configured_frenzy_charges"
	statConfiguredEnduranceCharges                     = "configured_endurance_charges"
)

// Charge config keys: whether the character has its maximum charges of a kind, as when it keeps
// them up in combat. Each is off by default, so the character has its minimum charges.
const (
	PowerCharges     config.Key[bool] = "power-charges"
	FrenzyCharges    config.Key[bool] = "frenzy-charges"
	EnduranceCharges config.Key[bool] = "endurance-charges"
)

// charge is one kind of charge's stats: the charges the character has, their limits and whether
// the character uses them, with the config key that says so.
type charge struct {
	current, minimum, maximum, uses string
	// configured is the stat of the count the config gives a minion.
	configured string
	key        config.Key[bool]
	// text names the condition in explanations.
	text string
}

func charges() []charge {
	return []charge{
		{
			current: statPowerCharges, minimum: statMinimumPowerCharges, maximum: statMaximumPowerCharges,
			uses: statUsesPowerCharges, configured: statConfiguredPowerCharges, key: PowerCharges, text: "maximum Power Charges",
		},
		{
			current: statFrenzyCharges, minimum: statMinimumFrenzyCharges, maximum: statMaximumFrenzyCharges,
			uses: statUsesFrenzyCharges, configured: statConfiguredFrenzyCharges, key: FrenzyCharges, text: "maximum Frenzy Charges",
		},
		{
			current: statEnduranceCharges, minimum: statMinimumEnduranceCharges, maximum: statMaximumEnduranceCharges,
			uses: statUsesEnduranceCharges, configured: statConfiguredEnduranceCharges, key: EnduranceCharges, text: "maximum Endurance Charges",
		},
	}
}

// fortificationThreshold is the 20 of "per Fortification above 20", from the stats' text.
const fortificationThreshold = 20

// Formula names shown in explanations.
const (
	formulaCharges                     = "maximum or configured charges while used, else minimum charges capped by the maximum"
	formulaMaximumEnduranceCharges     = "maximum endurance charges"
	formulaDefaultFortification        = "maximum fortification while fortified"
	formulaFortificationAboveThreshold = "fortification above 20"
	formulaDefaultInspirationCharges   = "maximum inspiration charges"
	formulaDefaultBloodCharges         = "maximum blood charges"
)

// chargeDef completes def when it is one of a charge's stats. A charge's limits are at least
// 0. The character has its maximum charges while it uses them, else its minimum, at most the
// maximum, as PoB counts them (CalcPerform.lua doActorCharges).
func chargeDef(def core.StatDef, ids map[string]core.StatID) core.StatDef {
	if def.Name == statMaximumEnduranceChargesAreMaximumFrenzyCharges {
		def.Flag = true
		return def
	}
	for _, c := range charges() {
		switch def.Name {
		case c.minimum, c.maximum, c.configured:
			def.HasMin = true
		case c.current:
			def.HasMin = true
			def.Formula, def.Attribute = currentCharges(c, ids)
			def.FormulaName = formulaCharges
		}
	}
	switch def.Name {
	case statMaximumEnduranceCharges:
		def.Formula, def.Attribute = maximumEnduranceCharges(ids)
		def.FormulaName = formulaMaximumEnduranceCharges
	case statMaximumInspirationCharges, statMaximumBloodCharges:
		def.HasMin = true
	case statInspirationCharges:
		// PoB gives the player its maximum Inspiration Charges and Blood Charges unless the
		// config says otherwise (CalcPerform.lua doActorCharges).
		def.Formula, def.Attribute = maximumCharges(mustID(ids, statMaximumInspirationCharges))
		def.FormulaName = formulaDefaultInspirationCharges
	case statBloodCharges:
		def.Formula, def.Attribute = maximumCharges(mustID(ids, statMaximumBloodCharges))
		def.FormulaName = formulaDefaultBloodCharges
	}
	return def
}

// maximumCharges is the character's maximum charges of a kind, maximum.
func maximumCharges(maximum core.StatID) (core.Formula, core.Attribute) {
	formula := func(rd core.Reader, _ core.Agg) float64 { return rd.Stat(maximum) }
	attribute := func(a *core.Attributor) core.Attribution {
		parts, amount := a.Of(core.SelfRef(maximum))
		return core.Attribution{Parts: parts, Amount: amount}
	}
	return formula, attribute
}

// currentCharges is the charges of c the character has. A configured count is not capped by
// the maximum, as PoB does not cap it (CalcPerform.lua doActorCharges, FrenzyCharges override).
func currentCharges(c charge, ids map[string]core.StatID) (core.Formula, core.Attribute) {
	uses, minimum, maximum := mustID(ids, c.uses), mustID(ids, c.minimum), mustID(ids, c.maximum)
	configured := mustID(ids, c.configured)
	// used is the stat the charges in use come from, and their number.
	used := func(stat func(core.StatID) float64) (core.StatID, float64) {
		if stat(configured) > 0 {
			if floor := min(stat(minimum), stat(maximum)); floor > stat(configured) {
				return minimum, floor
			}
			return configured, stat(configured)
		}
		return maximum, stat(maximum)
	}
	formula := func(rd core.Reader, _ core.Agg) float64 {
		if rd.Stat(uses) != 0 {
			_, n := used(rd.Stat)
			return n
		}
		return min(rd.Stat(minimum), rd.Stat(maximum))
	}
	capped := cappedAttribute(minimum, maximum)
	attribute := func(a *core.Attributor) core.Attribution {
		if a.Stat(uses) != 0 {
			from, _ := used(a.Stat)
			parts, amount := a.Of(core.SelfRef(from))
			return core.Attribution{Parts: parts, Amount: amount}
		}
		return capped(a)
	}
	return formula, attribute
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
			statMaximumBloodCharges,
			statBloodCharges,
			statConfiguredPowerCharges,
			statConfiguredFrenzyCharges,
			statConfiguredEnduranceCharges,
		},
		def: func(def core.StatDef, ids map[string]core.StatID) core.StatDef {
			return fortificationDef(chargeDef(def, ids), ids)
		},
	}
}
