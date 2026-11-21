package poe1_test

import (
	"slices"
	"testing"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/internal/poe1"
	"github.com/xaroth/pbt-engine/internal/resolve"
)

func TestSpellBlock(t *testing.T) {
	t.Parallel()
	// the characters' own maximum and power charges, from Character.ot
	limits := []resolve.StatValue{{Stat: "base_maximum_spell_block_%", Value: 75}, {Stat: "max_power_charges", Value: 3}}
	testCases := []struct {
		name  string
		extra []resolve.StatValue
		want  map[string]float64
	}{
		{name: "no spell block", want: map[string]float64{"spell_block_chance_capped": 0, "maximum_spell_block_chance": 75}},
		// Mystic Bulwark (18174): 1% increased mana regeneration rate per 1%, 0.8225 * 1.06 = 0.87185
		{
			name: "mana regeneration per spell block",
			extra: []resolve.StatValue{
				{Stat: "base_spell_block_%", Value: 6}, {Stat: "mana_regeneration_rate_+%_per_1%_spell_block_chance", Value: 1},
			},
			want: map[string]float64{"spell_block_chance_capped": 6, "mana_regeneration": 0.9},
		},
		{
			name:  "capped by maximum",
			extra: []resolve.StatValue{{Stat: "base_spell_block_%", Value: 80}},
			want:  map[string]float64{"spell_block_chance": 80, "spell_block_chance_capped": 75},
		},
		{
			name:  "maximum capped at 90",
			extra: []resolve.StatValue{{Stat: "base_spell_block_%", Value: 100}, {Stat: "base_maximum_spell_block_%", Value: 20}},
			want:  map[string]float64{"maximum_spell_block_chance": 90, "spell_block_chance_capped": 90},
		},
		{
			name:  "Glancing Blows doubles",
			extra: []resolve.StatValue{{Stat: "base_spell_block_%", Value: 30}, {Stat: "keystone_glancing_blows", Value: 1}},
			want:  map[string]float64{"spell_block_chance_capped": 60},
		},
		{
			name:  "Versatile Combatant lowers the maximum",
			extra: []resolve.StatValue{{Stat: "base_spell_block_%", Value: 70}, {Stat: "keystone_versatile_combatant", Value: 1}},
			want:  map[string]float64{"maximum_spell_block_chance": 65, "spell_block_chance_capped": 65},
		},
		{
			name: "per power charge",
			extra: []resolve.StatValue{
				{Stat: "base_minimum_power_charges", Value: 2}, {Stat: "additional_spell_block_%_per_power_charge", Value: 5},
			},
			want: map[string]float64{"spell_block_chance_capped": 10},
		},
		// Vile Bastion (37492): 200 energy shield, 1% increased from intelligence and 30% from spell block
		{
			name: "energy shield increased by spell block",
			extra: []resolve.StatValue{
				{Stat: "base_maximum_energy_shield", Value: 200}, {Stat: "base_spell_block_%", Value: 30},
				{Stat: "maximum_energy_shield_increased_by_spell_block_chance", Value: 1},
			},
			want: map[string]float64{"maximum_energy_shield": 262},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			sh := open(t, resolved(marauder(1, slices.Concat(limits, testCase.extra)...)...))
			for stat, want := range testCase.want {
				if got := value(t, sh, stat); got != want {
					t.Errorf("%s: got %v, want %v", stat, got, want)
				}
			}
		})
	}
}

