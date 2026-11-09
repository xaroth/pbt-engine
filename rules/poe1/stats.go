package poe1

import "github.com/xaroth/pbt-engine/core"

// attributeFamily is the core CycleFamily of the attributes, which can grant each other.
const attributeFamily = 1

// Resistance limits. Maximum resistances are at most maxResistanceCap; a resistance shown on
// the sheet is at least resistanceFloor. Both follow PoB (Data.lua MaxResistCap and
// ResistFloor); unvalidated.
const (
	maxResistanceCap = 90
	resistanceFloor  = -200
)

// mustID returns the id of name among ids, the stats registered so far. It panics when name is
// not registered yet: id 0 is a real stat, so a missing name would silently refer to it, and
// only a misordered engineStats can cause one.
func mustID(ids map[string]core.StatID, name string) core.StatID {
	id, ok := ids[name]
	if !ok {
		panic("poe1: engine stat " + name + " used before it is registered")
	}
	return id
}

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

// statDef returns the core definition of engine stat name; ids holds the stats registered
// before it. Rounding, minimums and the resistance limits and truncation follow PoB
// (CalcPerform.lua, CalcDefence.lua); the game text states none of them, so all are
// unvalidated.
func statDef(name string, ids map[string]core.StatID) core.StatDef {
	def := core.StatDef{Name: name}
	switch name {
	case statStrength, statDexterity, statIntelligence:
		def.Round, def.HasMin, def.CycleFamily = core.RoundHalf, true, attributeFamily
	case statMaximumLife:
		def.Round, def.HasMin, def.Min = core.RoundHalf, true, 1
	case statMaximumMana:
		def.Round = core.RoundHalf
	case statMaximumEnergyShield:
		def.Round, def.HasMin = core.RoundHalf, true
	case statArmour, statEvasionRating:
		def.Round, def.HasMin = core.RoundHalf, true
		def.Includes = []core.StatID{mustID(ids, statArmourAndEvasionRating)}
	case statNoStrengthBonusToMaximumLife, statNoIntelligenceBonusToMaximumMana,
		statNoDexterityBonusToEvasionRating, statNoIntelligenceBonusToMaximumEnergyShield,
		statChaosResistanceIsDoubled, statElementalMaximumResistancesAreHighest,
		statLifeRegenerationIsNotApplied, statLifeRegenerationAppliesToEnergyShield:
		def.Flag = true
	}
	return regenerationDef(resistanceDef(def, ids), ids)
}

// regeneration is one pool's regeneration stats; any of the flag stats off sets it to 0.
type regeneration struct {
	total, flat, percent, rate, pool string
	off                              []string
}

func regenerations() []regeneration {
	return []regeneration{
		{
			total: statLifeRegeneration, flat: statLifeRegeneratedPerSecond, percent: statPercentOfLifeRegeneratedPerSecond,
			rate: statLifeRegenerationRate, pool: statMaximumLife,
			off: []string{statLifeRegenerationIsNotApplied, statLifeRegenerationAppliesToEnergyShield},
		},
		{
			total: statManaRegeneration, flat: statManaRegeneratedPerSecond, percent: statPercentOfManaRegeneratedPerSecond,
			rate: statManaRegenerationRate, pool: statMaximumMana,
		},
	}
}

const (
	// regenerationPlaces is the decimal places regeneration per second keeps.
	regenerationPlaces = 1
	// percentScale turns a percentage into a fraction.
	percentScale = 100
)

// regenerationDef completes def when it is a pool's total regeneration: the flat amount plus
// the percentage of the pool, then the regeneration rate's increases and more multipliers,
// rounded half up to one decimal place. The order and rounding follow PoB (CalcDefence.lua);
// the increases scaling both parts is the game text's "Regeneration rate", the rest is
// unvalidated.
func regenerationDef(def core.StatDef, ids map[string]core.StatID) core.StatDef {
	for _, rg := range regenerations() {
		if def.Name != rg.total {
			continue
		}
		percent, pool := mustID(ids, rg.percent), mustID(ids, rg.pool)
		off := make([]core.StatID, 0, len(rg.off))
		for _, name := range rg.off {
			off = append(off, mustID(ids, name))
		}
		def.Includes = []core.StatID{mustID(ids, rg.flat), mustID(ids, rg.rate)}
		def.Round, def.Places = core.RoundHalf, regenerationPlaces
		def.FormulaName, def.Attribute = formulaRegeneration, regenerationAttribute(percent, pool, off)
		def.Formula = func(rd core.Reader, a core.Agg) float64 {
			for _, f := range off {
				if rd.Stat(f) != 0 {
					return 0
				}
			}
			base := a.Base + float64(rd.Stat(pool)*rd.Stat(percent))/percentScale
			return base * (1 + a.Inc/percentScale) * a.More
		}
	}
	if def.Name == statEnergyShieldRegeneration {
		def.Round, def.Places = core.RoundHalf, regenerationPlaces
		def.Formula = lifeRegenerationAsEnergyShield(ids)
		def.FormulaName, def.Attribute = formulaLifeRegenerationAsEnergyShield, lifeRegenerationAsEnergyShieldAttribute(ids)
	}
	return def
}

