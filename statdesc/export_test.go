package statdesc

import (
	"maps"
	"slices"
)

// HandlerNames returns the handlers that change a value, sorted.
func HandlerNames() []string {
	var out []string
	for _, name := range slices.Sorted(maps.Keys(handlers())) {
		if handlers()[name].apply != nil {
			out = append(out, name)
		}
	}
	return out
}

// NumberLen exposes how the reader takes numbers.
func NumberLen(s string) int { return numberLen(s) }
