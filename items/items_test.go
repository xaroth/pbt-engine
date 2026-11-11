package items_test

import (
	"errors"
	"math"
	"os"
	"slices"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/items"
	"github.com/xaroth/pbt-engine/statdesc"
)

const (
	hatBase   = "Metadata/Items/Armours/Helmets/Hat"
	ringBase  = "Metadata/Items/Rings/Ring"
	beltBase  = "Metadata/Items/Belts/Belt"
	knifeBase = "Metadata/Items/Weapons/OneHandWeapons/Daggers/Knife"
)

// testMod is a mod of testReader: one stat with its range, a family of its own.
type testMod struct {
	id, stat         string
	min, max         int32
	generation, mode int32
	weights          []dataset.TagWeight
}

func w(tag string, weight int32) dataset.TagWeight {
	return dataset.TagWeight{Tag: tag, Weight: weight}
}

// testMods are the mods of testReader, with the text of each stat. Every stat of a text reads
// the same line, in this order.
//   - "+N to maximum Life" rolls on anything.
//   - "+N to Armour" is global on rings, as two mods of one stat that add up, and as a Shaper
//     mod; Ruthless only on belts; local on strength armour.
//   - "+N to Spirit" is a unique's at 10 to 50 and rolls 60 to 70 on rings, and as another stat
//     rolls 10 to 50 on rings.
//   - "+N to Ward" rolls 10 to 50 on rings, and a retired ring mod gives 60.
//   - "N% increased Critical Strike Chance" rolls on anything; the knife's implicit rolls 40 to
//     45, and a former implicit no base has gives 30.
func testMods() ([]testMod, map[string]string) {
	mods := []testMod{
		{id: "Life", stat: "base_maximum_life", min: 10, max: 50, generation: 1, weights: []dataset.TagWeight{w("default", 1000)}},
		{id: "Armour", stat: "armour", min: 10, max: 50, generation: 1, weights: []dataset.TagWeight{w("ring", 1000), w("default", 0)}},
		{id: "ArmourSuffix", stat: "armour", min: 10, max: 50, generation: 2, weights: []dataset.TagWeight{w("ring", 1000), w("default", 0)}},
		{id: "ShaperArmour", stat: "armour", min: 100, max: 150, generation: 1, weights: []dataset.TagWeight{w("ring_shaper", 1000), w("default", 0)}},
		{id: "RuthlessArmour", stat: "armour", min: 10, max: 50, generation: 1, mode: 2, weights: []dataset.TagWeight{w("belt", 1000), w("default", 0)}},
		{id: "LocalArmour", stat: "local_armour", min: 10, max: 50, generation: 1, weights: []dataset.TagWeight{w("str_armour", 1000), w("default", 0)}},
		{id: "SpiritUnique", stat: "spirit", min: 10, max: 50, generation: 3},
		{id: "SpiritHigh", stat: "spirit", min: 60, max: 70, generation: 1, weights: []dataset.TagWeight{w("ring", 1000), w("default", 0)}},
		{id: "RingSpirit", stat: "ring_spirit", min: 10, max: 50, generation: 1, weights: []dataset.TagWeight{w("ring", 1000), w("default", 0)}},
		{id: "Ward", stat: "ward", min: 10, max: 50, generation: 1, weights: []dataset.TagWeight{w("ring", 1000), w("default", 0)}},
		{id: "OldWard", stat: "old_ward", min: 60, max: 60, generation: 1, weights: []dataset.TagWeight{w("ring", 0), w("default", 0)}},
		{id: "Crit", stat: "crit", min: 10, max: 50, generation: 2, weights: []dataset.TagWeight{w("default", 1000)}},
		{id: "CritImplicitKnife", stat: "knife_crit", min: 40, max: 45, generation: 3},
		{id: "CritImplicitOld", stat: "old_knife_crit", min: 30, max: 30, generation: 3},
	}
	texts := map[string]string{
		"base_maximum_life": "+{0} to maximum Life", "armour": "+{0} to Armour", "local_armour": "+{0} to Armour",
		"spirit": "+{0} to Spirit", "ring_spirit": "+{0} to Spirit", "ward": "+{0} to Ward", "old_ward": "+{0} to Ward",
		"crit": "{0}% increased Critical Strike Chance", "knife_crit": "{0}% increased Critical Strike Chance",
		"old_knife_crit": "{0}% increased Critical Strike Chance",
	}
	return mods, texts
}

