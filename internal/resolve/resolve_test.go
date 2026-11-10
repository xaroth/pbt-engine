package resolve_test

import (
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/internal/resolve"
)

const marauder = "Metadata/Characters/Str/Str"

// statIDs are the stats of testDataset. Handles in it index into this list.
func statIDs() []string {
	return []string{
		"level", "life_per_level", "base_maximum_all_resistances_%", "additional_strength", "maximum_life_+%", "aura_damage_+%",
		"base_strength", "base_dexterity", "base_intelligence", "base_maximum_life", "base_maximum_mana",
		"base_resist_all_elements_%", "base_chaos_damage_resistance_%",
	}
}

func testDataset() *dataset.Dataset {
	d := &dataset.Dataset{
		Manifest: dataset.Manifest{Game: "poe1", Version: "3.29.3.3"},
		Characters: []dataset.Character{{
			ID: marauder, BaseStrength: 32, BaseDexterity: 14, BaseIntelligence: 14, BaseMaxLife: 38, BaseMaxMana: 34,
			Stats: []dataset.StatValue{{Stat: 0, Value: 1}, {Stat: 1, Value: 12}, {Stat: 2, Value: 75}},
		}},
		Constants: []dataset.Constant{{ID: "life_per_strength", Value: 1, Per: 2}},
		Quests: []dataset.Quest{
			{Quest: "a2q2", Flag: "a2q2-QuestRewardTaken", Act: 2, Stats: []dataset.StatValue{{Stat: 11, Value: 15}}},
			{Quest: "a5q6", Flag: "a5q6-CompletedBoss", Act: 5, Stats: []dataset.StatValue{{Stat: 11, Value: -30}, {Stat: 12, Value: -30}}},
			{Quest: "a10q3", Flag: "a10q3-UsedRewardBook", Act: 10, PassivePoints: 2},
			{Quest: "a10q3", Flag: "EndGameAvailable", Act: 10, Stats: []dataset.StatValue{{Stat: 11, Value: -30}, {Stat: 12, Value: -30}}},
		},
		Trees: []dataset.Tree{{
			Variant: "standard",
			Classes: []dataset.Class{{Character: marauder, StartNode: 1, Ascendancies: []dataset.Ascendancy{{ID: "Juggernaut"}}}},
			Nodes: []dataset.Node{
				{ID: 1, Kind: dataset.ClassStart, Neighbours: []uint32{4, 7}},
				{ID: 4, Neighbours: []uint32{1}, Stats: []dataset.StatValue{{Stat: 3, Value: 10}}},
				{ID: 7, Kind: dataset.Notable, Neighbours: []uint32{1, 9}, Stats: []dataset.StatValue{{Stat: 3, Value: 20}, {Stat: 4, Value: 8}},
					Buffs: [][]dataset.StatValue{{{Stat: 5, Value: 5}}}},
				{ID: 9, Kind: dataset.Mastery, Neighbours: []uint32{7}, MasteryEffects: []dataset.MasteryEffect{{ID: 1, Stats: []dataset.StatValue{{Stat: 4, Value: 10}}}}},
			},
		}},
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

func testBuild(level int, nodes ...uint32) *build.Build {
	return &build.Build{Format: 1, Game: "poe1", Version: "3.29", Class: marauder, Level: level, Nodes: nodes}
}

func resolveBuild(t *testing.T, s *dataset.Store, b *build.Build) (*resolve.Resolved, error) {
	t.Helper()
	c, _, err := build.Check(b, s)
	if err != nil {
		t.Fatal(err)
	}
	r, err := resolve.Resolve(c)
	return r, wrap(t, "checked build", err)
}

func TestResolve(t *testing.T) {
	t.Parallel()
	s := open(t, testDataset())
	r, err := resolveBuild(t, s, testBuild(90, 7, 1, 4))
	if err != nil {
		t.Fatal(err)
	}
	global := func(src resolve.Source, stats ...resolve.StatValue) resolve.Bundle {
		return resolve.Bundle{Actor: resolve.Player, Scope: resolve.Global, Source: src, Stats: stats}
	}
	resists := func(v float64) []resolve.StatValue {
		return []resolve.StatValue{{"base_resist_all_elements_%", v}, {"base_chaos_damage_resistance_%", v}}
	}
	want := &resolve.Resolved{
		Game: "poe1", Version: "3.29.3.3",
		Constants: []resolve.Constant{{ID: "life_per_strength", Value: 1, Per: 2}},
		Bundles: []resolve.Bundle{
			global(resolve.CharacterSource{ID: "Metadata/Characters/Str/Str"},
				resolve.StatValue{"base_strength", 32}, resolve.StatValue{"base_dexterity", 14}, resolve.StatValue{"base_intelligence", 14},
				resolve.StatValue{"base_maximum_life", 38}, resolve.StatValue{"base_maximum_mana", 34}, resolve.StatValue{"life_per_level", 12},
				resolve.StatValue{"base_maximum_all_resistances_%", 75}),
			global(resolve.LevelSource{}, resolve.StatValue{"level", 90}),
			global(resolve.QuestSource{ID: "a2q2", Flag: "a2q2-QuestRewardTaken"}, resolve.StatValue{"base_resist_all_elements_%", 15}),
			global(resolve.QuestSource{ID: "a5q6", Flag: "a5q6-CompletedBoss"}, resists(-30)...),
			global(resolve.QuestSource{ID: "a10q3", Flag: "EndGameAvailable"}, resists(-30)...),
			global(resolve.NodeSource{ID: 4}, resolve.StatValue{"additional_strength", 10}),
			global(resolve.NodeSource{ID: 7}, resolve.StatValue{"additional_strength", 20}, resolve.StatValue{"maximum_life_+%", 8}),
			{Actor: resolve.Player, Scope: resolve.Buff, Source: resolve.NodeSource{ID: 7}, Stats: []resolve.StatValue{{"aura_damage_+%", 5}}},
		},
	}
	if diff := cmp.Diff(want, r); diff != "" {
		t.Errorf("resolved (-want +got):\n%s", diff)
	}
}

func TestResolveCapabilities(t *testing.T) {
	t.Parallel()
	d := testDataset()
	d.Manifest.Capabilities = dataset.Capabilities{"life-from-strength": 2, "armour": 3}
	r, err := resolveBuild(t, open(t, d), testBuild(1))
	if err != nil {
		t.Fatal(err)
	}
	if got := r.Capabilities["armour"]; got != 3 {
		t.Errorf("armour version: got %d, want 3", got)
	}
	if diff := cmp.Diff(d.Manifest.Capabilities, r.Capabilities); diff != "" {
		t.Errorf("capabilities (-want +got):\n%s", diff)
	}
}

func TestResolveSources(t *testing.T) {
	t.Parallel()
	s := open(t, testDataset())
	r, err := resolveBuild(t, s, testBuild(1, 1, 7))
	if err != nil {
		t.Fatal(err)
	}
	got := make([]resolve.Source, 0, len(r.Bundles))
	for _, b := range r.Bundles {
		got = append(got, b.Source)
	}
	want := []resolve.Source{
		resolve.CharacterSource{ID: marauder}, resolve.LevelSource{},
		resolve.QuestSource{ID: "a2q2", Flag: "a2q2-QuestRewardTaken"}, resolve.QuestSource{ID: "a5q6", Flag: "a5q6-CompletedBoss"},
		resolve.QuestSource{ID: "a10q3", Flag: "EndGameAvailable"},
		resolve.NodeSource{ID: 7}, resolve.NodeSource{ID: 7},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("sources (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff([]resolve.Constant{{ID: "life_per_strength", Value: 1, Per: 2}}, r.Constants); diff != "" {
		t.Errorf("constants (-want +got):\n%s", diff)
	}
}

// nodeSources lists the node sources of r's bundles in order.
func nodeSources(r *resolve.Resolved) []resolve.NodeSource {
	var out []resolve.NodeSource
	for _, b := range r.Bundles {
		if n, ok := b.Source.(resolve.NodeSource); ok {
			out = append(out, n)
		}
	}
	return out
}

func TestResolveMasteries(t *testing.T) {
	t.Parallel()
	s := open(t, testDataset())
	b := testBuild(1, 1, 7, 9)
	b.Masteries = map[uint32]uint32{9: 1}
	r, err := resolveBuild(t, s, b)
	if err != nil {
		t.Fatal(err)
	}
	want := []resolve.NodeSource{{ID: 7}, {ID: 7}, {ID: 9, Effect: 1}}
	if diff := cmp.Diff(want, nodeSources(r)); diff != "" {
		t.Errorf("node sources (-want +got):\n%s", diff)
	}
}

// A mastery's stat is its group's counter, in Counters whether the mastery is allocated or not.
func TestCounters(t *testing.T) {
	t.Parallel()
	d := testDataset()
	d.Stats = append(d.Stats, dataset.Stat{ID: "number_of_life_masteries"})
	d.Trees[0].Nodes[3].Stats = []dataset.StatValue{{Stat: uint32(len(d.Stats) - 1), Value: 1}}
	s := open(t, d)
	r, err := resolveBuild(t, s, testBuild(1, 1))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"number_of_life_masteries"}
	if diff := cmp.Diff(want, r.Counters); diff != "" {
		t.Errorf("Resolved.Counters (-want +got):\n%s", diff)
	}
	got, err := resolve.Counters(s, "standard")
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("Counters (-want +got):\n%s", diff)
	}
}

// Tree holds every node and mastery effect, allocated or not.
func TestTree(t *testing.T) {
	t.Parallel()
	s := open(t, testDataset())
	c, _, err := build.Check(testBuild(1, 1), s)
	if err != nil {
		t.Fatal(err)
	}
	r, err := resolve.Tree(c)
	if err != nil {
		t.Fatal(err)
	}
	want := []resolve.NodeSource{{ID: 4}, {ID: 7}, {ID: 7}, {ID: 9, Effect: 1}}
	if diff := cmp.Diff(want, nodeSources(r)); diff != "" {
		t.Errorf("node sources (-want +got):\n%s", diff)
	}
}

func TestResolveMissingStat(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name, stat string
	}{
		{name: "character column", stat: "base_maximum_mana"},
		{name: "level", stat: "level"},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			d := testDataset()
			// Swap the stat out for one no lookup uses, so handles in d stay valid.
			d.Stats[slices.Index(statIDs(), testCase.stat)].ID = "unused"
			_, err := resolveBuild(t, open(t, d), testBuild(1))
			if err == nil || !strings.Contains(err.Error(), "no stat "+testCase.stat) {
				t.Errorf("got %v, want an error naming %s", err, testCase.stat)
			}
		})
	}
}

