package dataset_test

import (
	"bytes"
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"math"
	"slices"
	"testing"
	"testing/fstest"

	flatbuffers "github.com/google/flatbuffers/go"
	gocmp "github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/dataset/internal/fb"
)

// Stat handles withMonsters adds after withSkills' seven.
const (
	zombieLimit uint32 = iota + 7
	activeZombies
	damageReduction
)

const (
	zombie        = "Metadata/Monsters/RaisedZombies/RaisedZombieStandard"
	slamTotem     = "Metadata/Monsters/Totems/SlamTotem"
	strengthTotem = "Metadata/Monsters/Totems/StrengthTotem"
	statue        = "Metadata/Monsters/Statue/GoldStatueMaleSword"
	statueSpectre = "Metadata/Monsters/Statue/GoldStatueMaleSwordSpectre"
)

// withMonsters returns withSkills with a monsters group: a zombie minion, two totems, and a
// statue with its spectre version.
func withMonsters() *dataset.Dataset {
	d := withSkills()
	d.Stats = append(d.Stats,
		dataset.Stat{ID: "number_of_zombies_allowed"}, dataset.Stat{ID: "number_of_active_zombies"},
		dataset.Stat{ID: "physical_damage_reduction_rating_+%"})
	d.Monsters = sampleMonsters()
	d.Texts[0].MonsterNames = []dataset.Name{{ID: zombie, Name: "Raised Zombie"}, {ID: slamTotem, Name: "Totem"}}
	return d
}

func sampleMonsters() *dataset.Monsters {
	return &dataset.Monsters{
		Types: []dataset.MonsterType{
			{ID: "RaisedZombie", Armour: 50, Evasion: 15, Accuracy: 100, DamageSpread: 20, Resistances: "Fire1", PlayerMinion: true},
			{ID: "AncestorTotem", EnergyShield: 40, BaseDamageIgnoresAttackSpeed: true, AltLife1: true, AltLife2: true},
		},
		Resistances: []dataset.MonsterResistance{
			{ID: "None"},
			{ID: "Fire1", Normal: dataset.Resistances{Chaos: -1}, Cruel: dataset.Resistances{Fire: 20, Cold: 1}, Merciless: dataset.Resistances{Fire: 40, Lightning: 2}},
		},
		Varieties: []dataset.MonsterVariety{
			{
				ID: zombie, Type: "RaisedZombie", LifeMultiplier: 375, DamageMultiplier: 165, AilmentThreshold: 375, AttackTime: 1170,
				Tags: []string{"undead", "zombie"}, GrantedEffects: []string{"MonsterSlam"},
				Mods: []string{"MonsterImplicitDamageReduction1"}, Mods2: []string{"StanceRun"},
			},
			{
				ID: slamTotem, Type: "AncestorTotem", LifeMultiplier: 120, DamageMultiplier: 100, MainHand: "Two Hand Mace", OffHand: "Shield",
				Tags: []string{"construct"}, SpecialMods: []string{"CannotBeAugmented"},
				Part1Mods: []string{"MonsterRare1"}, Part2Mods: []string{"MonsterRare1"}, EndgameMods: []string{"MonsterRare1", "CannotBeAugmented"},
			},
			{ID: strengthTotem, Type: "AncestorTotem", LifeMultiplier: 100},
			{ID: statue, Type: "RaisedZombie"},
			{ID: statueSpectre, Type: "RaisedZombie", GrantedEffects: []string{"MonsterSlam", "Fireball"}},
		},
		Levels: []dataset.MonsterLevel{
			{Level: 1, Difficulty: dataset.NormalDifficulty, Life: 22, AllyLife: 15, AltLife1: 10, AltLife2: 13, Damage: 4.99, AllyDamage: 5.62,
				AltDamage1: 4.98, AltDamage2: 4.97, Armour: 16, Evasion: 67, EvasiveEvasion: 76, Accuracy: 14, AilmentThreshold: 22},
			{Level: 68, Difficulty: dataset.MercilessDifficulty, Life: 6127, Damage: 373.55, PhysicalConversion: 240},
		},
		BaseStats: []dataset.StatValue{{Stat: 2, Value: 75}, {Stat: zombieLimit, Value: 1}},
		Mods: []dataset.Mod{
			{
				ID: "MonsterImplicitDamageReduction1", Domain: 3, Generation: 3, Level: 1, Type: "MonsterDamageReduction",
				Stats: []dataset.ModStat{{Stat: damageReduction, Min: 30, Max: 30, RuthlessMin: 30, RuthlessMax: 30}}, Tags: []string{"monster"},
			},
			{ID: "StanceRun", Domain: 3, Generation: 3, Type: "Stance", Families: []string{"Stance"}},
			{ID: "CannotBeAugmented", Domain: 3, Generation: 3, Type: "CannotBeAugmented"},
			{
				ID: "MonsterRare1", Domain: 3, Generation: 3, Level: 1, Type: "MonsterRare",
				Stats:        []dataset.ModStat{{Stat: damageReduction, Min: 10, Max: 20, RuthlessMin: 10, RuthlessMax: 20}},
				SpawnWeights: []dataset.TagWeight{{Tag: "default", Weight: 100}}, AddedTags: []string{"rare"},
			},
		},
		DisplayMinions: []dataset.DisplayMinion{{ID: 1, Variety: zombie}, {ID: 2, Variety: zombie}},
		Totems: []dataset.Totem{
			{Key: 1, Skins: []dataset.TotemSkin{{Skin: 0, Variety: strengthTotem}}},
			{Key: 15, Skins: []dataset.TotemSkin{{Skin: 0, Variety: slamTotem}, {Skin: 3, Variety: strengthTotem}}},
		},
		SpectreOverrides: []dataset.SpectreOverride{{Monster: statue, Spectre: statueSpectre}},
		MinionTypes: []dataset.MinionType{
			{ID: "ZombieMinion", LimitStat: new(zombieLimit), ActiveCountStat: new(activeZombies)},
			{ID: "GolemMinion", LimitStat: new(zombieLimit)},
			{ID: "GenericUnlimitedMinion"},
		},
		EditionTotems: []dataset.EditionTotem{{GrantedEffect: "SupportSpellTotem", Totem: 1, Source: "test source"}},
	}
}