// regenerationAttribute splits a pool's regeneration: flat terms and the pool times the
// percentage, credited to the percentage's sources, then the rate's increases and more. While
// an off flag is set the value is 0 and the flag's sources are listed at 0.
func regenerationAttribute(percent, pool core.StatID, off []core.StatID) core.Attribute {
	return func(a *core.Attributor) core.Attribution {
		for _, f := range off {
			if a.Stat(f) != 0 {
				return core.Attribution{Parts: a.Spread(core.SelfRef(f), 0, core.OpFlag)}
			}
		}
		base := a.BaseTerms()
		if pct := a.Stat(percent); pct != 0 {
			base = append(base, a.Spread(core.SelfRef(percent), float64(a.Stat(pool)*pct)/percentScale, core.OpBase)...)
		}
		return core.Attribution{Parts: a.Layers(base), Amount: a.Explanation().Total}
	}
}

// lifeRegenerationAsEnergyShieldAttribute credits the flat and percent life regeneration
// sources.
func lifeRegenerationAsEnergyShieldAttribute(ids map[string]core.StatID) core.Attribute {
	on := mustID(ids, statLifeRegenerationAppliesToEnergyShield)
	flat, percent := mustID(ids, statLifeRegeneratedPerSecond), mustID(ids, statPercentOfLifeRegeneratedPerSecond)
	pool := mustID(ids, statMaximumEnergyShield)
	return func(a *core.Attributor) core.Attribution {
		at := core.Attribution{Amount: a.Explanation().Total}
		if a.Stat(on) == 0 {
			return at
		}
		if v := a.Stat(flat); v != 0 {
			at.Parts = a.Spread(core.SelfRef(flat), v, core.OpBase)
		}
		if pct := a.Stat(percent); pct != 0 {
			at.Parts = append(at.Parts, a.Spread(core.SelfRef(percent), float64(a.Stat(pool)*pct)/percentScale, core.OpBase)...)
		}
		return at
	}
}

// lifeRegenerationAsEnergyShield is energy shield regeneration while "Life Regeneration is
// applied to Energy Shield instead": the flat life regeneration plus the life regeneration
// percentage of maximum energy shield. Life regeneration rate modifiers do not carry over, as
// in PoB; the text does not say.
func lifeRegenerationAsEnergyShield(ids map[string]core.StatID) core.Formula {
	on := mustID(ids, statLifeRegenerationAppliesToEnergyShield)
	flat, percent := mustID(ids, statLifeRegeneratedPerSecond), mustID(ids, statPercentOfLifeRegeneratedPerSecond)
	pool := mustID(ids, statMaximumEnergyShield)
	return func(rd core.Reader, _ core.Agg) float64 {
		if rd.Stat(on) == 0 {
			return 0
		}
		return rd.Stat(flat) + float64(rd.Stat(pool)*rd.Stat(percent))/percentScale
	}
}

// resistanceDef completes def when it is one of a resistance's stats.
func resistanceDef(def core.StatDef, ids map[string]core.StatID) core.StatDef {
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

// Formula names shown in explanations.
const (
	formulaRegeneration                   = "regeneration"
	formulaLifeRegenerationAsEnergyShield = "life regeneration applied to energy shield"
	formulaChaosResistance                = "chaos resistance"
	formulaEffectiveMaximum               = "effective maximum resistance"
	formulaCappedResistance               = "resistance capped by its maximum"
	formulaTransfers                      = "conversion and gain as extra"
	formulaTransferIncreases              = "increases of a transfer"
	formulaTransferMore                   = "more multipliers of a transfer"
	formulaTransferSource                 = "amount a transfer starts from"
)

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
