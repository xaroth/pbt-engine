package items_test

import (
	"errors"
	"math"
	"os"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/items"
	"github.com/xaroth/pbt-engine/statdesc"
)

const (
	hatBase  = "Metadata/Items/Armours/Helmets/Hat"
	ringBase = "Metadata/Items/Rings/Ring"
	beltBase = "Metadata/Items/Belts/Belt"
	// Three bases named "Twin Ring", told apart by their implicits.
	twinFire     = "Metadata/Items/Rings/TwinFire"
	twinCold     = "Metadata/Items/Rings/TwinCold"
	twinFireHigh = "Metadata/Items/Rings/TwinFireHigh"
)

// testReader reads items of a dataset with an iron hat (a strength helmet), an iron ring and an
// iron belt.
// "+N to Armour" reads as a local and as a global stat: the local one rolls on strength armour,
// the global one on rings. "+N to maximum Life" rolls on anything; "+N to Spirit" only on a
// unique. The Twin Rings' implicits give "+N% to Fire Resistance" at 10 to 50 or at 60 to 70,
// and "+N% to Cold Resistance" at 10 to 50.
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

// testDataset is the dataset of testReader.
func testDataset() *dataset.Dataset {
	anyValue := []statdesc.Limit{{Min: math.MinInt32, Max: math.MaxInt32}}
	desc := func(stat, text string) statdesc.Description {
		return statdesc.Description{Stats: []string{stat}, Variants: []statdesc.Variant{{Limits: anyValue, Text: text}}}
	}
	ranged := func(id string, stat uint32, gen, lo, hi int32, weights ...dataset.TagWeight) dataset.Mod {
		return dataset.Mod{
			ID: id, Domain: 1, Generation: gen, Type: id,
			Stats: []dataset.ModStat{{Stat: stat, Min: lo, Max: hi, RuthlessMin: lo, RuthlessMax: hi}}, SpawnWeights: weights,
		}
	}
	mod := func(id string, stat uint32, gen int32, weights ...dataset.TagWeight) dataset.Mod {
		return ranged(id, stat, gen, 10, 50, weights...)
	}
	return &dataset.Dataset{
		Manifest: dataset.Manifest{Game: "poe1", Version: "3.29.3.3"},
		Stats: []dataset.Stat{
			{ID: "base_maximum_life"}, {ID: "armour"}, {ID: "local_armour"}, {ID: "spirit"}, {ID: "fire_res"}, {ID: "cold_res"},
		},
		Items: &dataset.Items{
			Classes: []dataset.ItemClass{{ID: "Helmet"}, {ID: "Ring"}, {ID: "Belt"}},
			Tags:    []string{"str_armour", "default", "ring"},
			Bases: []dataset.ItemBase{
				{ID: hatBase, Class: "Helmet", Tags: []string{"str_armour"}, Armour: &dataset.ArmourValues{}},
				{ID: ringBase, Class: "Ring"},
				{ID: beltBase, Class: "Belt"},
				{ID: twinFire, Class: "Ring", Implicits: []string{"FireImplicit"}},
				{ID: twinCold, Class: "Ring", Implicits: []string{"ColdImplicit"}},
				{ID: twinFireHigh, Class: "Ring", Implicits: []string{"FireImplicitHigh"}},
			},
		},
		Mods: []dataset.Mod{
			mod("Life", 0, 1, dataset.TagWeight{Tag: "default", Weight: 1000}),
			mod("Armour", 1, 1, dataset.TagWeight{Tag: "ring", Weight: 1000}, dataset.TagWeight{Tag: "default", Weight: 0}),
			mod("LocalArmour", 2, 1, dataset.TagWeight{Tag: "str_armour", Weight: 1000}, dataset.TagWeight{Tag: "default", Weight: 0}),
			mod("SpiritUnique", 3, 3),
			mod("FireImplicit", 4, 3),
			mod("ColdImplicit", 5, 3),
			ranged("FireImplicitHigh", 4, 3, 60, 70),
		},
		Texts: []dataset.Text{{
			Language: "en",
			Descriptions: []statdesc.Scope{{Name: "Metadata/StatDescriptions/stat_descriptions.txt", Descriptions: []statdesc.Description{
				desc("base_maximum_life", "+{0} to maximum Life"),
				desc("armour", "+{0} to Armour"),
				desc("local_armour", "+{0} to Armour"),
				desc("spirit", "+{0} to Spirit"),
				desc("fire_res", "+{0}% to Fire Resistance"),
				desc("cold_res", "+{0}% to Cold Resistance"),
			}}},
			BaseNames: []dataset.Name{
				{ID: hatBase, Name: "Iron Hat"}, {ID: ringBase, Name: "Iron Ring"}, {ID: beltBase, Name: "Iron Belt"},
				{ID: twinFire, Name: "Twin Ring"}, {ID: twinCold, Name: "Twin Ring"}, {ID: twinFireHigh, Name: "Twin Ring"},
			},
		}},
	}
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
			"a line of two readings: the game's order, with a warning",
			"Rarity: NORMAL\nSuperior Iron Hat\nImplicits: 0\n+30 to Armour\n",
			build.Item{Rarity: build.Normal, Base: hatBase, Mods: []build.ModLine{line(build.Explicit, "+30 to Armour", "armour", 30)}},
			[]build.Code{items.LineAmbiguous},
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
			nil,
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
