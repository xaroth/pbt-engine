package statdesc_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/xaroth/pbt-engine/statdesc"
)

// TestReadRangeRoundTrip renders every variant of the main description file at two values per
// stat, writes the numbers that differ as ranges, and reads the line back with ReadRange: one
// reading must have the variant's stats, and every reading must render its Min and Max back to
// the two texts. Variants that Read does not read back at both ends are left out; the rest that
// fail are listed in the golden file.
func TestReadRangeRoundTrip(t *testing.T) {
	t.Parallel()
	set, sc := realText(t)
	var failed []string
	tried, ranged, ok := 0, 0, 0
	for i, d := range sc.Descriptions {
		for j, v := range d.Variants {
			lo, hi, why := renderEnds(set, d, v)
			if why != "" {
				continue
			}
			if !readsBack(t, set, d, lo) || !readsBack(t, set, d, hi) {
				continue
			}
			tried++
			text := joinRanges(lo, hi)
			if text != lo {
				ranged++
			}
			got, err := set.ReadRange(mainScope, strings.Split(text, "\n"))
			if err != nil {
				t.Fatal(err)
			}
			k := slices.IndexFunc(got, func(r statdesc.RangeReading) bool { return slices.Equal(r.Stats, d.Stats) })
			switch {
			case k < 0:
				failed = append(failed, fmt.Sprintf("%d/%d %s | no reading | %q", i, j, strings.Join(d.Stats, ","), text))
			case slices.ContainsFunc(got, func(r statdesc.RangeReading) bool { return !rendersEnds(set, r, lo, hi) }):
				failed = append(failed, fmt.Sprintf("%d/%d %s | a reading does not render back | %q", i, j, strings.Join(d.Stats, ","), text))
			default:
				ok++
			}
		}
	}
	t.Logf("%d variants read back at two values, %d of them written with a range, %d read back (%.2f%%)", tried, ranged, ok, pct(ok, tried))
	if pct(ok, tried) < 99 {
		t.Errorf("%.2f%% read back as a range, want at least 99%%", pct(ok, tried))
	}
	header := fmt.Sprintf("# Variants of %s on %s that read back at each end but not as a range (description/variant stats | reason | text).\n"+
		"# %d variants read back at two values, %d of them written with a range, %d read back.\n", mainScope, realVersion, tried, ranged, ok)
	golden(t, "read-range-roundtrip-"+realVersion+".txt", header+strings.Join(failed, "\n")+"\n")
}

// renderEnds renders v at sample values and at values next to them inside the same limits; the
// reason when either does not render v.
func renderEnds(set *statdesc.Set, d statdesc.Description, v statdesc.Variant) (string, string, string) {
	lo, hi := make(map[string]int32, len(d.Stats)), make(map[string]int32, len(d.Stats))
	for i, st := range d.Stats {
		lo[st] = sample(v.Limits[i], i)
		hi[st] = other(v.Limits[i], lo[st])
	}
	a, why := renderStats(set, d, lo)
	if why != "" {
		return "", "", why
	}
	b, why := renderStats(set, d, hi)
	return a, b, why
}

// other returns a value next to x that l allows, x when there is none.
func other(l statdesc.Limit, x int32) int32 {
	for _, y := range []int32{x + 60, x - 60, x + 1, x - 1} {
		if l.Not != (y >= l.Min && y <= l.Max) {
			return y
		}
	}
	return x
}

func renderStats(set *statdesc.Set, d statdesc.Description, stats map[string]int32) (string, string) {
	lines, err := set.Describe(mainScope, stats)
	if err != nil {
		return "", err.Error()
	}
	for _, l := range lines {
		if slices.Equal(l.Stats, d.Stats) {
			return l.Text, ""
		}
	}
	return "", "another description of its stats renders them"
}

// readsBack reports whether Read reads text back to d's stats.
func readsBack(t *testing.T, set *statdesc.Set, d statdesc.Description, text string) bool {
	t.Helper()
	rs, err := set.Read(mainScope, strings.Split(text, "\n"))
	if err != nil {
		t.Fatal(err)
	}
	return slices.ContainsFunc(rs, func(r statdesc.Reading) bool { return slices.Equal(r.Stats, d.Stats) })
}

// rendersEnds reports whether r renders the lines of lo and hi it spans with each stat at its
// Min or its Max: stats can range in opposite directions, and a "reduced" line's low text is
// its Max.
func rendersEnds(set *statdesc.Set, r statdesc.RangeReading, lo, hi string) bool {
	lo, hi = firstLines(lo, r.Lines), firstLines(hi, r.Lines)
	seenLo, seenHi := false, false
	for mask := range 1 << len(r.Stats) {
		stats := make(map[string]int32, len(r.Stats))
		for i, st := range r.Stats {
			stats[st] = r.Min[i]
			if mask&(1<<i) != 0 {
				stats[st] = r.Max[i]
			}
		}
		text, _ := renderStats(set, statdesc.Description{Stats: r.Stats}, stats)
		seenLo = seenLo || text == lo
		seenHi = seenHi || text == hi
	}
	return seenLo && seenHi
}

