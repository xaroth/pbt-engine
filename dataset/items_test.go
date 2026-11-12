package dataset_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"testing/fstest"

	flatbuffers "github.com/google/flatbuffers/go"
	gocmp "github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/dataset/internal/fb"
)

// withItems returns sample with an items group: a helmet, a shield, a sword and a flask, the
// mods they need, and a bench craft and an essence mod for helmets and shields. Stats 0 to 2 are sample's.
func withItems() *dataset.Dataset {
	d := sample()
	d.Items = &dataset.Items{
		Classes: []dataset.ItemClass{{ID: "Helmet"}, {ID: "Shield"}, {ID: "One Hand Sword"}, {ID: "LifeFlask"}},
		Tags:    []string{"default", "helmet", "str_armour", "shield", "weapon", "life", "has_attack_mod"},
		Bases: []dataset.ItemBase{
			{
				ID: "Metadata/Items/Armours/Helmets/HelmetStr1", Class: "Helmet", Tags: []string{"str_armour", "helmet", "default"}, ModDomain: 1,
				Armour: &dataset.ArmourValues{Armour: dataset.ValueRange{Min: 9, Max: 12}},
			},
			{
				ID: "Metadata/Items/Armours/Shields/ShieldStr1", Class: "Shield", Tags: []string{"shield"}, ModDomain: 1,
				Implicits: []string{"BlockImplicit"},
				Armour:    &dataset.ArmourValues{Armour: dataset.ValueRange{Min: 7, Max: 9}, MovementSpeed: -3},
				Block:     24,
			},
			{
				ID: "Metadata/Items/Weapons/OneHandWeapons/OneHandSwords/OneHandSword1", Class: "One Hand Sword",
				Tags: []string{"weapon"}, Implicits: []string{"AccuracyImplicit", "BlockImplicit"}, ModDomain: 1,
				Weapon: &dataset.WeaponValues{PhysicalDamage: dataset.ValueRange{Min: 4, Max: 9}, AttackTime: 645, CriticalChance: 500, Range: 11},
			},
			{
				ID: "Metadata/Items/Flasks/FlaskLife1", Class: "LifeFlask", ModDomain: 2,
				Flask: &dataset.FlaskValues{Kind: dataset.LifeFlask, Life: 70, Duration: 30, MaxCharges: 21, ChargesPerUse: 7},
			},
		},
		Crafting:    true,
		BenchCrafts: []dataset.ModClasses{{Mod: "CraftedLife1", Classes: []string{"Shield", "Helmet"}}},
		EssenceMods: []dataset.ModClasses{{Mod: "IncreasedLife1", Classes: []string{"Helmet", "Shield"}}},
	}
	d.Mods = []dataset.Mod{
		{
			ID: "IncreasedLife1", Domain: 1, Generation: 1, Level: 5, Type: "IncreasedLife",
			Families: []string{"IncreasedLife"},
			Stats:    []dataset.ModStat{{Stat: 1, Min: 10, Max: 24, RuthlessMin: 8, RuthlessMax: 20}},
			Tags:     []string{"life"},
			SpawnWeights: []dataset.TagWeight{
				{Tag: "weapon", Weight: 0}, {Tag: "default", Weight: 1000},
			},
		},
		{
			ID: "BlockImplicit", Domain: 1, Generation: 3, Type: "Block", GameMode: 1,
			Stats: []dataset.ModStat{{Stat: 2, Min: 4, Max: 4, RuthlessMin: 4, RuthlessMax: 4}, {Stat: 0, Min: 1, Max: 2, RuthlessMin: 1, RuthlessMax: 2}},
		},
		{
			ID: "AccuracyImplicit", Domain: 1, Generation: 3, Type: "Accuracy", Families: []string{"Accuracy", "Implicit"}, GameMode: 2,
			AddedTags:         []string{"has_attack_mod"},
			GenerationWeights: []dataset.TagWeight{{Tag: "has_attack_mod", Weight: 50}},
		},
		{
			ID: "CraftedLife1", Domain: 9, Generation: 1, Level: 15, Type: "IncreasedLife", Families: []string{"IncreasedLife"},
			Stats: []dataset.ModStat{{Stat: 1, Min: 15, Max: 25, RuthlessMin: 15, RuthlessMax: 25}},
		},
	}
	d.Texts[0].ItemClassNames = []dataset.Name{{ID: "Helmet", Name: "Helmets"}}
	d.Texts[0].BaseNames = []dataset.Name{{ID: "Metadata/Items/Armours/Helmets/HelmetStr1", Name: "Iron Hat"}}
	d.Texts[0].ModNames = []dataset.Name{{ID: "IncreasedLife1", Name: "Healthy"}}
	return d
}