func renumberMonsters(m *dataset.Monsters, h func(uint32) uint32) {
	renumber(m.BaseStats, h)
	slices.SortFunc(m.BaseStats, func(a, b dataset.StatValue) int { return cmp.Compare(a.Stat, b.Stat) })
	for _, mod := range m.Mods {
		for i := range mod.Stats {
			mod.Stats[i].Stat = h(mod.Stats[i].Stat)
		}
	}
	for _, t := range m.MinionTypes {
		for _, p := range []*uint32{t.LimitStat, t.ActiveCountStat} {
			if p != nil {
				*p = h(*p)
			}
		}
	}
}

func TestMonstersRoundTrip(t *testing.T) {
	t.Parallel()
	want := withMonsters()
	got, err := readAll(written(t, want))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	want.Manifest = got.Manifest
	if diff := gocmp.Diff(want, got, cmpopts.EquateEmpty()); diff != "" {
		t.Errorf("round trip mismatch (-want +got):\n%s", diff)
	}
}

func TestMonstersFileIndependentOfStatOrder(t *testing.T) {
	t.Parallel()
	renumbered := reversedStats(withMonsters())
	a, b := written(t, withMonsters()), written(t, renumbered)
	if !bytes.Equal(a["monsters.fb"].Data, b["monsters.fb"].Data) {
		t.Errorf("monsters.fb: bytes differ after renumbering stats, want equal")
	}
	got, err := readAll(b)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if diff := gocmp.Diff(renumbered.Monsters, got.Monsters, cmpopts.EquateEmpty()); diff != "" {
		t.Errorf("round trip mismatch (-want +got):\n%s", diff)
	}
}

