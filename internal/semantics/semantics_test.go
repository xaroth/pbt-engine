package semantics_test

import (
	"errors"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/internal/core"
	"github.com/xaroth/pbt-engine/internal/resolve"
	"github.com/xaroth/pbt-engine/internal/semantics"
	"github.com/xaroth/pbt-engine/statdesc"
)

const descriptions = "Metadata/StatDescriptions/stat_descriptions.txt"

type vocabulary struct{}

func (vocabulary) IsEngineStat(name string) bool {
	return slices.Contains([]string{
		"armour", "energy_shield", "life", "mana", "maximum_cold_resistance", "maximum_fire_resistance",
		"percent_of_life_regenerated_per_second", "strength",
	}, name)
}

func (vocabulary) IsContext(name string) bool {
	return slices.Contains([]string{"type.Attack", "type.Spell", "damage.fire", "damage.cold"}, name)
}

func (vocabulary) RuleParams(name string) ([]string, bool) {
	if name == "gain_as_extra" {
		return []string{"from", "to"}, true
	}
	return nil, false
}

func readFixture(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile("testdata/table.json")
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func load(t *testing.T, data string) *semantics.Table {
	t.Helper()
	tbl, err := semantics.Load([]byte(data), vocabulary{})
	if err != nil {
		t.Fatal(err)
	}
	return tbl
}

func version(t *testing.T, s string) dataset.GameVersion {
	t.Helper()
	if s == "" {
		return dataset.GameVersion{}
	}
	v, err := dataset.ParseGameVersion(s)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestLoad(t *testing.T) {
	t.Parallel()
	one := semantics.Ratio{Num: 1, Den: 1}
	desc := func(text string, handlers ...string) semantics.Source {
		return semantics.Source{Kind: semantics.Description, Ref: descriptions, Text: text, Handlers: handlers}
	}
	statID := semantics.Source{Kind: semantics.StatID}
	regen := []semantics.Mod{{Stat: "percent_of_life_regenerated_per_second", Op: core.OpBase, Factor: semantics.Ratio{Num: 1, Den: 60}}}
	v329 := version(t, "3.29")
	want := &semantics.Table{Game: "poe1", Entries: []semantics.Entry{
		{Stat: "X_to_armour_per_2_strength", Scope: resolve.Global, Status: semantics.Authored,
			Mods:    []semantics.Mod{{Stat: "armour", Op: core.OpBase, Factor: one, Per: &semantics.Per{Stat: "strength", Step: 2}}},
			Sources: []semantics.Source{desc("{0:+d} Armour per 2 Strength")}, Unvalidated: "whole steps"},
		{Stat: "base_maximum_life", Scope: resolve.Global, Status: semantics.Authored,
			Mods:    []semantics.Mod{{Stat: "life", Op: core.OpBase, Factor: one}},
			Sources: []semantics.Source{desc("{0:+d} to maximum Life"), {Kind: semantics.GameData, Ref: "Characters.BaseMaxLife"}}},
		{Stat: "keystone_blood_magic", Scope: resolve.Global, Status: semantics.Authored,
			Mods:    []semantics.Mod{{Stat: "mana", Op: core.OpOverride, Factor: semantics.Ratio{Num: 0, Den: 1}}},
			Sources: []semantics.Source{desc(`Removes all mana\nSkills Cost Life instead of Mana`)}, Omitted: "Skills Cost Life instead of Mana"},
		{Stat: "life_regeneration_rate_per_minute_%", Scope: resolve.Global, Until: v329, Status: semantics.Authored,
			Mods: regen, Sources: []semantics.Source{desc("Regenerate {0}% of Life per second", "per_minute_to_per_second")}},
		{Stat: "life_regeneration_rate_per_minute_%", Scope: resolve.Global, From: v329, Status: semantics.Authored,
			Mods: regen, Sources: []semantics.Source{desc("Regenerate {0}% of Life per second", "per_minute_to_per_second_2dp")}},
		{Stat: "life_regeneration_rate_per_minute_%", Scope: resolve.Buff, Status: semantics.Unknown,
			Sources: []semantics.Source{desc("You and nearby Allies Regenerate {0}% of Life per second", "per_minute_to_per_second")},
			Reason:  semantics.Reason{Kind: semantics.Condition, Text: "near the node's aura"}},
		{Stat: "mana_%_to_add_as_energy_shield", Scope: resolve.Global, Status: semantics.Unknown, Rule: "gain_as_extra", RuleFactor: semantics.Ratio{Num: 1, Den: 1},
			Sources: []semantics.Source{statID}, Reason: semantics.Reason{Kind: semantics.Rule, Text: "adds a share of maximum mana as energy shield"}},
		{Stat: "maximum_all_resistances_%", Scope: resolve.Global, Status: semantics.Authored,
			Mods: []semantics.Mod{{Stat: "maximum_fire_resistance", Op: core.OpBase, Factor: one}, {Stat: "maximum_cold_resistance", Op: core.OpBase, Factor: one}},
			Sources: []semantics.Source{
				{Kind: semantics.ObjectTemplate, Ref: "Metadata/Characters/Character.ot", Text: "maximum_all_resistances_% = 75"},
				statID,
			},
			Unvalidated: "no description text", Unchecked: "the condition"},
		{Stat: "removed_stat", Scope: resolve.Global, Until: v329, Status: semantics.DisplayOnly,
			Sources: []semantics.Source{statID}, Reason: semantics.Reason{Kind: semantics.Display, Text: "shows text only"}},
	}}
	if diff := cmp.Diff(want, load(t, readFixture(t)), cmp.Comparer(func(a, b dataset.GameVersion) bool { return a.Compare(b) == 0 })); diff != "" {
		t.Errorf("Load (-want +got):\n%s", diff)
	}
}

func TestLoadRejects(t *testing.T) {
	t.Parallel()
	const life = `{"stat": "a", "scope": "global", "status": "authored", "mods": [{"stat": "life", "op": "base"}], "sources": [{"kind": "stat-id"}]}`
	// entry builds a table of one entry of stat a with the given status and fields, with a
	// stat-id source unless fields have sources.
	entry := func(status, fields string) string {
		if !strings.Contains(fields, `"sources"`) {
			fields += `, "sources": [{"kind": "stat-id"}]`
		}
		return entries(`{"stat": "a", "scope": "global", "status": "` + status + `", ` + fields + `}`)
	}
	unknown := func(fields string) string { return entry("unknown", fields) }
	authored := func(fields string) string { return entry("authored", fields) }
	const reason = `"reason": {"kind": "condition", "text": "r"}`
	testCases := []struct {
		name, table, want string
	}{
		{"unknown field", `{"format": 1, "game": "poe1", "entries": [], "extra": 1}`, "unknown field"},
		{"other format", `{"format": 2, "game": "poe1", "entries": []}`, "format 2"},
		{"no game", `{"format": 1, "entries": []}`, "no game"},
		{"trailing data", `{"format": 1, "game": "poe1", "entries": []} {}`, "data after"},
		{"trailing brace", `{"format": 1, "game": "poe1", "entries": []}}`, "data after"},
		{"trailing bracket", `{"format": 1, "game": "poe1", "entries": []}]`, "data after"},
		{"no stat", entries(`{"scope": "global", "status": "unknown", ` + reason + `, "sources": [{"kind": "stat-id"}]}`), "no stat"},
		{"unknown scope", entries(`{"stat": "a", "scope": "local", "status": "unknown", ` + reason + `, "sources": [{"kind": "stat-id"}]}`), `unknown scope "local"`},
		{"unknown status", entries(`{"stat": "a", "scope": "global", "status": "guessed", "sources": [{"kind": "stat-id"}]}`), `unknown status "guessed"`},
		{"bad version", unknown(`"from": "3.x", ` + reason), `version "3.x"`},
		{"empty range", unknown(`"from": "3.29", "until": "3.29", ` + reason), "not before until"},
		{"authored without mods", authored(`"omitted": "o"`), "authored without mods or rule"},
		{"count without counter", authored(`"mods": [{"stat": "life", "op": "more", "if": {"atLeast": 6}}]`), "want a counter"},
		{"count at least 0", authored(`"mods": [{"stat": "life", "op": "more", "if": {"count": "n", "atLeast": 0}}]`), "want a counter"},
		{"params without rule", authored(`"mods": [{"stat": "life", "op": "base"}], "params": {"from": "life"}`), "params without a rule"},
		{"rule factor without rule", authored(`"mods": [{"stat": "life", "op": "base"}], "ruleFactor": "100"`), "rule factor without a rule"},
		{"zero rule factor", authored(`"rule": "gain_as_extra", "params": {"from": "life", "to": "armour"}, "ruleFactor": "0"`), "rule factor 0"},
		{"bad rule factor", authored(`"rule": "gain_as_extra", "params": {"from": "life", "to": "armour"}, "ruleFactor": "x"`), `rule factor "x"`},
		{"authored rule without params", authored(`"rule": "gain_as_extra"`), "params [], rule gain_as_extra takes [from to]"},
		{"missing param", authored(`"rule": "gain_as_extra", "params": {"from": "life"}`), "params [from], rule gain_as_extra takes [from to]"},
		{"extra param", authored(`"rule": "gain_as_extra", "params": {"from": "life", "to": "armour", "at": "life"}`), "params [at from to]"},
		{"param not a stat", authored(`"rule": "gain_as_extra", "params": {"from": "life", "to": "armor"}`), `params to: unknown engine stat "armor"`},
		{
			"unknown rule entry with partial params",
			unknown(`"rule": "gain_as_extra", "params": {"from": "life"}, "reason": {"kind": "rule", "text": "r"}`),
			"params [from]",
		},
		{"authored with a reason", authored(`"mods": [{"stat": "life", "op": "base"}], ` + reason), "authored with a reason"},
		{"unknown with mods", unknown(`"mods": [{"stat": "life", "op": "base"}], ` + reason), "unknown with mods"},
		{"unknown without reason", unknown(`"rule": "gain_as_extra"`), "unknown needs a reason"},
		{"unknown reason display", unknown(`"reason": {"kind": "display", "text": "r"}`), "unknown needs a reason other than display"},
		{"unknown reason kind", unknown(`"reason": {"kind": "vibes", "text": "r"}`), `unknown reason kind "vibes"`},
		{"reason without text", unknown(`"reason": {"kind": "condition"}`), "reason without text"},
		{"rule reason without rule", unknown(`"reason": {"kind": "rule", "text": "r"}`), "names a rule exactly when"},
		{"rule without rule reason", unknown(`"rule": "gain_as_extra", ` + reason), "names a rule exactly when"},
		{"unknown rule", unknown(`"rule": "conversion", "reason": {"kind": "rule", "text": "r"}`), `unknown rule "conversion"`},
		{
			"display-only reason",
			entries(`{"stat": "a", "scope": "global", "status": "display-only", ` + reason + `, "sources": [{"kind": "stat-id"}]}`),
			"display-only needs reason display",
		},
		{"unknown marked unvalidated", unknown(`"unvalidated": "u", ` + reason), "only authored entries"},
		{"unknown with omitted", unknown(`"omitted": "o", ` + reason), "only authored entries"},
		{"unknown with unchecked", unknown(`"unchecked": "c", ` + reason), "only authored entries"},
		{"unknown engine stat", authored(`"mods": [{"stat": "lifes", "op": "base"}]`), `unknown engine stat "lifes"`},
		{"unknown op", authored(`"mods": [{"stat": "life", "op": "add"}]`), `unknown op "add"`},
		{"zero factor on base", authored(`"mods": [{"stat": "life", "op": "base", "factor": "0/60"}]`), "only override may set a value of 0"},
		{
			"override of 0 is valid",
			authored(`"mods": [{"stat": "life", "op": "override", "factor": "0"}, {"stat": "life", "op": "base", "factor": "x"}]`),
			`mod 1: factor "x"`,
		},
		{"bad factor", authored(`"mods": [{"stat": "life", "op": "base", "factor": "1/-60"}]`), `factor "1/-60"`},
		{"unknown per stat", authored(`"mods": [{"stat": "life", "op": "base", "per": {"stat": "str", "step": 2}}]`), `per: unknown engine stat "str"`},
		{"zero step", authored(`"mods": [{"stat": "life", "op": "base", "per": {"stat": "strength"}}]`), "step 0"},
		{"no sources", entries(`{"stat": "a", "scope": "global", "status": "unknown", ` + reason + `}`), "no sources"},
		{
			"unknown source kind",
			entries(`{"stat": "a", "scope": "global", "status": "unknown", ` + reason + `, "sources": [{"kind": "guess"}]}`),
			`unknown kind "guess"`,
		},
		{"description without text", unknown(reason + `, "sources": [{"kind": "description", "ref": "f"}]`), "description without text"},
		{"game data without ref", unknown(reason + `, "sources": [{"kind": "game-data"}]`), "game-data without ref"},
		{"game data with text", unknown(reason + `, "sources": [{"kind": "game-data", "ref": "Characters.BaseMaxLife", "text": "38"}]`), "game-data with text"},
		{"bad template line", unknown(reason + `, "sources": [{"kind": "object-template", "ref": "a.ot", "text": "level 1"}]`), `object-template text "level 1"`},
		{"stat id with text", unknown(reason + `, "sources": [{"kind": "stat-id", "text": "t"}]`), "stat-id with ref or text"},
		{"handlers off a description", unknown(reason + `, "sources": [{"kind": "game-data", "ref": "T.C", "handlers": ["negate"]}]`), "game-data with handlers"},
		{"unsorted", entries(strings.Replace(life, `"a"`, `"b"`, 1), life), "not sorted"},
		{"repeated", entries(life, life), "versions overlap"},
		{"overlapping ranges", entries(
			strings.Replace(life, `"global"`, `"global", "until": "3.29.1"`, 1),
			strings.Replace(life, `"global"`, `"global", "from": "3.29"`, 1)), "versions overlap"},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			_, err := semantics.Load([]byte(testCase.table), vocabulary{})
			if !errors.Is(err, semantics.ErrInvalid) || !strings.Contains(err.Error(), testCase.want) {
				t.Errorf("got %v, want %v containing %q", err, semantics.ErrInvalid, testCase.want)
			}
		})
	}
}