// withFile returns a copy of fsys with name set to data and the manifest entry and id fixed to
// match, so the dataset fails only on data's content.
func withFile(t *testing.T, fsys fstest.MapFS, name string, data []byte) fstest.MapFS {
	t.Helper()
	out := maps.Clone(fsys)
	var m dataset.Manifest
	if err := json.Unmarshal(fsys[dataset.ManifestName].Data, &m); err != nil {
		t.Fatal(err)
	}
	i := slices.IndexFunc(m.Files, func(f dataset.File) bool { return f.Name == name })
	if i < 0 {
		t.Fatalf("no file %s in manifest", name)
	}
	sum := sha256.Sum256(data)
	m.Files[i] = dataset.File{Name: name, Size: len(data), SHA256: hex.EncodeToString(sum[:])}
	m.ID = dataset.ContentID(m)
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	out[dataset.ManifestName] = &fstest.MapFile{Data: b}
	out[name] = &fstest.MapFile{Data: data}
	return out
}

func TestItemsRoundTrip(t *testing.T) {
	t.Parallel()
	want := withItems()
	fsys := written(t, want)
	got, err := readAll(fsys)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	want.Manifest = got.Manifest
	if diff := gocmp.Diff(want, got, cmpopts.EquateEmpty()); diff != "" {
		t.Errorf("round trip mismatch (-want +got):\n%s", diff)
	}

	// Items reads only items.fb and stats are not needed; Mods needs both groups.
	s, err := dataset.Open(withFile(t, withFile(t, fsys, "mods.fb", []byte("x")), "stats.fb", []byte("x")))
	if err != nil {
		t.Fatal(err)
	}
	it, err := s.Items()
	if err != nil {
		t.Fatalf("Items with damaged mods.fb: %v", err)
	}
	if diff := gocmp.Diff(want.Items, it, cmpopts.EquateEmpty()); diff != "" {
		t.Errorf("Items mismatch (-want +got):\n%s", diff)
	}
	if _, err := s.Mods(); !errors.Is(err, dataset.ErrInvalid) {
		t.Errorf("Mods with damaged mods.fb: got %v, want ErrInvalid", err)
	}
}

func TestItemsFilesIndependentOfStatOrder(t *testing.T) {
	t.Parallel()
	b := renumbered()
	a := withItems()
	b.Items, b.Mods = a.Items, a.Mods
	for i, m := range b.Mods {
		for j, s := range m.Stats {
			b.Mods[i].Stats[j].Stat = uint32(len(a.Stats)) - s.Stat
		}
	}
	b.Texts = a.Texts
	x, y := written(t, withItems()), written(t, b)
	for _, name := range []string{"items.fb", "mods.fb"} {
		if !slices.Equal(x[name].Data, y[name].Data) {
			t.Errorf("%s: bytes differ when only stat handles differ", name)
		}
	}
}

func TestItemsMissingGroup(t *testing.T) {
	t.Parallel()
	s, err := dataset.Open(written(t, sample()))
	if err != nil {
		t.Fatal(err)
	}
	for name, get := range map[string]func() (any, error){
		"Items": func() (any, error) { return s.Items() },
		"Mods":  func() (any, error) { return s.Mods() },
	} {
		_, err := get()
		if !errors.Is(err, dataset.ErrNotInDataset) {
			t.Errorf("%s: got %v, want ErrNotInDataset", name, err)
		}
		missing, ok := errors.AsType[*dataset.MissingGroupError](err)
		if !ok || *missing != (dataset.MissingGroupError{Version: "3.29.3.3", Group: "items"}) {
			t.Errorf("%s: got %#v, want a MissingGroupError for items of 3.29.3.3", name, err)
		}
		if want := "dataset 3.29.3.3 has no items group"; err.Error() != want {
			t.Errorf("%s: got message %q, want %q", name, err, want)
		}
	}
}

