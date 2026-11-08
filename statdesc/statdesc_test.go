package statdesc_test

import (
	"errors"
	"math"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/xaroth/pbt-engine/statdesc"
)

var anyValue = statdesc.Limit{Min: math.MinInt32, Max: math.MaxInt32}

func atLeast(n int32) statdesc.Limit { return statdesc.Limit{Min: n, Max: math.MaxInt32} }
func atMost(n int32) statdesc.Limit  { return statdesc.Limit{Min: math.MinInt32, Max: n} }

// one is a single-stat description with one variant matching any value.
func one(stat, text string, handlers ...statdesc.Handler) statdesc.Description {
	return statdesc.Description{
		Stats:    []string{stat},
		Variants: []statdesc.Variant{{Limits: []statdesc.Limit{anyValue}, Text: text, Handlers: handlers}},
	}
}

func testSet(t *testing.T) *statdesc.Set {
	t.Helper()
	parent := statdesc.Scope{Name: "parent", Descriptions: []statdesc.Description{
		{
			Stats: []string{"life_+%"},
			Variants: []statdesc.Variant{
				{Limits: []statdesc.Limit{atLeast(1)}, Text: "{0}% increased maximum Life"},
				{Limits: []statdesc.Limit{atMost(-1)}, Text: "{0}% reduced maximum Life", Handlers: []statdesc.Handler{{Name: "negate"}}},
			},
		},
		{
			Stats: []string{"min_fire", "max_fire"},
			Variants: []statdesc.Variant{
				{Limits: []statdesc.Limit{anyValue, anyValue}, Text: "Adds {0} to {1} Fire Damage"},
			},
		},
		{
			Stats: []string{"charges"},
			Variants: []statdesc.Variant{
				{Limits: []statdesc.Limit{{Min: 1, Max: 1}}, Text: "Gain a [Charge|Charges]"},
				{Limits: []statdesc.Limit{{Min: 0, Not: true}}, Text: "Gain {} [Charges]"},
			},
		},
		one("strength", "{0:+d} to Strength"),
		one("hidden", "parent text"),
		{Stats: []string{"no_text"}},
		one("dup", "first"),
		one("dup", "second"),
		one("odd", "{0} odd", statdesc.Handler{Name: "made_up"}),
	}}
	child := statdesc.Scope{Name: "child", Parent: "parent", Descriptions: []statdesc.Description{
		one("hidden", "child text"),
		one("range", "{0:+d} metres to Melee Strike Range", statdesc.Handler{Name: "locations_to_metres"}),
		one("regen", "Regenerate {0}% of Life per second", statdesc.Handler{Name: "per_minute_to_per_second_2dp"}),
		one("regen_short", "Regenerate {0}% of Life per second", statdesc.Handler{Name: "per_minute_to_per_second_2dp_if_required"}),
		one("cooldown", "{0} second Cooldown\\nCan be used once", statdesc.Handler{Name: "milliseconds_to_seconds"}),
		one("reminder", "Shocks", statdesc.Handler{Name: "reminderstring"}),
		one("double", "{0}", statdesc.Handler{Name: "divide_by_twenty_then_double_0dp"}),
	}}
	s, err := statdesc.NewSet([]statdesc.Scope{child, parent})
	if err != nil {
		t.Fatalf("NewSet: %v", err)
	}
	return s
}

