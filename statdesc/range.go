package statdesc

import (
	"slices"
	"strings"
)

// RangeReading is one way lines whose numbers may be ranges, as "(38-43)%", render from
// stats: each stat's values at the low and the high end.
type RangeReading struct {
	// Stats are the description's stats, in its order.
	Stats []string
	// Min and Max are the stats' values at the two ends, Min[i] <= Max[i]. A stat whose
	// number is not a range, or not shown, has Min equal to Max.
	Min, Max []int32
	// Lines is how many input lines the description spans.
	Lines int
}

// ReadRange returns the readings of lines as Read does, where any number may be written as a
// range "(low-high)", such as "Adds (5-7) to (10-12) Fire Damage". The lines at the low ends
// and at the high ends are read, and a reading is kept when both ends read to its stats over
// the same lines; ranges whose ends render under different descriptions, such as one that
// crosses 0 on an "increased" line, have none. Lines without a range read as with Read, with
// Min equal to Max.
func (s *Set) ReadRange(scopeName string, lines []string) ([]RangeReading, error) {
	low, high, ranged := splitRanges(lines)
	lo, err := s.Read(scopeName, low)
	if err != nil {
		return nil, err
	}
	hi := lo
	if ranged {
		if hi, err = s.Read(scopeName, high); err != nil {
			return nil, err
		}
	}
	out := make([]RangeReading, 0, len(lo))
	for _, a := range lo {
		i := slices.IndexFunc(hi, func(b Reading) bool { return b.Lines == a.Lines && slices.Equal(b.Stats, a.Stats) })
		if i < 0 {
			continue
		}
		b := hi[i]
		r := RangeReading{Stats: a.Stats, Min: make([]int32, len(a.Stats)), Max: make([]int32, len(a.Stats)), Lines: a.Lines}
		for k := range a.Stats {
			// A "reduced" line's high end is the more negative value.
			r.Min[k], r.Max[k] = min(a.Values[k], b.Values[k]), max(a.Values[k], b.Values[k])
		}
		out = append(out, r)
	}
	return out, nil
}

// splitRanges returns lines with each range "(low-high)" replaced by its low end and by its
// high end, and whether any line had a range.
func splitRanges(lines []string) ([]string, []string, bool) {
	low, high := make([]string, len(lines)), make([]string, len(lines))
	ranged := false
	for i, l := range lines {
		var lo, hi strings.Builder
		for j := 0; j < len(l); {
			if a, b, n := rangeAt(l[j:]); n > 0 {
				lo.WriteString(a)
				hi.WriteString(b)
				j += n
				ranged = true
				continue
			}
			lo.WriteByte(l[j])
			hi.WriteByte(l[j])
			j++
		}
		low[i], high[i] = lo.String(), hi.String()
	}
	return low, high, ranged
}

// rangeAt returns the ends of the range "(a-b)" at the start of s and its length, or 0.
func rangeAt(s string) (string, string, int) {
	if !strings.HasPrefix(s, "(") {
		return "", "", 0
	}
	a := numberLen(s[1:])
	if a == 0 || 1+a >= len(s) || s[1+a] != '-' {
		return "", "", 0
	}
	b := numberLen(s[2+a:])
	if b == 0 || 2+a+b >= len(s) || s[2+a+b] != ')' {
		return "", "", 0
	}
	return s[1 : 1+a], s[2+a : 2+a+b], 3 + a + b
}
