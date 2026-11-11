package statdesc_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/xaroth/pbt-engine/statdesc"
)

// Each handler's display is read back to the value; a handler that rounds reads back the value
// nearest the exact inverse.
func TestReadHandlers(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		handler string
		value   int32
		text    string
		read    int32
	}{
		{"negate", 5, "-5", 5},
		{"negate_and_double", 5, "-10", 5},
		{"double", 5, "10", 5},
		{"times_twenty", 3, "60", 3},
		{"times_one_point_five", 4, "6", 4},
		{"30%_of_value", 10, "3", 10},
		{"60%_of_value", 10, "6", 10},
		{"plus_two_hundred", 5, "205", 5},
		{"multiplicative_damage_modifier", 20, "120", 20},
		{"divide_by_three", 10, "3.3333333", 10},
		{"divide_by_four", 10, "2.5", 10},
		{"divide_by_five", 12, "2.4", 12},
		{"divide_by_six", 9, "1.5", 9},
		{"divide_by_twelve", 6, "0.5", 6},
		{"divide_by_twenty", 30, "1.5", 30},
		{"divide_by_one_hundred", 25, "0.25", 25},
		{"divide_by_one_thousand", 1500, "1.5", 1500},
		{"divide_by_two_0dp", 7, "4", 8},
		{"divide_by_ten_0dp", 34, "3", 30},
		{"divide_by_fifteen_0dp", 31, "2", 30},
		{"divide_by_ten_1dp", 20, "2.0", 20},
		{"divide_by_ten_1dp_if_required", 20, "2", 20},
		{"divide_by_one_hundred_2dp", 150, "1.50", 150},
		{"divide_by_one_hundred_2dp_if_required", 150, "1.5", 150},
		{"divide_by_one_hundred_and_negate", 25, "-0.25", 25},
		{"divide_by_twenty_then_double_0dp", 50, "6", 60},
		{"per_minute_to_per_second", 100, "1.7", 102},
		{"per_minute_to_per_second_0dp", 130, "2", 120},
		{"per_minute_to_per_second_1dp", 60, "1.0", 60},
		{"per_minute_to_per_second_2dp", 61, "1.02", 61},
		{"per_minute_to_per_second_2dp_if_required", 90, "1.5", 90},
		{"permyriad_per_minute_to_%_per_second", 6000, "1", 6000},
		{"milliseconds_to_seconds", 1500, "1.5", 1500},
		{"milliseconds_to_seconds_0dp", 2400, "2", 2000},
		{"milliseconds_to_seconds_1dp", 1500, "1.5", 1500},
		{"milliseconds_to_seconds_2dp", 1500, "1.50", 1500},
		{"milliseconds_to_seconds_2dp_if_required", 1250, "1.25", 1250},
		{"deciseconds_to_seconds", 15, "1.5", 15},
		{"locations_to_metres", 3, "0.3", 3},
	}
	covered := make([]string, 0, len(testCases))
	for _, testCase := range testCases {
		covered = append(covered, testCase.handler)
		t.Run(testCase.handler, func(t *testing.T) {
			t.Parallel()
			s := set(t, one("x", "{0} x", statdesc.Handler{Name: testCase.handler}))
			lines, err := s.Describe("s", map[string]int32{"x": testCase.value})
			if err != nil {
				t.Fatal(err)
			}
			if len(lines) != 1 || lines[0].Text != testCase.text+" x" {
				t.Fatalf("Describe %v, want %q", lines, testCase.text+" x")
			}
			want := []statdesc.Reading{{Stats: []string{"x"}, Values: []int32{testCase.read}, Lines: 1}}
			if diff := cmp.Diff(want, read(t, s, "s", lines[0].Text)); diff != "" {
				t.Errorf("Read (-want +got):\n%s", diff)
			}
		})
	}
	slices.Sort(covered)
	if diff := cmp.Diff(statdesc.HandlerNames(), covered); diff != "" {
		t.Errorf("handlers without a case (-handlers +cases):\n%s", diff)
	}
}

