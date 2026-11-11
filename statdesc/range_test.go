package statdesc_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/xaroth/pbt-engine/statdesc"
)

func TestReadRange(t *testing.T) {
	t.Parallel()
	s := testSet(t)
	rr := func(lines int, stats []string, minMax ...int32) statdesc.RangeReading {
		n := len(stats)
		return statdesc.RangeReading{Stats: stats, Min: minMax[:n], Max: minMax[n:], Lines: lines}
	}
	life := []string{"life_+%"}
	fire := []string{"min_fire", "max_fire"}
	str := []string{"strength"}
	for _, testCase := range []struct {
		name  string
		scope string
		lines []string
		want  []statdesc.RangeReading
	}{
		{name: "range", scope: "parent", lines: []string{"(10-20)% increased maximum Life"}, want: []statdesc.RangeReading{rr(1, life, 10, 20)}},
		{name: "reduced range", scope: "parent", lines: []string{"(10-20)% reduced maximum Life"}, want: []statdesc.RangeReading{rr(1, life, -20, -10)}},
		{name: "range crossing 0 has no description", scope: "parent", lines: []string{"(-10-10)% increased maximum Life"}, want: []statdesc.RangeReading{}},
		{name: "two ranges", scope: "parent", lines: []string{"Adds (5-7) to (10-12) Fire Damage"}, want: []statdesc.RangeReading{rr(1, fire, 5, 10, 7, 12)}},
		{name: "two stats, one range", scope: "parent", lines: []string{"Adds 5 to (10-12) Fire Damage"}, want: []statdesc.RangeReading{rr(1, fire, 5, 10, 5, 12)}},
		{name: "plus before the range", scope: "parent", lines: []string{"+(3-5) to Strength"}, want: []statdesc.RangeReading{rr(1, str, 3, 5)}},
		{name: "plus inside the range", scope: "parent", lines: []string{"(+3-+5) to Strength"}, want: []statdesc.RangeReading{rr(1, str, 3, 5)}},
		{name: "no range", scope: "parent", lines: []string{"+5 to Strength"}, want: []statdesc.RangeReading{rr(1, str, 5, 5)}},
		{name: "not a range", scope: "parent", lines: []string{"(5-) to Strength"}, want: []statdesc.RangeReading{}},
		{
			name: "decimal range over two lines", scope: "child", lines: []string{"(1.5-2) second Cooldown", "Can be used once"},
			want: []statdesc.RangeReading{rr(2, []string{"cooldown"}, 1500, 2000)},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			got, err := s.ReadRange(testCase.scope, testCase.lines)
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(testCase.want, got); diff != "" {
				t.Errorf("ReadRange (-want +got):\n%s", diff)
			}
		})
	}
}