// A dataset without a monsters group writes its text file as before the group existed.
func TestTextWithoutMonsterNamesUnchanged(t *testing.T) {
	t.Parallel()
	plain, named := sample(), sample()
	named.Texts[0].MonsterNames = []dataset.Name{}
	a, b := written(t, plain), written(t, named)
	if !bytes.Equal(a["text-en.fb"].Data, b["text-en.fb"].Data) {
		t.Errorf("text-en.fb: bytes differ with empty monster names, want equal")
	}
}

func TestMonstersWriteErrors(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name, want string
		change     func(m *dataset.Monsters)
	}{
		{name: "type id empty", want: "monster type 1: empty id", change: func(m *dataset.Monsters) { m.Types[1].ID = "" }},
		{name: "type repeated", want: "monster type RaisedZombie repeated", change: func(m *dataset.Monsters) { m.Types[1].ID = "RaisedZombie" }},
		{name: "resistance repeated", want: "monster resistance None repeated", change: func(m *dataset.Monsters) { m.Resistances[1].ID = "None" }},
		{name: "variety repeated", want: "monster variety " + zombie + " repeated", change: func(m *dataset.Monsters) { m.Varieties[1].ID = zombie }},
		{name: "mod repeated", want: "mod StanceRun repeated", change: func(m *dataset.Monsters) { m.Mods[2].ID = "StanceRun" }},
		{name: "minion type repeated", want: "minion type ZombieMinion repeated", change: func(m *dataset.Monsters) { m.MinionTypes[2].ID = "ZombieMinion" }},
		{name: "spectre override repeated", want: "spectre override " + statue + " repeated", change: func(m *dataset.Monsters) {
			m.SpectreOverrides = append(m.SpectreOverrides, m.SpectreOverrides[0])
		}},
		{name: "edition totem repeated", want: "edition totem SupportSpellTotem repeated", change: func(m *dataset.Monsters) {
			m.EditionTotems = append(m.EditionTotems, m.EditionTotems[0])
		}},
		{name: "display minion repeated", want: "display minion 1 repeated", change: func(m *dataset.Monsters) { m.DisplayMinions[1].ID = 1 }},
		{name: "totem repeated", want: "totem 1 repeated", change: func(m *dataset.Monsters) { m.Totems[1].Key = 1 }},
		{name: "unknown resistances", want: `monster type RaisedZombie: resistances "x" not in monsters`, change: func(m *dataset.Monsters) {
			m.Types[0].Resistances = "x"
		}},
		{name: "unknown type", want: "monster variety " + zombie + `: type "x" not in monsters`, change: func(m *dataset.Monsters) { m.Varieties[0].Type = "x" }},
		{name: "empty variety tag", want: "monster variety " + zombie + ": empty tag", change: func(m *dataset.Monsters) { m.Varieties[0].Tags[1] = "" }},
		{name: "unknown granted effect", want: "monster variety " + zombie + `: granted effect "x" not in skills`, change: func(m *dataset.Monsters) {
			m.Varieties[0].GrantedEffects = []string{"x"}
		}},
		{name: "unknown mod", want: "monster variety " + slamTotem + `: mod "x" not in monsters`, change: func(m *dataset.Monsters) {
			m.Varieties[1].EndgameMods[1] = "x"
		}},
		{name: "levels unsorted", want: "monster levels not sorted or repeated at 1", change: func(m *dataset.Monsters) { m.Levels[1].Level = 1 }},
		{name: "difficulty", want: "monster level 68: difficulty 4 out of range", change: func(m *dataset.Monsters) { m.Levels[1].Difficulty = 4 }},
		{name: "difficulty zero", want: "monster level 1: difficulty 0 out of range", change: func(m *dataset.Monsters) { m.Levels[0].Difficulty = 0 }},
		{name: "damage NaN", want: "monster level 1: damage NaN not finite", change: func(m *dataset.Monsters) {
			m.Levels[0].AltDamage2 = float32(math.NaN())
		}},
		{name: "base stats unsorted", want: "monster base stats: stats not sorted or repeated", change: func(m *dataset.Monsters) {
			m.BaseStats[1].Stat = 2
		}},
		{name: "base stat out of range", want: "monster base stats: stat 99 out of range", change: func(m *dataset.Monsters) { m.BaseStats[1].Stat = 99 }},
		{name: "mod without type", want: "mod StanceRun: empty type", change: func(m *dataset.Monsters) { m.Mods[1].Type = "" }},
		{name: "mod stat out of range", want: "mod MonsterRare1: stat 99 out of range", change: func(m *dataset.Monsters) { m.Mods[3].Stats[0].Stat = 99 }},
		{name: "mod range", want: "mod MonsterRare1: range 30 to 20", change: func(m *dataset.Monsters) { m.Mods[3].Stats[0].Min = 30 }},
		{name: "mod empty tag", want: `mod MonsterRare1: unknown tag ""`, change: func(m *dataset.Monsters) { m.Mods[3].SpawnWeights[0].Tag = "" }},
		{name: "display minion variety", want: `display minion 2: variety "x" not in monsters`, change: func(m *dataset.Monsters) {
			m.DisplayMinions[1].Variety = "x"
		}},
		{name: "totem skins unsorted", want: "totem 15: skins not sorted", change: func(m *dataset.Monsters) { m.Totems[1].Skins[1].Skin = -1 }},
		{name: "totem skin repeated", want: "totem 15: skin 0 repeated", change: func(m *dataset.Monsters) { m.Totems[1].Skins[1].Skin = 0 }},
		{name: "totem variety", want: `totem 1: variety "x" not in monsters`, change: func(m *dataset.Monsters) { m.Totems[0].Skins[0].Variety = "x" }},
		{name: "spectre override monster", want: "spectre override x: monster \"x\" not in monsters", change: func(m *dataset.Monsters) {
			m.SpectreOverrides[0].Monster = "x"
		}},
		{name: "spectre override spectre", want: "spectre override " + statue + `: spectre "x" not in monsters`, change: func(m *dataset.Monsters) {
			m.SpectreOverrides[0].Spectre = "x"
		}},
		{name: "minion type stat", want: "minion type ZombieMinion: stats: stat 99 out of range", change: func(m *dataset.Monsters) {
			m.MinionTypes[0].ActiveCountStat = new(uint32(99))
		}},
		{name: "edition totem effect", want: `edition totem x: granted effect "x" not in skills`, change: func(m *dataset.Monsters) {
			m.EditionTotems[0].GrantedEffect = "x"
		}},
		{name: "edition totem key", want: "edition totem SupportSpellTotem: totem 2 not in monsters", change: func(m *dataset.Monsters) {
			m.EditionTotems[0].Totem = 2
		}},
		{name: "edition totem source", want: "edition totem SupportSpellTotem: no source", change: func(m *dataset.Monsters) {
			m.EditionTotems[0].Source = ""
		}},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			d := withMonsters()
			testCase.change(d.Monsters)
			_, err := dataset.Write(t.TempDir(), d)
			checkError(t, err, dataset.ErrInvalid, testCase.want)
		})
	}
}

