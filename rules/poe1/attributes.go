package poe1

import (
	"fmt"

	"github.com/xaroth/pbt-engine/core"
	"github.com/xaroth/pbt-engine/resolve"
)

// attributeBonus is an inherent bonus of an attribute: the constant's Value per Per points of
// the attribute, in whole steps.
type attributeBonus struct {
	constant, attribute, target string
	op                          core.Op
}

// attributeBonuses are the inherent bonuses on the sheet. The constants' notes say each is
// rounded down; a core Scale counts whole steps, which is that.
func attributeBonuses() []attributeBonus {
	return []attributeBonus{
		{constant: "life_per_strength", attribute: statStrength, target: statMaximumLife, op: core.OpBase},
		{constant: "mana_per_intelligence", attribute: statIntelligence, target: statMaximumMana, op: core.OpBase},
		{constant: "evasion_rating_+%_per_dexterity", attribute: statDexterity, target: statEvasionRating, op: core.OpInc},
		{constant: "energy_shield_+%_per_intelligence", attribute: statIntelligence, target: statMaximumEnergyShield, op: core.OpInc},
	}
}

// addAttributeBonuses adds the inherent attribute bonuses as modifiers of one source.
func (sh *Sheet) addAttributeBonuses(consts []resolve.Constant) error {
	src := sh.session.NewSource()
	for _, b := range attributeBonuses() {
		c, err := constant(consts, b.constant)
		if err != nil {
			return err
		}
		mod := core.Modifier{
			Stat: sh.stats[b.target], Op: b.op, Value: float64(c.Value), Source: src,
			Scales: []core.Scale{{Ref: core.SelfRef(sh.stats[b.attribute]), Step: float64(c.Per)}},
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
