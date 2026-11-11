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
)

// testReader reads items of a dataset with an iron hat (a strength helmet), an iron ring and an
// iron belt.
// "+N to Armour" reads as a local and as a global stat: the local one rolls on strength armour,
// the global one on rings. "+N to maximum Life" rolls on anything; "+N to Spirit" only on a
// unique. "+N to Ward" reads as a current stat rolling 10 to 50 on rings and as a retired one,
// fixed at 60.
func testReader(t *testing.T) *items.Reader {
	t.Helper()
	anyValue := []statdesc.Limit{{Min: math.MinInt32, Max: math.MaxInt32}}
	desc := func(stat, text string) statdesc.Description {
		return statdesc.Description{Stats: []string{stat}, Variants: []statdesc.Variant{{Limits: anyValue, Text: text}}}
	}
	mod := func(id string, stat uint32, gen int32, weights ...dataset.TagWeight) dataset.Mod {
		return dataset.Mod{
			ID: id, Domain: 1, Generation: gen, Type: id,
			Stats: []dataset.ModStat{{Stat: stat, Min: 10, Max: 50, RuthlessMin: 10, RuthlessMax: 50}}, SpawnWeights: weights,
		}
	}
	d := &dataset.Dataset{
		Manifest: dataset.Manifest{Game: "poe1", Version: "3.29.3.3"},
		Stats:    []dataset.Stat{{ID: "base_maximum_life"}, {ID: "armour"}, {ID: "local_armour"}, {ID: "spirit"}, {ID: "ward"}, {ID: "old_ward"}},
		Items: &dataset.Items{
			Classes: []dataset.ItemClass{{ID: "Helmet"}, {ID: "Ring"}, {ID: "Belt"}},
			Tags:    []string{"str_armour", "default", "ring"},
			Bases: []dataset.ItemBase{
				{ID: hatBase, Class: "Helmet", Tags: []string{"str_armour"}, Armour: &dataset.ArmourValues{}},
				{ID: ringBase, Class: "Ring"},
				{ID: beltBase, Class: "Belt"},
			},
		},
		Mods: []dataset.Mod{
			mod("Life", 0, 1, dataset.TagWeight{Tag: "default", Weight: 1000}),
			mod("Armour", 1, 1, dataset.TagWeight{Tag: "ring", Weight: 1000}, dataset.TagWeight{Tag: "default", Weight: 0}),
			mod("LocalArmour", 2, 1, dataset.TagWeight{Tag: "str_armour", Weight: 1000}, dataset.TagWeight{Tag: "default", Weight: 0}),
			mod("SpiritUnique", 3, 3),
			mod("Ward", 4, 1, dataset.TagWeight{Tag: "ring", Weight: 1000}, dataset.TagWeight{Tag: "default", Weight: 0}),
			retiredWard(),
		},
		Texts: []dataset.Text{{
			Language: "en",
			Descriptions: []statdesc.Scope{{Name: "Metadata/StatDescriptions/stat_descriptions.txt", Descriptions: []statdesc.Description{
				desc("base_maximum_life", "+{0} to maximum Life"),
				desc("armour", "+{0} to Armour"),
				desc("local_armour", "+{0} to Armour"),
				desc("spirit", "+{0} to Spirit"),
				desc("ward", "+{0} to Ward"),
				desc("old_ward", "+{0} to Ward"),
			}}},
			BaseNames: []dataset.Name{{ID: hatBase, Name: "Iron Hat"}, {ID: ringBase, Name: "Iron Ring"}, {ID: beltBase, Name: "Iron Belt"}},
		}},
	}
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

// retiredWard is a ring mod that spawns no more, fixed at 60.
func retiredWard() dataset.Mod {
	return dataset.Mod{
		ID: "OldWard", Domain: 1, Generation: 1, Type: "OldWard",
		Stats:        []dataset.ModStat{{Stat: 5, Min: 60, Max: 60, RuthlessMin: 60, RuthlessMax: 60}},
		SpawnWeights: []dataset.TagWeight{{Tag: "ring", Weight: 0}, {Tag: "default", Weight: 0}},
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
			"armour out of the ring mod's range: read as it, with a warning",
			"Rarity: NORMAL\nSuperior Iron Ring\nImplicits: 0\n+90 to Armour\n",
			build.Item{Rarity: build.Normal, Base: ringBase, Mods: []build.ModLine{line(build.Explicit, "+90 to Armour", "armour", 90)}},
			[]build.Code{items.RollOutOfRange},
		},
		{
			"an exact retired mod over a current one out of range",
			"Rarity: NORMAL\nIron Ring\nImplicits: 0\n+60 to Ward\n",
			build.Item{Rarity: build.Normal, Base: ringBase, Mods: []build.ModLine{line(build.Explicit, "+60 to Ward", "old_ward", 60)}},
			nil,
		},
		{
			"neither in range: the current mod, with a warning",
			"Rarity: NORMAL\nIron Ring\nImplicits: 0\n+70 to Ward\n",
			build.Item{Rarity: build.Normal, Base: ringBase, Mods: []build.ModLine{line(build.Explicit, "+70 to Ward", "ward", 70)}},
			[]build.Code{items.RollOutOfRange},
		},
		{
			"armour no mod of the belt has: the game's order, with a warning",
			"Rarity: NORMAL\nIron Belt\nImplicits: 0\n+30 to Armour\n",
			build.Item{Rarity: build.Normal, Base: beltBase, Mods: []build.ModLine{line(build.Explicit, "+30 to Armour", "armour", 30)}},
			[]build.Code{items.LineAmbiguous},
		},
		{
			"a unique's mod only on a unique",
			"Rarity: RARE\nX\nIron Ring\nImplicits: 0\n+20 to Spirit\n",
			build.Item{Rarity: build.Rare, Name: "X", Base: ringBase, Mods: []build.ModLine{line(build.Explicit, "+20 to Spirit", "spirit", 20)}},
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
