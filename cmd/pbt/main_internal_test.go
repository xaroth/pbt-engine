package main

// White-box: package main cannot be imported by an external test package.

import (
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/statdesc"
)

func testDataset(version string, strength int32) *dataset.Dataset {
	anyValue := statdesc.Limit{Min: math.MinInt32, Max: math.MaxInt32}
	return &dataset.Dataset{
		Manifest: dataset.Manifest{Game: "poe1", Version: version},
		Stats:    []dataset.Stat{{ID: "base_strength"}, {ID: "aura_damage_+%"}},
		Characters: []dataset.Character{{
			ID: "Metadata/Characters/Str/Str", BaseStrength: 32, BaseMaxLife: 38,
			Stats: []dataset.StatValue{{Stat: 0, Value: 1}},
		}},
		Texts: []dataset.Text{{
			Language:        "en",
			NodeNames:       []dataset.NodeName{{ID: 1, Name: "Start"}, {ID: 12345, Name: "Might"}},
			CharacterNames:  []dataset.Name{{ID: "Metadata/Characters/Str/Str", Name: "Marauder"}},
			AscendancyNames: []dataset.AscendancyName{{Variant: "standard", ID: "Juggernaut", Name: "Ancestral Commander"}},
			Descriptions: []statdesc.Scope{
				{Name: auraScope, Parent: passiveScope, Descriptions: []statdesc.Description{{
					Stats:    []string{"aura_damage_+%"},
					Variants: []statdesc.Variant{{Limits: []statdesc.Limit{anyValue}, Text: "Nearby Allies deal {0}% increased Damage"}},
				}}},
				{Name: passiveScope, Descriptions: []statdesc.Description{{
					Stats:    []string{"base_strength"},
					Variants: []statdesc.Variant{{Limits: []statdesc.Limit{anyValue}, Text: "{0:+d} to Strength\\nline two"}},
				}}},
			},
		}},
		Trees: []dataset.Tree{{
			Variant: "standard",
			Classes: []dataset.Class{{Character: "Metadata/Characters/Str/Str", StartNode: 1, Ascendancies: []dataset.Ascendancy{{ID: "Juggernaut"}}}},
			Nodes: []dataset.Node{
				{ID: 1, Kind: dataset.ClassStart, Neighbours: []uint32{12345}},
				{
					ID: 12345, Kind: dataset.Notable, Ascendancy: "Juggernaut",
					Stats:          []dataset.StatValue{{Stat: 0, Value: strength}},
					Buffs:          [][]dataset.StatValue{{{Stat: 1, Value: 5}}},
					SkillPoints:    2,
					MasteryEffects: []dataset.MasteryEffect{{ID: 7, Stats: []dataset.StatValue{{Stat: 0, Value: 1}}}},
					Neighbours:     []uint32{1},
				},
			},
		}},
	}
}

func writeRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, d := range []*dataset.Dataset{testDataset("3.28", 10), testDataset("3.29", 20)} {
		dir := filepath.Join(root, d.Manifest.Version)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if _, err := dataset.Write(dir, d); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestRun(t *testing.T) {
	t.Parallel()
	root := writeRoot(t)
	testCases := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "show node",
			args: []string{"data", "show", "node", "12345", "--version", "3.29", "--root", root},
			want: []string{
				"12345 Might (notable, Ancestral Commander)", "  +20 to Strength", "  line two",
				"  Nearby Allies deal 5% increased Damage (aura)", "  grants 2 passive skill points",
				"  mastery effect 7: +1 to Strength / line two", "  neighbours: 1",
			},
		},
		{
			name: "show stat",
			args: []string{"data", "show", "stat", "aura_damage_+%", "--version=3.29", "--root=" + root},
			want: []string{"aura_damage_+% (handle 1 of 2)"},
		},
		{
			name: "show character",
			args: []string{"data", "show", "character", "MARAUDER", "--version", "3.29", "--root", root},
			want: []string{"Marauder (Metadata/Characters/Str/Str)", "  base life 38, base mana 0", "  base_strength = 1"},
		},
		{
			name: "diff",
			args: []string{"data", "diff", "3.28", "3.29", "--root", root},
			want: []string{
				"3.28 -> 3.29", "stats: 2 -> 2, 0 added, 0 removed; ids kept: 100.00%",
				"text en: description scopes changed: none; names changed: 0 nodes, 0 characters, 0 ascendancies",
				"tree standard: 0 nodes added, 0 removed, 1 changed; node ids kept: 100.00%; mastery effects 0 added, 0 removed",
				"  12345 Might [stats]", "    - +10 to Strength", "    + +20 to Strength",
			},
		},
		{
			name: "show resolves latest",
			args: []string{"data", "show", "stat", "aura_damage_+%", "--version", "latest", "--root", root},
			want: []string{"(poe1 latest resolves to 3.29)", "aura_damage_+% (handle 1 of 2)"},
		},
		{
			name: "diff with limit",
			args: []string{"data", "diff", "3.28", "3.29", "--root", root, "--limit", "0"},
			want: []string{"  ... 1 more"},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			var out strings.Builder
			if err := run(testCase.args, &out); err != nil {
				t.Fatalf("run: %v", err)
			}
			for _, w := range testCase.want {
				if !strings.Contains(out.String(), w+"\n") {
					t.Errorf("output missing %q:\n%s", w, out.String())
				}
			}
		})
	}
}

