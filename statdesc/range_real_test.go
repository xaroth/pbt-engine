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
// stat, writes the numbers that differ as ranges, and reads the line back with ReadRange: each
// end must give what Read gives for that end's text. Variants that Read does not read back are
// left out; those that read back alone but not as a range are listed in the golden file.
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
			want, has := endsOf(t, set, d, lo, hi)
			if !has {
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
			case !cmp.Equal(got[k], want):
				failed = append(failed, fmt.Sprintf("%d/%d %s | read as %v | %q", i, j, strings.Join(d.Stats, ","), got[k], text))
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

// endsOf returns what Read gives for d at each end as a range reading, false when either end
// does not read back to d.
func endsOf(t *testing.T, set *statdesc.Set, d statdesc.Description, lo, hi string) (statdesc.RangeReading, bool) {
	t.Helper()
	own := func(text string) (statdesc.Reading, bool) {
		rs, err := set.Read(mainScope, strings.Split(text, "\n"))
		if err != nil {
			t.Fatal(err)
		}
		k := slices.IndexFunc(rs, func(r statdesc.Reading) bool { return slices.Equal(r.Stats, d.Stats) })
		if k < 0 {
			return statdesc.Reading{}, false
		}
		return rs[k], true
	}
	a, okA := own(lo)
	b, okB := own(hi)
	if !okA || !okB {
		return statdesc.RangeReading{}, false
	}
	r := statdesc.RangeReading{Stats: a.Stats, Min: make([]int32, len(a.Stats)), Max: make([]int32, len(a.Stats)), Lines: a.Lines}
	for i := range a.Stats {
		r.Min[i], r.Max[i] = min(a.Values[i], b.Values[i]), max(a.Values[i], b.Values[i])
	}
	return r, true
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
// a line, the negated ("reduced") variant, a description with reminder text, and a line more than
// one description renders, which keeps every reading.
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
