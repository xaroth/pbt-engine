package items

import (
	"math"
	"regexp"
	"strconv"
	"strings"
)

var (
	rangePattern   = regexp.MustCompile(`(\+?)\((-?\d+\.?\d*)-(-?\d+\.?\d*)\)`)
	negatedPercent = regexp.MustCompile(`-(\d+%) ([A-Za-z]+)`)
	decimal        = regexp.MustCompile(`\d+\.\d*`)
)

// withRoll returns line with each range "(min-max)" set to the value at roll, 0 to 1, as Path of
// Building's itemLib.applyRange sets it: rounded half up to a whole number, or, where that is not
// exact, to the decimals of the line's kind of value (see rollPrecision). A negative increase or
// more turns into a reduction or less.
func withRoll(line string, roll float64) string {
	if !rangePattern.MatchString(line) {
		return line
	}
	precision := 0
	if !wholeAt(line, roll) {
		precision = rollPrecision(line)
	}
	power := math.Pow(decimalBase, float64(precision))
	line = rangePattern.ReplaceAllStringFunc(line, func(m string) string {
		g := rangePattern.FindStringSubmatch(m)
		lo, _ := strconv.ParseFloat(g[2], 64)
		hi, _ := strconv.ParseFloat(g[3], 64)
		v := roundHalfUp((lo+roll*(hi-lo))*power) / power
		plus := g[1]
		if v < 0 {
			plus = ""
		}
		return plus + strconv.FormatFloat(v, 'f', -1, 64)
	})
	return negatedPercent.ReplaceAllStringFunc(line, func(m string) string {
		g := negatedPercent.FindStringSubmatch(m)
		if a, ok := antonyms()[g[2]]; ok {
			return g[1] + " " + a
		}
		return m
	})
}

// wholeAt reports whether every range of line is a whole number at roll.
func wholeAt(line string, roll float64) bool {
	for _, g := range rangePattern.FindAllStringSubmatch(line, -1) {
		lo, _ := strconv.ParseFloat(g[2], 64)
		hi, _ := strconv.ParseFloat(g[3], 64)
		if v := lo + roll*(hi-lo); roundHalfUp(v) != v {
			return false
		}
	}
	return true
}

// rollPrecision returns the decimals Path of Building keeps of a rolled value that is not whole:
// those of its high precision mods (data.highPrecisionMods), by the text of the lines that give
// them, else one.
func rollPrecision(line string) int {
	for _, p := range []struct {
		text      string
		precision int
	}{
		{"% to Critical Strike Chance", 2},
		{"% of Life per second", 2}, {"% of Mana per second", 2}, {"% of Energy Shield per second", 2},
		{"Leeched as", 2},
		{"Life per second", 1}, {"Mana per second", 1}, {"Energy Shield per second", 1}, {"Rage per second", 1},
	} {
		if strings.Contains(line, p.text) {
			return p.precision
		}
	}
	if decimal.MatchString(line) {
		return 1
	}
	return 0
}

const decimalBase = 10

// roundHalfUp rounds x to a whole number, halves up, as Lua's math.floor(x + 0.5).
func roundHalfUp(x float64) float64 {
	const half = 0.5
	return math.Floor(x + half)
}

const reduced = "reduced"

func antonyms() map[string]string {
	return map[string]string{"increased": reduced, reduced: "increased", "more": "less", "less": "more"}
}
