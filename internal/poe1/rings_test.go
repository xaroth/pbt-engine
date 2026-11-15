package poe1_test

import (
	"maps"
	"slices"
	"testing"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/internal/poe1"
	"github.com/xaroth/pbt-engine/internal/resolve"
)

// Kalandra's Touch ("Reflects opposite Ring") gains the stats of the ring opposite it: the first
// and second ring reflect each other, the third ring neither reflects nor is reflected, and a
// Kalandra's Touch reflects nothing from another.
func TestReflectRings(t *testing.T) {
	t.Parallel()
	kalandra := []resolve.StatValue{{Stat: "local_ring_duplicate_other_ring", Value: 1}}
	life := []resolve.StatValue{{Stat: "base_maximum_life", Value: 50}}
	fire := []resolve.StatValue{{Stat: "base_fire_damage_resistance_%", Value: 30}}
	for _, tc := range []struct {
		name  string
		rings map[build.Slot][]resolve.StatValue
		want  map[build.Slot][]resolve.StatValue
	}{
		{
			name:  "first reflects second",
			rings: map[build.Slot][]resolve.StatValue{build.SlotRing1: kalandra, build.SlotRing2: life},
			want:  map[build.Slot][]resolve.StatValue{build.SlotRing1: slices.Concat(kalandra, life), build.SlotRing2: life},
		},
		{
			name:  "second reflects first",
			rings: map[build.Slot][]resolve.StatValue{build.SlotRing1: life, build.SlotRing2: kalandra, build.SlotRing3: fire},
			want:  map[build.Slot][]resolve.StatValue{build.SlotRing1: life, build.SlotRing2: slices.Concat(kalandra, life), build.SlotRing3: fire},
		},
		{
			name:  "third reflects nothing",
			rings: map[build.Slot][]resolve.StatValue{build.SlotRing1: life, build.SlotRing2: fire, build.SlotRing3: kalandra},
			want:  map[build.Slot][]resolve.StatValue{build.SlotRing1: life, build.SlotRing2: fire, build.SlotRing3: kalandra},
		},
		{
			name:  "nothing opposite",
			rings: map[build.Slot][]resolve.StatValue{build.SlotRing1: kalandra, build.SlotRing3: fire},
			want:  map[build.Slot][]resolve.StatValue{build.SlotRing1: kalandra, build.SlotRing3: fire},
		},
		{
			name:  "two reflect nothing",
			rings: map[build.Slot][]resolve.StatValue{build.SlotRing1: kalandra, build.SlotRing2: kalandra},
			want:  map[build.Slot][]resolve.StatValue{build.SlotRing1: kalandra, build.SlotRing2: kalandra},
		},
	} {
		rings := map[build.Slot]*poe1.Item{}
		for s, stats := range tc.rings {
			rings[s] = &poe1.Item{Slot: s, Key: string(s), Stats: slices.Clone(stats)}
		}
		poe1.ReflectRings(rings)
		got := map[build.Slot][]resolve.StatValue{}
		for s, it := range rings {
			got[s] = it.Stats
		}
		if !maps.EqualFunc(got, tc.want, slices.Equal) {
			t.Errorf("%s: stats %v, want %v", tc.name, got, tc.want)
		}
	}
}

// A reflecting ring's key follows the ring it reflects, so the sheet replaces its source when
// that ring changes.
func TestReflectRingsKey(t *testing.T) {
	t.Parallel()
	key := func(opposite string) string {
		rings := map[build.Slot]*poe1.Item{
			build.SlotRing1: {Key: "kalandra", Stats: []resolve.StatValue{{Stat: "local_ring_duplicate_other_ring", Value: 1}}},
			build.SlotRing2: {Key: opposite},
		}
		poe1.ReflectRings(rings)
		return rings[build.SlotRing1].Key
	}
	if a, b := key("a"), key("b"); a == b {
		t.Errorf("key %q for both opposite rings", a)
	}
}
