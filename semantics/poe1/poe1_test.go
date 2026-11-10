package poe1_test

import (
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/resolve"
	"github.com/xaroth/pbt-engine/semantics"
	"github.com/xaroth/pbt-engine/semantics/poe1"
	"github.com/xaroth/pbt-engine/statdesc"
)

//nolint:gochecknoglobals // the testing package parses flags before any test runs, so the flag must exist at package init
var update = flag.Bool("update", false, "rewrite the golden files of excluded stats and the fixture datasets from $PBT_DATASETS")

// versions are the real datasets the tests read from $PBT_DATASETS.
func versions() []string {
	return []string{"3.28.0.16", "3.29.3.3"}
}

// openReal opens the dataset of exactly version among the datasets under $PBT_DATASETS
// (one per directory: pulled from pbt-data, or one per version), and skips without it.
func openReal(tb testing.TB, version string) *dataset.Store {
	tb.Helper()
	root := os.Getenv("PBT_DATASETS")
	if root == "" {
		tb.Skip("PBT_DATASETS not set")
	}
	reg, err := dataset.OpenRoot(os.DirFS(root))
	if reg == nil {
		tb.Fatal(err)
	}
	res, err := reg.Resolve("poe1", version)
	if err != nil {
		tb.Fatalf("dataset %s: %v", version, err)
	}
	if got := res.Store.Manifest().Version; got != version {
		tb.Skipf("PBT_DATASETS has %s, not %s", got, version)
	}
	return res.Store
}

func TestLoad(t *testing.T) {
	t.Parallel()
	if _, err := poe1.Load(); err != nil {
		t.Fatal(err)
	}
}

// checkTable checks the table against s and requires every stat that feeds the sheet to have
// an entry.
func checkTable(t *testing.T, s *dataset.Store) {
	t.Helper()
	tbl, err := poe1.Load()
	if err != nil {
		t.Fatal(err)
	}
	if err := tbl.Check(s, poe1.DescriptionFiles()); err != nil {
		t.Error(err)
	}
	feeds, err := poe1.FeedsSheet(s)
	if err != nil {
		t.Fatal(err)
	}
	c, err := semantics.Cover(s, "standard", tbl, feeds)
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range c.Stats {
		if u.Gap() {
			t.Errorf("%s %s feeds the sheet and has no entry (uses %d)", u.Stat, u.Scope, u.Uses)
		}
	}
}

func TestTableAgainstDatasets(t *testing.T) {
	t.Parallel()
	for _, version := range versions() {
		t.Run(version, func(t *testing.T) {
			t.Parallel()
			checkTable(t, openReal(t, version))
		})
	}
}

// TestTableAgainstFixtures runs the same checks on testdata/fixture, so they run without
// $PBT_DATASETS. TestFixtures keeps the fixtures in step with the real datasets.
func TestTableAgainstFixtures(t *testing.T) {
	t.Parallel()
	for _, version := range versions() {
		t.Run(version, func(t *testing.T) {
			t.Parallel()
			s, err := dataset.Open(os.DirFS(filepath.Join("testdata", "fixture", version)))
			if err != nil {
				t.Fatal(err)
			}
			checkTable(t, s)
		})
	}
}

// TestFixtures rebuilds each fixture from its real dataset under $PBT_DATASETS and compares it
// with testdata/fixture/<version>; -update rewrites them. A fixture holds only what the table
// touches: its stats, the characters with only those stats, the standard tree's nodes with
// only those stats, and the descriptions of those stats in the files the passive tree renders
// with and their parents, with only the variants the table quotes. Its manifest names the id
// of the dataset it was taken from.
func TestFixtures(t *testing.T) {
	t.Parallel()
	tbl, err := poe1.Load()
	if err != nil {
		t.Fatal(err)
	}
	for _, version := range versions() {
		t.Run(version, func(t *testing.T) {
			t.Parallel()
			d := fixture(t, tbl, openReal(t, version))
			dir := filepath.Join("testdata", "fixture", version)
			if *update {
				if err := os.RemoveAll(dir); err != nil {
					t.Fatal(err)
				}
				if err := os.MkdirAll(dir, 0o755); err != nil {
					t.Fatal(err)
				}
				if _, err := dataset.Write(dir, d); err != nil {
					t.Fatal(err)
				}
				return
			}
			fresh := t.TempDir()
			if _, err := dataset.Write(fresh, d); err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(readDir(t, fresh), readDir(t, dir)); diff != "" {
				t.Errorf("fixture differs from the dataset, rerun with -update (-want +got):\n%s", diff)
			}
		})
	}
}

