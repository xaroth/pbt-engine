package poe1_test

import (
	"slices"
	"testing"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/internal/poe1"
	"github.com/xaroth/pbt-engine/internal/realdata"
	"github.com/xaroth/pbt-engine/internal/resolve"
	"github.com/xaroth/pbt-engine/output"
)

// marauder returns the bundles of a Marauder at level with extra stats on one node, from the
// 3.29.3.3 Characters row and Character.ot: 32 strength, 14 dexterity, 14 intelligence, 38
// life plus 12 per level, 34 mana plus 6 per level, 15 evasion rating, 1.75% of mana
// regenerated per second (105 per minute).
func marauder(level float64, extra ...resolve.StatValue) []resolve.Bundle {
	return []resolve.Bundle{
		bundle(resolve.CharacterSource{ID: "Metadata/Characters/Str/Str"}, resolve.Global,
			resolve.StatValue{Stat: "base_strength", Value: 32},
			resolve.StatValue{Stat: "base_dexterity", Value: 14},
			resolve.StatValue{Stat: "base_intelligence", Value: 14},
			resolve.StatValue{Stat: "base_maximum_life", Value: 38},
			resolve.StatValue{Stat: "base_maximum_mana", Value: 34},
			resolve.StatValue{Stat: "life_per_level", Value: 12},
			resolve.StatValue{Stat: "mana_per_level", Value: 6},
			resolve.StatValue{Stat: "base_evasion_rating", Value: 15},
			resolve.StatValue{Stat: "mana_regeneration_rate_per_minute_%", Value: 105},
		),
		bundle(resolve.LevelSource{}, resolve.Global, resolve.StatValue{Stat: "level", Value: level}),
		bundle(resolve.NodeSource{ID: 1}, resolve.Global, extra...),
	}
}

func TestAttributes(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name    string
		bundles []resolve.Bundle
		stat    string
		want    float64
	}{
		// 38 + 12 at level 1 + floor(32 / 2)
		{name: "level 1 life", bundles: marauder(1), stat: "maximum_life", want: 66},
		// 38 + 12 x 90 + floor(32 / 2)
		{name: "level 90 life", bundles: marauder(90), stat: "maximum_life", want: 1134},
		// 34 + 6 at level 1 + floor(14 / 2)
		{name: "level 1 mana", bundles: marauder(1), stat: "maximum_mana", want: 47},
		// 33 strength gives floor(33 / 2) = 16, as 32 does
		{
			name: "strength bonus rounds down", stat: "maximum_life", want: 66,
			bundles: marauder(1, resolve.StatValue{Stat: "additional_strength", Value: 1}),
		},
		// 32 * 1.05 = 33.6 rounds to 34
		{
			name: "increased strength rounds half up", stat: "strength", want: 34,
			bundles: marauder(1, resolve.StatValue{Stat: "strength_+%", Value: 5}),
		},
		// the bonus reads the rounded 34 strength: 38 + 12 + 17
		{
			name: "life bonus from increased strength", stat: "maximum_life", want: 67,
			bundles: marauder(1, resolve.StatValue{Stat: "strength_+%", Value: 5}),
		},
		// 66 * 1.25 = 82.5 rounds to 83
		{
			name: "life rounds half up", stat: "maximum_life", want: 83,
			bundles: marauder(1, resolve.StatValue{Stat: "maximum_life_+%", Value: 25}),
		},
		// 15 base evasion with floor(14 / 5) = 2% increased: 15.3 rounds to 15
		{name: "evasion from dexterity", bundles: marauder(1), stat: "evasion_rating", want: 15},
		// 100 energy shield with floor(14 / 10) = 1% increased
		{
			name: "energy shield from intelligence", stat: "maximum_energy_shield", want: 101,
			bundles: marauder(1, resolve.StatValue{Stat: "base_maximum_energy_shield", Value: 100}),
		},
		// "Strength provides no bonus to Maximum Life": 38 + 12
		{
			name: "no life from strength", stat: "maximum_life", want: 50,
			bundles: marauder(1, resolve.StatValue{Stat: "gain_no_maximum_life_from_strength", Value: 1}),
		},
		// Magebane: 15 + 85 evasion without the 2% from dexterity, which would make it 102
		{
			name: "magebane", stat: "evasion_rating", want: 100,
			bundles: marauder(1, resolve.StatValue{Stat: "keystone_magebane", Value: 1}, resolve.StatValue{Stat: "base_evasion_rating", Value: 85}),
		},
		// Solipsism: 100 energy shield without the 1% from intelligence
		{
			name: "solipsism", stat: "maximum_energy_shield", want: 100,
			bundles: marauder(1, resolve.StatValue{Stat: "keystone_solipsism", Value: 1}, resolve.StatValue{Stat: "base_maximum_energy_shield", Value: 100}),
		},
		// no life at all still leaves 1
		{name: "life at least 1", bundles: nil, stat: "maximum_life", want: 1},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			if got := value(t, open(t, resolved(testCase.bundles...)), testCase.stat); got != testCase.want {
				t.Errorf("%s: got %v, want %v", testCase.stat, got, testCase.want)
			}
		})
	}
}

