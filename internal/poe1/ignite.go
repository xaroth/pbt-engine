package poe1

import (
	"github.com/xaroth/pbt-engine/internal/core"
	"github.com/xaroth/pbt-engine/output"
)

// statIgniteDPS, read in a hit's context, is the damage per second the enemy takes from the
// skill's ignite, by PoB's expected value (strongestDPS); the game shows no such number.
const statIgniteDPS = "ignite_dps"

// igniteKind is ignite: fire damage over time of 90% of the fire damage of the hit that
// inflicted it per second, for 4 seconds, as the game text says ("Ignited enemies take 90% of
// the Fire Damage of the Hit that Ignited them as Fire Damage over Time, over 4 seconds"). A
// critical strike always ignites.
func igniteKind() ailmentKind {
	const share, duration = 0.9, 4
	return ailmentKind{ailment: ailmentIgnite, damage: Fire, share: share, duration: duration, onCrit: true}
}

// mechanicIgnite names the ignite mechanic for those that come after it.
const mechanicIgnite = "ignite"

// formulaIgniteDPS is the formula name shown in explanations.
const formulaIgniteDPS = "ignite damage per second"

func igniteMechanic() mechanic {
	return mechanic{
		name: mechanicIgnite, after: []string{mechanicAilment, mechanicHit, mechanicAccuracy},
		stats: []string{statIgniteDPS}, def: igniteDef,
	}
}

func igniteDef(def core.StatDef, ids map[string]core.StatID) core.StatDef {
	if def.Name == statIgniteDPS {
		d := newStrongestDPS(igniteKind(), ids)
		def.FormulaName, def.Formula, def.Attribute = formulaIgniteDPS, d.formula(), d.attribute()
	}
	return def
}

// igniteOutputs are the main skill's ignite outputs.
func igniteOutputs() []Output {
	return []Output{
		{Key: IgniteDPS, Label: "Ignite DPS", Better: output.Higher, Places: hitPlaces, stat: statIgniteDPS, needs: needsAilment},
	}
}
