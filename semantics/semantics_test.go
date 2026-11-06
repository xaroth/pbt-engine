package semantics_test

import (
	"encoding/json"
	"math"
	"os"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/semantics"
	"github.com/xaroth/pbt-engine/statdesc"
)

var anyValue = []statdesc.Limit{{Min: math.MinInt32, Max: math.MaxInt32}}

func desc(text string, stats ...string) statdesc.Description {
	limits := make([]statdesc.Limit, len(stats))
	for i := range limits {
		limits[i] = anyValue[0]
	}
	return statdesc.Description{Stats: stats, Variants: []statdesc.Variant{{Limits: limits, Text: text}}}
}

func pobMod(name, typ string, value float64, tags ...string) semantics.PoBMod {
	m := semantics.PoBMod{Name: name, Type: typ, Value: &value}
	for _, t := range tags {
		m.Tags = append(m.Tags, json.RawMessage(t))
	}
	return m
}

// testInput has one stat per case, each on its own node unless the case needs more.
func testInput(t *testing.T) semantics.Input {
	t.Helper()
	ids := []string{
		"life", "regen_per_minute", "unparsed", "conditional", "global_es", "pair_a", "pair_b",
		"disagree", "zero", "not_sheet", "hidden", "overridden",
	}
	stats := make([]dataset.Stat, len(ids))
	index := map[string]uint32{}
	for i, id := range ids {
		stats[i] = dataset.Stat{ID: id}
		index[id] = uint32(i)
	}
	sv := func(id string, v int32) dataset.StatValue { return dataset.StatValue{Stat: index[id], Value: v} }
	set, err := statdesc.NewSet([]statdesc.Scope{{Name: semantics.PassiveScope, Descriptions: []statdesc.Description{
		desc("+{0} to maximum Life", "life"),
		desc("Regenerate {0} Life per minute", "regen_per_minute"),
		desc("Gain {0} wishes", "unparsed"),
		desc("{0}% increased Evasion while moving", "conditional"),
		desc("{0}% increased maximum Energy Shield", "global_es"),
		desc("{0} to {1} added Armour", "pair_a", "pair_b"),
		desc("+{0} to Mana", "disagree"),
		desc("Zero {0}", "zero"),
		desc("{0}% increased Damage", "not_sheet"),
		desc("Overridden {0}", "overridden"),
	}}})
	if err != nil {
		t.Fatal(err)
	}
	return semantics.Input{
		Game: "poe1", Version: "3.29.3.3", PoB: "abc",
		Stats: stats,
		Tree: &dataset.Tree{Variant: "standard", Nodes: []dataset.Node{
			{ID: 1, Stats: []dataset.StatValue{sv("life", 10)}},
			{ID: 2, Stats: []dataset.StatValue{sv("life", 20)}, MasteryEffects: []dataset.MasteryEffect{{ID: 1, Stats: []dataset.StatValue{sv("regen_per_minute", 60)}}}},
			{ID: 3, Stats: []dataset.StatValue{sv("regen_per_minute", 120), sv("unparsed", 1), sv("conditional", 5), sv("global_es", 8)}},
			{ID: 4, Stats: []dataset.StatValue{sv("pair_a", 1), sv("pair_b", 2)}},
			{ID: 5, Stats: []dataset.StatValue{sv("disagree", 10), sv("zero", 0), sv("not_sheet", 7), sv("hidden", 3)}},
			{ID: 6, Stats: []dataset.StatValue{sv("disagree", 20), sv("overridden", 1)}},
			{ID: 7, Stats: []dataset.StatValue{sv("life", 35)}}, // not cached; matched by template
		}},
		Text: set,
		ModCache: []semantics.PoBLine{
			{Text: "+10 to maximum Life", Mods: []semantics.PoBMod{pobMod("Life", "BASE", 10)}},
			{Text: "+20 to maximum Life", Mods: []semantics.PoBMod{pobMod("Life", "BASE", 20)}},
			{Text: "Regenerate 60 Life per minute", Mods: []semantics.PoBMod{pobMod("LifeRegen", "BASE", 1)}},
			{Text: "Regenerate 120 Life per minute", Mods: []semantics.PoBMod{pobMod("LifeRegen", "BASE", 2)}},
			{Text: "Gain 1 wishes", Unparsed: "Gain 1 wishes "},
			{Text: "5% increased Evasion while moving", Mods: []semantics.PoBMod{pobMod("Evasion", "INC", 5, `{"type":"Condition","var":"Moving"}`)}},
			{Text: "8% increased maximum Energy Shield", Mods: []semantics.PoBMod{pobMod("EnergyShield", "INC", 8, `{"type":"Global"}`)}},
			{Text: "1 to 2 added Armour", Mods: []semantics.PoBMod{pobMod("Armour", "BASE", 1)}},
			{Text: "+10 to Mana", Mods: []semantics.PoBMod{pobMod("Mana", "BASE", 10)}},
			{Text: "+20 to Mana", Mods: []semantics.PoBMod{pobMod("Mana", "BASE", 30)}},
			{Text: "7% increased Damage", Mods: []semantics.PoBMod{pobMod("Damage", "INC", 7)}},
			{Text: "Overridden 1", Mods: []semantics.PoBMod{pobMod("Life", "MORE", 1)}},
		},
		Overrides: []semantics.Entry{
			{Stat: "overridden", Mods: []semantics.Mod{{Stat: semantics.Life, Op: semantics.OpBase, Mul: 2, Div: 1}}, Reason: "test"},
			{Stat: "level", Reason: "read by rules"},
		},
	}
}

