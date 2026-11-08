package poe1_test

import (
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/resolve"
	rules "github.com/xaroth/pbt-engine/rules/poe1"
	"github.com/xaroth/pbt-engine/semantics"
	sempoe1 "github.com/xaroth/pbt-engine/semantics/poe1"
)

func table(t *testing.T) *semantics.Table {
	t.Helper()
	tb, err := sempoe1.Load()
	if err != nil {
		t.Fatal(err)
	}
	return tb
}

// constants are the dataset constants the rules read, as in 3.29.3.3.
func constants() []resolve.Constant {
	return []resolve.Constant{
		{ID: "life_per_strength", Value: 1, Per: 2},
		{ID: "mana_per_intelligence", Value: 1, Per: 2},
		{ID: "evasion_rating_+%_per_dexterity", Value: 1, Per: 5},
		{ID: "energy_shield_+%_per_intelligence", Value: 1, Per: 10},
	}
}

func resolved(bundles ...resolve.Bundle) *resolve.Resolved {
	return &resolve.Resolved{Game: "poe1", Version: "3.29.3.3", Bundles: bundles, Constants: constants()}
}

func bundle(src resolve.Source, scope resolve.Scope, stats ...resolve.StatValue) resolve.Bundle {
	return resolve.Bundle{Actor: resolve.Player, Scope: scope, Source: src, Stats: stats}
}

func open(t *testing.T, r *resolve.Resolved) *rules.Sheet {
	t.Helper()
	sh, err := rules.Open(r, table(t))
	if err != nil {
		t.Fatal(err)
	}
	return sh
}

func value(t *testing.T, sh *rules.Sheet, name string) float64 {
	t.Helper()
	v, err := sh.Value(name)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestOpenSumsModifiers(t *testing.T) {
	t.Parallel()
	sh := open(t, resolved(
		bundle(resolve.CharacterSource{ID: "Metadata/Characters/Str/Str"}, resolve.Global,
			resolve.StatValue{Stat: "base_maximum_life", Value: 38},
			resolve.StatValue{Stat: "life_per_level", Value: 12},
			resolve.StatValue{Stat: "base_strength", Value: 32},
		),
		bundle(resolve.LevelSource{}, resolve.Global, resolve.StatValue{Stat: "level", Value: 10}),
		bundle(resolve.NodeSource{ID: 1}, resolve.Global, resolve.StatValue{Stat: "maximum_life_+%", Value: 10}),
		bundle(resolve.NodeSource{ID: 2}, resolve.Buff, resolve.StatValue{Stat: "life_regeneration_rate_per_minute_%", Value: 60}),
	))
	testCases := []struct {
		stat string
		want float64
	}{
		// 38 base, 12 per level at level 10 and 16 from 32 strength, then 10% increased:
		// 191.4 rounds to 191.
		{stat: "maximum_life", want: 191},
		{stat: "strength", want: 32},
		// a node's aura scope applies to the player; per minute to per second
		{stat: "percent_of_life_regenerated_per_second", want: 1},
	}
	for _, testCase := range testCases {
		if got := value(t, sh, testCase.stat); got != testCase.want {
			t.Errorf("%s: got %v, want %v", testCase.stat, got, testCase.want)
		}
	}
	if len(sh.Notes) != 0 {
		t.Errorf("notes: got %v, want none", sh.Notes)
	}
}

func TestOpenNotes(t *testing.T) {
	t.Parallel()
	node := resolve.NodeSource{ID: 7}
	sh := open(t, resolved(bundle(node, resolve.Global,
		resolve.StatValue{Stat: "evasion_+%_if_hit_recently", Value: 10},
		resolve.StatValue{Stat: "not_a_stat", Value: 1},
		resolve.StatValue{Stat: "keystone_eternal_youth", Value: 1},
	)))
	want := []rules.Note{
		{Kind: rules.NotApplied, Source: node, Scope: resolve.Global, Stat: "evasion_+%_if_hit_recently", Text: "condition: hit recently"},
		{Kind: rules.NoEntry, Source: node, Scope: resolve.Global, Stat: "not_a_stat"},
		{
			Kind: rules.Partial, Source: node, Scope: resolve.Global, Stat: "keystone_eternal_youth",
			Text: "50% less maximum Total Life Recovery per Second from Leech; Energy Shield Recharge instead applies to Life",
		},
	}
	if diff := cmp.Diff(want, sh.Notes); diff != "" {
		t.Errorf("notes (-want +got):\n%s", diff)
	}
	if got := value(t, sh, "evasion_rating"); got != 0 {
		t.Errorf("evasion_rating: got %v, want 0, the unknown entry left out", got)
	}
}

func TestOpenRuleMissing(t *testing.T) {
	t.Parallel()
	tb, err := semantics.Load([]byte(`{"format": 1, "game": "poe1", "entries": [
		{"stat": "x", "scope": "global", "status": "authored", "rule": "gain_as_extra", "mods": [{"stat":"armour","op":"base"}], "sources": [{"kind":"stat-id"}]}
	]}`), rules.Vocabulary{})
	if err != nil {
		t.Fatal(err)
	}
	r := resolved(bundle(resolve.NodeSource{ID: 1}, resolve.Global, resolve.StatValue{Stat: "x", Value: 5}))
	sh, err := rules.Open(r, tb)
	if err != nil {
		t.Fatal(err)
	}
	want := []rules.Note{{Kind: rules.RuleMissing, Source: resolve.NodeSource{ID: 1}, Scope: resolve.Global, Stat: "x", Text: "gain_as_extra"}}
	if diff := cmp.Diff(want, sh.Notes); diff != "" {
		t.Errorf("notes (-want +got):\n%s", diff)
	}
	if got := value(t, sh, "armour"); got != 5 {
		t.Errorf("armour: got %v, want 5, the modifiers applied", got)
	}
}

func TestOpenErrors(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name string
		r    *resolve.Resolved
		is   error
	}{
		{name: "other game", r: &resolve.Resolved{Game: "poe2", Version: "0.3.0"}, is: rules.ErrOtherGame},
		{name: "bad version", r: &resolve.Resolved{Game: "poe1", Version: "x"}, is: dataset.ErrBadVersion},
		{
			name: "other actor", is: rules.ErrActor,
			r: resolved(resolve.Bundle{Actor: resolve.Player + 1, Source: resolve.LevelSource{}}),
		},
		{name: "missing constant", is: rules.ErrConstant, r: &resolve.Resolved{Game: "poe1", Version: "3.29"}},
		{
			name: "malformed constant", is: rules.ErrConstant,
			r: &resolve.Resolved{Game: "poe1", Version: "3.29", Constants: []resolve.Constant{{ID: "life_per_strength", Value: 1}}},
		},
		{
			name: "unsupported capability", is: dataset.ErrUnsupported,
			r: &resolve.Resolved{Game: "poe1", Version: "3.29", Capabilities: dataset.Capabilities{"armour": 2}},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			if _, err := rules.Open(testCase.r, table(t)); !errors.Is(err, testCase.is) {
				t.Errorf("got %v, want errors.Is %v", err, testCase.is)
			}
		})
	}
}

func TestValueUnknownStat(t *testing.T) {
	t.Parallel()
	if _, err := open(t, resolved()).Value("life"); !errors.Is(err, rules.ErrUnknownStat) {
		t.Errorf("got %v, want errors.Is %v", err, rules.ErrUnknownStat)
	}
}