func TestRunErrors(t *testing.T) {
	t.Parallel()
	root := writeRoot(t)
	testCases := []struct {
		name string
		args []string
	}{
		{name: "no args", args: nil},
		{name: "unknown option", args: []string{"data", "show", "node", "1", "--nope", "x"}},
		{name: "option without value", args: []string{"data", "show", "node", "1", "--version"}},
		{name: "no version", args: []string{"data", "show", "node", "1", "--root", root}},
		{name: "missing dataset", args: []string{"data", "show", "node", "1", "--version", "9.9", "--root", root}},
		{name: "unknown show", args: []string{"data", "show", "item", "1", "--version", "3.29", "--root", root}},
		{name: "bad node id", args: []string{"data", "show", "node", "x", "--version", "3.29", "--root", root}},
		{name: "unknown node", args: []string{"data", "show", "node", "9", "--version", "3.29", "--root", root}},
		{name: "unknown variant", args: []string{"data", "show", "node", "1", "--version", "3.29", "--variant", "ruthless", "--root", root}},
		{name: "unknown stat", args: []string{"data", "show", "stat", "x", "--version", "3.29", "--root", root}},
		{name: "unknown character", args: []string{"data", "show", "character", "x", "--version", "3.29", "--root", root}},
		{name: "bad limit", args: []string{"data", "diff", "3.28", "3.29", "--root", root, "--limit", "-1"}},
		{name: "diff missing a", args: []string{"data", "diff", "9.9", "3.29", "--root", root}},
		{name: "diff missing b", args: []string{"data", "diff", "3.28", "9.9", "--root", root}},
		{name: "missing language", args: []string{"data", "show", "node", "1", "--version", "3.29", "--root", root, "--language", "fr"}},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			var out strings.Builder
			if err := run(testCase.args, &out); err == nil {
				t.Errorf("run(%q): got nil error, want an error", testCase.args)
			}
		})
	}
}

// writeBuild writes a build file into dir and returns its path.
func writeBuild(t *testing.T, dir, name, data string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestBuildCheck(t *testing.T) {
	t.Parallel()
	root := writeRoot(t)
	dir := t.TempDir()
	good := writeBuild(t, dir, "good.json", `{"format":1,"game":"poe1","version":"3.29","class":"Metadata/Characters/Str/Str","ascendancy":"Juggernaut","level":90,"nodes":[1,12345]}`)
	var out strings.Builder
	if err := run([]string{"build", "check", good, "--root", root}, &out); err != nil {
		t.Fatal(err)
	}
	want := "ok: poe1 3.29 (dataset 3.29), standard tree, Metadata/Characters/Str/Str Juggernaut, level 90, 2 nodes\n"
	if out.String() != want {
		t.Errorf("got %q, want %q", out.String(), want)
	}

	testCases := []struct {
		name, data string
	}{
		{"not a build", `{"format":9}`},
		{"unknown version", `{"format":1,"game":"poe1","version":"9.9","class":"Metadata/Characters/Str/Str","level":1,"nodes":[]}`},
		{"unknown node", `{"format":1,"game":"poe1","version":"3.29","class":"Metadata/Characters/Str/Str","level":1,"nodes":[2]}`},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			path := writeBuild(t, t.TempDir(), "b.json", testCase.data)
			if err := run([]string{"build", "check", path, "--root", root}, io.Discard); err == nil {
				t.Error("got nil error, want error")
			}
		})
	}
	out.Reset()
	if err := run([]string{"build", "resolve", good, "--root", root}, &out); err != nil {
		t.Fatal(err)
	}
	for _, line := range []string{"bundle player global level\n  level = 90\n", "bundle player global node 12345\n  base_strength = 20\n", "bundle player buff node 12345\n"} {
		if !strings.Contains(out.String(), line) {
			t.Errorf("resolve: got:\n%s\nwant %q", out.String(), line)
		}
	}
	if err := run([]string{"build", "resolve", filepath.Join(dir, "missing.json"), "--root", root}, io.Discard); err == nil {
		t.Error("resolve missing file: got nil error, want error")
	}
	if err := run([]string{"build", "check", filepath.Join(dir, "missing.json"), "--root", root}, io.Discard); err == nil {
		t.Error("missing file: got nil error, want error")
	}
}