func TestGenerate(t *testing.T) {
	t.Parallel()
	got, err := semantics.Generate(testInput(t))
	if err != nil {
		t.Fatal(err)
	}
	want := []semantics.Entry{
		{Stat: "conditional", Status: semantics.Unknown, Sheet: true, Uses: 1, Reason: "PoB modifier Evasion INC has a Condition tag"},
		{Stat: "disagree", Status: semantics.Unknown, Sheet: true, Uses: 2, Reason: "lines disagree on the meaning: mana base 1/1; | mana base 3/2;"},
		{Stat: "global_es", Status: semantics.Generated, Sheet: true, Uses: 1, Mods: []semantics.Mod{{Stat: semantics.EnergyShield, Op: semantics.OpInc, Mul: 1, Div: 1}}},
		{Stat: "hidden", Status: semantics.Unknown, Uses: 1, Reason: "no description line"},
		{Stat: "level", Status: semantics.Override, Reason: "read by rules"},
		{Stat: "life", Status: semantics.Generated, Sheet: true, Uses: 3, Mods: []semantics.Mod{{Stat: semantics.Life, Op: semantics.OpBase, Mul: 1, Div: 1}}},
		{Stat: "not_sheet", Status: semantics.NotSheet, Uses: 1},
		{Stat: "overridden", Status: semantics.Override, Uses: 1, Reason: "test", Mods: []semantics.Mod{{Stat: semantics.Life, Op: semantics.OpBase, Mul: 2, Div: 1}}},
		{Stat: "pair_a", Status: semantics.Unknown, Sheet: true, Uses: 1, Reason: "shares a line with pair_b"},
		{Stat: "pair_b", Status: semantics.Unknown, Sheet: true, Uses: 1, Reason: "shares a line with pair_a"},
		{Stat: "regen_per_minute", Status: semantics.Generated, Sheet: true, Uses: 2, Mods: []semantics.Mod{{Stat: semantics.LifeRegen, Op: semantics.OpBase, Mul: 1, Div: 60}}},
		{Stat: "unparsed", Status: semantics.Unknown, Uses: 1, Reason: `PoB does not parse "Gain 1 wishes"`},
		{Stat: "zero", Status: semantics.NotSheet, Uses: 1, Reason: "only appears with value 0"},
	}
	if diff := cmp.Diff(want, got.Entries); diff != "" {
		t.Errorf("entries (-want +got):\n%s", diff)
	}
	var sheet []string
	for _, e := range got.SheetOnly().Entries {
		sheet = append(sheet, e.Stat)
	}
	if diff := cmp.Diff([]string{"conditional", "disagree", "global_es", "level", "life", "overridden", "pair_a", "pair_b", "regen_per_minute"}, sheet); diff != "" {
		t.Errorf("SheetOnly (-want +got):\n%s", diff)
	}
	if e, ok := got.Lookup("life"); !ok || e.Status != semantics.Generated {
		t.Errorf("Lookup(life): got %+v, %v, want generated", e, ok)
	}
	if _, ok := got.Lookup("nope"); ok {
		t.Error("Lookup(nope): got found, want not found")
	}

	report := semantics.Coverage(got)
	for _, line := range []string{
		"| generated | 3 | 6 |",
		"| unknown, feeds the sheet | 4 | 5 |",
		"| `regen_per_minute` | generated | 2 | base life_regen x1/60 |",
		"| `pair_a` | 1 | shares a line with pair_b |",
		"| PoB does not parse | 1 |",
		"| no description line | 1 |",
	} {
		if !strings.Contains(report, line+"\n") {
			t.Errorf("report: got:\n%s\nwant line %q", report, line)
		}
	}
}

