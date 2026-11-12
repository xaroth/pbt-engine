package items_test

import (
	"os"
	"testing"

	gocmp "github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/items"
)

// oddRingBase is a ring whose mod domain, in craftingDataset, is the jewels'.
const oddRingBase = "Metadata/Items/Rings/OddRing"

// craftingDataset is testDataset with the mod domains, bench crafts and essence mods a dataset
// holds when Items.Crafting is set. Its crafts and essences go where the stand-ins would not
// put them, so each case tells the two apart.
func craftingDataset() *dataset.Dataset {
	d := testDataset()
	it := d.Items
	it.Crafting = true
	it.Bases = append(it.Bases, dataset.ItemBase{ID: oddRingBase, Class: "Ring", Tags: []string{"ring", "default"}})
	domains := map[string]int32{
		"LifeFlask": 2, "ManaFlask": 2, "HybridFlask": 2, "UtilityFlask": 2, "Jewel": 10, "AbyssJewel": 13, "Tincture": 34,
	}
	for i := range it.Bases {
		it.Bases[i].ModDomain = max(domains[it.Bases[i].Class], 1)
	}
	it.Bases[len(it.Bases)-1].ModDomain = 10
	it.BenchCrafts = []dataset.ModClasses{
		{Mod: "CraftedEnergyShield", Classes: []string{"Helmet", "Gloves"}},
		{Mod: "CraftedLocalEnergyShield", Classes: []string{"Belt"}},
		{Mod: "CraftedWard", Classes: []string{"Amulet"}},
		{Mod: "CraftedAccuracy", Classes: []string{"Dagger"}},
		{Mod: "CraftedLocalAccuracy", Classes: []string{"Dagger"}},
		{Mod: "CraftEnchantDamage", Classes: []string{"Belt"}},
	}
	it.EssenceMods = []dataset.ModClasses{
		{Mod: "EssenceEnergyShield", Classes: []string{"Gloves"}},
		{Mod: "EssenceLocalEnergyShield", Classes: []string{"Belt"}},
		{Mod: "EssenceRingWard", Classes: []string{"Amulet"}},
	}
	d.Texts[0].BaseNames = append(d.Texts[0].BaseNames, dataset.Name{ID: oddRingBase, Name: "Odd Ring"})
	return d
}

func readerOf(t *testing.T, d *dataset.Dataset) *items.Reader {
	t.Helper()
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

type parseCase struct {
	name, text string
	want       build.Item
	codes      []build.Code
}

func runParseCases(t *testing.T, r *items.Reader, testCases []parseCase) {
	t.Helper()
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
			codes := make([]build.Code, 0, len(warnings))
			for _, w := range warnings {
				codes = append(codes, w.Code)
			}
			if diff := gocmp.Diff(testCase.codes, codes, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("codes (-want +got):\n%s\n%v", diff, warnings)
			}
		})
	}
}