func entries(es ...string) string {
	return `{"format": 1, "game": "poe1", "entries": [` + strings.Join(es, ",") + `]}`
}

func TestLookup(t *testing.T) {
	t.Parallel()
	tbl := load(t, readFixture(t))
	testCases := []struct {
		name, stat, version string
		scope               resolve.Scope
		wantFrom, wantUntil string
		wantOK              bool
	}{
		{"unbounded", "base_maximum_life", "3.28.0.16", resolve.Global, "", "", true},
		{"before a boundary", "life_regeneration_rate_per_minute_%", "3.28.0.16", resolve.Global, "", "3.29", true},
		{"at a boundary", "life_regeneration_rate_per_minute_%", "3.29", resolve.Global, "3.29", "", true},
		{"after a boundary", "life_regeneration_rate_per_minute_%", "3.29.3.3", resolve.Global, "3.29", "", true},
		{"other scope", "life_regeneration_rate_per_minute_%", "3.29.3.3", resolve.Buff, "", "", true},
		{"no entry in scope", "base_maximum_life", "3.29.3.3", resolve.Buff, "", "", false},
		{"out of range", "removed_stat", "3.29.3.3", resolve.Global, "", "", false},
		{"no entry", "base_maximum_mana", "3.29.3.3", resolve.Global, "", "", false},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			e, ok := tbl.Lookup(testCase.stat, testCase.scope, version(t, testCase.version))
			if ok != testCase.wantOK {
				t.Fatalf("got ok %v, want %v", ok, testCase.wantOK)
			}
			if ok && (e.Stat != testCase.stat || e.Scope != testCase.scope || e.From.String() != testCase.wantFrom || e.Until.String() != testCase.wantUntil) {
				t.Errorf("got %s, want %s %s [%s, %s)", e, testCase.stat, testCase.scope, testCase.wantFrom, testCase.wantUntil)
			}
		})
	}
}

