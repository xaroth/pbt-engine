package poe1

import (
	"slices"
	"testing"

	"github.com/xaroth/pbt-engine/internal/core"
)

func TestMechanics(t *testing.T) {
	t.Parallel()
	var b core.ModelBuilder
	if _, err := addMechanics(&b, mechanics()); err != nil {
		t.Fatal(err)
	}
}

// A mechanic out of order is an error naming what it read or included, not a panic.
func TestMechanicsOutOfOrder(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name string
		ms   []mechanic
		want string
	}{
		{
			name: "reads a mechanic it does not come after", ms: withoutAfter("regeneration", "defences"),
			want: "poe1: mechanics: mechanic regeneration, defining stat life_regeneration, " +
				"reads stat maximum_life, which is not its own or of a mechanic it comes after",
		},
		{
			name: "includes a stat listed after it", ms: withSwapped("defences", statArmourAndEvasionRating, statArmour),
			want: "poe1: mechanics: mechanic defences: stat armour includes or shares stat armour_and_evasion_rating, which is listed after it",
		},
		{
			name: "comes after an unknown mechanic", ms: append(mechanics(), mechanic{name: "new", after: []string{"old"}}),
			want: "poe1: mechanics: mechanic new comes after old, which is not a mechanic",
		},
		{
			name: "cycle", ms: append(mechanics(), mechanic{name: "a", after: []string{"b"}}, mechanic{name: "b", after: []string{"a"}}),
			want: "poe1: mechanics: a, b come after each other",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var b core.ModelBuilder
			_, err := addMechanics(&b, tc.ms)
			if err == nil || err.Error() != tc.want {
				t.Errorf("error %v, want %s", err, tc.want)
			}
		})
	}
}

// withoutAfter is mechanics() with mechanic name no longer coming after mechanic after.
func withoutAfter(name, after string) []mechanic {
	ms := mechanics()
	for i, m := range ms {
		if m.name == name {
			ms[i].after = slices.DeleteFunc(slices.Clone(m.after), func(a string) bool { return a == after })
		}
	}
	return ms
}

// withSwapped is mechanics() with stats first and second of mechanic name swapped.
func withSwapped(name, first, second string) []mechanic {
	ms := mechanics()
	for i, m := range ms {
		if m.name == name {
			stats := slices.Clone(m.stats)
			i1, i2 := slices.Index(stats, first), slices.Index(stats, second)
			stats[i1], stats[i2] = stats[i2], stats[i1]
			ms[i].stats = stats
		}
	}
	return ms
}
