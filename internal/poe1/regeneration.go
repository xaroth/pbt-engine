package poe1

import "github.com/xaroth/pbt-engine/internal/core"

// Engine stats of regeneration, per second. life_regenerated_per_second is a flat amount
// ("Regenerate 10 Life per second"), percent_of_life_regenerated_per_second a share of maximum
// life ("Regenerate 1% of Life per second"), and life_regeneration_rate holds the increases and
// more multipliers of the "Life Regeneration rate"; life_regeneration is the total the sheet
// shows. The same for mana and energy shield. The life_regeneration_ flags turn life
// regeneration off or apply it to energy shield instead.
const (
	statLifeRegenerationIsNotApplied              = "life_regeneration_is_not_applied"
	statLifeRegenerationAppliesToEnergyShield     = "life_regeneration_applies_to_energy_shield"
	statLifeRegeneratedPerSecond                  = "life_regenerated_per_second"
	statPercentOfLifeRegeneratedPerSecond         = "percent_of_life_regenerated_per_second"
	statLifeRegenerationRate                      = "life_regeneration_rate"
	statManaRegeneratedPerSecond                  = "mana_regenerated_per_second"
	statPercentOfManaRegeneratedPerSecond         = "percent_of_mana_regenerated_per_second"
	statManaRegenerationRate                      = "mana_regeneration_rate"
	statEnergyShieldRegeneratedPerSecond          = "energy_shield_regenerated_per_second"
	statPercentOfEnergyShieldRegeneratedPerSecond = "percent_of_energy_shield_regenerated_per_second"
	statEnergyShieldRegenerationRate              = "energy_shield_regeneration_rate"
	statLifeRegeneration                          = "life_regeneration"
	statManaRegeneration                          = "mana_regeneration"
	statEnergyShieldRegeneration                  = "energy_shield_regeneration"
)

// formulaRegeneration is the formula name shown in explanations.
const formulaRegeneration = "regeneration"

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

// regenerationPlaces is the decimal places regeneration per second keeps.
const regenerationPlaces = 1

// carriedIDs is a carriedRegeneration by stat id.
type carriedIDs struct {
	flag, flat, percent core.StatID
}

func regenerationMechanic() mechanic {
	return mechanic{name: "regeneration", after: []string{mechanicDefences}, stats: []string{
		statLifeRegenerationIsNotApplied,
		statLifeRegenerationAppliesToEnergyShield,
		statLifeRegeneratedPerSecond,
		statPercentOfLifeRegeneratedPerSecond,
		statLifeRegenerationRate,
		statManaRegeneratedPerSecond,
		statPercentOfManaRegeneratedPerSecond,
		statManaRegenerationRate,
		statEnergyShieldRegeneratedPerSecond,
		statPercentOfEnergyShieldRegeneratedPerSecond,
		statEnergyShieldRegenerationRate,
		statLifeRegeneration,
		statManaRegeneration,
		statEnergyShieldRegeneration,
	}, def: regenerationDef}
}

// regenerationDef completes def when it is a pool's total regeneration: the flat amount plus
// the percentage of the pool, then the regeneration rate's increases and more multipliers,
// rounded half up to one decimal place. The order and rounding follow PoB (CalcDefence.lua);
// the increases scaling both parts is the game text's "Regeneration rate", the rest is
// unvalidated.
func regenerationDef(def core.StatDef, ids map[string]core.StatID) core.StatDef {
	switch def.Name {
	case statLifeRegenerationIsNotApplied, statLifeRegenerationAppliesToEnergyShield:
		def.Flag = true
		return def
	}
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