func TestEntryHolds(t *testing.T) {
	t.Parallel()
	e := semantics.Entry{Stat: "a", From: version(t, "3.28"), Until: version(t, "3.29")}
	open := semantics.Entry{Stat: "a"}
	for _, testCase := range []struct {
		version        string
		want, wantOpen bool
	}{
		{"3.27.9", false, true},
		{"3.28", true, true},
		{"3.28.0.16", true, true},
		{"3.29", false, true},
	} {
		v := version(t, testCase.version)
		if got := e.Holds(v); got != testCase.want {
			t.Errorf("%s: Holds(%s) got %v, want %v", e.String(), v, got, testCase.want)
		}
		if got := open.Holds(v); got != testCase.wantOpen {
			t.Errorf("entry without range: Holds(%s) got %v, want %v", v, got, testCase.wantOpen)
		}
	}
}

func TestRatioApply(t *testing.T) {
	t.Parallel()
	if got := (semantics.Ratio{Num: 1, Den: 60}).Apply(105); got != 1.75 {
		t.Errorf("got %v, want 1.75", got)
	}
}

// statIDs are the stats of testDataset; its handles index this list.
func statIDs() []string {
	return []string{
		"level", "base_maximum_life", "life_regeneration_rate_per_minute_%", "X_to_armour_per_2_strength", "unmapped_stat",
		"maximum_all_resistances_%", "keystone_blood_magic", "mana_%_to_add_as_energy_shield",
		"base_resist_all_elements_%", "base_chaos_damage_resistance_%",
	}
}

