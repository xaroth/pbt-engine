package pob

import (
	"fmt"
	"math"
	"strconv"

	"github.com/xaroth/pbt-engine/config"
	"github.com/xaroth/pbt-engine/poe1"
)

// countInput is a PoB count input the config holds, with its key.
type countInput struct {
	input, key string
	set        func(v *config.Values, n int) error
	get        func(v config.Values) (int, error)
}

func countOf[T ~int](input string, k config.Key[T]) countInput {
	return countInput{
		input: input, key: k.Name(),
		set: func(v *config.Values, n int) error { return config.Set(v, k, T(n)) },
		get: func(v config.Values) (int, error) {
			n, _, err := config.Get(v, k)
			return int(n), err
		},
	}
}

// countInputs are PoB's count inputs the config holds: the lowest Resonance, shown while a
// Trinity support is in use, the Withered stacks on the enemy, shown while the build can
// inflict Withered, and the charges of minions, shown while they use them.
func countInputs() []countInput {
	return []countInput{
		countOf("configResonanceCount", poe1.LowestResonance),
		countOf("multiplierWitheredStackCount", poe1.EnemyWitheredStacks),
		countOf("minionsOverridePowerCharges", poe1.MinionPowerChargeCount),
		countOf("minionsOverrideFrenzyCharges", poe1.MinionFrenzyChargeCount),
		countOf("minionsOverrideEnduranceCharges", poe1.MinionEnduranceChargeCount),
	}
}

// countFrom imports PoB's input name with value into v when it is a count input. It reports
// whether it is, and whether its value was read.
func countFrom(v *config.Values, name, value string) (known, imported bool) {
	for _, c := range countInputs() {
		if name != c.input {
			continue
		}
		f, err := strconv.ParseFloat(value, 64)
		if err != nil || f != math.Trunc(f) || f < 0 || f > math.MaxInt32 {
			return true, false
		}
		return true, c.set(v, int(f)) == nil
	}
	return false, false
}

// countOutputs returns the PoB inputs of v's counts and the names of their keys. A count of 0
// is PoB's default, so it is left out.
func countOutputs(v config.Values) (inputs []outInput, keys []string, err error) {
	for _, c := range countInputs() {
		n, err := c.get(v)
		if err != nil {
			return nil, nil, fmt.Errorf("pob: %w", err)
		}
		if n > 0 {
			inputs = append(inputs, outInput{Name: c.input, Number: strconv.Itoa(n)})
		}
		keys = append(keys, c.key)
	}
	return inputs, keys, nil
}
