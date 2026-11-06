package dataset

import (
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
)

// maxVersionParts is the most components a game version has: league, patch and hotfix numbers
// after the major version, as in 3.29.3.3.
const maxVersionParts = 4

// GameVersion is a game version of numbers separated by dots, such as 3.29.3.3. Versions
// order component by component, and a prefix orders before its extensions: 3.29 < 3.29.0.0 <
// 3.29.3.3. The zero value is no version and orders before every other. GameVersions compare
// with == and can be map keys.
type GameVersion struct {
	parts [maxVersionParts]uint16 // zero past n
	n     uint8
}

// ParseGameVersion parses s, such as "3.29.3.3". It has 1 to 4 components, each a decimal
// number from 0 to 65535 without leading zeros.
func ParseGameVersion(s string) (GameVersion, error) {
	var v GameVersion
	parts := strings.Split(s, ".")
	if len(parts) > maxVersionParts {
		return GameVersion{}, fmt.Errorf("version %q: more than %d components", s, maxVersionParts)
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 || p != strconv.Itoa(n) {
			return GameVersion{}, fmt.Errorf("version %q: want numbers separated by dots", s)
		}
		if n > math.MaxUint16 {
			return GameVersion{}, fmt.Errorf("version %q: component %d above %d", s, n, math.MaxUint16)
		}
		v.parts[i] = uint16(n)
	}
	v.n = uint8(len(parts))
	return v, nil
}

// IsZero reports whether v is the zero GameVersion.
func (v GameVersion) IsZero() bool { return v.n == 0 }

// Compare returns -1, 0 or 1 as v orders before, equal to or after w.
func (v GameVersion) Compare(w GameVersion) int {
	return slices.Compare(v.components(), w.components())
}

func (v GameVersion) len() int { return int(v.n) }

func (v GameVersion) components() []uint16 { return v.parts[:v.n] }

// HasPrefix reports whether p's components start v's. A version is a prefix of itself.
func (v GameVersion) HasPrefix(p GameVersion) bool {
	return v.n >= p.n && slices.Equal(v.parts[:p.n], p.components())
}

// League returns the first two components, which name a league, such as 3.29. A version
// with fewer components is its own league.
func (v GameVersion) League() GameVersion {
	l := GameVersion{n: min(2, v.n)}
	copy(l.parts[:l.n], v.parts[:l.n])
	return l
}

// String returns v as parsed; the zero value is "".
func (v GameVersion) String() string {
	var b strings.Builder
	for i, p := range v.components() {
		if i > 0 {
			b.WriteByte('.')
		}
		b.WriteString(strconv.FormatUint(uint64(p), 10))
	}
	return b.String()
}