// testDataset has the stats and English descriptions the fixture table refers to, a tree and
// one character.
func testDataset(version string) *dataset.Dataset {
	anyValue := []statdesc.Limit{{Min: 1, Max: 2147483647}}
	d := &dataset.Dataset{
		Manifest: dataset.Manifest{Game: "poe1", Version: version},
		Characters: []dataset.Character{{
			ID: "Metadata/Characters/Str/Str", BaseStrength: 32, BaseMaxLife: 38,
			Stats: []dataset.StatValue{{Stat: 0, Value: 1}, {Stat: 5, Value: 75}},
		}},
		Quests: []dataset.Quest{
			{Quest: "a5q6", Flag: "a5q6-CompletedBoss", Act: 5, Stats: []dataset.StatValue{{Stat: 8, Value: -30}, {Stat: 9, Value: -30}}},
			{Quest: "a10q3", Flag: "EndGameAvailable", Act: 10, Stats: []dataset.StatValue{{Stat: 8, Value: -30}, {Stat: 9, Value: -30}}},
		},
		Trees: []dataset.Tree{{
			Variant: "standard",
			Nodes: []dataset.Node{
				{ID: 1, Stats: []dataset.StatValue{{Stat: 1, Value: 10}}},
				{ID: 2, Stats: []dataset.StatValue{{Stat: 1, Value: 20}, {Stat: 2, Value: 30}}, Buffs: [][]dataset.StatValue{{{Stat: 2, Value: 60}}}},
				{ID: 3, Kind: dataset.Mastery, MasteryEffects: []dataset.MasteryEffect{{ID: 1, Stats: []dataset.StatValue{{Stat: 3, Value: 1}, {Stat: 4, Value: 5}}}}},
				{ID: 4, Kind: dataset.Keystone, Stats: []dataset.StatValue{{Stat: 6, Value: 1}}},
			},
		}},
		Texts: []dataset.Text{{Language: "en", Descriptions: []statdesc.Scope{{
			Name: descriptions,
			Descriptions: []statdesc.Description{
				{Stats: []string{"base_maximum_life"}, Variants: []statdesc.Variant{{Limits: anyValue, Text: "{0:+d} to maximum Life"}}},
				{Stats: []string{"life_regeneration_rate_per_minute_%"}, Variants: []statdesc.Variant{{
					Limits: anyValue, Text: "Regenerate {0}% of Life per second",
					Handlers: []statdesc.Handler{{Name: "per_minute_to_per_second_2dp", Stat: 0}},
				}}},
				{Stats: []string{"X_to_armour_per_2_strength"}, Variants: []statdesc.Variant{{Limits: anyValue, Text: "{0:+d} Armour per 2 Strength"}}},
				{Stats: []string{"keystone_blood_magic"}, Variants: []statdesc.Variant{{Limits: anyValue, Text: `Removes all mana\nSkills Cost Life instead of Mana`}}},
			},
		}}}},
	}
	for _, id := range statIDs() {
		d.Stats = append(d.Stats, dataset.Stat{ID: id})
	}
	return d
}