// TestParseCrafting reads lines with a dataset that holds the mod domains, bench crafts and
// essence mods.
func TestParseCrafting(t *testing.T) {
	t.Parallel()
	runParseCases(t, readerOf(t, craftingDataset()), []parseCase{
		{
			"the base's mod domain from the dataset",
			"Rarity: RARE\nX\nOdd Ring\nImplicits: 0\n+30 to maximum Mana\n",
			rare(oddRingBase, line(build.Explicit, "+30 to maximum Mana", "jewel_mana", 30)),
			nil,
		},
		{
			"a local bench craft the dataset lists for belts",
			"Rarity: RARE\nX\nIron Belt\nImplicits: 0\n{crafted}+25 to maximum Energy Shield\n",
			rare(beltBase, line(build.Crafted, "+25 to maximum Energy Shield", "local_energy_shield", 25)),
			nil,
		},
		{
			"no local bench craft the dataset does not list for helmets",
			"Rarity: RARE\nX\nIron Hat\nImplicits: 0\n{crafted}+25 to maximum Energy Shield\n",
			rare(hatBase, line(build.Crafted, "+25 to maximum Energy Shield", "energy_shield", 25)),
			nil,
		},
		{
			"no bench craft the dataset does not list for rings",
			"Rarity: RARE\nX\nIron Ring\nImplicits: 0\n{crafted}+25 to Ward\n",
			rare(ringBase, line(build.Crafted, "+25 to Ward", "ward", 25)),
			[]build.Code{items.LineAmbiguous},
		},
		{
			"an essence mod the dataset lists for gloves, over a retired mod",
			"Rarity: RARE\nX\nIron Gloves\nImplicits: 0\n+33 to maximum Energy Shield\n",
			rare(glovesBase, line(build.Explicit, "+33 to maximum Energy Shield", "essence_energy_shield", 33)),
			nil,
		},
		{
			"no essence mod the dataset does not list for belts, not even as retired",
			"Rarity: RARE\nX\nIron Belt\nImplicits: 0\n+33 to maximum Energy Shield\n",
			rare(beltBase, line(build.Explicit, "+33 to maximum Energy Shield", "recombinator_energy_shield", 33)),
			nil,
		},
		{
			"a local essence mod the dataset lists for belts",
			"Rarity: RARE\nX\nIron Belt\nImplicits: 0\n+45 to maximum Energy Shield\n",
			rare(beltBase, line(build.Explicit, "+45 to maximum Energy Shield", "local_energy_shield", 45)),
			nil,
		},
		{
			"a bench enchantment the dataset lists for belts",
			"Rarity: RARE\nX\nIron Belt\nImplicits: 1\n{enchant}40% increased Skill Damage\n",
			rare(beltBase, line(build.Enchant, "40% increased Skill Damage", "craft_enchant_damage", 40)),
			nil,
		},
		{
			"no bench craft of the crafted domain as an enchantment",
			"Rarity: RARE\nX\nIron Belt\nImplicits: 1\n{enchant}+25 to maximum Energy Shield\n",
			rare(beltBase, line(build.Enchant, "+25 to maximum Energy Shield", "energy_shield", 25)),
			[]build.Code{items.LineAmbiguous},
		},
		{
			"a former bench craft, as retired",
			"Rarity: RARE\nX\nIron Ring\nImplicits: 0\n{crafted}+260 to Ward\n",
			rare(ringBase, line(build.Crafted, "+260 to Ward", "old_crafted_ward", 260)),
			nil,
		},
		{
			"a former bench craft without its marker, as retired",
			"Rarity: RARE\nX\nIron Ring\nImplicits: 0\n+260 to Ward\n",
			rare(ringBase, line(build.Explicit, "+260 to Ward", "old_crafted_ward", 260)),
			nil,
		},
		{
			"a former local bench craft on gloves, as retired",
			"Rarity: RARE\nX\nIron Gloves\nImplicits: 0\n+60 to maximum Energy Shield\n",
			rare(glovesBase, line(build.Explicit, "+60 to maximum Energy Shield", "local_energy_shield", 60)),
			nil,
		},
		{
			"no former local bench craft on a ring",
			"Rarity: RARE\nX\nIron Ring\nImplicits: 0\n+60 to maximum Energy Shield\n",
			rare(ringBase, line(build.Explicit, "+60 to maximum Energy Shield", "energy_shield", 60)),
			[]build.Code{items.LineAmbiguous},
		},
		{
			"a retired local mod that names no item kind, on gloves",
			"Rarity: RARE\nX\nIron Gloves\nImplicits: 0\n+70 to maximum Energy Shield\n",
			rare(glovesBase, line(build.Explicit, "+70 to maximum Energy Shield", "local_energy_shield", 70)),
			nil,
		},
		{
			"no retired local mod that names no item kind on a ring",
			"Rarity: RARE\nX\nIron Ring\nImplicits: 0\n+70 to maximum Energy Shield\n",
			rare(ringBase, line(build.Explicit, "+70 to maximum Energy Shield", "energy_shield", 70)),
			[]build.Code{items.LineAmbiguous},
		},
		{
			"no essence mod as retired where it is not listed, though its weights name the item",
			"Rarity: RARE\nX\nIron Ring\nImplicits: 0\n+280 to Ward\n",
			rare(ringBase, line(build.Explicit, "+280 to Ward", "ward", 280)),
			[]build.Code{items.LineAmbiguous, items.RollOutOfRange},
		},
		{
			"a former bench craft whose weights name amulets, on an amulet",
			"Rarity: RARE\nX\nIron Amulet\nImplicits: 0\n{crafted}+290 to Ward\n",
			rare(amuletBase, line(build.Crafted, "+290 to Ward", "amulet_craft_ward", 290)),
			nil,
		},
		{
			"no former bench craft whose weights name amulets on a ring",
			"Rarity: RARE\nX\nIron Ring\nImplicits: 0\n{crafted}+290 to Ward\n",
			rare(ringBase, line(build.Crafted, "+290 to Ward", "ward", 290)),
			[]build.Code{items.LineAmbiguous},
		},
		{
			"no former bench craft on a normal item",
			"Rarity: NORMAL\nIron Ring\nImplicits: 0\n+260 to Ward\n",
			build.Item{Rarity: build.Normal, Base: ringBase, Mods: []build.ModLine{line(build.Explicit, "+260 to Ward", "ward", 260)}},
			[]build.Code{items.LineAmbiguous},
		},
		{
			"no retired mod without spawn weights",
			"Rarity: RARE\nX\nIron Ring\nImplicits: 0\n+220 to Ward\n",
			rare(ringBase, line(build.Explicit, "+220 to Ward", "ward", 220)),
			[]build.Code{items.LineAmbiguous, items.RollOutOfRange},
		},
	})
}

// TestClassDomains reads a mod of each class's mod domain with a dataset that does not hold the
// bases' mod domains.
func TestClassDomains(t *testing.T) {
	t.Parallel()
	mana := func(base, stat string) build.Item {
		return rare(base, line(build.Explicit, "+30 to maximum Mana", stat, 30))
	}
	text := func(name string) string { return "Rarity: RARE\nX\n" + name + "\nImplicits: 0\n+30 to maximum Mana\n" }
	runParseCases(t, testReader(t), []parseCase{
		{"a life flask", text("Iron Flask"), mana(flaskBase, "flask_mana"), nil},
		{"a mana flask", text("Iron Mana Flask"), mana(manaFlaskBase, "flask_mana"), nil},
		{"a hybrid flask", text("Iron Hybrid Flask"), mana(hybridFlaskBase, "flask_mana"), nil},
		{"a utility flask", text("Iron Utility Flask"), mana(utilityFlaskBase, "flask_mana"), nil},
		{"a jewel", text("Iron Jewel"), mana(jewelBase, "jewel_mana"), nil},
		{"an abyss jewel", text("Iron Abyss Jewel"), mana(abyssJewelBase, "abyss_mana"), nil},
		{"a tincture", text("Iron Tincture"), mana(tinctureBase, "tincture_mana"), nil},
		{"an item", text("Iron Ring"), mana(ringBase, "mana"), nil},
	})
}