func TestDescribe(t *testing.T) {
	t.Parallel()
	s := testSet(t)
	testCases := []struct {
		name  string
		scope string
		stats map[string]int32
		want  []string
	}{
		{name: "increased", scope: "parent", stats: map[string]int32{"life_+%": 10}, want: []string{"10% increased maximum Life"}},
		{name: "negated", scope: "parent", stats: map[string]int32{"life_+%": -10}, want: []string{"10% reduced maximum Life"}},
		{name: "zero is not described", scope: "parent", stats: map[string]int32{"life_+%": 0}, want: nil},
		{name: "two stats, one missing", scope: "parent", stats: map[string]int32{"max_fire": 9}, want: []string{"Adds 0 to 9 Fire Damage"}},
		{name: "exact limit and markup", scope: "parent", stats: map[string]int32{"charges": 1}, want: []string{"Gain a Charges"}},
		{name: "not limit and next placeholder", scope: "parent", stats: map[string]int32{"charges": 3}, want: []string{"Gain 3 Charges"}},
		{name: "plus sign", scope: "parent", stats: map[string]int32{"strength": 5}, want: []string{"+5 to Strength"}},
		{name: "minus sign", scope: "parent", stats: map[string]int32{"strength": -5}, want: []string{"-5 to Strength"}},
		{name: "no text", scope: "parent", stats: map[string]int32{"no_text": 1}, want: nil},
		{name: "later duplicate wins", scope: "parent", stats: map[string]int32{"dup": 1}, want: []string{"second"}},
		{name: "unknown stat", scope: "parent", stats: map[string]int32{"nope": 1}, want: nil},
		{name: "child overrides parent", scope: "child", stats: map[string]int32{"hidden": 1}, want: []string{"child text"}},
		{
			name: "parent lines first", scope: "child", stats: map[string]int32{"range": 3, "strength": 1},
			want: []string{"+1 to Strength", "+0.3 metres to Melee Strike Range"},
		},
		{name: "fixed decimals", scope: "child", stats: map[string]int32{"regen": 60}, want: []string{"Regenerate 1.00% of Life per second"}},
		{name: "decimals if required", scope: "child", stats: map[string]int32{"regen_short": 90}, want: []string{"Regenerate 1.5% of Life per second"}},
		{name: "newline", scope: "child", stats: map[string]int32{"cooldown": 1500}, want: []string{"1.5 second Cooldown\nCan be used once"}},
		{name: "reminder only", scope: "child", stats: map[string]int32{"reminder": 1}, want: []string{"Shocks"}},
		{name: "rounded then doubled", scope: "child", stats: map[string]int32{"double": 50}, want: []string{"6"}},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			lines, err := s.Describe(testCase.scope, testCase.stats)
			if err != nil {
				t.Fatalf("Describe: %v", err)
			}
			var got []string
			for _, l := range lines {
				got = append(got, l.Text)
			}
			if diff := cmp.Diff(testCase.want, got); diff != "" {
				t.Errorf("Describe mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestDescribeLineStats(t *testing.T) {
	t.Parallel()
	got, err := testSet(t).Describe("parent", map[string]int32{"min_fire": 1, "max_fire": 2})
	if err != nil {
		t.Fatalf("Describe: %v", err)
	}
	want := []statdesc.Line{{Text: "Adds 1 to 2 Fire Damage", Stats: []string{"min_fire", "max_fire"}}}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("Describe mismatch (-want +got):\n%s", diff)
	}
}

func TestDescribeErrors(t *testing.T) {
	t.Parallel()
	s := testSet(t)
	if _, err := s.Describe("missing", map[string]int32{"x": 1}); err == nil {
		t.Errorf("Describe(missing scope): got nil error, want an error")
	}
	if _, err := s.Describe("parent", map[string]int32{"odd": 1}); !errors.Is(err, statdesc.ErrUnknownHandler) {
		t.Errorf("Describe(unknown handler): got %v, want ErrUnknownHandler", err)
	}
}

func TestNewSetErrors(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name   string
		scopes []statdesc.Scope
	}{
		{name: "scope twice", scopes: []statdesc.Scope{{Name: "a"}, {Name: "a"}}},
		{name: "missing parent", scopes: []statdesc.Scope{{Name: "a", Parent: "b"}}},
		{name: "parent cycle", scopes: []statdesc.Scope{{Name: "a", Parent: "b"}, {Name: "b", Parent: "a"}}},
		{name: "no stats", scopes: []statdesc.Scope{{Name: "a", Descriptions: []statdesc.Description{{}}}}},
		{name: "limit count", scopes: []statdesc.Scope{{Name: "a", Descriptions: []statdesc.Description{{
			Stats: []string{"x", "y"}, Variants: []statdesc.Variant{{Limits: []statdesc.Limit{anyValue}}},
		}}}}},
		{name: "handler stat out of range", scopes: []statdesc.Scope{{Name: "a", Descriptions: []statdesc.Description{
			one("x", "{0}", statdesc.Handler{Name: "negate", Stat: 1}),
		}}}},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			if _, err := statdesc.NewSet(testCase.scopes); err == nil {
				t.Errorf("NewSet: got nil error, want an error")
			}
		})
	}
}

// TestKnownOrderDifferences holds node line orders where GGG's tree export differs from
// description order (5 of about 1500 multi-line standard nodes on 3.29.3.3, same on 3.28.0.16).
// Stat ids and description order are as in passive_skill_stat_descriptions.txt; the rule
// GGG uses is not known. When a change makes one match, move it to TestDescribe.
func TestKnownOrderDifferences(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name   string
		descs  []statdesc.Description
		stats  map[string]int32
		want   []string // description order
		export []string // GGG order
	}{
		{
			name: "critical strike multiplier before chance (nodes 6064, 9014, 28754, 30471)",
			descs: []statdesc.Description{
				one("critical_strike_chance_+%", "{0}% increased Critical Strike Chance"),
				one("base_critical_strike_multiplier_+", "{0:+d}% to Critical Strike Multiplier"),
			},
			stats:  map[string]int32{"critical_strike_chance_+%": 15, "base_critical_strike_multiplier_+": 10},
			want:   []string{"15% increased Critical Strike Chance", "+10% to Critical Strike Multiplier"},
			export: []string{"+10% to Critical Strike Multiplier", "15% increased Critical Strike Chance"},
		},
		{
			name: "spectres before zombies (node 19897)",
			descs: []statdesc.Description{
				one("base_number_of_zombies_allowed", "{0:+d} to Maximum number of Raised Zombies"),
				one("base_number_of_spectres_allowed", "{0:+d} to Maximum number of Spectres"),
				one("base_number_of_skeletons_allowed", "{0:+d} to Maximum number of Skeletons"),
			},
			stats: map[string]int32{"base_number_of_zombies_allowed": 1, "base_number_of_spectres_allowed": 1, "base_number_of_skeletons_allowed": 1},
			want: []string{
				"+1 to Maximum number of Raised Zombies", "+1 to Maximum number of Spectres", "+1 to Maximum number of Skeletons",
			},
			export: []string{
				"+1 to Maximum number of Spectres", "+1 to Maximum number of Raised Zombies", "+1 to Maximum number of Skeletons",
			},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			set, err := statdesc.NewSet([]statdesc.Scope{{Name: "passive", Descriptions: testCase.descs}})
			if err != nil {
				t.Fatal(err)
			}
			lines, err := set.Describe("passive", testCase.stats)
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, l := range lines {
				got = append(got, l.Text)
			}
			if diff := cmp.Diff(testCase.want, got); diff != "" {
				t.Errorf("Describe lines (-want +got):\n%s", diff)
			}
			if cmp.Equal(got, testCase.export) {
				t.Errorf("now matches GGG order %q; move this case to TestDescribe", got)
			}
		})
	}
}