func TestNodeBundles(t *testing.T) {
	t.Parallel()
	s := open(t, testDataset())
	node7 := resolve.NodeSource{ID: 7}
	testCases := []struct {
		name string
		node uint32
		want []resolve.Bundle
	}{
		{name: "no stats", node: 1},
		{name: "global stats and a buff", node: 7, want: []resolve.Bundle{
			{Actor: resolve.Player, Scope: resolve.Global, Source: node7, Stats: []resolve.StatValue{{"additional_strength", 20}, {"maximum_life_+%", 8}}},
			{Actor: resolve.Player, Scope: resolve.Buff, Source: node7, Stats: []resolve.StatValue{{"aura_damage_+%", 5}}},
		}},
		{name: "mastery effects only", node: 9},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			// No build allocates these nodes; NodeBundles needs only the dataset.
			got, err := resolve.NodeBundles(s, "standard", testCase.node)
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(testCase.want, got); diff != "" {
				t.Errorf("(-want +got):\n%s", diff)
			}
		})
	}
}

func TestNodeBundlesErrors(t *testing.T) {
	t.Parallel()
	s := open(t, testDataset())
	testCases := []struct {
		name, variant, want string
		node                uint32
	}{
		{name: "unknown node", variant: "standard", node: 5, want: "resolve: dataset 3.29.3.3 has no node 5 in the standard tree"},
		{name: "unknown variant", variant: "ruthless", node: 1, want: "has no ruthless tree"},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			_, err := resolve.NodeBundles(s, testCase.variant, testCase.node)
			if err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Errorf("got %v, want an error containing %q", err, testCase.want)
			}
		})
	}
}