func TestItemsWriteErrors(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name, want string
		change     func(d *dataset.Dataset)
	}{
		{name: "mods without items", want: "mods without items", change: func(d *dataset.Dataset) { d.Items = nil }},
		{name: "empty class id", want: "item class 1: empty id", change: func(d *dataset.Dataset) { d.Items.Classes[1].ID = "" }},
		{name: "repeated class", want: "item class Helmet repeated", change: func(d *dataset.Dataset) { d.Items.Classes[1].ID = "Helmet" }},
		{name: "repeated tag", want: "tag default repeated", change: func(d *dataset.Dataset) { d.Items.Tags[1] = "default" }},
		{name: "repeated base", want: "base Metadata/Items/Armours/Helmets/HelmetStr1 repeated", change: func(d *dataset.Dataset) {
			d.Items.Bases[1].ID = d.Items.Bases[0].ID
		}},
		{name: "unknown class", want: `base Metadata/Items/Flasks/FlaskLife1: unknown item class "Ring"`, change: func(d *dataset.Dataset) {
			d.Items.Bases[3].Class = "Ring"
		}},
		{name: "unknown base tag", want: `unknown tag "ring"`, change: func(d *dataset.Dataset) { d.Items.Bases[0].Tags[0] = "ring" }},
		{name: "armour range reversed", want: "range 12 to 9", change: func(d *dataset.Dataset) {
			d.Items.Bases[0].Armour.Armour = dataset.ValueRange{Min: 12, Max: 9}
		}},
		{name: "damage range reversed", want: "range 9 to 4", change: func(d *dataset.Dataset) {
			d.Items.Bases[2].Weapon.PhysicalDamage = dataset.ValueRange{Min: 9, Max: 4}
		}},
		{name: "negative mod domain", want: "base Metadata/Items/Flasks/FlaskLife1: mod domain -1 with crafting true", change: func(d *dataset.Dataset) {
			d.Items.Bases[3].ModDomain = -1
		}},
		{name: "no mod domain with crafting", want: "base Metadata/Items/Flasks/FlaskLife1: mod domain 0 with crafting true", change: func(d *dataset.Dataset) {
			d.Items.Bases[3].ModDomain = 0
		}},
		{name: "mod domain without crafting", want: "mod domain 1 with crafting false", change: func(d *dataset.Dataset) { d.Items.Crafting = false }},
		{name: "no bench crafts with crafting", want: "0 bench crafts with crafting true", change: func(d *dataset.Dataset) { d.Items.BenchCrafts = nil }},
		{name: "no essence mods with crafting", want: "0 essence mods with crafting true", change: func(d *dataset.Dataset) { d.Items.EssenceMods = nil }},
		{name: "bench crafts without crafting", want: "1 bench crafts with crafting false", change: func(d *dataset.Dataset) {
			withoutCrafting(d.Items)
			d.Items.BenchCrafts = withItems().Items.BenchCrafts
		}},
		{name: "flask kind 0", want: "unknown flask kind 0", change: func(d *dataset.Dataset) { d.Items.Bases[3].Flask.Kind = 0 }},
		{name: "flask kind past utility", want: "unknown flask kind 5", change: func(d *dataset.Dataset) { d.Items.Bases[3].Flask.Kind = 5 }},
		{name: "implicit not in mods", want: "implicit BlockImplicit not in mods", change: func(d *dataset.Dataset) { d.Mods = d.Mods[:1] }},
		{name: "craft mod not in mods", want: "bench craft CraftedLife1: mod not in mods", change: func(d *dataset.Dataset) { d.Mods = d.Mods[:3] }},
		{name: "repeated craft", want: "bench craft CraftedLife1 repeated", change: func(d *dataset.Dataset) {
			d.Items.BenchCrafts = append(d.Items.BenchCrafts, dataset.ModClasses{Mod: "CraftedLife1", Classes: []string{"Helmet"}})
		}},
		{name: "empty craft mod", want: "bench craft 0: empty id", change: func(d *dataset.Dataset) { d.Items.BenchCrafts[0].Mod = "" }},
		{name: "craft without classes", want: "bench craft CraftedLife1: no item classes", change: func(d *dataset.Dataset) {
			d.Items.BenchCrafts[0].Classes = nil
		}},
		{name: "unknown craft class", want: `bench craft CraftedLife1: unknown item class "Ring"`, change: func(d *dataset.Dataset) {
			d.Items.BenchCrafts[0].Classes[1] = "Ring"
		}},
		{name: "repeated craft class", want: "bench craft CraftedLife1: item class Shield repeated", change: func(d *dataset.Dataset) {
			d.Items.BenchCrafts[0].Classes[1] = "Shield"
		}},
		{name: "essence mod not in mods", want: "essence mod IncreasedLife1: mod not in mods", change: func(d *dataset.Dataset) {
			d.Mods[0].ID = "IncreasedLife2"
			d.Texts[0].ModNames = nil
		}},
		{name: "repeated essence mod", want: "essence mod IncreasedLife1 repeated", change: func(d *dataset.Dataset) {
			d.Items.EssenceMods = append(d.Items.EssenceMods, d.Items.EssenceMods[0])
		}},
		{name: "unknown essence mod class", want: `essence mod IncreasedLife1: unknown item class "Ring"`, change: func(d *dataset.Dataset) {
			d.Items.EssenceMods[0].Classes[0] = "Ring"
		}},
		{name: "repeated mod", want: "mod IncreasedLife1 repeated", change: func(d *dataset.Dataset) { d.Mods[1].ID = "IncreasedLife1" }},
		{name: "empty mod id", want: "mod 2: empty id", change: func(d *dataset.Dataset) { d.Mods[2].ID = "" }},
		{name: "empty type", want: "mod BlockImplicit: empty type", change: func(d *dataset.Dataset) { d.Mods[1].Type = "" }},
		{name: "negative game mode", want: "mod AccuracyImplicit: game mode -1", change: func(d *dataset.Dataset) { d.Mods[2].GameMode = -1 }},
		{name: "mod stat out of range", want: "mod IncreasedLife1: stat 3 out of range", change: func(d *dataset.Dataset) { d.Mods[0].Stats[0].Stat = 3 }},
		{name: "mod range reversed", want: "range 24 to 10", change: func(d *dataset.Dataset) { d.Mods[0].Stats[0].Min, d.Mods[0].Stats[0].Max = 24, 10 }},
		{name: "ruthless range reversed", want: "range 20 to 8", change: func(d *dataset.Dataset) {
			d.Mods[0].Stats[0].RuthlessMin, d.Mods[0].Stats[0].RuthlessMax = 20, 8
		}},
		{name: "unknown mod tag", want: `mod IncreasedLife1: unknown tag "mana"`, change: func(d *dataset.Dataset) { d.Mods[0].Tags[0] = "mana" }},
		{name: "unknown added tag", want: `unknown tag "x"`, change: func(d *dataset.Dataset) { d.Mods[2].AddedTags[0] = "x" }},
		{name: "unknown spawn tag", want: `unknown tag "x"`, change: func(d *dataset.Dataset) { d.Mods[0].SpawnWeights[1].Tag = "x" }},
		{name: "unknown generation tag", want: `unknown tag "x"`, change: func(d *dataset.Dataset) { d.Mods[2].GenerationWeights[0].Tag = "x" }},
		{name: "name for unknown class", want: "text en: name for unknown item class Ring", change: func(d *dataset.Dataset) {
			d.Texts[0].ItemClassNames[0].ID = "Ring"
		}},
		{name: "name for unknown base", want: "text en: name for unknown base x", change: func(d *dataset.Dataset) { d.Texts[0].BaseNames[0].ID = "x" }},
		{name: "name for unknown mod", want: "text en: name for unknown mod x", change: func(d *dataset.Dataset) { d.Texts[0].ModNames[0].ID = "x" }},
		{name: "item names without items", want: "text en: name for unknown item class Helmet", change: func(d *dataset.Dataset) {
			d.Items, d.Mods = nil, nil
		}},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			d := withItems()
			testCase.change(d)
			_, err := dataset.Write(t.TempDir(), d)
			checkError(t, err, dataset.ErrInvalid, testCase.want)
		})
	}
}

