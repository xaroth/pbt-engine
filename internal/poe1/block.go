package poe1

import "github.com/xaroth/pbt-engine/internal/core"

// blockChanceCap is the most any maximum block chance can be: PoB's BlockChanceCap (Data.lua);
// unvalidated.
const blockChanceCap = 90

// formulaCappedSpellBlock is the formula name shown in explanations.
const formulaCappedSpellBlock = "spell block chance capped by its maximum"

// spellBlockDef completes def when it is a spell block stat. The total is its base times its
// increases and more multipliers, capped by the maximum; neither is rounded. Both follow PoB
// (CalcDefence.lua, Block); unvalidated.
func spellBlockDef(def core.StatDef, ids map[string]core.StatID) core.StatDef {
	switch def.Name {
	case statMaximumSpellBlockChance:
		def.HasMax, def.Max = true, blockChanceCap
	case statSpellBlockChanceCapped:
		total, maximum := mustID(ids, statSpellBlockChance), mustID(ids, statMaximumSpellBlockChance)
		def.Formula = func(rd core.Reader, _ core.Agg) float64 { return min(rd.Stat(total), rd.Stat(maximum)) }
		def.FormulaName, def.Attribute = formulaCappedSpellBlock, cappedAttribute(total, maximum)
	}
	return def
}
