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

// enemyNumber is one of PoB's enemy stat inputs and the config key that holds it.
type enemyNumber struct {
	input, key string
	set        func(v *config.Values, n int) error
	get        func(v config.Values) (int, bool, error)
}

func number[T ~int](input string, k config.Key[T]) enemyNumber {
	return enemyNumber{
		input: input,
		key:   k.Name(),
		set:   func(v *config.Values, n int) error { return config.Set(v, k, T(n)) },
		get: func(v config.Values) (int, bool, error) {
			x, ok, err := config.Get(v, k)
			return int(x), ok, err
		},
	}
}

// enemyNumbers are PoB's enemy stat inputs the config holds, in PoB's order.
func enemyNumbers() []enemyNumber {
	return []enemyNumber{
		number("enemyLevel", poe1.EnemyLevel),
		number("enemyLightningResist", poe1.EnemyLightningResistance),
		number("enemyColdResist", poe1.EnemyColdResistance),
		number("enemyFireResist", poe1.EnemyFireResistance),
		number("enemyChaosResist", poe1.EnemyChaosResistance),
		number("enemyEvasion", poe1.EnemyEvasion),
		number("enemyArmour", poe1.EnemyArmour),
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

// enemyInput imports PoB's enemy input name with value into v. It reports whether name is an
// enemy input the config holds, and whether its value was imported.
func enemyInput(v *config.Values, name, value string) (known, imported bool) {
	if name == enemyKindInput {
		k, ok := enemyKindOf(value)
		return true, ok && config.Set(v, poe1.Enemy, k) == nil
	}
	for _, e := range enemyNumbers() {
		if e.input != name {
			continue
		}
		f, err := strconv.ParseFloat(value, 64)
		if err != nil || f != math.Trunc(f) || math.Abs(f) > math.MaxInt32 {
			return true, false
		}
		return true, e.set(v, int(f)) == nil
	}
	return false, false
}

// enemyInputs returns the PoB inputs of v's enemy keys, and the names of the keys.
func enemyInputs(v config.Values) ([]outInput, map[string]bool, error) {
	var out []outInput
	keys := map[string]bool{poe1.Enemy.Name(): true}
	k, ok, err := config.Get(v, poe1.Enemy)
	if err != nil {
		return nil, nil, fmt.Errorf("pob: %w", err)
	}
	if ok {
		out = append(out, outInput{Name: enemyKindInput, String: pobEnemyKind(k)})
	}
	for _, e := range enemyNumbers() {
		keys[e.key] = true
		n, ok, err := e.get(v)
		if err != nil {
			return nil, nil, fmt.Errorf("pob: %w", err)
		}
		if ok {
			out = append(out, outInput{Name: e.input, Number: strconv.Itoa(n)})
		}
	}
	return out, keys, nil
}
