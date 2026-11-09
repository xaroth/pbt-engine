package main

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xaroth/pbt-engine/dataset"
)

// simDataset is a Marauder dataset the sheet rules can run on: 32 strength, 38 life plus 12
// per level, 75% maximum resistances, both Kitava rewards, node 2 giving life2 life.
func simDataset(version string, life2 int32) *dataset.Dataset {
	stats := []string{
		"level", "life_per_level", "base_maximum_all_resistances_%", "base_strength", "base_dexterity", "base_intelligence",
		"base_maximum_life", "base_maximum_mana", "base_resist_all_elements_%", "base_chaos_damage_resistance_%",
	}
	d := &dataset.Dataset{
		Manifest: dataset.Manifest{Game: "poe1", Version: version},
		Characters: []dataset.Character{{
			ID: "Metadata/Characters/Str/Str", BaseStrength: 32, BaseMaxLife: 38,
			Stats: []dataset.StatValue{{Stat: 0, Value: 1}, {Stat: 1, Value: 12}, {Stat: 2, Value: 75}},
		}},
		Constants: []dataset.Constant{
			{ID: "life_per_strength", Value: 1, Per: 2}, {ID: "mana_per_intelligence", Value: 1, Per: 2},
			{ID: "evasion_rating_+%_per_dexterity", Value: 1, Per: 5}, {ID: "energy_shield_+%_per_intelligence", Value: 1, Per: 10},
		},
		Quests: []dataset.Quest{
			{Quest: "a5q6", Flag: "a5q6-CompletedBoss", Act: 5, Stats: []dataset.StatValue{{Stat: 8, Value: -30}, {Stat: 9, Value: -30}}},
			{Quest: "a10q3", Flag: "EndGameAvailable", Act: 10, Stats: []dataset.StatValue{{Stat: 8, Value: -30}, {Stat: 9, Value: -30}}},
		},
		Trees: []dataset.Tree{{
			Variant: "standard",
			Classes: []dataset.Class{{Character: "Metadata/Characters/Str/Str", StartNode: 1}},
			Nodes: []dataset.Node{
				{ID: 1, Kind: dataset.ClassStart, Neighbours: []uint32{2}},
				{ID: 2, Neighbours: []uint32{1}, Stats: []dataset.StatValue{{Stat: 6, Value: life2}}},
			},
		}},
		Texts: []dataset.Text{{Language: "en", CharacterNames: []dataset.Name{{ID: "Metadata/Characters/Str/Str", Name: "Marauder"}}}},
	}
	for _, id := range stats {
		d.Stats = append(d.Stats, dataset.Stat{ID: id})
	}
	return d
}

func writeSimRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, d := range []*dataset.Dataset{simDataset("3.28.0.16", 10), simDataset("3.29.3.3", 20)} {
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

func runOut(t *testing.T, args ...string) string {
	t.Helper()
	var out strings.Builder
	if err := run(args, &out, io.Discard); err != nil {
		t.Fatal(err)
	}
	return out.String()
}

const level90 = `{"format":1,"game":"poe1","version":"3.29","class":"Metadata/Characters/Str/Str","level":90,"nodes":[%s]}`

func buildWith(t *testing.T, nodes string) string {
	t.Helper()
	return writeBuild(t, strings.Replace(level90, "%s", nodes, 1))
}

func TestEval(t *testing.T) {
	t.Parallel()
	root := writeSimRoot(t)
	path := buildWith(t, "2")
	got := runOut(t, "eval", path, "--root", root)
	// 38 + 1080 + 16 + 20
	for _, want := range []string{
		"poe1 3.29 on dataset 3.29.3.3\n", "Life: 1154\n", "Fire Resistance: -60%\n",
		"config campaign = \"complete\" (default)\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	got = runOut(t, "eval", path, "--root", root, "--set", "campaign=acts-1-5", "--set", "bandits=kill-all")
	if !strings.Contains(got, "Fire Resistance: 0%\n") || !strings.Contains(got, "config campaign = \"acts-1-5\" (call)") {
		t.Errorf("--set:\n%s", got)
	}
	got = runOut(t, "eval", path, "--root", root, "--explain", "player.life", "--view", "proportional")
	if !strings.Contains(got, "Life: 1154  [proportional, dataset 3.29.3.3]") || !strings.Contains(got, "Marauder (character)") {
		t.Errorf("--explain:\n%s", got)
	}
	got = runOut(t, "eval", path, "--root", root, "--version", "3.28", "--set", "campaign=act 3")
	if !strings.Contains(got, "Life: 1144\n") || !strings.Contains(got, "warning version-mismatch") || !strings.Contains(got, "warning config-bad-value") {
		t.Errorf("--version 3.28:\n%s", got)
	}
	for name, args := range map[string][]string{
		"set without =":  {"--set", "campaign"},
		"unknown view":   {"--explain", "player.life", "--view", "pie"},
		"unknown output": {"--explain", "player.nothing"},
	} {
		if err := run(append([]string{"eval", path, "--root", root}, args...), io.Discard, io.Discard); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
	if err := run([]string{"eval", path, "--root", root, "--set", "x"}, io.Discard, io.Discard); !errors.Is(err, errSet) {
		t.Errorf("--set x: %v", err)
	}
}

func TestCompare(t *testing.T) {
	t.Parallel()
	root := writeSimRoot(t)
	a, b := buildWith(t, ""), buildWith(t, "2")
	got := runOut(t, "compare", a, b, "--root", root)
	for _, want := range []string{"3.29.3.3 -> 3.29.3.3\n", "Life: 1134 -> 1154 (+20, +1.76%)\n", `edit {"op":"allocate","nodes":[2]}`} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	got = runOut(t, "compare", b, b, "--root", root, "--version", "3.28", "--version-b", "3.29")
	for _, want := range []string{
		"3.28.0.16 -> 3.29.3.3\n", "Life: 1144 -> 1154",
		"data node 2 changed (stats): map[base_maximum_life:10] -> map[base_maximum_life:20]",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if got := runOut(t, "compare", a, a, "--root", root); !strings.Contains(got, "no output changed") {
		t.Errorf("same build:\n%s", got)
	}
}
