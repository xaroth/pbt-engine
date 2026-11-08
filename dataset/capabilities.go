package dataset

import (
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
)

// Capabilities maps a mechanic to the version of its rule a dataset needs, such as
// {"armour": 2}. A mechanic not listed is version 1, the original rule, so a dataset lists only
// the rules that changed. Rules select a formula by capability version, never by game version,
// and read versions through Check.
type Capabilities map[string]int

// minListedVersion is the lowest version a manifest may list; version 1 is the default.
const minListedVersion = 2

// Check returns c checked against known, which maps each mechanic the caller implements to
// the highest version it implements. It fails, matching ErrUnsupported, when c lists a
// mechanic known lacks or a version above the one known gives.
func (c Capabilities) Check(known map[string]int) (Checked, error) {
	var bad []string
	for _, name := range slices.Sorted(maps.Keys(c)) {
		top, ok := known[name]
		switch {
		case !ok:
			bad = append(bad, fmt.Sprintf("%s %d (unknown mechanic)", name, c[name]))
		case c[name] > top:
			bad = append(bad, fmt.Sprintf("%s %d (implemented up to %d)", name, c[name], top))
		}
	}
	if bad != nil {
		return Checked{}, fmt.Errorf("%w: %s", ErrUnsupported, strings.Join(bad, ", "))
	}
	return Checked{versions: maps.Clone(c), known: maps.Clone(known)}, nil
}

// Checked is a dataset's capabilities checked against the mechanics its rules implement.
// Rules read versions only through it, so every version they see is one they implement.
type Checked struct {
	versions, known map[string]int
}

// Version returns the rule version of mechanic name, 1 when the dataset does not list it. It
// panics when name is not a mechanic Check was given: a misspelt name is a bug in the rules,
// not a property of the dataset.
func (c Checked) Version(name string) int {
	if _, ok := c.known[name]; !ok {
		panic(fmt.Sprintf("dataset: capability %q is not a known mechanic", name))
	}
	if v, ok := c.versions[name]; ok {
		return v
	}
	return 1
}

// validate checks that each name is lowercase kebab-case and each version is at least
// minListedVersion. Leaving version 1 out gives a dataset one spelling and one id.
func (c Capabilities) validate() error {
	for _, name := range slices.Sorted(maps.Keys(c)) {
		if !isKebab(name) {
			return fmt.Errorf("%s: capability %q: name is not lowercase kebab-case: %w", ManifestName, name, ErrInvalid)
		}
		if c[name] < minListedVersion {
			return fmt.Errorf("%s: capability %q: version %d, want %d or more: %w", ManifestName, name, c[name], minListedVersion, ErrInvalid)
		}
	}
	return nil
}

// idLines returns the capabilities as content id input, sorted by name.
func (c Capabilities) idLines() string {
	var s strings.Builder
	for _, name := range slices.Sorted(maps.Keys(c)) {
		s.WriteString("capability " + name + " " + strconv.Itoa(c[name]) + "\n")
	}
	return s.String()
}

func isKebab(s string) bool {
	if s == "" || s[0] < 'a' || s[0] > 'z' {
		return false
	}
	for part := range strings.SplitSeq(s, "-") {
		if part == "" || strings.Trim(part, "abcdefghijklmnopqrstuvwxyz0123456789") != "" {
			return false
		}
	}
	return true
}
