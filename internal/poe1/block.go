package poe1

import (
	"fmt"
	"math"

	"github.com/xaroth/pbt-engine/internal/core"
	"github.com/xaroth/pbt-engine/internal/resolve"
)

// Engine stats of block. shield_block_chance is the held shield's own chance to block,
// attack_block_chance is the uncapped chance to block attack damage,
// maximum_attack_block_chance its cap, attack_block_chance_capped the value the sheet shows and
// attack_block_chance_overcapped what the uncapped chance has above the cap; the spell_ stats are
// the same for spell damage.
const (
	statShieldBlockChance           = "shield_block_chance"
	statAttackBlockChance           = "attack_block_chance"
	statMaximumAttackBlockChance    = "maximum_attack_block_chance"
	statAttackBlockChanceCapped     = "attack_block_chance_capped"
	statAttackBlockChanceOvercapped = "attack_block_chance_overcapped"
	statSpellBlockChance            = "spell_block_chance"
	statMaximumSpellBlockChance     = "maximum_spell_block_chance"
	statSpellBlockChanceCapped      = "spell_block_chance_capped"
)

// blockChanceCap is the most any maximum block chance can be: PoB's BlockChanceCap (Data.lua);
// unvalidated.
const blockChanceCap = 90

// Formula names shown in explanations.
const (
	formulaCappedBlock     = "block chance capped by its maximum"
	formulaOvercappedBlock = "block chance above its maximum"
)

// blockDef completes def when it is a block stat. A total is its base times its increases and
// more multipliers, rounded half up, then capped by its maximum; the part above the maximum is
// whole too. All follow PoB (CalcDefence.lua, Block); unvalidated.
func blockDef(def core.StatDef, ids map[string]core.StatID) core.StatDef {
	switch def.Name {
	case statAttackBlockChance, statSpellBlockChance:
		def.Round = core.RoundHalf
	case statMaximumAttackBlockChance, statMaximumSpellBlockChance:
		def.HasMax, def.Max = true, blockChanceCap
	case statAttackBlockChanceCapped:
		return cappedBlockDef(def, mustID(ids, statAttackBlockChance), mustID(ids, statMaximumAttackBlockChance))
	case statSpellBlockChanceCapped:
		return cappedBlockDef(def, mustID(ids, statSpellBlockChance), mustID(ids, statMaximumSpellBlockChance))
	case statAttackBlockChanceOvercapped:
		total, maximum := mustID(ids, statAttackBlockChance), mustID(ids, statMaximumAttackBlockChance)
		def.Formula = func(rd core.Reader, _ core.Agg) float64 { return max(rd.Stat(total)-rd.Stat(maximum), 0) }
		def.FormulaName = formulaOvercappedBlock
	}
	return def
}

func cappedBlockDef(def core.StatDef, total, maximum core.StatID) core.StatDef {
	def.Formula = func(rd core.Reader, _ core.Agg) float64 { return min(rd.Stat(total), rd.Stat(maximum)) }
	def.FormulaName, def.Attribute = formulaCappedBlock, cappedAttribute(total, maximum)
	return def
}

// blockMechanic lists attack block first: spell block reads it.
func blockMechanic() mechanic {
	return mechanic{name: "block", stats: []string{
		statShieldBlockChance,
		statAttackBlockChance,
		statMaximumAttackBlockChance,
		statAttackBlockChanceCapped,
		statAttackBlockChanceOvercapped,
		statSpellBlockChance,
		statMaximumSpellBlockChance,
		statSpellBlockChanceCapped,
	}, def: blockDef}
}

// addShieldBlock adds the held shield's own chance to block to the chance to block attack
// damage, as a rule's source, so what replaces the shield's value (shield_block_chance's
// overrides) replaces it there too.
func (sh *Sheet) addShieldBlock() error {
	src := sh.newSource(Source{RuleID: "shield-block", Rule: "chance to block of the held shield"})
	mod := core.Modifier{
		Stat: sh.stats[statAttackBlockChance], Op: core.OpBase, Value: 1, Source: src,
		Scales: []core.Scale{{Ref: core.SelfRef(sh.stats[statShieldBlockChance]), Step: 1}},
	}
	if _, err := sh.session.AddModifier(mod); err != nil {
		return fmt.Errorf("sheet: shield block: %w", err)
	}
	return nil
}

// Stats of a shield's own chance to block. statShieldBlock carries it to the character.
const (
	statShieldBlock      = "base_block_chance_%_from_shield"
	statLocalBlockFlat   = "local_additional_block_chance_%"
	statLocalBlockInc    = "local_block_chance_+%"
	statLocalBlockRemove = "local_no_block_chance"
)

// localBlockStats are the stats that change a shield's own chance to block.
func localBlockStats() map[string]bool {
	return map[string]bool{statLocalBlockFlat: true, statLocalBlockInc: true, statLocalBlockRemove: true}
}

// shieldBlockStats returns a shield's own chance to block (shieldBlock) as statShieldBlock, none
// when it is 0.
func shieldBlockStats(base int32, sums map[string]float64) []resolve.StatValue {
	if b := shieldBlock(base, sums); b != 0 {
		return []resolve.StatValue{{Stat: statShieldBlock, Value: b}}
	}
	return nil
}

// shieldBlock returns a shield's chance to block from its base's and its local stat sums: the
// base's plus the flat, increased, rounded down, as PoB has it (Item.lua,
// BuildModListForSlotNum; PoB 2.65 added the flat after the increases).
func shieldBlock(base int32, sums map[string]float64) float64 {
	if sums[statLocalBlockRemove] != 0 {
		return 0
	}
	return math.Floor((float64(base) + sums[statLocalBlockFlat]) * (1 + sums[statLocalBlockInc]/percentScale))
}