func set(t *testing.T, descs ...statdesc.Description) *statdesc.Set {
	t.Helper()
	s, err := statdesc.NewSet([]statdesc.Scope{{Name: "s", Descriptions: descs}})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func read(t *testing.T, s *statdesc.Set, scope string, lines ...string) []statdesc.Reading {
	t.Helper()
	got, err := s.Read(scope, lines)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestRead(t *testing.T) {
	t.Parallel()
	s := testSet(t)
	reading := func(lines int, stats []string, values ...int32) statdesc.Reading {
		return statdesc.Reading{Stats: stats, Values: values, Lines: lines}
	}
	life := []string{"life_+%"}
	testCases := []struct {
		name  string
		scope string
		lines []string
		want  []statdesc.Reading
	}{
		{name: "increased", scope: "parent", lines: []string{"10% increased maximum Life"}, want: []statdesc.Reading{reading(1, life, 10)}},
		{name: "reduced variant negates", scope: "parent", lines: []string{"10% reduced maximum Life"}, want: []statdesc.Reading{reading(1, life, -10)}},
		{name: "limits pick the variant", scope: "parent", lines: []string{"-10% increased maximum Life"}, want: nil},
		{
			name: "two stats", scope: "parent", lines: []string{"Adds 37 to 72 Fire Damage"},
			want: []statdesc.Reading{reading(1, []string{"min_fire", "max_fire"}, 37, 72)},
		},
		{
			name: "tag and a value the text does not show", scope: "parent", lines: []string{"Gain a Charges"},
			want: []statdesc.Reading{reading(1, []string{"charges"}, 1)},
		},
		{name: "next placeholder", scope: "parent", lines: []string{"Gain 3 Charges"}, want: []statdesc.Reading{reading(1, []string{"charges"}, 3)}},
		{name: "plus flag", scope: "parent", lines: []string{"+5 to Strength"}, want: []statdesc.Reading{reading(1, []string{"strength"}, 5)}},
		{name: "plus flag with a negative value", scope: "parent", lines: []string{"-5 to Strength"}, want: []statdesc.Reading{reading(1, []string{"strength"}, -5)}},
		{name: "plus flag missing", scope: "parent", lines: []string{"5 to Strength"}, want: nil},
		{name: "decimals where the handler shows none", scope: "parent", lines: []string{"+5.5 to Strength"}, want: nil},
		{name: "later description of a stat wins", scope: "parent", lines: []string{"second"}, want: []statdesc.Reading{reading(1, []string{"dup"}, 1)}},
		{name: "earlier description of a stat never renders", scope: "parent", lines: []string{"first"}, want: nil},
		{name: "unknown handler", scope: "parent", lines: []string{"5 odd"}, want: nil},
		{
			name: "parent description through the child", scope: "child", lines: []string{"+5 to Strength"},
			want: []statdesc.Reading{reading(1, []string{"strength"}, 5)},
		},
		{name: "child overrides parent", scope: "child", lines: []string{"parent text"}, want: nil},
		{name: "child text", scope: "child", lines: []string{"child text"}, want: []statdesc.Reading{reading(1, []string{"hidden"}, 1)}},
		{
			name: "two lines", scope: "child", lines: []string{"1.5 second Cooldown", "Can be used once", "+5 to Strength"},
			want: []statdesc.Reading{reading(2, []string{"cooldown"}, 1500)},
		},
		{name: "second line alone", scope: "child", lines: []string{"Can be used once"}, want: nil},
		{name: "first line alone", scope: "child", lines: []string{"1.5 second Cooldown"}, want: nil},
		{name: "no match", scope: "parent", lines: []string{"+5 to Dexterity"}, want: nil},
		{name: "no lines", scope: "parent", lines: nil, want: nil},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			if diff := cmp.Diff(testCase.want, read(t, s, testCase.scope, testCase.lines...)); diff != "" {
				t.Errorf("Read (-want +got):\n%s", diff)
			}
		})
	}
}

// Two descriptions render the same text: both readings come back, in the order the game
// prefers.
func TestReadAmbiguous(t *testing.T) {
	t.Parallel()
	cooldown := one("cooldown_short", "{0} second Cooldown", statdesc.Handler{Name: "milliseconds_to_seconds"})
	twoLines := one("cooldown_long", "{0} second Cooldown\\nCan be used once", statdesc.Handler{Name: "milliseconds_to_seconds"})
	parent := statdesc.Scope{Name: "parent", Descriptions: []statdesc.Description{
		one("parent_armour", "+{0} to Armour"),
	}}
	child := statdesc.Scope{Name: "child", Parent: "parent", Descriptions: []statdesc.Description{
		one("global_armour", "+{0} to Armour"),
		one("local_armour", "+{0} to Armour"),
		cooldown, twoLines,
	}}
	s, err := statdesc.NewSet([]statdesc.Scope{parent, child})
	if err != nil {
		t.Fatal(err)
	}
	for name, testCase := range map[string]struct {
		lines []string
		want  []statdesc.Reading
	}{
		"scope position, then parent": {
			lines: []string{"+20 to Armour"},
			want: []statdesc.Reading{
				{Stats: []string{"global_armour"}, Values: []int32{20}, Lines: 1},
				{Stats: []string{"local_armour"}, Values: []int32{20}, Lines: 1},
				{Stats: []string{"parent_armour"}, Values: []int32{20}, Lines: 1},
			},
		},
		"more lines first": {
			lines: []string{"2 second Cooldown", "Can be used once"},
			want: []statdesc.Reading{
				{Stats: []string{"cooldown_long"}, Values: []int32{2000}, Lines: 2},
				{Stats: []string{"cooldown_short"}, Values: []int32{2000}, Lines: 1},
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if diff := cmp.Diff(testCase.want, read(t, s, "child", testCase.lines...)); diff != "" {
				t.Errorf("Read (-want +got):\n%s", diff)
			}
		})
	}
}

func TestReadUnknownScope(t *testing.T) {
	t.Parallel()
	if _, err := testSet(t).Read("missing", []string{"x"}); !errors.Is(err, statdesc.ErrUnknownScope) {
		t.Errorf("Read(missing scope): got %v, want ErrUnknownScope", err)
	}
}