func open(t *testing.T, d *dataset.Dataset) *dataset.Store {
	t.Helper()
	dir := t.TempDir()
	if _, err := dataset.Write(dir, d); err != nil {
		t.Fatal(err)
	}
	s, err := dataset.Open(os.DirFS(dir))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// oneFile renders every scope from the one description file of testDataset.
func oneFile() map[resolve.Scope]string {
	return map[resolve.Scope]string{resolve.Global: descriptions, resolve.Buff: descriptions}
}

func TestCheckScopeFiles(t *testing.T) {
	t.Parallel()
	const (
		passive = "Metadata/StatDescriptions/passive_skill_stat_descriptions.txt"
		aura    = "Metadata/StatDescriptions/passive_skill_aura_stat_descriptions.txt"
	)
	files := map[resolve.Scope]string{resolve.Global: passive, resolve.Buff: aura}
	anyValue := []statdesc.Limit{{Min: 1, Max: 2147483647}}
	d := testDataset("3.29.3.3")
	d.Texts[0].Descriptions = append(d.Texts[0].Descriptions,
		statdesc.Scope{Name: passive, Parent: descriptions},
		statdesc.Scope{Name: aura, Parent: passive, Descriptions: []statdesc.Description{
			{Stats: []string{"life_regeneration_rate_per_minute_%"}, Variants: []statdesc.Variant{{
				Limits: anyValue, Text: "You and nearby Allies Regenerate {0}% of Life per second",
				Handlers: []statdesc.Handler{{Name: "per_minute_to_per_second_2dp", Stat: 0}},
			}}},
		}},
	)
	s := open(t, d)
	// entry is an unknown entry of stat in scope citing text in file.
	entry := func(stat, scope, file, text string) string {
		return entries(`{"stat": "` + stat + `", "scope": "` + scope + `", "status": "unknown", "reason": {"kind": "condition", "text": "r"}, ` +
			`"sources": [{"kind": "description", "ref": "` + file + `", "text": "` + text + `", "handlers": ["per_minute_to_per_second_2dp"]}]}`)
	}
	const (
		regen     = "life_regeneration_rate_per_minute_%"
		regenText = "Regenerate {0}% of Life per second"
		auraText  = "You and nearby Allies Regenerate {0}% of Life per second"
	)
	testCases := []struct {
		name, table string
		want        error
	}{
		{"buff citing its scope's file", entry(regen, "buff", aura, auraText), nil},
		{"global citing a parent file", entry(regen, "global", descriptions, regenText), nil},
		{"buff citing a parent its text does not render from", entry(regen, "buff", descriptions, regenText), semantics.ErrSourceMismatch},
		{"global citing the aura file", entry(regen, "global", aura, auraText), semantics.ErrSourceMismatch},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			if err := load(t, testCase.table).Check(s, files); !errors.Is(err, testCase.want) {
				t.Errorf("got %v, want %v", err, testCase.want)
			}
		})
	}
}

