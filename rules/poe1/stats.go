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

// resistance is one damage type's resistance stats: the uncapped total, its maximum and the
// capped sheet value.
type resistance struct {
	total, maximum, capped string
	elemental              bool
}

func resistances() []resistance {
	return []resistance{
		{total: statFireResistance, maximum: statMaximumFireResistance, capped: statFireResistanceCapped, elemental: true},
		{total: statColdResistance, maximum: statMaximumColdResistance, capped: statColdResistanceCapped, elemental: true},
		{total: statLightningResistance, maximum: statMaximumLightningResistance, capped: statLightningResistanceCapped, elemental: true},
		{total: statChaosResistance, maximum: statMaximumChaosResistance, capped: statChaosResistanceCapped},
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
	case statMaximumEnergyShield, statArmour, statEvasionRating:
		def.Round, def.HasMin = core.RoundHalf, true
	case statNoStrengthBonusToMaximumLife, statNoIntelligenceBonusToMaximumMana,
		statNoDexterityBonusToEvasionRating, statNoIntelligenceBonusToMaximumEnergyShield:
		def.Flag = true
	}
	return regenerationDef(resistanceDef(def, ids), ids)
}

// regeneration is one pool's regeneration stats.
type regeneration struct {
	total, flat, percent, rate, pool string
}

func regenerations() []regeneration {
	return []regeneration{
		{
			total: statLifeRegeneration, flat: statLifeRegeneratedPerSecond, percent: statPercentOfLifeRegeneratedPerSecond,
			rate: statLifeRegenerationRate, pool: statMaximumLife,
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
		def.Includes = []core.StatID{mustID(ids, rg.flat), mustID(ids, rg.rate)}
		def.Round, def.Places = core.RoundHalf, regenerationPlaces
		def.Formula = func(rd core.Reader, a core.Agg) float64 {
			base := a.Base + float64(rd.Stat(pool)*rd.Stat(percent))/percentScale
			return base * (1 + a.Inc/percentScale) * a.More
		}
	}
	return def
}

// resistanceDef completes def when it is one of a resistance's stats.
func resistanceDef(def core.StatDef, ids map[string]core.StatID) core.StatDef {
	for _, r := range resistances() {
		switch def.Name {
		case r.total:
			def.Round = core.RoundTrunc
			if r.elemental {
				def.Includes = []core.StatID{mustID(ids, statElementalResistance)}
			}
		case r.maximum:
			def.Round, def.HasMax, def.Max = core.RoundTrunc, true, maxResistanceCap
			def.Includes = []core.StatID{mustID(ids, statMaximumAllResistances)}
			if r.elemental {
				def.Includes = append(def.Includes, mustID(ids, statMaximumElementalResistance))
			}
		case r.capped:
			total, maximum := mustID(ids, r.total), mustID(ids, r.maximum)
			def.HasMin, def.Min = true, resistanceFloor
			def.Formula = func(rd core.Reader, _ core.Agg) float64 { return min(rd.Stat(total), rd.Stat(maximum)) }
		}
	}
	return def
}
