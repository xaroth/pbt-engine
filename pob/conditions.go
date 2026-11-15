package pob

import (
	"fmt"
	"math"
	"strconv"

	"github.com/xaroth/pbt-engine/config"
	"github.com/xaroth/pbt-engine/internal/poe1"
)

// shockEffectInput is PoB's "Effect of Shock" input, read while the enemy is Shocked.
const shockEffectInput = "conditionShockEffect"

// conditionInput is one of PoB's condition checkboxes and the config key that holds it.
type conditionInput struct {
	input string
	key   config.Key[bool]
}

// conditionInputs are PoB's condition inputs the config holds, in PoB's order.
func conditionInputs() []conditionInput {
	return []conditionInput{
		{"conditionFullLife", poe1.OnFullLife},
		{"conditionFullEnergyShield", poe1.OnFullEnergyShield},
		{"conditionFocused", poe1.Focused},
		{"buffFortification", poe1.Fortified},
		{"conditionLeeching", poe1.Leeching},
		{"conditionEnemyBleeding", poe1.EnemyBleeding},
		{"conditionEnemyMoving", poe1.EnemyMoving},
		{"conditionEnemyTaunted", poe1.EnemyTaunted},
		{"conditionEnemyIgnited", poe1.EnemyIgnited},
		{"conditionEnemyChilled", poe1.EnemyChilled},
		{"conditionEnemyFrozen", poe1.EnemyFrozen},
		{"conditionEnemyShocked", poe1.EnemyShocked},
		{"conditionEnemyIntimidated", poe1.EnemyIntimidated},
		{"conditionEnemyUnnerved", poe1.EnemyUnnerved},
		{"conditionEnemyOnConsecratedGround", poe1.EnemyOnConsecratedGround},
	}
}

// conditionOf imports PoB's condition input name with value into v. It reports whether name
// is a condition input the config holds, and whether its value was read.
func conditionOf(v *config.Values, name, value string) (known, imported bool) {
	if name == shockEffectInput {
		f, err := strconv.ParseFloat(value, 64)
		if err != nil || f != math.Trunc(f) || f < 0 || f > math.MaxInt32 {
			return true, false
		}
		return true, config.Set(v, poe1.EnemyShockEffect, poe1.ShockEffect(f)) == nil
	}
	for _, c := range conditionInputs() {
		if c.input != name {
			continue
		}
		on, err := strconv.ParseBool(value)
		return true, err == nil && config.Set(v, c.key, on) == nil
	}
	return false, false
}

// conditionOutputs returns the PoB inputs of v's condition keys and the names of the keys. A
// condition that is off and a Shock effect of 0 are PoB's defaults, so they are left out.
func conditionOutputs(v config.Values) (inputs []outInput, keys map[string]bool, err error) {
	keys = map[string]bool{poe1.EnemyShockEffect.Name(): true}
	for _, c := range conditionInputs() {
		keys[c.key.Name()] = true
		on, _, err := config.Get(v, c.key)
		if err != nil {
			return nil, nil, fmt.Errorf("pob: %w", err)
		}
		if on {
			inputs = append(inputs, outInput{Name: c.input, Boolean: "true"})
		}
	}
	effect, _, err := config.Get(v, poe1.EnemyShockEffect)
	if err != nil {
		return nil, nil, fmt.Errorf("pob: %w", err)
	}
	if effect > 0 {
		inputs = append(inputs, outInput{Name: shockEffectInput, Number: strconv.Itoa(int(effect))})
	}
	return inputs, keys, nil
}