func TestMonstersDatasetWriteErrors(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name, want string
		change     func(d *dataset.Dataset)
	}{
		{name: "monsters without skills", want: "monsters without skills", change: func(d *dataset.Dataset) {
			d.Skills = nil
			d.Texts[0].GemNames, d.Texts[0].SkillNames = nil, nil
		}},
		{name: "unknown monster name", want: "text en: name for unknown monster x", change: func(d *dataset.Dataset) { d.Texts[0].MonsterNames[1].ID = "x" }},
		{name: "names without monsters", want: "text en: name for unknown monster " + zombie, change: func(d *dataset.Dataset) { d.Monsters = nil }},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			d := withMonsters()
			testCase.change(d)
			_, err := dataset.Write(t.TempDir(), d)
			checkError(t, err, dataset.ErrInvalid, testCase.want)
		})
	}
}

// monstersLayout is a monsters file Write never produces: one variety with tags, one minion
// type and base stats, all as raw file indexes.
type monstersLayout struct {
	statIDs, tagIDs []string
	tags            []uint32
	varietyType     string
	limitStat       uint32
	baseStat        uint32
}

func MonstersFile(l monstersLayout) []byte {
	b := flatbuffers.NewBuilder(0)
	vector := func(start func(*flatbuffers.Builder, int) flatbuffers.UOffsetT, offs ...flatbuffers.UOffsetT) flatbuffers.UOffsetT {
		start(b, len(offs))
		for _, o := range slices.Backward(offs) {
			b.PrependUOffsetT(o)
		}
		return b.EndVector(len(offs))
	}
	id, typ := b.CreateString("v"), b.CreateString(l.varietyType)
	tags := uint32Vec(b, fb.MonsterVarietyStartTagsVector, l.tags)
	fb.MonsterVarietyStart(b)
	fb.MonsterVarietyAddId(b, id)
	fb.MonsterVarietyAddType(b, typ)
	fb.MonsterVarietyAddTags(b, tags)
	varieties := vector(fb.MonstersStartVarietiesVector, fb.MonsterVarietyEnd(b))
	minionID := b.CreateString("m")
	fb.MinionTypeStart(b)
	fb.MinionTypeAddId(b, minionID)
	fb.MinionTypeAddLimitStat(b, l.limitStat)
	minions := vector(fb.MonstersStartMinionTypesVector, fb.MinionTypeEnd(b))
	fb.MonstersStartBaseStatsVector(b, 1)
	fb.CreateStatValue(b, l.baseStat, 1)
	base := b.EndVector(1)
	statIDs := stringVec(b, fb.MonstersStartStatIdsVector, l.statIDs)
	tagIDs := stringVec(b, fb.MonstersStartTagIdsVector, l.tagIDs)
	typeID := b.CreateString("t")
	fb.MonsterTypeStart(b)
	fb.MonsterTypeAddId(b, typeID)
	types := vector(fb.MonstersStartTypesVector, fb.MonsterTypeEnd(b))
	fb.MonstersStart(b)
	fb.MonstersAddTypes(b, types)
	fb.MonstersAddVarieties(b, varieties)
	fb.MonstersAddMinionTypes(b, minions)
	fb.MonstersAddBaseStats(b, base)
	fb.MonstersAddStatIds(b, statIDs)
	fb.MonstersAddTagIds(b, tagIDs)
	fb.FinishMonstersBuffer(b, fb.MonstersEnd(b))
	return b.FinishedBytes()
}