// itemsLayout is the content of an items file with one class and one base, for layouts Write
// never produces.
type itemsLayout struct {
	tags []string
	// class and baseTags are the base's indexes into the classes and tags.
	class    uint32
	baseTags []uint32
	// flaskKind, when set, gives the base flask values of that kind.
	flaskKind *uint8
	// craftClasses and essenceClasses, when not nil, add a bench craft or an essence mod of mod
	// "Crafted" with these class indexes.
	craftClasses, essenceClasses []uint32
}

func itemsFile(l itemsLayout) []byte {
	b := flatbuffers.NewBuilder(0)
	baseID := b.CreateString("Base")
	tagRefs := uint32Vec(b, fb.ItemBaseStartTagsVector, l.baseTags)
	var flask flatbuffers.UOffsetT
	if l.flaskKind != nil {
		fb.FlaskValuesStart(b)
		fb.FlaskValuesAddKind(b, *l.flaskKind)
		flask = fb.FlaskValuesEnd(b)
	}
	fb.ItemBaseStart(b)
	fb.ItemBaseAddId(b, baseID)
	fb.ItemBaseAddClass(b, l.class)
	fb.ItemBaseAddTags(b, tagRefs)
	if flask != 0 {
		fb.ItemBaseAddFlask(b, flask)
	}
	base := fb.ItemBaseEnd(b)
	fb.ItemsStartBasesVector(b, 1)
	b.PrependUOffsetT(base)
	bases := b.EndVector(1)
	classID := b.CreateString("Helmet")
	fb.ItemClassStart(b)
	fb.ItemClassAddId(b, classID)
	cls := fb.ItemClassEnd(b)
	fb.ItemsStartClassesVector(b, 1)
	b.PrependUOffsetT(cls)
	classes := b.EndVector(1)
	tagVec := stringVec(b, fb.ItemsStartTagsVector, l.tags)
	crafts := modClassesVec(b, fb.ItemsStartBenchCraftsVector, l.craftClasses)
	essences := modClassesVec(b, fb.ItemsStartEssenceModsVector, l.essenceClasses)
	fb.ItemsStart(b)
	fb.ItemsAddClasses(b, classes)
	fb.ItemsAddTags(b, tagVec)
	fb.ItemsAddBases(b, bases)
	if crafts != 0 {
		fb.ItemsAddBenchCrafts(b, crafts)
	}
	if essences != 0 {
		fb.ItemsAddEssenceMods(b, essences)
	}
	fb.FinishItemsBuffer(b, fb.ItemsEnd(b))
	return b.FinishedBytes()
}

