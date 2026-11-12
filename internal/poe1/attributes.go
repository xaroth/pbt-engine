package poe1

import (
	"fmt"

	"github.com/xaroth/pbt-engine/internal/core"
	"github.com/xaroth/pbt-engine/internal/resolve"
)

// attributeBonus is an inherent bonus of an attribute: the constant's Value per Per points of
// the attribute, in whole steps, unless flag stat off is set.
type attributeBonus struct {
	// id is the rule source's RuleID.
	id                               string
	constant, attribute, target, off string
	op                               core.Op
	label                            string
}

// attributeBonuses are the inherent bonuses on the sheet. The constants' notes say each is
// rounded down; a core Scale counts whole steps, which is that.
func attributeBonuses() []attributeBonus {
	return []attributeBonus{
		{
			id:       "strength-bonus-life",
			constant: "life_per_strength", label: "Strength bonus to maximum life", attribute: statStrength, target: statMaximumLife, op: core.OpBase,
			off: statNoStrengthBonusToMaximumLife,
		},
		{
			id:       "intelligence-bonus-mana",
			constant: "mana_per_intelligence", label: "Intelligence bonus to maximum mana", attribute: statIntelligence, target: statMaximumMana, op: core.OpBase,
			off: statNoIntelligenceBonusToMaximumMana,
		},
		{
			id:       "dexterity-bonus-evasion",
			constant: "evasion_rating_+%_per_dexterity", label: "Dexterity bonus to evasion rating", attribute: statDexterity, target: statEvasionRating, op: core.OpInc,
			off: statNoDexterityBonusToEvasionRating,
		},
		{
			id:       "intelligence-bonus-energy-shield",
			constant: "energy_shield_+%_per_intelligence", label: "Intelligence bonus to maximum energy shield",
			attribute: statIntelligence, target: statMaximumEnergyShield, op: core.OpInc,
			off: statNoIntelligenceBonusToMaximumEnergyShield,
		},
	}
}

// addAttributeBonuses adds the inherent attribute bonuses as modifiers, one source each so
// each explains as its own line.
func (sh *Sheet) addAttributeBonuses(consts []resolve.Constant) error {
	for _, b := range attributeBonuses() {
		c, err := constant(consts, b.constant)
		if err != nil {
			return err
		}
		src := sh.newSource(Source{RuleID: b.id, Rule: fmt.Sprintf("%s (%d per %d, constant %s)", b.label, c.Value, c.Per, b.constant)})
		mod := core.Modifier{
			Stat: sh.stats[b.target], Op: b.op, Value: float64(c.Value), Source: src,
			Scales: []core.Scale{{Ref: core.SelfRef(sh.stats[b.attribute]), Step: float64(c.Per)}},
			Conds:  []core.Cond{{Ref: core.SelfRef(sh.stats[b.off]), Cmp: core.CmpEQ}},
		}
		if _, err := sh.session.AddModifier(mod); err != nil {
			return fmt.Errorf("sheet: %s: %w", b.constant, err)
		}
	}
	return nil
}

func constant(consts []resolve.Constant, id string) (resolve.Constant, error) {
	for _, c := range consts {
		if c.ID == id {
			if c.Per <= 0 {
				return c, fmt.Errorf("sheet: constant %s: per %d, want > 0: %w", id, c.Per, ErrConstant)
			}
			return c, nil
		}
	}
	return resolve.Constant{}, fmt.Errorf("sheet: constant %s: %w", id, ErrConstant)
}
