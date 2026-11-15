package poe1

import "github.com/xaroth/pbt-engine/internal/core"

// Engine stats of resistances. Resistances are uncapped totals; the maximum_ stats are their
// caps and the _capped stats the values the sheet shows. elemental_resistance and the maximum_
// stats for all elemental and all resistances hold modifiers to several resistances at once,
// which each resistance includes. The _to_cold_and_lightning_% stats are the share of fire's own
// modifiers, or maximum fire's, that cold and lightning take. effective_maximum_ elemental
// resistances are the maximums after "capped by your highest Maximum Elemental Resistance".
// chaos_resistance_is_doubled and elemental_maximum_resistances_are_highest are flags.
const (
	statChaosResistanceIsDoubled                         = "chaos_resistance_is_doubled"
	statElementalMaximumResistancesAreHighest            = "elemental_maximum_resistances_are_highest"
	statElementalResistance                              = "elemental_resistance"
	statMaximumElementalResistance                       = "maximum_elemental_resistance"
	statMaximumAllResistances                            = "maximum_all_resistances"
	statFireResistanceModifiersToColdAndLightning        = "fire_resistance_modifiers_to_cold_and_lightning_%"
	statMaximumFireResistanceModifiersToColdAndLightning = "maximum_fire_resistance_modifiers_to_cold_and_lightning_%"
	statFireResistance                                   = "fire_resistance"
	statColdResistance                                   = "cold_resistance"
	statLightningResistance                              = "lightning_resistance"
	statChaosResistance                                  = "chaos_resistance"
	statMaximumFireResistance                            = "maximum_fire_resistance"
	statMaximumColdResistance                            = "maximum_cold_resistance"
	statMaximumLightningResistance                       = "maximum_lightning_resistance"
	statMaximumChaosResistance                           = "maximum_chaos_resistance"
	statEffectiveMaximumFireResistance                   = "effective_maximum_fire_resistance"
	statEffectiveMaximumColdResistance                   = "effective_maximum_cold_resistance"
	statEffectiveMaximumLightningResistance              = "effective_maximum_lightning_resistance"
	statFireResistanceCapped                             = "fire_resistance_capped"
	statColdResistanceCapped                             = "cold_resistance_capped"
	statLightningResistanceCapped                        = "lightning_resistance_capped"
	statChaosResistanceCapped                            = "chaos_resistance_capped"
)

// Resistance limits. Maximum resistances are at most maxResistanceCap; a resistance shown on
// the sheet is at least resistanceFloor. Both follow PoB (Data.lua MaxResistCap and
// ResistFloor); unvalidated.
const (
	maxResistanceCap = 90
	resistanceFloor  = -200
)

// Formula names shown in explanations.
const (
	formulaChaosResistance  = "chaos resistance"
	formulaEffectiveMaximum = "effective maximum resistance"
	formulaCappedResistance = "resistance capped by its maximum"
)

// resistance is one damage type's resistance stats: the uncapped total, its maximum, the
// maximum that caps it and the capped sheet value.
type resistance struct {
	total, maximum, effective, capped string
	elemental                         bool
}

func resistances() []resistance {
	return []resistance{
		{
			total: statFireResistance, maximum: statMaximumFireResistance, effective: statEffectiveMaximumFireResistance,
			capped: statFireResistanceCapped, elemental: true,
		},
		{
			total: statColdResistance, maximum: statMaximumColdResistance, effective: statEffectiveMaximumColdResistance,
			capped: statColdResistanceCapped, elemental: true,
		},
		{
			total: statLightningResistance, maximum: statMaximumLightningResistance, effective: statEffectiveMaximumLightningResistance,
			capped: statLightningResistanceCapped, elemental: true,
		},
		// chaos has no effective maximum: "capped by your highest" is about elemental ones
		{total: statChaosResistance, maximum: statMaximumChaosResistance, effective: statMaximumChaosResistance, capped: statChaosResistanceCapped},
	}
}

func resistanceMechanic() mechanic {
	return mechanic{name: "resistances", stats: []string{
		statChaosResistanceIsDoubled,
		statElementalMaximumResistancesAreHighest,
		statElementalResistance,
		statMaximumElementalResistance,
		statMaximumAllResistances,
		statFireResistanceModifiersToColdAndLightning,
		statMaximumFireResistanceModifiersToColdAndLightning,
		statFireResistance,
		statColdResistance,
		statLightningResistance,
		statChaosResistance,
		statMaximumFireResistance,
		statMaximumColdResistance,
		statMaximumLightningResistance,
		statMaximumChaosResistance,
		statEffectiveMaximumFireResistance,
		statEffectiveMaximumColdResistance,
		statEffectiveMaximumLightningResistance,
		statFireResistanceCapped,
		statColdResistanceCapped,
		statLightningResistanceCapped,
		statChaosResistanceCapped,
	}, def: resistanceDef}
}