// modClassesVec returns a vector started by start holding one entry of mod "Crafted" with
// classes, or 0 for nil classes.
func modClassesVec(b *flatbuffers.Builder, start func(*flatbuffers.Builder, int) flatbuffers.UOffsetT, classes []uint32) flatbuffers.UOffsetT {
	if classes == nil {
		return 0
	}
	mod := b.CreateString("Crafted")
	refs := uint32Vec(b, fb.ModClassesStartClassesVector, classes)
	fb.ModClassesStart(b)
	fb.ModClassesAddMod(b, mod)
	fb.ModClassesAddClasses(b, refs)
	entry := fb.ModClassesEnd(b)
	start(b, 1)
	b.PrependUOffsetT(entry)
	return b.EndVector(1)
}

// withoutCrafting clears the crafting fields of it, as a dataset written before them has.
func withoutCrafting(it *dataset.Items) {
	it.Crafting, it.BenchCrafts, it.EssenceMods = false, nil, nil
	for i := range it.Bases {
		it.Bases[i].ModDomain = 0
	}
}

// modsFile builds a mods file with the given stat and tag ids and one mod whose stats and tags
// use the given indexes, for layouts Write never produces.
func modsFile(statIDs, tagIDs []string, stats, tags []uint32) []byte {
	b := flatbuffers.NewBuilder(0)
	id, typ := b.CreateString("Mod"), b.CreateString("Type")
	fb.ModStartStatsVector(b, len(stats))
	for _, s := range slices.Backward(stats) {
		fb.CreateModStat(b, s, 1, 1, 1, 1)
	}
	statVec := b.EndVector(len(stats))
	tagVec := uint32Vec(b, fb.ModStartTagsVector, tags)
	fb.ModStart(b)
	fb.ModAddId(b, id)
	fb.ModAddType(b, typ)
	fb.ModAddStats(b, statVec)
	fb.ModAddTags(b, tagVec)
	mod := fb.ModEnd(b)
	fb.ModsStartModsVector(b, 1)
	b.PrependUOffsetT(mod)
	mods := b.EndVector(1)
	statIDVec := stringVec(b, fb.ModsStartStatIdsVector, statIDs)
	tagIDVec := stringVec(b, fb.ModsStartTagIdsVector, tagIDs)
	fb.ModsStart(b)
	fb.ModsAddMods(b, mods)
	fb.ModsAddStatIds(b, statIDVec)
	fb.ModsAddTagIds(b, tagIDVec)
	fb.FinishModsBuffer(b, fb.ModsEnd(b))
	return b.FinishedBytes()
}

