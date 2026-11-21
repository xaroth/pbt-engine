package items

import (
	"slices"
	"strings"
)

// variants are the variants of a unique as Path of Building writes them: "Variant:" lines, the
// "Selected Variant", and up to five alt variants, each with "Has Alt Variant ..." and
// "Selected Alt Variant ...". Alt variants are keyed by suffix: "" for the first, " Two" and on
// for the rest.
type variants struct {
	count       int
	selected    int
	hasAlt      map[string]bool
	altSelected map[string]int
}

// property reads one property of the variants and reports whether key is one.
func (v *variants) property(key string, n int) bool {
	if suffix, ok := strings.CutPrefix(key, "Has Alt Variant"); ok {
		if v.hasAlt == nil {
			v.hasAlt = map[string]bool{}
		}
		v.hasAlt[suffix] = true
		return true
	}
	if suffix, ok := strings.CutPrefix(key, "Selected Alt Variant"); ok {
		if v.altSelected == nil {
			v.altSelected = map[string]int{}
		}
		v.altSelected[suffix] = n
		return true
	}
	switch key {
	case "Variant":
		v.count++
	case "Selected Variant":
		v.selected = n
	default:
		return false
	}
	return true
}

// chosen returns the selected variant and alt variants. As in Path of Building, a missing or too
// high selection is the last variant, and an alt variant counts only where the item has it.
func (v *variants) chosen() []int {
	pick := func(n int) int {
		if v.count > 0 && (n == 0 || n > v.count) {
			return v.count
		}
		return n
	}
	out := make([]int, 0, 1+len(v.hasAlt))
	out = append(out, pick(v.selected))
	for suffix := range v.hasAlt {
		out = append(out, pick(v.altSelected[suffix]))
	}
	return out
}

// keeps reports whether a line of the given variants is on the item: a line of no variant is on
// every one.
func (v *variants) keeps(of []int) bool {
	if len(of) == 0 {
		return true
	}
	chosen := v.chosen()
	return slices.ContainsFunc(of, func(n int) bool { return slices.Contains(chosen, n) })
}