// testReader reads items of a dataset with an iron hat (a strength helmet), an iron ring, an iron
// belt and a knife, and the mods of testMods.
func testReader(t *testing.T) *items.Reader {
	t.Helper()
	mods, texts := testMods()
	d := &dataset.Dataset{
		Manifest: dataset.Manifest{Game: "poe1", Version: "3.29.3.3"},
		Items: &dataset.Items{
			Classes: []dataset.ItemClass{{ID: "Helmet"}, {ID: "Ring"}, {ID: "Belt"}, {ID: "Dagger"}},
			Tags:    []string{"str_armour", "helmet", "armour", "ring", "ring_shaper", "belt", "dagger", "weapon", "default"},
			// Each base's full tag list: its own, then those its templates give it.
			Bases: []dataset.ItemBase{
				{ID: hatBase, Class: "Helmet", Tags: []string{"str_armour", "helmet", "armour", "default"}, Armour: &dataset.ArmourValues{}},
				{ID: ringBase, Class: "Ring", Tags: []string{"ring", "default"}},
				{ID: beltBase, Class: "Belt", Tags: []string{"belt", "default"}},
				{ID: knifeBase, Class: "Dagger", Tags: []string{"dagger", "weapon", "default"}, Implicits: []string{"CritImplicitKnife"}, Weapon: &dataset.WeaponValues{}},
			},
		},
	}
	anyValue := []statdesc.Limit{{Min: math.MinInt32, Max: math.MaxInt32}}
	var descs []statdesc.Description
	for i, m := range mods {
		if !slices.ContainsFunc(d.Stats, func(s dataset.Stat) bool { return s.ID == m.stat }) {
			d.Stats = append(d.Stats, dataset.Stat{ID: m.stat})
			descs = append(descs, statdesc.Description{Stats: []string{m.stat}, Variants: []statdesc.Variant{{Limits: anyValue, Text: texts[m.stat]}}})
		}
		stat := slices.IndexFunc(d.Stats, func(s dataset.Stat) bool { return s.ID == m.stat })
		d.Mods = append(d.Mods, dataset.Mod{
			ID: mods[i].id, Domain: 1, Generation: m.generation, GameMode: m.mode, Type: m.id, Families: []string{m.id},
			Stats:        []dataset.ModStat{{Stat: uint32(stat), Min: m.min, Max: m.max, RuthlessMin: m.min, RuthlessMax: m.max}},
			SpawnWeights: m.weights,
		})
	}
	d.Texts = []dataset.Text{{
		Language:     "en",
		Descriptions: []statdesc.Scope{{Name: "Metadata/StatDescriptions/stat_descriptions.txt", Descriptions: descs}},
		BaseNames: []dataset.Name{
			{ID: hatBase, Name: "Iron Hat"}, {ID: ringBase, Name: "Iron Ring"}, {ID: beltBase, Name: "Iron Belt"}, {ID: knifeBase, Name: "Knife"},
		},
	}}
	dir := t.TempDir()
	if _, err := dataset.Write(dir, d); err != nil {
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

// oneLine is an item of one mod line.
func oneLine(r build.Rarity, base string, l build.ModLine, flags ...string) build.Item {
	return build.Item{Rarity: r, Base: base, Flags: flags, Mods: []build.ModLine{l}}
}

// named gives it the name "X".
func named(it build.Item) build.Item {
	it.Name = "X"
	return it
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
			"rare helmet: properties, tags, local armour, a flag",
			"Rarity: RARE\nDoom Cap\nIron Hat\nShaper Item\nArmour: 80\nItem Level: 70\nQuality: 20\nSockets: R-R\nLevelReq: 40\n" +
				"Implicits: 1\n{enchant}+20 to maximum Life\n+30 to Armour\n{crafted}+40 to maximum Life\n{fractured}{range:0.5}+12 to maximum Life\nCorrupted\n",
			build.Item{
				Rarity: build.Rare, Name: "Doom Cap", Base: hatBase, ItemLevel: 70, Quality: 20, Sockets: "R-R", Armour: 80,
				Flags: []string{"Shaper Item", "Corrupted"},
				Mods: []build.ModLine{
					line(build.Enchant, "+20 to maximum Life", "base_maximum_life", 20),
					line(build.Explicit, "+30 to Armour", "local_armour", 30),
					line(build.Crafted, "+40 to maximum Life", "base_maximum_life", 40),
					line(build.Fractured, "+12 to maximum Life", "base_maximum_life", 12),
				},
			},
			nil,
		},
		{
			"magic ring: base inside the name, global armour, a range, an unread line",
			"Rarity: MAGIC\nSturdy Iron Ring of Life\nImplicits: 0\n+(20-30) to Armour\nGrants a wish\n",
			build.Item{
				Rarity: build.Magic, Name: "Sturdy Iron Ring of Life", Base: ringBase,
				Mods: []build.ModLine{line(build.Explicit, "+(20-30) to Armour", "armour", 30), {Kind: build.Explicit, Text: "Grants a wish"}},
			},
			[]build.Code{items.RollAssumed, items.LineUnread},
		},
		{
			"two mods of one stat add up",
			"Rarity: NORMAL\nIron Ring\nImplicits: 0\n+70 to Armour\n",
			oneLine(build.Normal, ringBase, line(build.Explicit, "+70 to Armour", "armour", 70)),
			nil,
		},
		{
			"armour beyond the ring's mods: read as them, with a warning",
			"Rarity: NORMAL\nIron Ring\nImplicits: 0\n+120 to Armour\n",
			oneLine(build.Normal, ringBase, line(build.Explicit, "+120 to Armour", "armour", 120)),
			[]build.Code{items.RollOutOfRange},
		},
		{
			"a Shaper ring's mod spawns on the tag its influence gives",
			"Rarity: NORMAL\nIron Ring\nShaper Item\nImplicits: 0\n+120 to Armour\n",
			oneLine(build.Normal, ringBase, line(build.Explicit, "+120 to Armour", "armour", 120), "Shaper Item"),
			nil,
		},
		{
			"an exact retired mod over a current one out of range",
			"Rarity: NORMAL\nIron Ring\nImplicits: 0\n+60 to Ward\n",
			oneLine(build.Normal, ringBase, line(build.Explicit, "+60 to Ward", "old_ward", 60)),
			nil,
		},
		{
			"neither in range: the current mod, with a warning",
			"Rarity: NORMAL\nIron Ring\nImplicits: 0\n+70 to Ward\n",
			oneLine(build.Normal, ringBase, line(build.Explicit, "+70 to Ward", "ward", 70)),
			[]build.Code{items.RollOutOfRange},
		},
		{
			"armour only a Ruthless mod of the belt has: the game's order, with a warning",
			"Rarity: NORMAL\nIron Belt\nImplicits: 0\n+30 to Armour\n",
			oneLine(build.Normal, beltBase, line(build.Explicit, "+30 to Armour", "armour", 30)),
			[]build.Code{items.LineAmbiguous},
		},
		{
			"a unique's mod not on a rare",
			"Rarity: RARE\nX\nIron Ring\nImplicits: 0\n+20 to Spirit\n",
			named(oneLine(build.Rare, ringBase, line(build.Explicit, "+20 to Spirit", "ring_spirit", 20))),
			nil,
		},
		{
			"a unique's mod on a unique, beside a ring mod: the game's order, with a warning",
			"Rarity: UNIQUE\nX\nIron Ring\nImplicits: 0\n+20 to Spirit\n",
			named(oneLine(build.Unique, ringBase, line(build.Explicit, "+20 to Spirit", "spirit", 20))),
			[]build.Code{items.LineAmbiguous},
		},
		{
			"an implicit as the base's implicit",
			"Rarity: RARE\nX\nKnife\nImplicits: 1\n42% increased Critical Strike Chance\n",
			named(oneLine(build.Rare, knifeBase, line(build.Implicit, "42% increased Critical Strike Chance", "knife_crit", 42))),
			nil,
		},
		{
			"an implicit beyond the base's implicit: read as it, with a warning",
			"Rarity: RARE\nX\nKnife\nImplicits: 1\n48% increased Critical Strike Chance\n",
			named(oneLine(build.Rare, knifeBase, line(build.Implicit, "48% increased Critical Strike Chance", "knife_crit", 48))),
			[]build.Code{items.RollOutOfRange},
		},
		{
			"an implicit as a former implicit, on a rare",
			"Rarity: RARE\nX\nKnife\nImplicits: 1\n30% increased Critical Strike Chance\n",
			named(oneLine(build.Rare, knifeBase, line(build.Implicit, "30% increased Critical Strike Chance", "old_knife_crit", 30))),
			nil,
		},
		{
			"the same line as an explicit",
			"Rarity: RARE\nX\nKnife\nImplicits: 0\n30% increased Critical Strike Chance\n",
			named(oneLine(build.Rare, knifeBase, line(build.Explicit, "30% increased Critical Strike Chance", "crit", 30))),
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
			if diff := cmp.Diff(testCase.want, it); diff != "" {
				t.Errorf("item (-want +got):\n%s", diff)
			}
			var codes []build.Code
			for _, w := range warnings {
				codes = append(codes, w.Code)
			}
			if diff := cmp.Diff(testCase.codes, codes); diff != "" {
				t.Errorf("codes (-want +got):\n%s\n%v", diff, warnings)
			}
		})
	}
}

func TestParseRejects(t *testing.T) {
	t.Parallel()
	r := testReader(t)
	for text, want := range map[string]error{
		"":                                      items.ErrFormat,
		"Rarity: RARE\nDoom Cap":                items.ErrFormat,
		"Rarity: SHINY\nIron Hat\nImplicits: 0": items.ErrFormat,
		"Rarity: RARE\nDoom Cap\nIron Hat\nQuality: 20": items.ErrFormat,
		"Rarity: NORMAL\nPaper Hat\nImplicits: 0":       items.ErrUnknownBase,
		"Rarity: MAGIC\nSturdy Paper Hat\nImplicits: 0": items.ErrUnknownBase,
	} {
		if _, _, err := r.Parse(text); !errors.Is(err, want) {
			t.Errorf("%q: got %v, want %v", text, err, want)
		}
	}
}