func TestMonstersReadErrors(t *testing.T) {
	t.Parallel()
	good := written(t, withMonsters())
	replaced := func(name string, data []byte) fstest.MapFS { return rewritten(t, good, name, data) }
	monsters := good["monsters.fb"].Data
	fewStats := written(t, withSkills())["stats.fb"].Data
	noSlam := withSkills()
	noSlam.Skills.GrantedEffects = slices.DeleteFunc(noSlam.Skills.GrantedEffects, func(g dataset.GrantedEffect) bool { return g.ID == "MonsterSlam" })
	layout := monstersLayout{statIDs: []string{"level"}, tagIDs: []string{"a", "b"}, tags: []uint32{1}, varietyType: "t"}
	with := func(change func(l *monstersLayout)) []byte {
		l := layout
		change(&l)
		return MonstersFile(l)
	}
	testCases := []struct {
		name, want string
		fsys       fstest.MapFS
	}{
		{name: "truncated", want: "monsters.fb: malformed", fsys: replaced("monsters.fb", monsters[:len(monsters)/2])},
		{name: "wrong identifier", want: "monsters.fb: wrong file identifier", fsys: replaced("monsters.fb", good["skills.fb"].Data)},
		{name: "stat not in stats group", want: `monsters.fb: unknown stat "number_of_active_zombies"`, fsys: replaced("stats.fb", fewStats)},
		{name: "granted effect not in skills", want: `monsters.fb: monster variety ` + zombie + `: granted effect "MonsterSlam" not in skills`,
			fsys: replaced("skills.fb", written(t, noSlam)["skills.fb"].Data)},
		{name: "tag ids unsorted", want: `monsters.fb: tag ids not sorted and unique at "a"`, fsys: replaced("monsters.fb", with(func(l *monstersLayout) {
			l.tagIDs = []string{"b", "a"}
		}))},
		{name: "tag out of range", want: "monsters.fb: tag 2 out of range (2)", fsys: replaced("monsters.fb", with(func(l *monstersLayout) {
			l.tags = []uint32{2}
		}))},
		{name: "minion stat out of range", want: "monsters.fb: stat 1 out of range (1 ids)", fsys: replaced("monsters.fb", with(func(l *monstersLayout) {
			l.limitStat = 1
		}))},
		{
			name: "base stat out of range", want: "monsters.fb: monster base stats: stat 1 out of range (1 ids)",
			fsys: replaced("monsters.fb", with(func(l *monstersLayout) { l.baseStat = 1 })),
		},
		{name: "fails validation", want: `monsters.fb: monster variety v: type "x" not in monsters`, fsys: replaced("monsters.fb", with(func(l *monstersLayout) {
			l.varietyType = "x"
		}))},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			s, err := dataset.Open(testCase.fsys)
			if err != nil {
				t.Fatalf("Open: %v", err)
			}
			_, err = s.Monsters()
			checkError(t, err, dataset.ErrInvalid, testCase.want)
		})
	}
}

