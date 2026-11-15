package poe1

import (
	"slices"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/internal/resolve"
)

// statReflectsOppositeRing is Kalandra's Touch's "Reflects opposite Ring".
const statReflectsOppositeRing = "local_ring_duplicate_other_ring"

// oppositeRing returns the ring slot opposite s. The first and second ring are opposite each
// other; the third ring has none, as in Path of Building.
func oppositeRing(s build.Slot) (build.Slot, bool) {
	switch s {
	case build.SlotRing1:
		return build.SlotRing2, true
	case build.SlotRing2:
		return build.SlotRing1, true
	default:
		return "", false
	}
}

// ReflectRings gives each ring in rings that reflects the opposite ring the stats of that ring
// besides its own, and a key that follows both. rings holds the equipped rings by slot, abyss
// jewels apart. A reflecting ring gains nothing from an opposite ring that reflects too.
// Gem levels and supports an opposite ring gives the gems socketed in it stay with that ring:
// gems read them from the build's items.
func ReflectRings(rings map[build.Slot]*Item) {
	for s, it := range rings {
		other, ok := oppositeRing(s)
		if o := rings[other]; ok && o != nil && reflects(it) && !reflects(o) {
			it.Stats = append(slices.Clone(it.Stats), o.Stats...)
			it.Key += " reflecting " + o.Key
		}
	}
}

func reflects(it *Item) bool {
	return slices.ContainsFunc(it.Stats, func(sv resolve.StatValue) bool { return sv.Stat == statReflectsOppositeRing })
}
