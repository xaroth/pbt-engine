package poe1

import "github.com/xaroth/pbt-engine/internal/core"

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
		statLifeRegenerationIsNotApplied, statLifeRegenerationAppliesToEnergyShield,
		statMaximumEnduranceChargesAreMaximumFrenzyCharges, statHasMaximumFortification,
		statSkillReservesLifeInsteadOfMana, statSkillHasNoReservation, statSkillCostsLifeInsteadOfMana:
		def.Flag = true
	}
	def = hitDef(speedDef(costDef(reservationDef(def, ids), ids)), ids)
	return spellBlockDef(fortificationDef(chargeDef(regenerationDef(resistanceDef(def, ids), ids), ids), ids), ids)
}

// regeneration is one pool's regeneration stats; any of the flag stats off sets it to 0.
type regeneration struct {
	total, flat, percent, rate, pool string
	off                              []string
	// carried is another pool's regeneration that applies to this pool instead, or nil.
	carried *carriedRegeneration
}

// carriedRegeneration is the flat and percent regeneration of another pool that applies to
// this one while flag is set: "Life Regeneration is applied to Energy Shield instead". The
// percentage is of this pool. Each part carries over only when positive, and the other pool's
// regeneration rate does not; this pool's rate scales both. All follow PoB (CalcDefence.lua,
// Zealot's Oath); the text does not say.
type carriedRegeneration struct {
	flag, flat, percent string
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
		{
			total: statEnergyShieldRegeneration, flat: statEnergyShieldRegeneratedPerSecond,
			percent: statPercentOfEnergyShieldRegeneratedPerSecond, rate: statEnergyShieldRegenerationRate, pool: statMaximumEnergyShield,
			carried: &carriedRegeneration{
				flag: statLifeRegenerationAppliesToEnergyShield, flat: statLifeRegeneratedPerSecond, percent: statPercentOfLifeRegeneratedPerSecond,
			},
		},
	}
}

const (
	// regenerationPlaces is the decimal places regeneration per second keeps.
	regenerationPlaces = 1
	// percentScale turns a percentage into a fraction.
	percentScale = 100
)

// carriedIDs is a carriedRegeneration by stat id.
type carriedIDs struct {
	flag, flat, percent core.StatID
}

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
		var carried *carriedIDs
		if c := rg.carried; c != nil {
			carried = &carriedIDs{flag: mustID(ids, c.flag), flat: mustID(ids, c.flat), percent: mustID(ids, c.percent)}
		}
		def.Includes = []core.StatID{mustID(ids, rg.flat), mustID(ids, rg.rate)}
		def.Round, def.Places = core.RoundHalf, regenerationPlaces
		def.FormulaName, def.Attribute = formulaRegeneration, regenerationAttribute(percent, pool, off, carried)
		def.Formula = func(rd core.Reader, a core.Agg) float64 {
			for _, f := range off {
				if rd.Stat(f) != 0 {
					return 0
				}
			}
			base := a.Base + float64(rd.Stat(pool)*rd.Stat(percent))/percentScale
			if carried != nil && rd.Stat(carried.flag) != 0 {
				base += max(rd.Stat(carried.flat), 0) + float64(rd.Stat(pool)*max(rd.Stat(carried.percent), 0))/percentScale
			}
			return base * (1 + a.Inc/percentScale) * a.More
		}
	}
	return def
}

// regenerationAttribute splits a pool's regeneration: flat terms and the pool times the
// percentage, credited to the percentage's sources, and the carried parts credited to theirs,
// then the rate's increases and more. While an off flag is set the value is 0 and the flag's
// sources are listed at 0.
func regenerationAttribute(percent, pool core.StatID, off []core.StatID, carried *carriedIDs) core.Attribute {
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
		if carried != nil && a.Stat(carried.flag) != 0 {
			if v := a.Stat(carried.flat); v > 0 {
				base = append(base, a.Spread(core.SelfRef(carried.flat), v, core.OpBase)...)
			}
			if pct := a.Stat(carried.percent); pct > 0 {
				base = append(base, a.Spread(core.SelfRef(carried.percent), float64(a.Stat(pool)*pct)/percentScale, core.OpBase)...)
			}
		}
		return core.Attribution{Parts: a.Layers(base), Amount: a.Explanation().Total}
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
	formulaRegeneration      = "regeneration"
	formulaChaosResistance   = "chaos resistance"
	formulaEffectiveMaximum  = "effective maximum resistance"
	formulaCappedResistance  = "resistance capped by its maximum"
	formulaTransfers         = "conversion and gain as extra"
	formulaTransferIncreases = "increases of a transfer"
	formulaTransferMore      = "more multipliers of a transfer"
	formulaTransferSource    = "amount a transfer starts from"
	formulaSkillReservation  = "reservation of a skill"
	formulaReserved          = "reservation of a pool"
	formulaUnreserved        = "pool less its reservation"
	formulaSkillCost         = "cost of a skill"
	formulaHitDamage         = "hit damage of a damage type"
	formulaHitAgainstEnemy   = "hit damage the enemy takes"
	formulaAverageHit        = "average hit"
	formulaHitsPerSecond     = "hits per second"
	formulaRoundedMore       = "more multipliers rounded to hundredths"
	formulaHitDPS            = "hit damage per second"
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