func TestCheck(t *testing.T) {
	t.Parallel()
	noText := testDataset("3.29.3.3")
	noText.Texts = nil
	otherGame := testDataset("3.29.3.3")
	otherGame.Manifest.Game = "poe2"
	otherTemplate := testDataset("3.29.3.3")
	otherTemplate.Characters[0].Stats[1].Value = 70
	fixture := readFixture(t)
	mismatch := semantics.ErrSourceMismatch
	testCases := []struct {
		name  string
		table string
		d     *dataset.Dataset
		want  []error
	}{
		// The buff entry quotes text the dataset lacks; removed_stat is out of range.
		{"3.29", fixture, testDataset("3.29.3.3"), []error{mismatch}},
		// The global regen entry before 3.29 quotes other handlers; removed_stat is missing.
		{"3.28", fixture, testDataset("3.28.0.16"), []error{mismatch, mismatch, semantics.ErrUnknownStat}},
		{"template value differs", fixture, otherTemplate, []error{mismatch, mismatch}},
		{
			"column carries another stat",
			strings.Replace(fixture, "Characters.BaseMaxLife", "Characters.BaseMaxMana", 1),
			testDataset("3.29.3.3"),
			[]error{mismatch, mismatch},
		},
		{
			"counter not in dataset",
			strings.Replace(fixture, `"step": 2}`, `"step": 2}, "if": {"count": "no_such_counter", "atLeast": 6}`, 1),
			testDataset("3.29.3.3"),
			[]error{semantics.ErrUnknownStat, mismatch},
		},
		{"no text", fixture, noText, slices.Repeat([]error{semantics.ErrTextNotChecked}, 5)},
		{"other game", fixture, otherGame, []error{semantics.ErrOtherGame}},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			err := load(t, testCase.table).Check(open(t, testCase.d), oneFile())
			var got []error
			if j, ok := err.(interface{ Unwrap() []error }); ok {
				got = j.Unwrap()
			} else if err != nil {
				got = []error{err}
			}
			if len(got) != len(testCase.want) {
				t.Fatalf("got %v, want %d errors", err, len(testCase.want))
			}
			for i := range got {
				if !errors.Is(got[i], testCase.want[i]) {
					t.Errorf("error %d: got %v, want %v", i, got[i], testCase.want[i])
				}
			}
		})
	}
}