func TestGenerateErrors(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name string
		edit func(in *semantics.Input)
	}{
		{"override without reason", func(in *semantics.Input) { in.Overrides[0].Reason = "" }},
		{"override twice", func(in *semantics.Input) { in.Overrides = append(in.Overrides, in.Overrides[0]) }},
		{"override with unknown engine stat", func(in *semantics.Input) { in.Overrides[0].Mods[0].Stat = "nope" }},
		{"override with zero div", func(in *semantics.Input) { in.Overrides[0].Mods[0].Div = 0 }},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			in := testInput(t)
			testCase.edit(&in)
			if _, err := semantics.Generate(in); err == nil {
				t.Error("got nil error, want error")
			}
		})
	}
}

func TestEncodeLoad(t *testing.T) {
	t.Parallel()
	table, err := semantics.Generate(testInput(t))
	if err != nil {
		t.Fatal(err)
	}
	data, err := table.Encode()
	if err != nil {
		t.Fatal(err)
	}
	got, err := semantics.Load(data)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(table, got); diff != "" {
		t.Errorf("Load(Encode) (-want +got):\n%s", diff)
	}
}

func TestLoadErrors(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name, data string
	}{
		{"not json", `{`},
		{"unknown field", `{"format":1,"x":1}`},
		{"other format", `{"format":2}`},
		{"unsorted", `{"format":1,"entries":[{"stat":"b","status":"not-sheet"},{"stat":"a","status":"not-sheet"}]}`},
		{"repeated", `{"format":1,"entries":[{"stat":"a","status":"not-sheet"},{"stat":"a","status":"not-sheet"}]}`},
		{"unknown status", `{"format":1,"entries":[{"stat":"a","status":"maybe"}]}`},
		{"mods on unknown", `{"format":1,"entries":[{"stat":"a","status":"unknown","mods":[{"stat":"life","op":"base","mul":1,"div":1}]}]}`},
		{"unknown op", `{"format":1,"entries":[{"stat":"a","status":"generated","mods":[{"stat":"life","op":"less","mul":1,"div":1}]}]}`},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			if _, err := semantics.Load([]byte(testCase.data)); err == nil {
				t.Error("got nil error, want error")
			}
		})
	}
}

// TestCommittedTables checks the generated tables in the repository load and carry the
// stats the sheet needs most.
func TestCommittedTables(t *testing.T) {
	t.Parallel()
	for _, version := range []string{"3.28.0.16", "3.29.3.3"} {
		t.Run(version, func(t *testing.T) {
			t.Parallel()
			data, err := os.ReadFile("poe1/" + version + ".json")
			if err != nil {
				t.Fatal(err)
			}
			table, err := semantics.Load(data)
			if err != nil {
				t.Fatal(err)
			}
			for _, stat := range []string{"base_maximum_life", "maximum_life_+%", "base_strength", "base_resist_all_elements_%", "life_per_level"} {
				if e, ok := table.Lookup(stat); !ok || (e.Status != semantics.Generated && e.Status != semantics.Override) {
					t.Errorf("%s: got %+v, want generated or override", stat, e)
				}
			}
		})
	}
}

func TestModValue(t *testing.T) {
	t.Parallel()
	m := semantics.Mod{Mul: 1, Div: 60}
	if got := m.Value(24); got != 0.4 {
		t.Errorf("24 per minute: got %v, want 0.4", got)
	}
}