func uint32Vec(b *flatbuffers.Builder, start func(*flatbuffers.Builder, int) flatbuffers.UOffsetT, vs []uint32) flatbuffers.UOffsetT {
	start(b, len(vs))
	for _, v := range slices.Backward(vs) {
		b.PrependUint32(v)
	}
	return b.EndVector(len(vs))
}

func stringVec(b *flatbuffers.Builder, start func(*flatbuffers.Builder, int) flatbuffers.UOffsetT, ss []string) flatbuffers.UOffsetT {
	offs := make([]flatbuffers.UOffsetT, len(ss))
	for i, s := range ss {
		offs[i] = b.CreateString(s)
	}
	start(b, len(ss))
	for _, o := range slices.Backward(offs) {
		b.PrependUOffsetT(o)
	}
	return b.EndVector(len(ss))
}

// TestModGameModeAbsent reads a mods file written without game modes, as files from before the
// field existed are: every mod is in all game modes.
func TestModGameModeAbsent(t *testing.T) {
	t.Parallel()
	d := withItems()
	for i := range d.Items.Bases {
		d.Items.Bases[i].Implicits = nil
	}
	withoutCrafting(d.Items)
	d.Mods, d.Texts = nil, nil
	s, err := dataset.Open(withFile(t, written(t, d), "mods.fb", modsFile(nil, nil, nil, nil)))
	if err != nil {
		t.Fatal(err)
	}
	mods, err := s.Mods()
	if err != nil {
		t.Fatalf("Mods: %v", err)
	}
	if diff := gocmp.Diff([]dataset.Mod{{ID: "Mod", Type: "Type"}}, mods, cmpopts.EquateEmpty()); diff != "" {
		t.Errorf("Mods mismatch (-want +got):\n%s", diff)
	}
}

// TestItemsWithoutCrafting reads an items file written without the crafting fields, as files
// from before them are: Crafting is false, every base has domain 0 and there are no bench
// crafts or essence mods.
func TestItemsWithoutCrafting(t *testing.T) {
	t.Parallel()
	d := withItems()
	withoutCrafting(d.Items)
	d.Items.Classes, d.Items.Tags, d.Items.Bases = d.Items.Classes[:1], []string{"default"}, nil
	d.Mods, d.Texts = nil, nil
	s, err := dataset.Open(withFile(t, written(t, d), "items.fb", itemsFile(itemsLayout{tags: []string{"default"}})))
	if err != nil {
		t.Fatal(err)
	}
	it, err := s.Items()
	if err != nil {
		t.Fatalf("Items: %v", err)
	}
	want := &dataset.Items{Classes: d.Items.Classes, Tags: d.Items.Tags, Bases: []dataset.ItemBase{{ID: "Base", Class: "Helmet"}}}
	if diff := gocmp.Diff(want, it, cmpopts.EquateEmpty()); diff != "" {
		t.Errorf("Items mismatch (-want +got):\n%s", diff)
	}
}