// readDir returns the files in dir by name, with their bytes as hex for a readable diff.
func readDir(t *testing.T, dir string) map[string]string {
	t.Helper()
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, e := range ents {
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		out[e.Name()] = hex.EncodeToString(b)
	}
	return out
}

// fixture returns the part of the dataset in s that tbl touches; see TestFixtures.
func fixture(t *testing.T, tbl *semantics.Table, s *dataset.Store) *dataset.Dataset {
	t.Helper()
	m := s.Manifest()
	version, err := dataset.ParseGameVersion(m.Version)
	if err != nil {
		t.Fatal(err)
	}
	stats, err := s.Stats()
	if err != nil {
		t.Fatal(err)
	}
	chars, err := s.Characters()
	if err != nil {
		t.Fatal(err)
	}
	tree, err := s.Tree("standard")
	if err != nil {
		t.Fatal(err)
	}
	text, err := s.Text("en")
	if err != nil {
		t.Fatal(err)
	}
	wanted := wantedStats(tbl)
	d := &dataset.Dataset{Manifest: dataset.Manifest{Game: m.Game, Version: m.Version, Source: map[string]string{"fixtureOf": m.ID}}}
	handle := map[uint32]uint32{} // real handle to fixture handle
	for i, st := range stats {
		if wanted[st.ID] {
			handle[uint32(i)] = uint32(len(d.Stats))
			d.Stats = append(d.Stats, st)
		}
	}
	for _, c := range chars {
		c.Stats = keepValues(handle, c.Stats)
		d.Characters = append(d.Characters, c)
	}
	d.Trees = []dataset.Tree{fixtureTree(tree, handle)}
	d.Texts = []dataset.Text{fixtureText(text, quotedVariants(tbl, version), wanted)}
	return d
}

// wantedStats returns the stats of tbl's entries, the counters their modifiers hold on and
// their object-template sources.
func wantedStats(tbl *semantics.Table) map[string]bool {
	wanted := map[string]bool{}
	for _, e := range tbl.Entries {
		wanted[e.Stat] = true
		for _, m := range e.Mods {
			if m.If != nil {
				wanted[m.If.Stat] = true
			}
		}
		for _, src := range e.Sources {
			if src.Kind == semantics.ObjectTemplate {
				stat, _, _ := strings.Cut(src.Text, " = ")
				wanted[stat] = true
			}
		}
	}
	return wanted
}

// keepValues returns the values of svs whose stat is in handle, under the fixture handles.
func keepValues(handle map[uint32]uint32, svs []dataset.StatValue) []dataset.StatValue {
	var out []dataset.StatValue
	for _, sv := range svs {
		if h, ok := handle[sv.Stat]; ok {
			out = append(out, dataset.StatValue{Stat: h, Value: sv.Value})
		}
	}
	slices.SortFunc(out, func(a, b dataset.StatValue) int { return int(a.Stat) - int(b.Stat) })
	return out
}

func fixtureTree(tree *dataset.Tree, handle map[uint32]uint32) dataset.Tree {
	ft := dataset.Tree{Variant: tree.Variant}
	for _, n := range tree.Nodes {
		fn := dataset.Node{ID: n.ID, Kind: n.Kind, Stats: keepValues(handle, n.Stats)}
		for _, b := range n.Buffs {
			if kept := keepValues(handle, b); len(kept) > 0 {
				fn.Buffs = append(fn.Buffs, kept)
			}
		}
		for _, e := range n.MasteryEffects {
			if kept := keepValues(handle, e.Stats); len(kept) > 0 {
				fn.MasteryEffects = append(fn.MasteryEffects, dataset.MasteryEffect{ID: e.ID, Stats: kept})
			}
		}
		if len(fn.Stats)+len(fn.Buffs)+len(fn.MasteryEffects) > 0 {
			ft.Nodes = append(ft.Nodes, fn)
		}
	}
	return ft
}

// quote is a variant the table quotes, by file, stat and text.
type quote struct{ file, stat, text string }

// quotedVariants returns the handlers of each variant tbl quotes at version.
func quotedVariants(tbl *semantics.Table, version dataset.GameVersion) map[quote][]string {
	quoted := map[quote][]string{}
	for _, e := range tbl.Entries {
		if !e.Holds(version) {
			continue
		}
		for _, src := range e.Sources {
			if src.Kind == semantics.Description {
				quoted[quote{src.Ref, e.Stat, src.Text}] = src.Handlers
			}
		}
	}
	return quoted
}