// A hand-built monsters file that breaks none of the read checks reads without error, so the
// cases of TestMonstersReadErrors fail for their own reason.
func TestMonstersLayoutValid(t *testing.T) {
	t.Parallel()
	fsys := rewritten(t, written(t, withMonsters()), "monsters.fb",
		MonstersFile(monstersLayout{statIDs: []string{"level"}, tagIDs: []string{"a", "b"}, tags: []uint32{1}, varietyType: "t"}))
	s, err := dataset.Open(fsys)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if _, err := s.Monsters(); err != nil {
		t.Errorf("Monsters: got %v, want no error", err)
	}
}

func TestOpenMonstersWithoutSkills(t *testing.T) {
	t.Parallel()
	fsys := maps.Clone(written(t, withMonsters()))
	m, err := dataset.ReadManifest(fsys)
	if err != nil {
		t.Fatal(err)
	}
	m.Files = slices.DeleteFunc(m.Files, func(f dataset.File) bool { return f.Name == "skills.fb" })
	m.ID = dataset.ContentID(m)
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	fsys[dataset.ManifestName] = &fstest.MapFile{Data: b}
	_, err = dataset.Open(fsys)
	checkError(t, err, dataset.ErrInvalid, "manifest.json: monsters.fb needs skills.fb")
}

func TestStoreMonstersMissing(t *testing.T) {
	t.Parallel()
	s, err := dataset.Open(written(t, withSkills()))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	_, err = s.Monsters()
	checkError(t, err, dataset.ErrNotInDataset, "dataset 3.29.3.3 has no monsters group")
	missing, ok := errors.AsType[*dataset.MissingGroupError](err)
	if !ok || *missing != (dataset.MissingGroupError{Version: "3.29.3.3", Group: "monsters"}) {
		t.Errorf("got %#v, want a MissingGroupError for monsters of 3.29.3.3", err)
	}
}

func TestStoreReadsMonstersOnUse(t *testing.T) {
	t.Parallel()
	want := withMonsters()
	fsys := maps.Clone(written(t, want))
	for _, n := range []string{"characters.fb", "constants.fb", "quests.fb", "tree-standard.fb", "tree-ruthless.fb", "text-en.fb", "text-zh-hant.fb"} {
		fsys[n] = &fstest.MapFile{Data: []byte("x")}
	}
	s, err := dataset.Open(fsys)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	got, err := s.Monsters()
	if err != nil {
		t.Fatalf("Monsters: %v", err)
	}
	if diff := gocmp.Diff(want.Monsters, got, cmpopts.EquateEmpty()); diff != "" {
		t.Errorf("Monsters (-want +got):\n%s", diff)
	}
}