// A shield's own chance to block is its base's plus its local flat chance, increased by its
// local increases and rounded down: (25 + 3) * 1.81 = 50.68. "No Chance to Block" removes it.
func TestShieldBlock(t *testing.T) {
	t.Parallel()
	line := func(stat string, v int32) build.ModLine {
		return build.ModLine{Kind: build.Explicit, Stats: []build.StatValue{{Stat: stat, Value: v}}}
	}
	base := &dataset.ItemBase{Class: "Shield", Block: 25}
	local := []build.ModLine{line("local_additional_block_chance_%", 3), line("local_block_chance_+%", 81)}
	for _, tc := range []struct {
		name string
		mods []build.ModLine
		want []resolve.StatValue
	}{
		{"local flat and increase", local, []resolve.StatValue{{Stat: "base_block_chance_%_from_shield", Value: 50}}},
		{"no chance to block", append(slices.Clone(local), line("local_no_block_chance", 1)), nil},
	} {
		got := poe1.CompileItem(&build.Item{Mods: tc.mods}, base).Stats
		if !slices.Equal(got, tc.want) {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestAttackBlock(t *testing.T) {
	t.Parallel()
	// the character's own maximums and inherent dual wielding block, from Character.ot
	limits := []resolve.StatValue{
		{Stat: "maximum_block_%", Value: 75}, {Stat: "base_maximum_spell_block_%", Value: 75},
		{Stat: "inherent_block_while_dual_wielding_%", Value: 20},
	}
	shield := []resolve.StatValue{{Stat: "base_block_chance_%_from_shield", Value: 28}}
	shieldHands := poe1.Hands{Shield: true}
	claw := weapon("Claw", 10, 20, 666, 500)
	testCases := []struct {
		name  string
		extra []resolve.StatValue
		hands poe1.Hands
		want  map[string]float64
	}{
		{name: "no block", want: map[string]float64{"attack_block_chance_capped": 0, "maximum_attack_block_chance": 75}},
		{name: "held shield", extra: shield, hands: shieldHands, want: map[string]float64{"attack_block_chance_capped": 28}},
		// Determined Survivor (Gladiator, 8419) replaces the shield's 28 with 50
		{
			name:  "shield's value replaced",
			extra: append(slices.Clone(shield), resolve.StatValue{Stat: "block_chance_from_equipped_shield_is_%", Value: 50}),
			hands: shieldHands, want: map[string]float64{"attack_block_chance_capped": 50},
		},
		{name: "dual wielding", hands: poe1.Hands{MainHand: claw, OffHand: claw}, want: map[string]float64{"attack_block_chance_capped": 20}},
		{
			name:  "capped by the maximum",
			extra: []resolve.StatValue{{Stat: "additional_block_%", Value: 80}},
			want:  map[string]float64{"attack_block_chance": 80, "attack_block_chance_capped": 75, "attack_block_chance_overcapped": 5},
		},
		{
			name:  "glancing blows doubles it",
			extra: []resolve.StatValue{{Stat: "additional_block_%", Value: 30}, {Stat: "keystone_glancing_blows", Value: 1}},
			want:  map[string]float64{"attack_block_chance_capped": 60},
		},
		// 20 * 1.5 = 30 and 10 * 1.5 = 15
		{
			name: "increased chance to block attack and spell damage",
			extra: []resolve.StatValue{
				{Stat: "additional_block_%", Value: 20}, {Stat: "base_spell_block_%", Value: 10}, {Stat: "block_chance_+%", Value: 50},
			},
			want: map[string]float64{"attack_block_chance_capped": 30, "spell_block_chance_capped": 15},
		},
		// Block Mastery: 23% attack block is 4 whole steps of 5
		{
			name:  "spell block per 5% attack block",
			extra: []resolve.StatValue{{Stat: "additional_block_%", Value: 23}, {Stat: "spell_block_%_per_5_attack_block", Value: 1}},
			want:  map[string]float64{"spell_block_chance_capped": 4},
		},
		// maximum 65; 80 is 15 over it, for 2% spell block each
		{
			name:  "versatile combatant",
			extra: []resolve.StatValue{{Stat: "additional_block_%", Value: 80}, {Stat: "keystone_versatile_combatant", Value: 1}},
			want: map[string]float64{
				"maximum_attack_block_chance": 65, "attack_block_chance_capped": 65, "spell_block_chance_capped": 30,
			},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			sh := open(t, resolved(marauder(1, slices.Concat(limits, testCase.extra)...)...))
			setHands(t, sh, testCase.hands)
			for stat, want := range testCase.want {
				if got := value(t, sh, stat); got != want {
					t.Errorf("%s: got %v, want %v", stat, got, want)
				}
			}
		})
	}
}
