package main

// White-box: package main cannot be imported by an external test package.

import (
	"bytes"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

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
	for _, d := range []*dataset.Dataset{testDataset("3.27.0.5", 5), testDataset("3.28", 10), testDataset("3.29", 20)} {
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
		name  string
		args  []string
		want  []string // lines on stdout
		notes []string // lines on stderr
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
			name:  "show resolves latest",
			args:  []string{"data", "show", "stat", "aura_damage_+%", "--version", "latest", "--root", root},
			want:  []string{"aura_damage_+% (handle 1 of 2)"},
			notes: []string{"(poe1 latest resolves to 3.29)"},
		},
		{
			name:  "show falls back to an earlier patch",
			args:  []string{"data", "show", "stat", "aura_damage_+%", "--version", "3.27.0.6", "--root", root},
			want:  []string{"aura_damage_+% (handle 1 of 2)"},
			notes: []string{"(poe1 3.27.0.6 has no dataset; using 3.27.0.5, the newest earlier patch of its league)"},
		},
		{
			name:  "show falls forward to a later patch",
			args:  []string{"data", "show", "stat", "aura_damage_+%", "--version", "3.27.0.1", "--root", root},
			want:  []string{"aura_damage_+% (handle 1 of 2)"},
			notes: []string{"(poe1 3.27.0.1 has no dataset or earlier patch; using 3.27.0.5, the newest patch of its league)"},
		},
		{
			name: "semantics coverage",
			args: []string{"semantics", "coverage", "--version", "3.29", "--root", root},
			want: []string{"coverage poe1 3.29 standard tree and characters: 9 stats; 8 authored, 0 unknown, 0 display-only, 0 gaps, 1 outside"},
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
			var out, notes strings.Builder
			if err := run(testCase.args, &out, &notes); err != nil {
				t.Fatalf("run: %v", err)
			}
			wantLines(t, "stdout", out.String(), testCase.want)
			wantLines(t, "stderr", notes.String(), testCase.notes)
		})
	}
}

// wantLines checks that got holds each line of want, and nothing else when want is empty.
func wantLines(t *testing.T, name, got string, want []string) {
	t.Helper()
	if len(want) == 0 && got != "" {
		t.Errorf("%s: got %q, want nothing", name, got)
	}
	for _, w := range want {
		if !strings.Contains(got, w+"\n") {
			t.Errorf("%s missing %q:\n%s", name, w, got)
		}
	}
}

func TestRunSkipsBadDataset(t *testing.T) {
	t.Parallel()
	root := writeRoot(t)
	path := filepath.Join(root, "3.28", dataset.ManifestName)
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	b = bytes.Replace(b, []byte(`"format": 1`), []byte(`"format": 2`), 1)
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
	var out, notes strings.Builder
	if err := run([]string{"data", "show", "stat", "aura_damage_+%", "--version", "3.29", "--root", root}, &out, &notes); err != nil {
		t.Fatalf("run: %v", err)
	}
	wantLines(t, "stdout", out.String(), []string{"aura_damage_+% (handle 1 of 2)"})
	wantLines(t, "stderr", notes.String(), []string{"pbt: skipped " + root + "/3.28: dataset format 2, want 1"})
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
		{name: "other game", args: []string{"data", "show", "stat", "x", "--version", "3.29", "--root", root, "--game", "poe2"}},
		{name: "diff missing b", args: []string{"data", "diff", "3.28", "9.9", "--root", root}},
		{name: "missing language", args: []string{"data", "show", "node", "1", "--version", "3.29", "--root", root, "--language", "fr"}},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			var out strings.Builder
			if err := run(testCase.args, &out, &out); err == nil {
				t.Errorf("run(%q): got nil error, want an error", testCase.args)
			}
		})
	}
}