// A Scion at level 90 with an Elegant Hubris of Caspiro's second revision in socket 61834, whose
// Large radius holds keystone 35255, gains no inherent bonus from attributes once the keystone,
// conquered into Supreme Ostentation, is allocated: each output loses its bonus and explains
// none; without it, each explains its bonus. 100 energy shield on the body armour slot gives the
// intelligence bonus something to increase; the default attack, unarmed, reads the accuracy and
// melee physical damage.
func TestSupremeOstentation(t *testing.T) {
	t.Parallel()
	const (
		version  = "3.29.3.6"
		scion    = "Metadata/Characters/StrDexInt/StrDexInt"
		socket   = 61834
		keystone = 35255
	)
	bonuses := []string{
		"strength-bonus-life", "strength-bonus-melee-physical-damage", "dexterity-bonus-accuracy",
		"dexterity-bonus-evasion", "intelligence-bonus-mana", "intelligence-bonus-energy-shield",
	}
	s := realdata.Open(t, version)
	chars, err := s.Characters()
	if err != nil {
		t.Fatal(err)
	}
	ch := &chars[slices.IndexFunc(chars, func(c dataset.Character) bool { return c.ID == scion })]
	sheet := func(nodes ...uint32) *poe1.Sheet {
		b := build.Build{
			Format: 1, Game: "poe1", Version: version, Class: scion, Level: 90, Nodes: nodes,
			Jewels: build.Jewels{socket: {
				Rarity: build.Unique, Base: "Metadata/Items/Jewels/JewelTimeless", Mods: []build.ModLine{{
					Kind: build.Explicit, Text: "Caspiro", Stats: []build.StatValue{
						{Stat: "local_unique_jewel_alternate_tree_version", Value: 5},
						{Stat: "local_unique_jewel_alternate_tree_seed", Value: 2000},
						{Stat: "local_unique_jewel_alternate_tree_keystone", Value: 3},
						{Stat: "local_unique_jewel_alternate_tree_internal_revision", Value: 1},
					},
				}},
			}},
		}
		c, _, err := build.Check(&b, s)
		if err != nil {
			t.Fatal(err)
		}
		r, err := resolve.Resolve(c)
		if err != nil {
			t.Fatal(err)
		}
		sh := open(t, r)
		setItems(t, sh, poe1.Item{
			Slot: build.SlotBodyArmour, Name: "robe", Key: "robe",
			Stats: []resolve.StatValue{{Stat: "base_maximum_energy_shield", Value: 100}},
		})
		hands := poe1.HandsOf(nil, ch)
		if err := sh.SetHands(hands); err != nil {
			t.Fatal(err)
		}
		skills, err := poe1.SheetSkillsOf(c, hands, false)
		if err != nil {
			t.Fatal(err)
		}
		setSkills(t, sh, skills.Skills...)
		return sh
	}
	without, with := sheet(socket), sheet(socket, keystone)
	// 20 of each attribute: 10 life, 4% melee physical damage, 40 accuracy rating, 4% evasion
	// rating, 10 mana, 2% energy shield.
	testCases := []struct {
		key          output.Key
		before, want float64
	}{
		{key: poe1.Life, before: 1128, want: 1118},
		{key: poe1.PhysicalDamageModifier, before: 104, want: 100},
		{key: poe1.AccuracyRating, before: 218, want: 178},
		{key: poe1.EvasionRating, before: 16, want: 15},
		{key: poe1.Mana, before: 584, want: 574},
		{key: poe1.EnergyShield, before: 102, want: 100},
	}
	for _, testCase := range testCases {
		bonus := func(sh *poe1.Sheet) (float64, []string) {
			ex := explainOf(t, sh, testCase.key, poe1.Proportional)
			var ids []string
			for _, l := range ex.Lines {
				if slices.Contains(bonuses, l.Source.RuleID) {
					ids = append(ids, l.Source.RuleID)
				}
			}
			return ex.Value, ids
		}
		if got, ids := bonus(without); got != testCase.before || len(ids) != 1 {
			t.Errorf("%s without the keystone: got %v from bonuses %v, want %v from one", testCase.key, got, ids, testCase.before)
		}
		if got, ids := bonus(with); got != testCase.want || len(ids) != 0 {
			t.Errorf("%s with the keystone: got %v from bonuses %v, want %v from none", testCase.key, got, ids, testCase.want)
		}
	}
}