func firstLines(text string, n int) string {
	lines := strings.Split(text, "\n")
	return strings.Join(lines[:min(n, len(lines))], "\n")
}

// joinRanges writes lo with each number that differs in hi as "(lo-hi)"; texts with another
// shape are returned as lo.
func joinRanges(lo, hi string) string {
	a, b := tokens(lo), tokens(hi)
	if len(a) != len(b) {
		return lo
	}
	var out strings.Builder
	for i := range a {
		switch {
		case a[i] == b[i]:
			out.WriteString(a[i])
		case statdesc.NumberLen(a[i]) > 0 && statdesc.NumberLen(b[i]) > 0:
			fmt.Fprintf(&out, "(%s-%s)", a[i], b[i])
		default:
			return lo
		}
	}
	return out.String()
}

// tokens splits s into numbers, as the reader takes them, and single other bytes.
func tokens(s string) []string {
	var out []string
	for len(s) > 0 {
		n := max(statdesc.NumberLen(s), 1)
		out = append(out, s[:n])
		s = s[n:]
	}
	return out
}

// TestReadForms reads one real line of each form an item shows: value ranges, several stats on
// a line, the negated ("reduced") variant, a description with reminder text, and lines more than
// one description or variant renders, which keep every reading.
func TestReadForms(t *testing.T) {
	t.Parallel()
	set, _ := realText(t)
	rr := func(stats []string, minMax ...int32) statdesc.RangeReading {
		n := len(stats)
		return statdesc.RangeReading{Stats: stats, Min: minMax[:n], Max: minMax[n:], Lines: 1}
	}
	global := []string{"global_minimum_added_physical_damage", "global_maximum_added_physical_damage"}
	local := []string{"local_minimum_added_physical_damage", "local_maximum_added_physical_damage"}
	markers := []string{"heist_coins_from_monsters_+%"}
	conquered := statdesc.RangeReading{
		Stats: []string{
			"local_unique_jewel_alternate_tree_version", "local_unique_jewel_alternate_tree_seed",
			"local_unique_jewel_alternate_tree_keystone", "local_unique_jewel_alternate_tree_internal_revision",
		},
		Min: []int32{1, 180, 1, 0}, Max: []int32{1, 180, 1, 0}, Lines: 2,
	}
	for name, testCase := range map[string]struct {
		lines []string
		want  []statdesc.RangeReading
	}{
		"range":                 {[]string{"+(70-79) to maximum Life"}, []statdesc.RangeReading{rr([]string{"base_maximum_life"}, 70, 79)}},
		"several stats":         {[]string{"Adds 37 to 72 Physical Damage"}, []statdesc.RangeReading{rr(global, 37, 72, 37, 72), rr(local, 37, 72, 37, 72)}},
		"several stats, ranges": {[]string{"Adds (5-7) to (10-12) Physical Damage"}, []statdesc.RangeReading{rr(global, 5, 10, 7, 12), rr(local, 5, 10, 7, 12)}},
		"negated":               {[]string{"120% reduced Rogue's Markers dropped by monsters"}, []statdesc.RangeReading{rr(markers, -120, -120)}},
		"negated range":         {[]string{"(10-15)% reduced Rogue's Markers dropped by monsters"}, []statdesc.RangeReading{rr(markers, -15, -10)}},
		"reminder, two lines": {
			[]string{"Bathed in the blood of 180 sacrificed in the name of Xibaqua", "Passives in radius are Conquered by the Vaal"},
			[]statdesc.RangeReading{conquered},
		},
		// Two variants of one description read the line; each keeps its own value.
		"ambiguous within a description": {[]string{"Area contains Einhar"}, []statdesc.RangeReading{
			rr([]string{"display_map_mission_id"}, 1, 1), rr([]string{"display_map_mission_id"}, 2, 2),
		}},
		"ambiguous within a description, range": {
			[]string{"Socketed Gems have +(3.5-4.5)% Critical Strike Chance"},
			[]statdesc.RangeReading{rr([]string{"local_display_socketed_gems_additional_critical_strike_chance_%"}, 350, 450)},
		},
		// 0 renders nothing, so the shown "0" is the smallest value that rounds to it.
		"shown 0": {[]string{"Lose 0% Life per Second per Minion"}, []statdesc.RangeReading{rr([]string{"life_degeneration_permyriad_per_minute_per_minion"}, 1, 1)}},
		"ambiguous, every reading": {[]string{"+12% chance to Suppress Spell Damage"}, []statdesc.RangeReading{
			rr([]string{"base_spell_suppression_chance_150%_of_value"}, 8, 8), rr([]string{"base_spell_suppression_chance_%"}, 12, 12),
		}},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got, err := set.ReadRange(mainScope, testCase.lines)
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(testCase.want, got); diff != "" {
				t.Errorf("ReadRange (-want +got):\n%s", diff)
			}
		})
	}
}
