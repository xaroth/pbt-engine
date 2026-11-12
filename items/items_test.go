package items_test

import (
	"cmp"
	"errors"
	"math"
	"os"
	"slices"
	"testing"

	gocmp "github.com/google/go-cmp/cmp"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/items"
	"github.com/xaroth/pbt-engine/statdesc"
)

const (
	hatBase    = "Metadata/Items/Armours/Helmets/Hat"
	glovesBase = "Metadata/Items/Armours/Gloves/Gloves"
	ringBase   = "Metadata/Items/Rings/Ring"
	amuletBase = "Metadata/Items/Amulets/Amulet"
	beltBase   = "Metadata/Items/Belts/Belt"
	knifeBase  = "Metadata/Items/Weapons/OneHandWeapons/Daggers/Knife"
	// Three bases named "Twin Ring", told apart by their implicits.
	twinFire     = "Metadata/Items/Rings/TwinFire"
	twinCold     = "Metadata/Items/Rings/TwinCold"
	twinFireHigh = "Metadata/Items/Rings/TwinFireHigh"
)

// testReader reads items of testDataset.
func testReader(t *testing.T) *items.Reader {
	t.Helper()
	dir := t.TempDir()
	if _, err := dataset.Write(dir, testDataset()); err != nil {
		t.Fatal(err)
	}
	s, err := dataset.Open(os.DirFS(dir))
	if err != nil {
		t.Fatal(err)
	}
	r, err := items.NewReader(s)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// testStats are the stats of testDataset with their text, in the game's order: stats of one
// text read the same line, in this order.
func testStats() [][2]string {
	return [][2]string{
		{"base_maximum_life", "+{0} to maximum Life"},
		{"armour", "+{0} to Armour"}, {"local_armour", "+{0} to Armour"},
		{"spirit", "+{0} to Spirit"}, {"ring_spirit", "+{0} to Spirit"},
		{"ward", "+{0} to Ward"}, {"old_ward", "+{0} to Ward"},
		{"crit", "{0}% increased Critical Strike Chance"},
		// A number written into the text: the reader gives the stat a filler value.
		{"old_knife_crit", "30% increased Critical Strike Chance"},
		{"unique_speed", "{0}% increased Attack Speed"}, {"gloves_speed", "{0}% increased Attack Speed"},
		{"chaos_res", "+{0}% to Chaos Resistance"}, {"corrupted_chaos_res", "+{0}% to Chaos Resistance"},
		{"energy_shield", "+{0} to maximum Energy Shield"}, {"local_energy_shield", "+{0} to maximum Energy Shield"},
		{"skill_damage", "{0}% increased Skill Damage"}, {"enchant_damage", "{0}% increased Skill Damage"},
		{"anoint_damage", "{0}% increased Skill Damage"},
		{"fire_res", "+{0}% to Fire Resistance"}, {"cold_res", "+{0}% to Cold Resistance"},
	}
}

// testMod is a mod of testDataset: one stat with its range, and a family of its own unless
// family names one.
type testMod struct {
	id, stat, family         string
	min, max                 int32
	domain, generation, mode int32
	weights                  []dataset.TagWeight
}

func w(tag string, weight int32) dataset.TagWeight {
	return dataset.TagWeight{Tag: tag, Weight: weight}
}

// testMods are the mods of testDataset, one or a few per rule; testDataset's bases name the
// implicits.
func testMods() []testMod {
	none := []dataset.TagWeight{w("default", 0)}
	return []testMod{
		{id: "Life", stat: "base_maximum_life", min: 10, max: 50, generation: 1, weights: []dataset.TagWeight{w("default", 1000)}},
		// Global armour on rings: a prefix and a suffix that add up, and a Shaper mod. Ruthless
		// only on belts. Local on strength armour.
		{id: "Armour", stat: "armour", min: 10, max: 50, generation: 1, weights: []dataset.TagWeight{w("ring", 1000), w("default", 0)}},
		{id: "ArmourSuffix", stat: "armour", min: 10, max: 50, generation: 2, weights: []dataset.TagWeight{w("ring", 1000), w("default", 0)}},
		{id: "ShaperArmour", stat: "armour", min: 100, max: 150, generation: 1, weights: []dataset.TagWeight{w("ring_shaper", 1000), w("default", 0)}},
		{id: "RuthlessArmour", stat: "armour", min: 10, max: 50, generation: 1, mode: 2, weights: []dataset.TagWeight{w("belt", 1000), w("default", 0)}},
		{id: "LocalArmour", stat: "local_armour", min: 10, max: 50, generation: 1, weights: []dataset.TagWeight{w("str_armour", 1000), w("default", 0)}},
		// A unique's global and local armour.
		{id: "UniqueArmour", stat: "armour", min: 10, max: 50, generation: 3},
		{id: "UniqueLocalArmour", stat: "local_armour", min: 10, max: 50, generation: 3},
		// A unique's spirit, and spirit as another stat on rings.
		{id: "SpiritUnique", stat: "spirit", min: 10, max: 50, generation: 3},
		{id: "RingSpirit", stat: "ring_spirit", min: 10, max: 50, generation: 1, weights: []dataset.TagWeight{w("ring", 1000), w("default", 0)}},
		// Ward on rings as a prefix and a suffix of one family, and a retired mod that rolled on
		// rings and amulets.
		{id: "Ward", stat: "ward", min: 10, max: 50, generation: 1, weights: []dataset.TagWeight{w("ring", 1000), w("default", 0)}},
		{id: "WardSuffix", stat: "ward", family: "Ward", min: 10, max: 50, generation: 2, weights: []dataset.TagWeight{w("ring", 1000), w("default", 0)}},
		{id: "OldWard", stat: "old_ward", min: 60, max: 60, generation: 1, weights: []dataset.TagWeight{w("amulet", 0), w("ring", 0), w("default", 0)}},
		// Crit as an explicit, the knife's implicit, a unique's mod and a former implicit no base
		// has.
		{id: "Crit", stat: "crit", min: 10, max: 50, generation: 2, weights: []dataset.TagWeight{w("default", 1000)}},
		{id: "CritImplicitKnife", stat: "crit", min: 40, max: 45, generation: 3},
		{id: "CritUnique", stat: "crit", min: 25, max: 35, generation: 3},
		{id: "CritImplicitOld", stat: "old_knife_crit", min: 30, max: 30, generation: 3},
		// An eldritch implicit of gloves, and a unique's mod of the same text.
		{id: "UniqueSpeed", stat: "unique_speed", min: 5, max: 10, generation: 3},
		{id: "EldritchSpeed", stat: "gloves_speed", min: 5, max: 10, generation: 28, weights: []dataset.TagWeight{w("gloves", 1000), w("default", 0)}},
		// Chaos resistance as an explicit, and as a corrupted implicit of gloves.
		{id: "ChaosRes", stat: "chaos_res", min: 10, max: 20, generation: 2, weights: []dataset.TagWeight{w("default", 1000)}},
		{id: "CorruptedChaosRes", stat: "corrupted_chaos_res", min: 10, max: 20, generation: 5, weights: []dataset.TagWeight{w("gloves", 1000), w("default", 0)}},
		// Bench crafts of global and local energy shield, and local energy shield rolling on strength armour.
		{id: "CraftedEnergyShield", stat: "energy_shield", min: 20, max: 30, domain: 9, generation: 1, weights: none},
		{id: "CraftedLocalEnergyShield", stat: "local_energy_shield", min: 20, max: 30, domain: 9, generation: 1, weights: none},
		{id: "LocalEnergyShield", stat: "local_energy_shield", min: 5, max: 15, generation: 1, weights: []dataset.TagWeight{w("str_armour", 1000), w("default", 0)}},
		// Skill damage as an explicit, a helmet enchantment and an anointment.
		{id: "SkillDamage", stat: "skill_damage", min: 10, max: 50, generation: 1, weights: []dataset.TagWeight{w("default", 1000)}},
		{id: "EnchantDamage", stat: "enchant_damage", min: 10, max: 20, generation: 10, weights: []dataset.TagWeight{w("helmet", 1000), w("default", 0)}},
		{id: "AnointDamage", stat: "anoint_damage", min: 10, max: 20, generation: 19},
		// The Twin Rings' implicits.
		{id: "FireImplicit", stat: "fire_res", min: 10, max: 50, generation: 3},
		{id: "ColdImplicit", stat: "cold_res", min: 10, max: 50, generation: 3},
		{id: "FireImplicitHigh", stat: "fire_res", min: 60, max: 70, generation: 3},
	}
}

// testDataset has an iron hat (a strength helmet), iron gloves, an iron ring, an iron amulet, an
// iron belt, a knife and three Twin Rings, with the mods of testMods.
func testDataset() *dataset.Dataset {
	anyValue := []statdesc.Limit{{Min: math.MinInt32, Max: math.MaxInt32}}
	d := &dataset.Dataset{
		Manifest: dataset.Manifest{Game: "poe1", Version: "3.29.3.3"},
		Items: &dataset.Items{
			Classes: []dataset.ItemClass{{ID: "Helmet"}, {ID: "Gloves"}, {ID: "Ring"}, {ID: "Amulet"}, {ID: "Belt"}, {ID: "Dagger"}},
			Tags: []string{
				"str_armour", "helmet", "gloves", "armour", "ring", "ring_shaper", "amulet", "belt", "dagger", "weapon", "default",
			},
			// Each base's full tag list: its own, then those its templates give it.
			Bases: []dataset.ItemBase{
				{ID: hatBase, Class: "Helmet", Tags: []string{"str_armour", "helmet", "armour", "default"}, Armour: &dataset.ArmourValues{}},
				{ID: glovesBase, Class: "Gloves", Tags: []string{"gloves", "armour", "default"}, Armour: &dataset.ArmourValues{}},
				{ID: ringBase, Class: "Ring", Tags: []string{"ring", "default"}},
				{ID: amuletBase, Class: "Amulet", Tags: []string{"amulet", "default"}},
				{ID: beltBase, Class: "Belt", Tags: []string{"belt", "default"}},
				{ID: knifeBase, Class: "Dagger", Tags: []string{"dagger", "weapon", "default"}, Implicits: []string{"CritImplicitKnife"}, Weapon: &dataset.WeaponValues{}},
				{ID: twinFire, Class: "Ring", Tags: []string{"ring", "default"}, Implicits: []string{"FireImplicit"}},
				{ID: twinCold, Class: "Ring", Tags: []string{"ring", "default"}, Implicits: []string{"ColdImplicit"}},
				{ID: twinFireHigh, Class: "Ring", Tags: []string{"ring", "default"}, Implicits: []string{"FireImplicitHigh"}},
			},
		},
	}
	stats := testStats()
	descs := make([]statdesc.Description, 0, len(stats))
	for _, st := range stats {
		d.Stats = append(d.Stats, dataset.Stat{ID: st[0]})
		descs = append(descs, statdesc.Description{Stats: []string{st[0]}, Variants: []statdesc.Variant{{Limits: anyValue, Text: st[1]}}})
	}
	for _, m := range testMods() {
		stat := slices.IndexFunc(d.Stats, func(s dataset.Stat) bool { return s.ID == m.stat })
		d.Mods = append(d.Mods, dataset.Mod{
			ID: m.id, Domain: cmp.Or(m.domain, int32(1)), Generation: m.generation, GameMode: m.mode, Type: m.id, Families: []string{cmp.Or(m.family, m.id)},
			Stats:        []dataset.ModStat{{Stat: uint32(stat), Min: m.min, Max: m.max, RuthlessMin: m.min, RuthlessMax: m.max}},
			SpawnWeights: m.weights,
		})
	}
	d.Texts = []dataset.Text{{
		Language:     "en",
		Descriptions: []statdesc.Scope{{Name: "Metadata/StatDescriptions/stat_descriptions.txt", Descriptions: descs}},
		BaseNames: []dataset.Name{
			{ID: hatBase, Name: "Iron Hat"}, {ID: glovesBase, Name: "Iron Gloves"}, {ID: ringBase, Name: "Iron Ring"},
			{ID: amuletBase, Name: "Iron Amulet"}, {ID: beltBase, Name: "Iron Belt"}, {ID: knifeBase, Name: "Knife"},
			{ID: twinFire, Name: "Twin Ring"}, {ID: twinCold, Name: "Twin Ring"}, {ID: twinFireHigh, Name: "Twin Ring"},
		},
	}}
	return d
}

func line(kind build.ModKind, text, stat string, v int32) build.ModLine {
	return build.ModLine{Kind: kind, Text: text, Stats: []build.StatValue{{Stat: stat, Value: v}}}
}

func TestParse(t *testing.T) {
	t.Parallel()
	r := testReader(t)
	testCases := []struct {
		name, text string
		want       build.Item
		codes      []build.Code
	}{
		{
			"rare helmet: properties, tags, a flag",
			"Rarity: RARE\nDoom Cap\nIron Hat\nShaper Item\nArmour: 80\nItem Level: 70\nQuality: 20\nSockets: R-R\nLevelReq: 40\n" +
				"Implicits: 1\n{enchant}+20 to maximum Life\n+30 to maximum Life\n{crafted}+40 to maximum Life\n{fractured}{range:0.5}+12 to maximum Life\nCorrupted\n",
			build.Item{
				Rarity: build.Rare, Name: "Doom Cap", Base: hatBase, ItemLevel: 70, Quality: 20, Sockets: "R-R", Armour: 80,
				Flags: []string{"Shaper Item", "Corrupted"},
				Mods: []build.ModLine{
					line(build.Enchant, "+20 to maximum Life", "base_maximum_life", 20),
					line(build.Explicit, "+30 to maximum Life", "base_maximum_life", 30),
					line(build.Crafted, "+40 to maximum Life", "base_maximum_life", 40),
					line(build.Fractured, "+12 to maximum Life", "base_maximum_life", 12),
				},
			},
			nil,
		},
		{
			"magic ring: base inside the name, a range, an unread line",
			"Rarity: MAGIC\nSturdy Iron Ring of Life\nImplicits: 0\n+(20-30) to maximum Life\nGrants a wish\n",
			build.Item{
				Rarity: build.Magic, Name: "Sturdy Iron Ring of Life", Base: ringBase,
				Mods: []build.ModLine{line(build.Explicit, "+(20-30) to maximum Life", "base_maximum_life", 30), {Kind: build.Explicit, Text: "Grants a wish"}},
			},
			[]build.Code{items.RollAssumed, items.LineUnread},
		},
		{
			"normal ring: no explicit mods, so the game's order, with a warning",
			"Rarity: NORMAL\nIron Ring\nImplicits: 0\n+30 to Armour\n",
			build.Item{Rarity: build.Normal, Base: ringBase, Mods: []build.ModLine{line(build.Explicit, "+30 to Armour", "armour", 30)}},
			[]build.Code{items.LineAmbiguous},
		},
		{
			"magic ring: global armour",
			"Rarity: MAGIC\nIron Ring\nImplicits: 0\n+30 to Armour\n",
			build.Item{Rarity: build.Magic, Name: "Iron Ring", Base: ringBase, Mods: []build.ModLine{line(build.Explicit, "+30 to Armour", "armour", 30)}},
			nil,
		},
		{
			"rare strength helmet: local armour",
			"Rarity: RARE\nX\nIron Hat\nImplicits: 0\n+30 to Armour\n",
			rare(hatBase, line(build.Explicit, "+30 to Armour", "local_armour", 30)),
			nil,
		},
		{
			"two mods of one stat add up",
			"Rarity: RARE\nX\nIron Ring\nImplicits: 0\n+70 to Armour\n",
			rare(ringBase, line(build.Explicit, "+70 to Armour", "armour", 70)),
			nil,
		},
		{
			"armour beyond the ring's mods: read as them, with a warning",
			"Rarity: RARE\nX\nIron Ring\nImplicits: 0\n+120 to Armour\n",
			rare(ringBase, line(build.Explicit, "+120 to Armour", "armour", 120)),
			[]build.Code{items.RollOutOfRange},
		},
		{
			"a Shaper ring's mod spawns on the tag its influence gives",
			"Rarity: RARE\nX\nIron Ring\nShaper Item\nImplicits: 0\n+120 to Armour\n",
			rare(ringBase, line(build.Explicit, "+120 to Armour", "armour", 120), "Shaper Item"),
			nil,
		},
		{
			"armour only a Ruthless mod of the belt has: the game's order, with a warning",
			"Rarity: RARE\nX\nIron Belt\nImplicits: 0\n+30 to Armour\n",
			rare(beltBase, line(build.Explicit, "+30 to Armour", "armour", 30)),
			[]build.Code{items.LineAmbiguous},
		},
		{
			"an exact retired mod of rings and amulets over a current one out of range",
			"Rarity: RARE\nX\nIron Ring\nImplicits: 0\n+60 to Ward\n",
			rare(ringBase, line(build.Explicit, "+60 to Ward", "old_ward", 60)),
			nil,
		},
		{
			"neither in range, nor two mods of one family: the current mod, with a warning",
			"Rarity: RARE\nX\nIron Ring\nImplicits: 0\n+70 to Ward\n",
			rare(ringBase, line(build.Explicit, "+70 to Ward", "ward", 70)),
			[]build.Code{items.RollOutOfRange},
		},
		{
			"a unique's mod not on a rare",
			"Rarity: RARE\nX\nIron Ring\nImplicits: 0\n+20 to Spirit\n",
			rare(ringBase, line(build.Explicit, "+20 to Spirit", "ring_spirit", 20)),
			nil,
		},
		{
			"a unique's mod on a unique, not the ring's",
			"Rarity: UNIQUE\nX\nIron Ring\nImplicits: 0\n+20 to Spirit\n",
			unique(ringBase, line(build.Explicit, "+20 to Spirit", "spirit", 20)),
			nil,
		},
		{
			"an implicit as the base's implicit",
			"Rarity: RARE\nX\nKnife\nImplicits: 1\n42% increased Critical Strike Chance\n",
			rare(knifeBase, line(build.Implicit, "42% increased Critical Strike Chance", "crit", 42)),
			nil,
		},
		{
			"an implicit beyond the base's implicit: read as it, with a warning",
			"Rarity: RARE\nX\nKnife\nImplicits: 1\n48% increased Critical Strike Chance\n",
			rare(knifeBase, line(build.Implicit, "48% increased Critical Strike Chance", "crit", 48)),
			[]build.Code{items.RollOutOfRange},
		},
		{
			"an implicit as a former implicit whose text writes the number, not as a unique's mod of the base implicit's stat",
			"Rarity: RARE\nX\nKnife\nImplicits: 1\n30% increased Critical Strike Chance\n",
			rare(knifeBase, line(build.Implicit, "30% increased Critical Strike Chance", "old_knife_crit", 1)),
			nil,
		},
		{
			"the same line as an explicit",
			"Rarity: RARE\nX\nKnife\nImplicits: 0\n30% increased Critical Strike Chance\n",
			rare(knifeBase, line(build.Explicit, "30% increased Critical Strike Chance", "crit", 30)),
			nil,
		},
		{
			"a unique knife's armour: a local stat that does not roll on it is not preferred",
			"Rarity: UNIQUE\nX\nKnife\nImplicits: 0\n+30 to Armour\n",
			unique(knifeBase, line(build.Explicit, "+30 to Armour", "armour", 30)),
			[]build.Code{items.LineAmbiguous},
		},
		{
			"an eldritch implicit over a unique's mod",
			"Rarity: RARE\nX\nIron Gloves\nImplicits: 1\n8% increased Attack Speed\n",
			rare(glovesBase, line(build.Implicit, "8% increased Attack Speed", "gloves_speed", 8)),
			nil,
		},
		{
			"no eldritch implicit on a unique",
			"Rarity: UNIQUE\nX\nIron Gloves\nImplicits: 1\n8% increased Attack Speed\n",
			unique(glovesBase, line(build.Implicit, "8% increased Attack Speed", "unique_speed", 8)),
			nil,
		},
		{
			"a corrupted implicit on a corrupted item",
			"Rarity: RARE\nX\nIron Gloves\nImplicits: 1\n+15% to Chaos Resistance\nCorrupted\n",
			rare(glovesBase, line(build.Implicit, "+15% to Chaos Resistance", "corrupted_chaos_res", 15), "Corrupted"),
			nil,
		},
		{
			"no corrupted implicit on an item not corrupted: the game's order, with a warning",
			"Rarity: RARE\nX\nIron Gloves\nImplicits: 1\n+15% to Chaos Resistance\n",
			rare(glovesBase, line(build.Implicit, "+15% to Chaos Resistance", "chaos_res", 15)),
			[]build.Code{items.LineAmbiguous},
		},
		{
			"a bench craft on a helmet: the local stat that rolls on it",
			"Rarity: RARE\nX\nIron Hat\nImplicits: 0\n{crafted}+25 to maximum Energy Shield\n",
			rare(hatBase, line(build.Crafted, "+25 to maximum Energy Shield", "local_energy_shield", 25)),
			nil,
		},
		{
			"a bench craft on a belt: no local stat",
			"Rarity: RARE\nX\nIron Belt\nImplicits: 0\n{crafted}+25 to maximum Energy Shield\n",
			rare(beltBase, line(build.Crafted, "+25 to maximum Energy Shield", "energy_shield", 25)),
			nil,
		},
		{
			"a bench craft without its marker",
			"Rarity: RARE\nX\nIron Belt\nImplicits: 0\n+25 to maximum Energy Shield\n",
			rare(beltBase, line(build.Explicit, "+25 to maximum Energy Shield", "energy_shield", 25)),
			nil,
		},
		{
			"no bench craft as a fractured line: the game's order, with a warning",
			"Rarity: RARE\nX\nIron Belt\nImplicits: 0\n{fractured}+25 to maximum Energy Shield\n",
			rare(beltBase, line(build.Fractured, "+25 to maximum Energy Shield", "energy_shield", 25)),
			[]build.Code{items.LineAmbiguous},
		},
		{
			"a helmet enchantment",
			"Rarity: RARE\nX\nIron Hat\nImplicits: 1\n{enchant}15% increased Skill Damage\n",
			rare(hatBase, line(build.Enchant, "15% increased Skill Damage", "enchant_damage", 15)),
			nil,
		},
		{
			"the same line as an explicit",
			"Rarity: RARE\nX\nIron Hat\nImplicits: 0\n15% increased Skill Damage\n",
			rare(hatBase, line(build.Explicit, "15% increased Skill Damage", "skill_damage", 15)),
			nil,
		},
		{
			"an anointment on an amulet",
			"Rarity: RARE\nX\nIron Amulet\nImplicits: 1\n{enchant}15% increased Skill Damage\n",
			rare(amuletBase, line(build.Enchant, "15% increased Skill Damage", "anoint_damage", 15)),
			nil,
		},
		{
			"bases of one name: the one whose implicit reads the line",
			"Rarity: NORMAL\nTwin Ring\nImplicits: 1\n+20% to Cold Resistance\n",
			twin(twinCold, line(build.Implicit, "+20% to Cold Resistance", "cold_res", 20)),
			nil,
		},
		{
			"bases of one name, as Path of Building names them",
			"Rarity: NORMAL\nTwin Ring (Cold)\nImplicits: 1\n+20% to Cold Resistance\n",
			twin(twinCold, line(build.Implicit, "+20% to Cold Resistance", "cold_res", 20)),
			nil,
		},
		{
			"bases of one name: the one whose implicit rolls the value",
			"Rarity: NORMAL\nTwin Ring\nImplicits: 1\n+65% to Fire Resistance\n",
			twin(twinFireHigh, line(build.Implicit, "+65% to Fire Resistance", "fire_res", 65)),
			nil,
		},
		{
			"bases of one name, a value none rolls: the one with the stat",
			"Rarity: NORMAL\nTwin Ring\nImplicits: 1\n+80% to Cold Resistance\n",
			twin(twinCold, line(build.Implicit, "+80% to Cold Resistance", "cold_res", 80)),
			[]build.Code{items.RollOutOfRange},
		},
		{
			"bases of one name, no implicit line: the first",
			"Rarity: NORMAL\nTwin Ring\nImplicits: 0\n+20% to Cold Resistance\n",
			twin(twinFire, line(build.Explicit, "+20% to Cold Resistance", "cold_res", 20)),
			nil,
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			it, warnings, err := r.Parse(testCase.text)
			if err != nil {
				t.Fatal(err)
			}
			if diff := gocmp.Diff(testCase.want, it); diff != "" {
				t.Errorf("item (-want +got):\n%s", diff)
			}
			var codes []build.Code
			for _, w := range warnings {
				codes = append(codes, w.Code)
			}
			if diff := gocmp.Diff(testCase.codes, codes); diff != "" {
				t.Errorf("codes (-want +got):\n%s\n%v", diff, warnings)
			}
		})
	}
}

