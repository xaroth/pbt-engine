package pob

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/xaroth/pbt-engine/config"
	"github.com/xaroth/pbt-engine/internal/poe1"
)

// enemyKindInput is PoB's "Is the enemy a Boss?" input.
const enemyKindInput = "enemyIsBoss"

// maxEnemyLevel is the highest enemy level PoB uses; it lowers a higher input to it.
const maxEnemyLevel = 85

// enemyNumber is one of PoB's enemy stat inputs and the config key that holds it.
type enemyNumber struct {
	input, key string
	// zeroUnset is an input PoB reads as empty when 0, using its preset instead.
	zeroUnset bool
	// atMost caps the value; 0 for no cap.
	atMost int
	set    func(v *config.Values, n int) error
	get    func(v config.Values) (int, bool, error)
}

func number[T ~int](input string, k config.Key[T], zeroUnset bool, atMost int) enemyNumber {
	return enemyNumber{
		input:     input,
		key:       k.Name(),
		zeroUnset: zeroUnset,
		atMost:    atMost,
		set:       func(v *config.Values, n int) error { return config.Set(v, k, T(n)) },
		get: func(v config.Values) (int, bool, error) {
			x, ok, err := config.Get(v, k)
			return int(x), ok, err
		},
	}
}

// enemyNumbers are PoB's enemy stat inputs the config holds, in PoB's order.
func enemyNumbers() []enemyNumber {
	return []enemyNumber{
		number("enemyLevel", poe1.EnemyLevel, true, maxEnemyLevel),
		number("enemyLightningResist", poe1.EnemyLightningResistance, true, 0),
		number("enemyColdResist", poe1.EnemyColdResistance, true, 0),
		number("enemyFireResist", poe1.EnemyFireResistance, true, 0),
		number("enemyChaosResist", poe1.EnemyChaosResistance, true, 0),
		number("enemyEvasion", poe1.EnemyEvasion, false, 0),
		number("enemyArmour", poe1.EnemyArmour, false, 0),
	}
}

// enemyKindOf maps PoB's enemyIsBoss value, read as PoB reads it: in any case, and with the
// values of older versions (Uber Atziri a standard boss, Shaper and Sirus pinnacle bosses).
func enemyKindOf(pob string) (poe1.EnemyKind, bool) {
	switch strings.ToLower(pob) {
	case "none":
		return poe1.EnemyNone, true
	case "boss", "uber atziri":
		return poe1.EnemyBoss, true
	case "pinnacle", "shaper", "sirus":
		return poe1.EnemyPinnacle, true
	case "uber":
		return poe1.EnemyUber, true
	}
	return "", false
}

// pobEnemyKind is the inverse of enemyKindOf.
func pobEnemyKind(k poe1.EnemyKind) string {
	switch k {
	case poe1.EnemyNone:
		return none
	case poe1.EnemyBoss:
		return "Boss"
	case poe1.EnemyPinnacle:
		return "Pinnacle"
	case poe1.EnemyUber:
		return "Uber"
	}
	return ""
}

// enemyInput imports PoB's enemy input name with value into v, as PoB reads it: a 0 where PoB
// takes 0 as empty is left out, and a level above PoB's highest is lowered to it. It reports
// whether name is an enemy input the config holds, and whether its value was read.
func enemyInput(v *config.Values, name, value string) (known, imported bool) {
	if name == enemyKindInput {
		k, ok := enemyKindOf(value)
		return true, ok && config.Set(v, poe1.Enemy, k) == nil
	}
	for _, e := range enemyNumbers() {
		if e.input == name {
			return true, e.read(v, value)
		}
	}
	return false, false
}

// read sets e's key in v from PoB's value, and reports whether the value was read.
func (e enemyNumber) read(v *config.Values, value string) bool {
	f, err := strconv.ParseFloat(value, 64)
	if err != nil || f != math.Trunc(f) || math.Abs(f) > math.MaxInt32 {
		return false
	}
	n := int(f)
	if n == 0 && e.zeroUnset {
		return true
	}
	if e.atMost > 0 {
		n = min(n, e.atMost)
	}
	return e.set(v, n) == nil
}

// enemyInputs returns the PoB inputs of v's enemy keys, the names of the keys, and the keys
// whose value PoB cannot hold: a 0 PoB would read as empty.
func enemyInputs(v config.Values) (inputs []outInput, keys map[string]bool, unwritable []string, err error) {
	keys = map[string]bool{poe1.Enemy.Name(): true}
	k, ok, err := config.Get(v, poe1.Enemy)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("pob: %w", err)
	}
	if ok {
		inputs = append(inputs, outInput{Name: enemyKindInput, String: pobEnemyKind(k)})
	}
	for _, e := range enemyNumbers() {
		keys[e.key] = true
		n, ok, err := e.get(v)
		switch {
		case err != nil:
			return nil, nil, nil, fmt.Errorf("pob: %w", err)
		case !ok:
		case n == 0 && e.zeroUnset:
			unwritable = append(unwritable, e.key)
		default:
			inputs = append(inputs, outInput{Name: e.input, Number: strconv.Itoa(n)})
		}
	}
	return inputs, keys, unwritable, nil
}
