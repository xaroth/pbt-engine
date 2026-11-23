package poe1

import (
	"fmt"
	"slices"

	"github.com/xaroth/pbt-engine/internal/core"
	"github.com/xaroth/pbt-engine/internal/resolve"
	"github.com/xaroth/pbt-engine/internal/semantics"
)

// attributeFamily is the core CycleFamily of the attributes, which can grant each other.
const attributeFamily = 1

// Engine stats of the attributes. The no_ stats are flags that turn off one inherent attribute
// bonus each. accuracy_rating_per_dexterity_instead, when not 0, is the accuracy rating the
// dexterity bonus grants per dexterity in place of the constant's.
const (
	statStrength                                 = "strength"
	statDexterity                                = "dexterity"
	statIntelligence                             = "intelligence"
	statNoStrengthBonusToMaximumLife             = "no_strength_bonus_to_maximum_life"
	statNoIntelligenceBonusToMaximumMana         = "no_intelligence_bonus_to_maximum_mana"
	statNoDexterityBonusToEvasionRating          = "no_dexterity_bonus_to_evasion_rating"
	statNoIntelligenceBonusToMaximumEnergyShield = "no_intelligence_bonus_to_maximum_energy_shield"
	statNoDexterityBonusToAccuracyRating         = "no_dexterity_bonus_to_accuracy_rating"
	statNoStrengthBonusToMeleePhysicalDamage     = "no_strength_bonus_to_melee_physical_damage"
	statAccuracyRatingPerDexterityInstead        = "accuracy_rating_per_dexterity_instead"
)

// attributeBonus is an inherent bonus of an attribute: the constant's Value per Per points of
// the attribute, in whole steps, unless flag stat off is set (none when empty). While stat
// instead is not 0 (never when empty), the bonus is instead its value per point of the attribute.
type attributeBonus struct {
	// id is the rule source's RuleID.
	id                                        string
	constant, attribute, target, off, instead string
	op                                        core.Op
	label                                     string
	// in are the contexts the bonus requires, as a table modifier's: one of each term; notIn
	// those it excludes.
	in    [][]string
	notIn []string
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
			id:       "dexterity-bonus-accuracy",
			constant: "accuracy_rating_per_dexterity", label: "Dexterity bonus to accuracy rating",
			attribute: statDexterity, target: statAccuracyRating, op: core.OpBase,
			off: statNoDexterityBonusToAccuracyRating, instead: statAccuracyRatingPerDexterityInstead,
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
		{
			// PoB applies it to melee skills' hits (CalcPerform.lua, ModFlag.Melee), not to their
			// ailments; the constant names melee physical damage.
			id:       "strength-bonus-melee-physical-damage",
			constant: "melee_physical_damage_+%_per_strength", label: "Strength bonus to melee physical damage",
			attribute: statStrength, target: statDamage, op: core.OpInc,
			in:    [][]string{{dimType + "." + "Melee"}, {dimDamage + "." + Physical.String()}},
			notIn: []string{dimForm + "." + damageFormNames()[formAilment-1]},
			off:   statNoStrengthBonusToMeleePhysicalDamage,
		},
	}
}

// addAttributeBonuses adds the inherent attribute bonuses of actor as modifiers, one source
// each so each explains as its own line, and returns the sources. slot is the skill slot whose
// context they hold in, -1 for all (the player's). PoB gives every actor the same bonuses
// (CalcPerform.lua doActorAttribsConditions, run for the player and for the minion).
func (sh *Sheet) addAttributeBonuses(actor core.ActorID, slot int) ([]core.SourceID, error) {
	var srcs []core.SourceID
	for _, b := range attributeBonuses() {
		c, err := constant(sh.constants, b.constant)
		if err != nil {
			return nil, err
		}
		pred, err := predicate(sh.session.Model(), semantics.ContextFilter{In: b.in, NotIn: b.notIn}, sh.ctx, slot)
		if err != nil {
			return nil, err
		}
		src := sh.newSource(Source{RuleID: b.id, Rule: fmt.Sprintf("%s (%d per %d, constant %s)", b.label, c.Value, c.Per, b.constant)})
		srcs = append(srcs, src)
		mod := core.Modifier{
			Actor: actor,
			Stat:  sh.stats[b.target], Op: b.op, Value: float64(c.Value), Source: src, Pred: pred,
			Scales: []core.Scale{{Ref: core.SelfRef(sh.stats[b.attribute]), Step: float64(c.Per)}},
		}
		if b.off != "" {
			mod.Conds = []core.Cond{{Ref: core.SelfRef(sh.stats[b.off]), Cmp: core.CmpEQ}}
		}
		mods := []core.Modifier{mod}
		if b.instead != "" {
			src := sh.newSource(Source{RuleID: b.id + "-instead", Rule: fmt.Sprintf("%s (%s per 1)", b.label, b.instead)})
			srcs = append(srcs, src)
			mods = append(mods, insteadModifier(mod, src, sh.stats[b.instead]))
			mods[0].Conds = append(slices.Clone(mod.Conds), core.Cond{Ref: core.SelfRef(sh.stats[b.instead]), Cmp: core.CmpEQ})
		}
		for _, m := range mods {
			if _, err := sh.session.AddModifier(m); err != nil {
				return nil, fmt.Errorf("sheet: %s: %w", b.constant, err)
			}
		}
	}
	return srcs, nil
}

// insteadModifier is mod from src granting the value of stat instead per point of the
// attribute, held while instead is not 0.
func insteadModifier(mod core.Modifier, src core.SourceID, instead core.StatID) core.Modifier {
	mod.Value, mod.Source = 1, src
	mod.Scales = []core.Scale{mod.Scales[0], {Ref: core.SelfRef(instead), Step: 1}}
	mod.Conds = append(slices.Clone(mod.Conds), core.Cond{Ref: core.SelfRef(instead), Cmp: core.CmpNE})
	return mod
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

func attributeMechanic() mechanic {
	return mechanic{name: "attributes", stats: []string{
		statStrength,
		statDexterity,
		statIntelligence,
		statNoStrengthBonusToMaximumLife,
		statNoIntelligenceBonusToMaximumMana,
		statNoDexterityBonusToEvasionRating,
		statNoIntelligenceBonusToMaximumEnergyShield,
		statNoDexterityBonusToAccuracyRating,
		statNoStrengthBonusToMeleePhysicalDamage,
		statAccuracyRatingPerDexterityInstead,
	}, def: attributeDef}
}

// attributeDef completes def when it is an attribute or a flag that turns off its bonus. An
// attribute is rounded half up and at least 0, as in PoB (CalcPerform.lua); the game text does
// not say.
func attributeDef(def core.StatDef, _ map[string]core.StatID) core.StatDef {
	switch def.Name {
	case statStrength, statDexterity, statIntelligence:
		def.Round, def.HasMin, def.CycleFamily = core.RoundHalf, true, attributeFamily
	case statNoStrengthBonusToMaximumLife, statNoIntelligenceBonusToMaximumMana,
		statNoDexterityBonusToEvasionRating, statNoIntelligenceBonusToMaximumEnergyShield,
		statNoDexterityBonusToAccuracyRating, statNoStrengthBonusToMeleePhysicalDamage:
		def.Flag = true
	}
	return def
}
