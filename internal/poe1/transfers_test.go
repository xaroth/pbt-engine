package poe1_test

import (
	"testing"

	"github.com/xaroth/pbt-engine/internal/poe1"
	"github.com/xaroth/pbt-engine/internal/resolve"
	"github.com/xaroth/pbt-engine/internal/semantics"
)

func TestTransfers(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name  string
		extra []resolve.StatValue
		want  map[string]float64
	}{
		// 10% of 66 life: 6.6 rounds to 7
		{
			name:  "life as extra armour",
			extra: []resolve.StatValue{{Stat: "maximum_life_%_to_add_as_maximum_armour", Value: 10}},
			want:  map[string]float64{"armour": 7, "maximum_life": 66},
		},
		// life 66 * 1.2 = 79.2; armour 6.6 with life's 20% and armour's 30% increased: 6.6 * 1.5 = 9.9
		{
			name: "gained share takes both increases",
			extra: []resolve.StatValue{
				{Stat: "maximum_life_%_to_add_as_maximum_armour", Value: 10},
				{Stat: "maximum_life_+%", Value: 20}, {Stat: "physical_damage_reduction_rating_+%", Value: 30},
			},
			want: map[string]float64{"armour": 10, "maximum_life": 79},
		},
		// half of 66 life goes to energy shield, which takes 1% increased from 14 intelligence
		{
			name:  "life converted to energy shield",
			extra: []resolve.StatValue{{Stat: "maximum_life_%_to_convert_to_maximum_energy_shield", Value: 50}},
			want:  map[string]float64{"maximum_life": 33, "maximum_energy_shield": 33},
		},
		// 20% of 47 mana with energy shield's 1% increased: 9.494
		{
			name:  "mana as extra energy shield",
			extra: []resolve.StatValue{{Stat: "mana_%_to_add_as_energy_shield", Value: 20}},
			want:  map[string]float64{"maximum_energy_shield": 9, "maximum_mana": 47},
		},
		// Iron Reflexes: 100 evasion (no dexterity bonus) becomes armour; "Evasion Rating and
		// Armour" counts once: 100 * 1.2
		{
			name: "iron reflexes",
			extra: []resolve.StatValue{
				{Stat: "keystone_iron_reflexes", Value: 1}, {Stat: "base_evasion_rating", Value: 85},
				{Stat: "evasion_and_physical_damage_reduction_rating_+%", Value: 20},
			},
			want: map[string]float64{"evasion_rating": 0, "armour": 120},
		},
		// Chaos Inoculation: the share starts from life's override of 1 and takes life's 50% and
		// energy shield's 1% increased: 100 * 1.01 + 1 * 1.51 = 102.51
		{
			name: "overridden source transfers its override",
			extra: []resolve.StatValue{
				{Stat: "keystone_chaos_inoculation", Value: 1}, {Stat: "maximum_life_+%", Value: 50},
				{Stat: "maximum_life_%_to_add_as_maximum_energy_shield", Value: 100}, {Stat: "base_maximum_energy_shield", Value: 100},
			},
			want: map[string]float64{"maximum_life": 1, "maximum_energy_shield": 103},
		},
		// Blood Magic removes mana, so mana as extra energy shield adds nothing: 100 * 1.01
		{
			name: "removed source transfers nothing",
			extra: []resolve.StatValue{
				{Stat: "keystone_blood_magic", Value: 1}, {Stat: "mana_%_to_add_as_energy_shield", Value: 50},
				{Stat: "base_maximum_energy_shield", Value: 100},
			},
			want: map[string]float64{"maximum_mana": 0, "maximum_energy_shield": 101},
		},
		// without Iron Reflexes the shared increase reaches each once: 100 * 1.22 with dexterity's 2%
		{
			name:  "armour and evasion increase",
			extra: []resolve.StatValue{{Stat: "base_evasion_rating", Value: 85}, {Stat: "evasion_and_physical_damage_reduction_rating_+%", Value: 20}},
			want:  map[string]float64{"evasion_rating": 122, "armour": 0},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			sh := open(t, resolved(marauder(1, testCase.extra...)...))
			for stat, want := range testCase.want {
				if got := value(t, sh, stat); got != want {
					t.Errorf("%s: got %v, want %v", stat, got, want)
				}
			}
			for _, n := range sh.Notes() {
				if n.Kind != poe1.Partial {
					t.Errorf("note: %s", n)
				}
			}
		})
	}
}

// TestTableRulesImplemented opens a sheet with every authored entry of the poe1 table that
// names a rule, so a rule or pair the table uses but these rules lack fails here.
func TestTableRulesImplemented(t *testing.T) {
	t.Parallel()
	for _, e := range table(t).Entries {
		if e.Status != semantics.Authored || e.Rule == "" {
			continue
		}
		sh := open(t, resolved(bundle(resolve.NodeSource{ID: 1}, e.Scope, resolve.StatValue{Stat: e.Stat, Value: 1})))
		for _, n := range sh.Notes() {
			if n.Kind == poe1.RuleMissing {
				t.Errorf("%s: %s", e.Stat, n)
			}
		}
	}
}