func TestItemsReadErrors(t *testing.T) {
	t.Parallel()
	good := written(t, withItems())
	// other has fewer tags, no implicits and a mod whose tags are not among good's.
	other := withItems()
	other.Items.Tags = other.Items.Tags[:3]
	for i := range other.Items.Bases {
		other.Items.Bases[i].Tags, other.Items.Bases[i].Implicits = nil, nil
	}
	withoutCrafting(other.Items)
	other.Mods = []dataset.Mod{{ID: "Other", Domain: 1, Generation: 1, Type: "Other", Tags: []string{"default"}}}
	other.Texts = nil
	// noCraftMod has the implicits but not the crafted mod.
	noCraftMod := withItems()
	withoutCrafting(noCraftMod.Items)
	noCraftMod.Mods = noCraftMod.Mods[:3]
	noCraftModFiles := written(t, noCraftMod)
	otherFiles := written(t, other)
	otherTags := withItems()
	otherTags.Items.Tags = append(otherTags.Items.Tags, "zzz")
	otherTags.Mods[0].Tags = []string{"zzz"}
	otherTagsFiles := written(t, otherTags)

	// unlisted returns good with name left out of the manifest.
	unlisted := func(name string) fstest.MapFS {
		var m dataset.Manifest
		if err := json.Unmarshal(good[dataset.ManifestName].Data, &m); err != nil {
			t.Fatal(err)
		}
		m.Files = slices.DeleteFunc(m.Files, func(f dataset.File) bool { return f.Name == name })
		m.ID = dataset.ContentID(m)
		b, err := json.Marshal(m)
		if err != nil {
			t.Fatal(err)
		}
		out := maps.Clone(good)
		out[dataset.ManifestName] = &fstest.MapFile{Data: b}
		return out
	}
	unknownKind := uint8(9)

	testCases := []struct {
		name, want string
		fsys       fstest.MapFS
	}{
		{name: "only items.fb listed", want: "items group needs both items.fb and mods.fb", fsys: unlisted("mods.fb")},
		{name: "only mods.fb listed", want: "items group needs both items.fb and mods.fb", fsys: unlisted("items.fb")},
		{name: "items not an items file", want: "items.fb: wrong file identifier", fsys: withFile(t, good, "items.fb", good["mods.fb"].Data)},
		{name: "mods not a mods file", want: "mods.fb: wrong file identifier", fsys: withFile(t, good, "mods.fb", good["items.fb"].Data)},
		{name: "truncated mods", want: "mods.fb: malformed", fsys: withFile(t, good, "mods.fb", good["mods.fb"].Data[:40])},
		{
			name: "class out of range", want: "items.fb: base Base: class 1 out of range (1 classes)",
			fsys: withFile(t, good, "items.fb", itemsFile(itemsLayout{tags: []string{"default"}, class: 1})),
		},
		{
			name: "base tag out of range", want: "items.fb: base Base: tag 1 out of range (1 tags)",
			fsys: withFile(t, good, "items.fb", itemsFile(itemsLayout{tags: []string{"default"}, baseTags: []uint32{1}})),
		},
		{
			name: "repeated tag", want: "tag default repeated",
			fsys: withFile(t, good, "items.fb", itemsFile(itemsLayout{tags: []string{"default", "default"}})),
		},
		{
			name: "unknown flask kind", want: "base Base: unknown flask kind 9",
			fsys: withFile(t, good, "items.fb", itemsFile(itemsLayout{tags: []string{"default"}, flaskKind: &unknownKind})),
		},
		{
			name: "mod tag ids unsorted", want: `mods.fb: tag ids not sorted and unique at "default"`,
			fsys: withFile(t, good, "mods.fb", modsFile(nil, []string{"life", "default"}, nil, nil)),
		},
		{
			name: "mod tag ids repeated", want: `mods.fb: tag ids not sorted and unique at "default"`,
			fsys: withFile(t, good, "mods.fb", modsFile(nil, []string{"default", "default"}, nil, nil)),
		},
		{
			name: "mod stat index out of range", want: "mods.fb: mod Mod: stat 1 out of range (1 ids)",
			fsys: withFile(t, good, "mods.fb", modsFile([]string{"level"}, nil, []uint32{1}, nil)),
		},
		{
			name: "mod tag index out of range", want: "mods.fb: mod Mod: tag 1 out of range (1 tags)",
			fsys: withFile(t, good, "mods.fb", modsFile(nil, []string{"default"}, nil, []uint32{1})),
		},
		{name: "implicit not in mods", want: "implicit BlockImplicit not in mods", fsys: withFile(t, good, "mods.fb", otherFiles["mods.fb"].Data)},
		{name: "craft mod not in mods", want: "bench craft CraftedLife1: mod not in mods", fsys: withFile(t, good, "mods.fb", noCraftModFiles["mods.fb"].Data)},
		{
			name: "craft class out of range", want: "items.fb: bench craft Crafted: class 1 out of range (1 classes)",
			fsys: withFile(t, good, "items.fb", itemsFile(itemsLayout{tags: []string{"default"}, craftClasses: []uint32{1}})),
		},
		{
			name: "essence mod class out of range", want: "items.fb: essence mod Crafted: class 1 out of range (1 classes)",
			fsys: withFile(t, good, "items.fb", itemsFile(itemsLayout{tags: []string{"default"}, essenceClasses: []uint32{1}})),
		},
		{
			name: "crafting fields without the flag", want: "1 bench crafts with crafting false",
			fsys: withFile(t, good, "items.fb", itemsFile(itemsLayout{tags: []string{"default"}, craftClasses: []uint32{0}})),
		},
		{
			name: "craft without classes", want: "bench craft Crafted: no item classes",
			fsys: withFile(t, good, "items.fb", itemsFile(itemsLayout{tags: []string{"default"}, craftClasses: []uint32{}})),
		},
		{name: "mod tag not in items", want: `mods.fb: unknown tag "zzz"`, fsys: withFile(t, good, "mods.fb", otherTagsFiles["mods.fb"].Data)},
		{
			name: "mod stat unknown", want: `mods.fb: unknown stat "base_maximum_all_resistances_%"`,
			fsys: withFile(t, good, "stats.fb", written(t, &dataset.Dataset{Stats: sample().Stats[:1]})["stats.fb"].Data),
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			s, err := dataset.Open(testCase.fsys)
			if err == nil {
				_, err = s.Mods()
			}
			checkError(t, err, dataset.ErrInvalid, testCase.want)
		})
	}
}

