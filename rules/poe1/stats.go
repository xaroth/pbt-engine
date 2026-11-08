package poe1

import "github.com/xaroth/pbt-engine/core"

// attributeFamily is the core CycleFamily of the attributes, which can grant each other.
const attributeFamily = 1

// statDef returns the core definition of engine stat name. Rounding is floor(x + 0.5) and the
// minimums follow PoB (CalcPerform.lua: attributes at least 0, life at least 1; CalcDefence.lua:
// energy shield, armour and evasion rating at least 0); the game text states neither, so both
// are unvalidated.
func statDef(name string) core.StatDef {
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
	}
	return def
}