func TestBundleParts(t *testing.T) {
	t.Parallel()
	s := open(t, testDataset())
	chars, err := resolve.CharacterStats(s, marauder)
	if err != nil {
		t.Fatal(err)
	}
	wantChars := []resolve.StatValue{
		{"base_strength", 32}, {"base_dexterity", 14}, {"base_intelligence", 14}, {"base_maximum_life", 38}, {"base_maximum_mana", 34},
		{"life_per_level", 12}, {"base_maximum_all_resistances_%", 75},
	}
	if diff := cmp.Diff(wantChars, chars); diff != "" {
		t.Errorf("CharacterStats (-want +got):\n%s", diff)
	}
	effect, err := resolve.MasteryEffectBundle(s, "standard", 9, 1)
	if err != nil {
		t.Fatal(err)
	}
	wantEffect := resolve.Bundle{
		Actor: resolve.Player, Scope: resolve.Global, Source: resolve.NodeSource{ID: 9, Effect: 1},
		Stats: []resolve.StatValue{{"maximum_life_+%", 10}},
	}
	if diff := cmp.Diff(wantEffect, effect); diff != "" {
		t.Errorf("MasteryEffectBundle (-want +got):\n%s", diff)
	}
	quests, err := resolve.QuestBundles(s)
	if err != nil {
		t.Fatal(err)
	}
	wantQuest := resolve.Bundle{
		Actor: resolve.Player, Scope: resolve.Global, Source: resolve.QuestSource{ID: "a2q2", Flag: "a2q2-QuestRewardTaken"},
		Stats: []resolve.StatValue{{"base_resist_all_elements_%", 15}},
	}
	if len(quests) != 3 || !cmp.Equal(wantQuest, quests[0]) {
		t.Errorf("QuestBundles: got %v, want 3 bundles starting with %v (the points-only reward has none)", quests, wantQuest)
	}
}

func TestBundlePartsErrors(t *testing.T) {
	t.Parallel()
	s := open(t, testDataset())
	testCases := []struct {
		name, want string
		call       func() error
	}{
		{"unknown character", "resolve: dataset 3.29.3.3 has no character x", func() error {
			_, err := resolve.CharacterStats(s, "x")
			return wrap(t, "character stats", err)
		}},
		{"unknown mastery effect", "resolve: node 9 has no mastery effect 2", func() error {
			_, err := resolve.MasteryEffectBundle(s, "standard", 9, 2)
			return wrap(t, "mastery effect bundle", err)
		}},
		{"mastery effect of unknown node", "has no node 5 in the standard tree", func() error {
			_, err := resolve.MasteryEffectBundle(s, "standard", 5, 1)
			return wrap(t, "mastery effect bundle", err)
		}},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			if err := testCase.call(); err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Errorf("got %v, want an error containing %q", err, testCase.want)
			}
		})
	}
}

// wrap adds context to err and keeps nil as nil.
func wrap(tb testing.TB, what string, err error) error {
	tb.Helper()
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", what, err)
}