func TestFlaskKindString(t *testing.T) {
	t.Parallel()
	for kind, want := range map[dataset.FlaskKind]string{
		dataset.LifeFlask: "life", dataset.ManaFlask: "mana", dataset.HybridFlask: "hybrid",
		dataset.UtilityFlask: "utility", dataset.FlaskKind(9): "unknown",
	} {
		if got := kind.String(); got != want {
			t.Errorf("FlaskKind(%d).String(): got %q, want %q", kind, got, want)
		}
	}
}

// itemsGroup returns a dataset whose items group has the size of a full 3.29.3.3 one: about
// 1100 bases and 29,000 mods with two stats, two tags and three spawn weights each.
func itemsGroup() *dataset.Dataset {
	d := &dataset.Dataset{Manifest: dataset.Manifest{Game: "poe1", Version: "3.29.3.3"}}
	for i := range 2000 {
		d.Stats = append(d.Stats, dataset.Stat{ID: fmt.Sprintf("stat_%d", i)})
	}
	it := &dataset.Items{}
	for i := range 32 {
		it.Classes = append(it.Classes, dataset.ItemClass{ID: fmt.Sprintf("Class%d", i)})
	}
	for i := range 1400 {
		it.Tags = append(it.Tags, fmt.Sprintf("tag_%d", i))
	}
	for i := range 29000 {
		d.Mods = append(d.Mods, dataset.Mod{
			ID: fmt.Sprintf("Mod%d", i), Domain: 1, Generation: 1, Level: int32(i % 86),
			Type: fmt.Sprintf("Type%d", i/2), Families: []string{fmt.Sprintf("Family%d", i/4)},
			Stats: []dataset.ModStat{
				{Stat: uint32(i % 2000), Min: 10, Max: 20, RuthlessMin: 10, RuthlessMax: 20},
				{Stat: uint32(i*7) % 2000, Min: 1, Max: 2, RuthlessMin: 1, RuthlessMax: 2},
			},
			Tags: []string{it.Tags[i%50], it.Tags[50+i%50]},
			SpawnWeights: []dataset.TagWeight{
				{Tag: it.Tags[i%1400], Weight: 0}, {Tag: it.Tags[(i+1)%1400], Weight: 500}, {Tag: "tag_0", Weight: 1000},
			},
		})
	}
	for i := range 1100 {
		it.Bases = append(it.Bases, dataset.ItemBase{
			ID: fmt.Sprintf("Metadata/Items/Base%d", i), Class: it.Classes[i%32].ID,
			Implicits: []string{d.Mods[i].ID}, Tags: []string{it.Tags[i%1400], "tag_0"},
			Armour: &dataset.ArmourValues{Armour: dataset.ValueRange{Min: 100, Max: 120}},
		})
	}
	d.Items = it
	return d
}

// BenchmarkLoadItems opens a dataset with an items group the size of a full 3.29.3.3 one and
// reads its bases, or its bases and mods.
func BenchmarkLoadItems(b *testing.B) {
	dir := b.TempDir()
	if _, err := dataset.Write(dir, itemsGroup()); err != nil {
		b.Fatal(err)
	}
	fsys := fstest.MapFS{}
	ents, err := fs.ReadDir(os.DirFS(dir), ".")
	if err != nil {
		b.Fatal(err)
	}
	for _, e := range ents {
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			b.Fatal(err)
		}
		fsys[e.Name()] = &fstest.MapFile{Data: data}
	}
	for _, bench := range []struct {
		name string
		get  func(*dataset.Store) (any, error)
	}{
		{"items", func(s *dataset.Store) (any, error) { return s.Items() }},
		{"mods", func(s *dataset.Store) (any, error) { return s.Mods() }},
	} {
		b.Run(bench.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				s, err := dataset.Open(fsys)
				if err != nil {
					b.Fatal(err)
				}
				if _, err := bench.get(s); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
