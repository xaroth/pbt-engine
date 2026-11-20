package pob

import (
	"fmt"
	"math"
	"strconv"

	"github.com/xaroth/pbt-engine/config"
	"github.com/xaroth/pbt-engine/poe1"
)

// resonanceInput is PoB's "Lowest Resonance Count" input, shown while a Trinity support is in
// use.
const resonanceInput = "configResonanceCount"

// resonanceOf imports PoB's input name with value into v when it is the Resonance count. It
// reports whether it is, and whether its value was read.
func resonanceOf(v *config.Values, name, value string) (known, imported bool) {
	if name != resonanceInput {
		return false, false
	}
	f, err := strconv.ParseFloat(value, 64)
	if err != nil || f != math.Trunc(f) || f < 0 || f > math.MaxInt32 {
		return true, false
	}
	return true, config.Set(v, poe1.LowestResonance, poe1.Resonance(f)) == nil
}

// resonanceOutputs returns the PoB input of v's Resonance count and the name of its key. A
// count of 0 is PoB's default, so it is left out.
func resonanceOutputs(v config.Values) (inputs []outInput, key string, err error) {
	count, _, err := config.Get(v, poe1.LowestResonance)
	if err != nil {
		return nil, "", fmt.Errorf("pob: %w", err)
	}
	if count > 0 {
		inputs = append(inputs, outInput{Name: resonanceInput, Number: strconv.Itoa(int(count))})
	}
	return inputs, poe1.LowestResonance.Name(), nil
}