func fixtureText(text *dataset.Text, quoted map[quote][]string, wanted map[string]bool) dataset.Text {
	byName := map[string]statdesc.Scope{}
	for _, sc := range text.Descriptions {
		byName[sc.Name] = sc
	}
	var files []string
	for _, file := range poe1.DescriptionFiles() {
		for f := file; f != "" && !slices.Contains(files, f); f = byName[f].Parent {
			files = append(files, f)
		}
	}
	slices.Sort(files)
	ftext := dataset.Text{Language: "en"}
	for _, f := range files {
		ftext.Descriptions = append(ftext.Descriptions, fixtureScope(byName[f], quoted, wanted))
	}
	return ftext
}

func fixtureScope(sc statdesc.Scope, quoted map[quote][]string, wanted map[string]bool) statdesc.Scope {
	fsc := statdesc.Scope{Name: sc.Name, Parent: sc.Parent}
	for _, desc := range sc.Descriptions {
		// A description without a quoted variant stays, without text, so a stat renders from
		// the same file as in the real dataset.
		if slices.ContainsFunc(desc.Stats, func(stat string) bool { return wanted[stat] }) {
			fsc.Descriptions = append(fsc.Descriptions, statdesc.Description{Stats: desc.Stats, Variants: quotedOf(sc.Name, desc, quoted)})
		}
	}
	return fsc
}

// quotedOf returns the variants of desc that the table quotes.
func quotedOf(file string, desc statdesc.Description, quoted map[quote][]string) []statdesc.Variant {
	var out []statdesc.Variant
	for _, va := range desc.Variants {
		names := make([]string, len(va.Handlers))
		for i, h := range va.Handlers {
			names[i] = h.Name
		}
		if slices.ContainsFunc(desc.Stats, func(stat string) bool {
			handlers, ok := quoted[quote{file, stat, va.Text}]
			return ok && slices.Equal(handlers, names)
		}) {
			out = append(out, va)
		}
	}
	return out
}