// resistanceDef completes def when it is one of a resistance's stats. The limits and truncation
// follow PoB (CalcDefence.lua); the game text states none of them, so all are unvalidated.
func resistanceDef(def core.StatDef, ids map[string]core.StatID) core.StatDef {
	switch def.Name {
	case statChaosResistanceIsDoubled, statElementalMaximumResistancesAreHighest:
		def.Flag = true
		return def
	}
	for _, r := range resistances() {
		switch def.Name {
		case r.total:
			def.Round = core.RoundTrunc
			if r.elemental {
				def.Includes = []core.StatID{mustID(ids, statElementalResistance)}
				def.Shares = fireShare(def.Name, statFireResistance, statFireResistanceModifiersToColdAndLightning, ids)
			} else {
				def.Formula = chaosResistance(mustID(ids, statChaosResistanceIsDoubled))
				def.FormulaName, def.Attribute = formulaChaosResistance, chaosResistanceAttribute(mustID(ids, statChaosResistanceIsDoubled))
			}
		case r.maximum:
			def.Round, def.HasMax, def.Max = core.RoundTrunc, true, maxResistanceCap
			def.Includes = []core.StatID{mustID(ids, statMaximumAllResistances)}
			if r.elemental {
				def.Includes = append(def.Includes, mustID(ids, statMaximumElementalResistance))
				def.Shares = fireShare(def.Name, statMaximumFireResistance, statMaximumFireResistanceModifiersToColdAndLightning, ids)
			}
		case r.effective:
			def.Formula = effectiveMaximum(mustID(ids, r.maximum), ids)
			def.FormulaName, def.Attribute = formulaEffectiveMaximum, effectiveMaximumAttribute(mustID(ids, r.maximum), ids)
		case r.capped:
			total, maximum := mustID(ids, r.total), mustID(ids, r.effective)
			def.HasMin, def.Min = true, resistanceFloor
			def.Formula = func(rd core.Reader, _ core.Agg) float64 { return min(rd.Stat(total), rd.Stat(maximum)) }
			def.FormulaName, def.Attribute = formulaCappedResistance, cappedAttribute(total, maximum)
		}
	}
	return def
}

// fireShare is the share cold and lightning stat name take of fire stat's own modifiers, at
// the percentage rate holds: "Modifiers to Fire Resistance also apply to Cold and Lightning
// Resistances at X% of their Value". Own modifiers only, so all-elemental modifiers, the
// character's base maximum and the Kitava penalty are not shared, as in PoB
// (CalcDefence.lua:529); unvalidated.
func fireShare(name, fire, rate string, ids map[string]core.StatID) []core.Share {
	if name == fire {
		return nil
	}
	return []core.Share{{Stat: mustID(ids, fire), Rate: core.SelfRef(mustID(ids, rate))}}
}

// chaosResistance doubles the total once while doubled is set, as PoB does (a 100% more
// multiplier limited to one); the game text does not say.
func chaosResistance(doubled core.StatID) core.Formula {
	return func(rd core.Reader, a core.Agg) float64 {
		v := a.Default()
		if rd.Stat(doubled) != 0 {
			v *= 2
		}
		return v
	}
}

// effectiveMaximum is an elemental resistance's own maximum, or the highest elemental maximum
// while "capped by your highest Maximum Elemental Resistance" is set.
func effectiveMaximum(own core.StatID, ids map[string]core.StatID) core.Formula {
	highest := mustID(ids, statElementalMaximumResistancesAreHighest)
	maxima := []core.StatID{
		mustID(ids, statMaximumFireResistance), mustID(ids, statMaximumColdResistance), mustID(ids, statMaximumLightningResistance),
	}
	return func(rd core.Reader, _ core.Agg) float64 {
		if rd.Stat(highest) == 0 {
			return rd.Stat(own)
		}
		v := rd.Stat(maxima[0])
		for _, m := range maxima[1:] {
			v = max(v, rd.Stat(m))
		}
		return v
	}
}

// chaosResistanceAttribute is the default rule, plus the doubling as a more part credited to
// the flag's sources.
func chaosResistanceAttribute(doubled core.StatID) core.Attribute {
	return func(a *core.Attributor) core.Attribution {
		parts := a.Default()
		if a.Stat(doubled) != 0 {
			parts = append(parts, a.Spread(core.SelfRef(doubled), a.Explanation().Agg.Default(), core.OpMore)...)
		}
		return core.Attribution{Parts: parts, Amount: a.Explanation().Total}
	}
}

// effectiveMaximumAttribute passes the whole value through the maximum it takes: its own, or
// the highest elemental one (the first on ties, as the formula).
func effectiveMaximumAttribute(own core.StatID, ids map[string]core.StatID) core.Attribute {
	highest := mustID(ids, statElementalMaximumResistancesAreHighest)
	maxima := []core.StatID{
		mustID(ids, statMaximumFireResistance), mustID(ids, statMaximumColdResistance), mustID(ids, statMaximumLightningResistance),
	}
	return func(a *core.Attributor) core.Attribution {
		from := own
		if a.Stat(highest) != 0 {
			from = maxima[0]
			for _, m := range maxima[1:] {
				if a.Stat(m) > a.Stat(from) {
					from = m
				}
			}
		}
		parts, amount := a.Of(core.SelfRef(from))
		return core.Attribution{Parts: parts, Amount: amount}
	}
}

// cappedAttribute explains the uncapped total; when the maximum is lower it caps the value.
func cappedAttribute(total, maximum core.StatID) core.Attribute {
	return func(a *core.Attributor) core.Attribution {
		parts, amount := a.Of(core.SelfRef(total))
		at := core.Attribution{Parts: parts, Amount: amount}
		if a.Stat(maximum) < a.Stat(total) {
			at.CapBy = []core.Node{a.Node(core.SelfRef(maximum))}
		}
		return at
	}
}