func TestCover(t *testing.T) {
	t.Parallel()
	s := open(t, testDataset("3.29.3.3"))
	feeds := func(stat string, scope resolve.Scope) bool {
		return scope == resolve.Global && stat != "unmapped_stat" && stat != "level"
	}
	c, err := semantics.Cover(s, "standard", load(t, readFixture(t)), feeds)
	if err != nil {
		t.Fatal(err)
	}
	type row struct {
		Stat   string
		Scope  resolve.Scope
		Uses   int
		Feeds  bool
		Status semantics.Status
	}
	got := make([]row, 0, len(c.Stats))
	for _, u := range c.Stats {
		r := row{u.Stat, u.Scope, u.Uses, u.Feeds, 0}
		if u.Entry != nil {
			r.Status = u.Entry.Status
		}
		got = append(got, r)
	}
	want := []row{
		{"base_maximum_life", resolve.Global, 3, true, semantics.Authored},
		{"base_chaos_damage_resistance_%", resolve.Global, 2, true, 0},
		{"base_resist_all_elements_%", resolve.Global, 2, true, 0},
		{"X_to_armour_per_2_strength", resolve.Global, 1, true, semantics.Authored},
		{"base_dexterity", resolve.Global, 1, true, 0},
		{"base_intelligence", resolve.Global, 1, true, 0},
		{"base_maximum_mana", resolve.Global, 1, true, 0},
		{"base_strength", resolve.Global, 1, true, 0},
		{"keystone_blood_magic", resolve.Global, 1, true, semantics.Authored},
		{"level", resolve.Global, 1, false, 0},
		{"life_regeneration_rate_per_minute_%", resolve.Global, 1, true, semantics.Authored},
		{"life_regeneration_rate_per_minute_%", resolve.Buff, 1, false, semantics.Unknown},
		{"maximum_all_resistances_%", resolve.Global, 1, true, semantics.Authored},
		{"unmapped_stat", resolve.Global, 1, false, 0},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("Cover (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff(semantics.Summary{Authored: 5, Unknown: 1, Gaps: 6, Outside: 2}, c.Summary()); diff != "" {
		t.Errorf("Summary (-want +got):\n%s", diff)
	}

	var out strings.Builder
	if err := c.WriteText(&out); err != nil {
		t.Fatal(err)
	}
	wantText := `coverage poe1 3.29.3.3 standard tree and characters: 14 stats; 5 authored, 1 unknown, 0 display-only, 6 gaps, 2 outside
unknown by reason: condition 1
gap base_chaos_damage_resistance_% global, uses 2
gap base_resist_all_elements_% global, uses 2
unvalidated X_to_armour_per_2_strength global, uses 1: whole steps
gap base_dexterity global, uses 1
gap base_intelligence global, uses 1
gap base_maximum_mana global, uses 1
gap base_strength global, uses 1
omitted keystone_blood_magic global, uses 1: Skills Cost Life instead of Mana
unknown life_regeneration_rate_per_minute_% buff, uses 1: condition: near the node's aura
unvalidated maximum_all_resistances_% global, uses 1: no description text
unchecked maximum_all_resistances_% global, uses 1: the condition
`
	if diff := cmp.Diff(wantText, out.String()); diff != "" {
		t.Errorf("WriteText (-want +got):\n%s", diff)
	}
}

// A mastery's counter needs no entry, so Cover leaves it out.
func TestCoverCounters(t *testing.T) {
	t.Parallel()
	d := testDataset("3.29.3.3")
	d.Trees[0].Nodes[2].Stats = []dataset.StatValue{{Stat: 4, Value: 1}}
	c, err := semantics.Cover(open(t, d), "standard", load(t, readFixture(t)), func(string, resolve.Scope) bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range c.Stats {
		if u.Stat == "unmapped_stat" {
			t.Errorf("counter covered: %+v", u)
		}
	}
}

func TestCoverRuleLine(t *testing.T) {
	t.Parallel()
	d := testDataset("3.29.3.3")
	d.Trees[0].Nodes[0].Stats = []dataset.StatValue{{Stat: 7, Value: 20}}
	c, err := semantics.Cover(open(t, d), "standard", load(t, readFixture(t)), func(string, resolve.Scope) bool { return false })
	if err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	if err := c.WriteText(&out); err != nil {
		t.Fatal(err)
	}
	const want = "unknown mana_%_to_add_as_energy_shield global, uses 1: rule gain_as_extra: adds a share of maximum mana as energy shield\n"
	if !strings.Contains(out.String(), want) {
		t.Errorf("WriteText missing %q:\n%s", want, out.String())
	}
}

func TestLoadRuleParams(t *testing.T) {
	t.Parallel()
	tb, err := semantics.Load([]byte(entries(
		`{"stat": "a", "scope": "global", "status": "authored", "rule": "gain_as_extra", "params": {"from": "life", "to": "armour"}, `+
			`"ruleFactor": "100", "sources": [{"kind": "stat-id"}]}`,
		`{"stat": "b", "scope": "global", "status": "unknown", "rule": "gain_as_extra", "reason": {"kind": "rule", "text": "r"}, "sources": [{"kind": "stat-id"}]}`,
	)), vocabulary{})
	if err != nil {
		t.Fatal(err)
	}
	want := []map[string]string{{"from": "life", "to": "armour"}, nil}
	factors := []semantics.Ratio{{Num: 100, Den: 1}, {Num: 1, Den: 1}}
	for i, e := range tb.Entries {
		if diff := cmp.Diff(want[i], e.Params); diff != "" {
			t.Errorf("entry %s params (-want +got):\n%s", e.Stat, diff)
		}
		if e.RuleFactor != factors[i] {
			t.Errorf("entry %s rule factor: got %v, want %v", e.Stat, e.RuleFactor, factors[i])
		}
	}
}