// rare is a rare item named "X" of base with one line.
func rare(base string, l build.ModLine, flags ...string) build.Item {
	return build.Item{Rarity: build.Rare, Name: "X", Base: base, Flags: flags, Mods: []build.ModLine{l}}
}

// unique is a unique item named "X" of base with one line.
func unique(base string, l build.ModLine) build.Item {
	return build.Item{Rarity: build.Unique, Name: "X", Base: base, Mods: []build.ModLine{l}}
}

// twin is a normal Twin Ring of base with one line.
func twin(base string, l build.ModLine) build.Item {
	return build.Item{Rarity: build.Normal, Base: base, Mods: []build.ModLine{l}}
}

func TestParseRejects(t *testing.T) {
	t.Parallel()
	r := testReader(t)
	for text, want := range map[string]error{
		"":                                      items.ErrFormat,
		"Rarity: RARE\nDoom Cap":                items.ErrFormat,
		"Rarity: SHINY\nIron Hat\nImplicits: 0": items.ErrFormat,
		"Rarity: RARE\nDoom Cap\nIron Hat\nQuality: 20":  items.ErrFormat,
		"Rarity: NORMAL\nPaper Hat\nImplicits: 0":        items.ErrUnknownBase,
		"Rarity: NORMAL\nPaper Hat (Cold)\nImplicits: 0": items.ErrUnknownBase,
		"Rarity: MAGIC\nSturdy Paper Hat\nImplicits: 0":  items.ErrUnknownBase,
	} {
		if _, _, err := r.Parse(text); !errors.Is(err, want) {
			t.Errorf("%q: got %v, want %v", text, err, want)
		}
	}
}