// syntheticMonsters returns syntheticSkills with a monsters group of about the row counts of
// the 3.29.3.3 game data: 10,691 varieties, 3,600 types, 2,298 mods, 100 levels and 285
// totem variations.
func syntheticMonsters() *dataset.Dataset {
	d := syntheticSkills()
	m := &dataset.Monsters{
		Resistances: []dataset.MonsterResistance{{ID: "None"}, {ID: "Fire1", Merciless: dataset.Resistances{Fire: 40}}},
		BaseStats:   []dataset.StatValue{{Stat: 1, Value: 75}, {Stat: 2, Value: 130}},
		MinionTypes: []dataset.MinionType{{ID: "ZombieMinion", LimitStat: new(uint32(3)), ActiveCountStat: new(uint32(4))}},
	}
	for i := range 3600 {
		m.Types = append(m.Types, dataset.MonsterType{ID: syntheticID("type", i), Armour: 50, Evasion: 15, Accuracy: 100, DamageSpread: 20, Resistances: "Fire1"})
	}
	for i := range 2298 {
		m.Mods = append(m.Mods, dataset.Mod{
			ID: syntheticID("MonsterMod", i), Domain: 3, Generation: 3, Level: 1, Type: syntheticID("ModType", i%1500),
			Stats: []dataset.ModStat{{Stat: uint32(i % 2000), Min: 30, Max: 30, RuthlessMin: 30, RuthlessMax: 30}},
		})
	}
	tags := []string{"undead", "zombie", "melee", "physical_affinity", "medium_height", "red_blood", "humanoid", "fast_movement", "construct", "is_unarmed"}
	for i := range 10691 {
		m.Varieties = append(m.Varieties, dataset.MonsterVariety{
			ID: fmt.Sprintf("Metadata/Monsters/Family%03d/Monster%05d", i%600, i), Type: syntheticID("type", i%3600),
			LifeMultiplier: 175, DamageMultiplier: 130, AilmentThreshold: 175, AttackTime: 1170, MainHand: "One Hand Sword",
			Tags: tags[:3+i%7], GrantedEffects: []string{syntheticID("effect", i%12829), syntheticID("effect", (i+1)%12829)},
			Mods: []string{syntheticID("MonsterMod", i%1000)}, SpecialMods: []string{syntheticID("MonsterMod", 2000+i%100)},
		})
	}
	for i := range 100 {
		m.Levels = append(m.Levels, dataset.MonsterLevel{Level: int32(i + 1), Difficulty: 1 + int32(i/34), Life: int32(20 * i), Damage: 4.99 * float32(i)})
	}
	for k := range 20 {
		totem := dataset.Totem{Key: int32(k + 1)}
		for s := range 15 {
			totem.Skins = append(totem.Skins, dataset.TotemSkin{Skin: int32(s), Variety: m.Varieties[k*15+s].ID})
		}
		m.Totems = append(m.Totems, totem)
	}
	for i := range 31 {
		m.DisplayMinions = append(m.DisplayMinions, dataset.DisplayMinion{ID: int32(i), Variety: m.Varieties[i].ID})
	}
	for i := range 972 {
		m.SpectreOverrides = append(m.SpectreOverrides, dataset.SpectreOverride{Monster: m.Varieties[2*i].ID, Spectre: m.Varieties[2*i+1].ID})
	}
	d.Monsters = m
	return d
}

// BenchmarkMonsters opens a dataset and loads its monsters group, which has about the row
// counts of the 3.29.3.3 game data, with the skills group it checks against (target: < 70 ms
// native, of which BenchmarkSkills is the skills part).
func BenchmarkMonsters(b *testing.B) {
	fsys := written(b, syntheticMonsters())
	b.ReportAllocs()
	for b.Loop() {
		s, err := dataset.Open(fsys)
		if err != nil {
			b.Fatal(err)
		}
		if _, err := s.Monsters(); err != nil {
			b.Fatal(err)
		}
	}
	b.ReportMetric(float64(len(fsys["monsters.fb"].Data))/1e6, "file-MB")
}