// TestExcluded lists, per real dataset, the stats whose text matches the sheet phrases but
// that FeedsSheet rejects, with why and the line, against testdata/excluded-<version>.txt, so
// a change to the exclusions shows in review. Rewrite the files with -update.
func TestExcluded(t *testing.T) {
	t.Parallel()
	for _, version := range versions() {
		t.Run(version, func(t *testing.T) {
			t.Parallel()
			s := openReal(t, version)
			excluded, err := poe1.Excluded(s)
			if err != nil {
				t.Fatal(err)
			}
			c, err := semantics.Cover(s, "standard", &semantics.Table{Game: "poe1"}, func(string, resolve.Scope) bool { return false })
			if err != nil {
				t.Fatal(err)
			}
			var lines []string
			for _, u := range c.Stats {
				if why, line, ok := excluded(u.Stat, u.Scope); ok {
					lines = append(lines, fmt.Sprintf("%s %s: %s: %s", u.Stat, u.Scope, why, line))
				}
			}
			slices.Sort(lines)
			got := strings.Join(lines, "\n") + "\n"
			golden := filepath.Join("testdata", "excluded-"+version+".txt")
			if *update {
				if err := os.WriteFile(golden, []byte(got), 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(string(want), got); diff != "" {
				t.Errorf("excluded stats (-want +got):\n%s", diff)
			}
		})
	}
}

// TestRules gives each rule of FeedsSheet a stat id or line it must match, and fails for a
// rule without a case.
func TestRules(t *testing.T) {
	t.Parallel()
	rules := poe1.Rules()
	testCases := []struct{ rule, input string }{
		{"feeds by id: level", "level"},
		{"feeds by id: attribute, life or mana per level", "strength_per_level"},
		{"feeds by id: base maximum resistance", "base_maximum_all_resistances_%"},
		{"not theirs by id: minion, totem or golem stat", "minion_maximum_life_+%"},
		{"not theirs by id: enemy stat", "enemies_chill_as_unfrozen"},
		{"not sheet by id: mana cost", "base_mana_cost_-%"},
		{"not sheet by id: reservation", "base_mana_reservation_efficiency_+%"},
		{"not sheet by id: penetration", "base_reduce_enemy_fire_resistance_%"},
		{"not sheet by id: recovery on events", "base_life_gained_on_enemy_death"},
		{"not sheet by id: energy shield recharge and recovery", "energy_shield_recharge_rate_+%"},
		{"not sheet by id: flask recovery", "flask_life_to_recover_+%"},
		{"not sheet by id: exposure", "fire_exposure_you_inflict_applies_extra_fire_resistance_+%"},
		{"sheet line: flat to quantity", "{0:+d} to maximum Life"},
		{"sheet line: increased or more quantity", "{0}% increased Armour"},
		{"sheet line: value of quantity", "{0:+d} Strength and Dexterity"},
		{"sheet line: regenerate life or mana", "Regenerate {0}% of Life per second"},
		{"sheet line: life or mana regeneration", "{0}% increased Mana Regeneration Rate"},
		{"sheet line: gain as extra", "Gain {0}% of Maximum Mana as Extra Maximum Energy Shield"},
		{"sheet line: converted to quantity", "{0}% of Maximum Life Converted to Energy Shield"},
		{"sheet line: quantity increased or doubled", "Armour from Equipped Body Armour is doubled"},
		{"sheet line: modifiers to quantity", "Modifiers to Attributes instead apply to Omniscience"},
		{"sheet line: grants quantity", "Grants Armour equal to {0}% of your Reserved Life to you and nearby Allies"},
		{"sheet line: gain quantity instead", "Gain Maximum Life instead of Maximum Energy Shield from Equipped Armour Items"},
		{"sheet line: defences from equipped items", "{0}% increased Energy Shield from Equipped Shield"},
		{"sheet line: how regeneration recovers", "Life Regeneration is applied to Energy Shield instead"},
		{"sheet line: removes all mana or energy shield", "Removes all Energy Shield"},
		{"sheet line: maximum life becomes", "Maximum Life becomes 1, Immune to Chaos Damage"},
		{"sheet line: resistance doubled", "Chaos Resistance is doubled"},
		{"sheet line: no attribute bonus", "Dexterity provides no bonus to Evasion Rating"},
		{"sheet line: evasion rating converted to armour", "Converts all Evasion Rating to Armour"},
		{"sheet line: resistances capped", "Elemental Resistances are capped by your highest Maximum Elemental Resistance instead"},
		{"not their line: enemies", "Nearby Enemies have {0:+d}% to Cold Resistance"},
		{"not their line: minions and totems", "Minions have {0}% increased maximum Life"},
		{"not sheet line: mana cost", "{0}% reduced Mana Cost of Skills"},
		{"not sheet line: reservation", "{0}% increased Mana Reservation Efficiency of Skills"},
		{"not sheet line: penetration", "Damage Penetrates {0}% Fire Resistance"},
		{"not sheet line: mana burn", "{0}% chance for Tinctures to not inflict Mana Burn"},
		{"not sheet line: exposure", "Fire Exposure you inflict applies an extra {0:+d}% to Fire Resistance"},
		{"not sheet line: recovery on events", "Gain {0} Life per Enemy Killed"},
		{"not sheet line: flask recovery", "{0}% increased Life Recovery from Flasks"},
		{"not sheet line: energy shield recharge and recovery", "{0}% increased Energy Shield Recharge Rate"},
		{"not sheet line: corpses", "Corpses you Spawn have {0}% increased Maximum Life"},
	}
	covered := map[string]bool{}
	for _, testCase := range testCases {
		covered[testCase.rule] = true
		t.Run(testCase.rule, func(t *testing.T) {
			t.Parallel()
			re, ok := rules[testCase.rule]
			if !ok {
				t.Fatalf("no rule %q", testCase.rule)
			}
			if !re.MatchString(testCase.input) {
				t.Errorf("%q does not match %s", testCase.input, re)
			}
		})
	}
	for name := range rules {
		if !covered[name] {
			t.Errorf("rule %q has no test case", name)
		}
	}
}

func TestFeedsSheet(t *testing.T) {
	t.Parallel()
	const (
		descriptions = "Metadata/StatDescriptions/stat_descriptions.txt"
		passive      = "Metadata/StatDescriptions/passive_skill_stat_descriptions.txt"
		aura         = "Metadata/StatDescriptions/passive_skill_aura_stat_descriptions.txt"
	)
	describe := func(stat string, texts ...string) statdesc.Description {
		d := statdesc.Description{Stats: []string{stat}}
		for _, text := range texts {
			d.Variants = append(d.Variants, statdesc.Variant{Limits: []statdesc.Limit{{Min: 1, Max: 2147483647}}, Text: text})
		}
		return d
	}
	d := &dataset.Dataset{
		Manifest: dataset.Manifest{Game: "poe1", Version: "3.29.3.3"},
		Texts: []dataset.Text{{Language: "en", Descriptions: []statdesc.Scope{
			{Name: descriptions, Descriptions: []statdesc.Description{
				describe("base_maximum_life", "{0:+d} to maximum Life"),
				describe("evasion_rating_+%", "{0}% increased Evasion Rating", "{0}% reduced Evasion Rating"),
				describe("life_regeneration_rate_per_minute_%", "Regenerate {0}% of Life per second"),
				describe("base_life_gained_on_enemy_death", "Gain {0} Life per Enemy Killed", "Lose {0} Life per Enemy Killed"),
				describe("minion_maximum_life_+%", "Minions have {0}% increased maximum Life"),
				describe("base_mana_cost_-%", "{0}% reduced Mana Cost of Skills"),
				describe("keystone_eternal_youth", `50% less Life Regeneration Rate\n50% less maximum Total Life Recovery per Second from Leech`),
				describe("keystone_ghost_reaver", `Leech Energy Shield instead of Life\nCannot Recharge Energy Shield`),
				describe("cold_damage_+%_per_25_dexterity", "{0}% increased Cold Damage per 25 Dexterity"),
				describe("maximum_physical_damage_reduction_%", "{0:+d}% to maximum Armour"),
				describe("mana_regeneration_rate_per_minute_%_if_inflicted_exposure_recently", "Regenerate {0}% of Mana per second if you've inflicted Exposure Recently"),
				describe("evasion_rating_+%_if_energy_shield_recharge_started_in_past_2_seconds",
					"{0}% increased Evasion Rating if Energy Shield Recharge has started in the past 2 seconds"),
				describe("energy_shield_recharge_rate_+%", "{0}% increased Energy Shield Recharge Rate"),
				describe("fire_exposure_you_inflict_applies_extra_fire_resistance_+%", "Fire Exposure you inflict applies an extra {0:+d}% to Fire Resistance"),
				{Stats: []string{"level"}},
			}},
			{Name: passive, Parent: descriptions},
			{Name: aura, Parent: passive, Descriptions: []statdesc.Description{
				describe("base_cold_damage_resistance_%", "Nearby Enemies have {0:+d}% to Cold Resistance"),
			}},
		}}},
	}
	dir := t.TempDir()
	if _, err := dataset.Write(dir, d); err != nil {
		t.Fatal(err)
	}
	v, err := dataset.Open(os.DirFS(dir))
	if err != nil {
		t.Fatal(err)
	}
	feeds, err := poe1.FeedsSheet(v)
	if err != nil {
		t.Fatal(err)
	}
	testCases := []struct {
		stat  string
		scope resolve.Scope
		want  bool
	}{
		{"base_maximum_life", resolve.Global, true},
		{"evasion_rating_+%", resolve.Global, true},
		{"life_regeneration_rate_per_minute_%", resolve.Global, true},
		{"life_regeneration_rate_per_minute_%", resolve.Buff, true},
		{"keystone_eternal_youth", resolve.Global, true},
		{"base_maximum_all_resistances_%", resolve.Global, true},
		{"level", resolve.Global, true},
		{"mana_regeneration_rate_per_minute_%_if_inflicted_exposure_recently", resolve.Global, true},
		{"evasion_rating_+%_if_energy_shield_recharge_started_in_past_2_seconds", resolve.Global, true},
		{"energy_shield_recharge_rate_+%", resolve.Global, false},
		{"fire_exposure_you_inflict_applies_extra_fire_resistance_+%", resolve.Global, false},
		{"base_cold_damage_resistance_%", resolve.Buff, false},
		{"base_life_gained_on_enemy_death", resolve.Global, false},
		{"minion_maximum_life_+%", resolve.Global, false},
		{"base_mana_cost_-%", resolve.Global, false},
		{"keystone_ghost_reaver", resolve.Global, false},
		{"cold_damage_+%_per_25_dexterity", resolve.Global, false},
		{"maximum_physical_damage_reduction_%", resolve.Global, false},
		{"is_player", resolve.Global, false},
	}
	for _, testCase := range testCases {
		t.Run(testCase.stat+" "+testCase.scope.String(), func(t *testing.T) {
			t.Parallel()
			if got := feeds(testCase.stat, testCase.scope); got != testCase.want {
				t.Errorf("got %v, want %v", got, testCase.want)
			}
		})
	}
}
