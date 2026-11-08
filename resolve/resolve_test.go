package resolve_test

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/resolve"
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

func testBuild(level int, scenario build.Scenario, nodes ...uint32) *build.Build {
	return &build.Build{Format: 1, Game: "poe1", Version: "3.29", Class: marauder, Level: level, Nodes: nodes, Scenario: scenario}
}

func resolveBuild(t *testing.T, s *dataset.Store, b *build.Build) (*resolve.Resolved, error) {
	t.Helper()
	c, err := build.Check(b, s)
	if err != nil {
		t.Fatal(err)
	}
	r, err := resolve.Resolve(c)
	return r, wrap(t, "checked build", err)
}

func dump(t *testing.T, r *resolve.Resolved) string {
	t.Helper()
	var out strings.Builder
	if err := r.Dump(&out); err != nil {
		t.Fatal(err)
	}
	return out.String()
}

func TestResolve(t *testing.T) {
	t.Parallel()
	s := open(t, testDataset())
	r, err := resolveBuild(t, s, testBuild(90, build.Scenario{}, 7, 1, 4))
	if err != nil {
		t.Fatal(err)
	}
	//nolint:dupword // a level bundle header followed by its level stat
	want := `dataset poe1 3.29.3.3
constant life_per_strength = 1 per 2 (unvalidated)
bundle player global character Metadata/Characters/Str/Str
  base_strength = 32
  base_dexterity = 14
  base_intelligence = 14
  base_maximum_life = 38
  base_maximum_mana = 34
  life_per_level = 12
  base_maximum_all_resistances_% = 75
bundle player global level
  level = 90
bundle player global quest a5q6
  base_resist_all_elements_% = -30
  base_chaos_damage_resistance_% = -30
bundle player global quest a10q3
  base_resist_all_elements_% = -30
  base_chaos_damage_resistance_% = -30
bundle player global node 4
  additional_strength = 10
bundle player global node 7
  additional_strength = 20
  maximum_life_+% = 8
bundle player buff node 7
  aura_damage_+% = 5
`
	if diff := cmp.Diff(want, dump(t, r)); diff != "" {
		t.Errorf("dump (-want +got):\n%s", diff)
	}
}

func TestResolveCapabilities(t *testing.T) {
	t.Parallel()
	d := testDataset()
	d.Manifest.Capabilities = dataset.Capabilities{"life-from-strength": 2, "armour": 3}
	r, err := resolveBuild(t, open(t, d), testBuild(1, build.Scenario{}))
	if err != nil {
		t.Fatal(err)
	}
	if got := r.Capabilities["armour"]; got != 3 {
		t.Errorf("armour version: got %d, want 3", got)
	}
	want := "dataset poe1 3.29.3.3\ncapability armour = 3\ncapability life-from-strength = 2\nconstant "
	if got := dump(t, r); !strings.HasPrefix(got, want) {
		t.Errorf("dump: got\n%s\nwant prefix\n%s", got, want)
	}
}

func TestResolveSources(t *testing.T) {
	t.Parallel()
	s := open(t, testDataset())
	r, err := resolveBuild(t, s, testBuild(1, build.Scenario{Campaign: build.CampaignActs6To10}, 1, 7))
	if err != nil {
		t.Fatal(err)
	}
	got := make([]resolve.Source, 0, len(r.Bundles))
	for _, b := range r.Bundles {
		got = append(got, b.Source)
	}
	want := []resolve.Source{
		resolve.CharacterSource{ID: marauder}, resolve.LevelSource{}, resolve.QuestSource{ID: "a5q6"},
		resolve.NodeSource{ID: 7}, resolve.NodeSource{ID: 7},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("sources (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff([]resolve.Constant{{ID: "life_per_strength", Value: 1, Per: 2, Validated: false}}, r.Constants); diff != "" {
		t.Errorf("constants (-want +got):\n%s", diff)
	}
}

func TestResolveScenarioAndNotes(t *testing.T) {
	t.Parallel()
	s := open(t, testDataset())
	testCases := []struct {
		name     string
		scenario build.Scenario
		nodes    []uint32
		quests   []string
		notes    []string
	}{
		{name: "campaign complete", quests: []string{"a5q6", "a10q3"}},
		{name: "acts 6 to 10", scenario: build.Scenario{Campaign: build.CampaignActs6To10}, quests: []string{"a5q6"}},
		{name: "acts 1 to 5", scenario: build.Scenario{Campaign: build.CampaignActs1To5}},
		{name: "bandits", scenario: build.Scenario{Bandits: build.BanditsHelpAlira}, quests: []string{"a5q6", "a10q3"},
			notes: []string{"bandits help-alira: quest rewards are not in the dataset yet"}},
		{name: "mastery without a choice", nodes: []uint32{1, 7, 9}, quests: []string{"a5q6", "a10q3"},
			notes: []string{"node 9: mastery effect not chosen; builds have no mastery choices yet"}},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			r, err := resolveBuild(t, s, testBuild(1, testCase.scenario, testCase.nodes...))
			if err != nil {
				t.Fatal(err)
			}
			var quests []string
			for _, b := range r.Bundles {
				if q, ok := b.Source.(resolve.QuestSource); ok {
					quests = append(quests, q.ID)
				}
			}
			if diff := cmp.Diff(testCase.quests, quests); diff != "" {
				t.Errorf("quest bundles (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(testCase.notes, r.Notes); diff != "" {
				t.Errorf("notes (-want +got):\n%s", diff)
			}
		})
	}
}

func TestResolveMissingStat(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name, stat string
		scenario   build.Scenario
	}{
		{name: "character column", stat: "base_maximum_mana", scenario: build.Scenario{Campaign: build.CampaignActs1To5}},
		{name: "level", stat: "level", scenario: build.Scenario{Campaign: build.CampaignActs1To5}},
		{name: "quest reward", stat: "base_chaos_damage_resistance_%", scenario: build.Scenario{Campaign: build.CampaignActs6To10}},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			d := testDataset()
			// Swap the stat out for one no lookup uses, so handles in d stay valid.
			d.Stats[slices.Index(statIDs(), testCase.stat)].ID = "unused"
			_, err := resolveBuild(t, open(t, d), testBuild(1, testCase.scenario))
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

func TestResolveOtherGame(t *testing.T) {
	t.Parallel()
	d := testDataset()
	d.Manifest.Game = "poe2"
	b := testBuild(1, build.Scenario{Campaign: build.CampaignActs1To5})
	b.Game = "poe2"
	_, err := resolveBuild(t, open(t, d), b)
	if !errors.Is(err, resolve.ErrNoRewards) || !strings.Contains(err.Error(), "poe2") {
		t.Errorf("got %v, want ErrNoRewards naming poe2", err)
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
		Actor: resolve.Player, Scope: resolve.Global, Source: resolve.NodeSource{ID: 9},
		Stats: []resolve.StatValue{{"maximum_life_+%", 10}},
	}
	if diff := cmp.Diff(wantEffect, effect); diff != "" {
		t.Errorf("MasteryEffectBundle (-want +got):\n%s", diff)
	}
	quests, err := resolve.CampaignBundles("poe1", build.CampaignActs6To10)
	if err != nil || len(quests) != 1 || quests[0].Source != (resolve.QuestSource{ID: "a5q6"}) {
		t.Errorf("CampaignBundles(poe1, acts 6 to 10): got %v, %v, want the a5q6 bundle", quests, err)
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
		{"campaign of another game", "resolve: no campaign quest rewards known for poe2", func() error {
			_, err := resolve.CampaignBundles("poe2", build.CampaignActs1To5)
			return wrap(t, "campaign bundles", err)
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