// writeBuild writes a build file into a new directory and returns its path.
func writeBuild(t *testing.T, data string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "build.json")
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestBuildCheck(t *testing.T) {
	t.Parallel()
	root := writeRoot(t)
	testCases := []struct {
		name, data, want, notes string
	}{
		{
			name: "exact version",
			data: `{"format":1,"game":"poe1","version":"3.29","class":"Metadata/Characters/Str/Str","ascendancy":"Juggernaut","level":90,"nodes":[1,12345]}`,
			want: "ok: poe1 3.29 (dataset 3.29), standard tree, Metadata/Characters/Str/Str Juggernaut, level 90, 2 nodes\n",
		},
		{
			name:  "earlier patch",
			data:  `{"format":1,"game":"poe1","version":"3.27.0.9","class":"Metadata/Characters/Str/Str","level":1,"nodes":[1]}`,
			want:  "ok: poe1 3.27.0.9 (dataset 3.27.0.5), standard tree, Metadata/Characters/Str/Str no ascendancy, level 1, 1 nodes\n",
			notes: "(poe1 3.27.0.9 has no dataset; using 3.27.0.5, the newest earlier patch of its league)\n",
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			var out, notes strings.Builder
			if err := run([]string{"build", "check", writeBuild(t, testCase.data), "--root", root}, &out, &notes); err != nil {
				t.Fatal(err)
			}
			if out.String() != testCase.want {
				t.Errorf("stdout: got %q, want %q", out.String(), testCase.want)
			}
			if notes.String() != testCase.notes {
				t.Errorf("stderr: got %q, want %q", notes.String(), testCase.notes)
			}
		})
	}
}

func TestBuildCheckErrors(t *testing.T) {
	t.Parallel()
	root := writeRoot(t)
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
			path := writeBuild(t, testCase.data)
			err := run([]string{"build", "check", path, "--root", root}, io.Discard, io.Discard)
			if err == nil || !strings.HasPrefix(err.Error(), path+": ") {
				t.Errorf("got %v, want an error starting with the build file path", err)
			}
		})
	}
	missing := filepath.Join(t.TempDir(), "missing.json")
	if err := run([]string{"build", "check", missing, "--root", root}, io.Discard, io.Discard); err == nil {
		t.Error("missing file: got nil error, want error")
	}
}

func TestBuildResolve(t *testing.T) {
	t.Parallel()
	d := testDataset("3.29", 20)
	for _, id := range []string{"base_dexterity", "base_intelligence", "base_maximum_life", "base_maximum_mana", "level"} {
		d.Stats = append(d.Stats, dataset.Stat{ID: id})
	}
	root := t.TempDir()
	dir := filepath.Join(root, "3.29")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := dataset.Write(dir, d); err != nil {
		t.Fatal(err)
	}
	path := writeBuild(t, `{"format":1,"game":"poe1","version":"3.29","class":"Metadata/Characters/Str/Str","ascendancy":"Juggernaut","level":90,
		"nodes":[1,12345],"scenario":{"campaign":"acts-1-5"}}`)
	var out, notes strings.Builder
	if err := run([]string{"build", "resolve", path, "--root", root}, &out, &notes); err != nil {
		t.Fatal(err)
	}
	//nolint:dupword // a level bundle header followed by its level stat
	want := `dataset poe1 3.29
bundle player global character Metadata/Characters/Str/Str
  base_strength = 32
  base_dexterity = 0
  base_intelligence = 0
  base_maximum_life = 38
  base_maximum_mana = 0
  base_strength = 1
bundle player global level
  level = 90
bundle player global node 12345
  base_strength = 20
bundle player buff node 12345
  aura_damage_+% = 5
note node 12345: mastery effect not chosen; builds have no mastery choices yet
`
	if diff := cmp.Diff(want, out.String()); diff != "" {
		t.Errorf("(-want +got):\n%s", diff)
	}
	if notes.String() != "" {
		t.Errorf("stderr: got %q, want nothing", notes.String())
	}

	// The shared test datasets lack the base column stats, which resolve requires.
	err := run([]string{"build", "resolve", path, "--root", writeRoot(t)}, io.Discard, io.Discard)
	if err == nil || !strings.HasPrefix(err.Error(), path+": resolve: ") {
		t.Errorf("got %v, want a resolve error prefixed with the build file path", err)
	}
}
