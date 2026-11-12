package dataset_test

import (
	"bytes"
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"math"
	"slices"
	"strconv"
	"testing"
	"testing/fstest"

	flatbuffers "github.com/google/flatbuffers/go"
	gocmp "github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/dataset/internal/fb"
)

// Stat handles withSkills adds after sample's three.
const (
	minFire uint32 = iota + 3
	maxFire
	minionType
	totemFlag
)

// Skill type indexes of sampleSkills.
const (
	typeSpell uint16 = iota
	typeProjectile
	typeOr
	typeTotem
)

// withSkills returns sample with a skills group: a gem with a transfigured variant, its Vaal
// gem, a support, and a monster skill no gem grants.
func withSkills() *dataset.Dataset {
	d := sample()
	d.Stats = append(d.Stats,
		dataset.Stat{ID: "spell_minimum_base_fire_damage"}, dataset.Stat{ID: "spell_maximum_base_fire_damage"},
		dataset.Stat{ID: "display_minion_monster_type"}, dataset.Stat{ID: "is_totem"})
	d.Skills = sampleSkills()
	d.Texts[0].GemNames = []dataset.Name{{ID: "Metadata/Items/Gems/SkillGemFireball", Name: "Fireball"}}
	d.Texts[0].SkillNames = []dataset.Name{{ID: "fireball", Name: "Fireball"}, {ID: "fireball_alt_x", Name: "Fireball of Something"}}
	return d
}

func sampleSkills() *dataset.Skills {
	fireLevel := func(level, minDmg, maxDmg int32) dataset.StatSetLevel {
		return dataset.StatSetLevel{
			Level: level, PlayerLevel: level, SpellCritChance: 600, DamageEffectiveness: 20000,
			FloatStats: []dataset.FloatStat{
				{Stat: minFire, Value: 0.8, Resolved: minDmg, Interpolation: dataset.EffectivenessInterpolation, Base: "DamageValue"},
				{Stat: maxFire, Value: 1.2, Resolved: maxDmg, Interpolation: dataset.EffectivenessInterpolation, Base: "DamageValue"},
			},
			Stats: []dataset.LevelStat{{Stat: 1, Value: 10, Interpolation: dataset.NoInterpolation}, {Stat: 1, Value: 2, Interpolation: dataset.LinearInterpolation}},
		}
	}
	return &dataset.Skills{
		Types:          []string{"Spell", "Projectile", "OR", "Totem"},
		Gems:           sampleGems(),
		GrantedEffects: sampleGrantedEffects(),
		StatSets: []dataset.StatSet{
			{
				ID: "Fireball", ImplicitStats: []uint32{0, 0}, ConstantStats: []dataset.StatValue{{Stat: minionType, Value: 1}, {Stat: 2, Value: 25}},
				BaseEffectiveness: 2.921, IncrementalEffectiveness: 0.0495,
				Levels: []dataset.StatSetLevel{fireLevel(1, 9, 14), fireLevel(20, 603, 905)},
			},
			{ID: "MonsterSlam", Levels: []dataset.StatSetLevel{{Level: 1, AttackCritChance: 500, BaseMultiplier: -2500}}},
			{ID: "SupportSpellTotem", Levels: []dataset.StatSetLevel{{Level: 1, Flags: []uint32{totemFlag}}}},
		},
		ActiveSkills: []dataset.ActiveSkill{
			{ID: "fireball", Types: []uint16{typeSpell, typeProjectile}, Totem: 22},
			{ID: "fireball_alt_x", Types: []uint16{typeSpell}, TransfigureBase: "fireball", Totem: 22},
			{ID: "monster_slam", MinionTypes: []uint16{typeProjectile}, WeaponRestrictions: []string{"Staff", "Two Hand Mace"}, Totem: 5, Mine: 8},
			{ID: "vaal_fireball", Types: []uint16{typeSpell}},
		},
	}
}

func sampleGems() []dataset.Gem {
	return []dataset.Gem{
		{
			ID: "Metadata/Items/Gems/SkillGemFireball", Colour: dataset.Blue, IntelligencePercent: 100,
			VaalGem: "Metadata/Items/Gems/SkillGemVaalFireball",
			Effects: []dataset.GemEffect{
				{ID: "Fireball", GrantedEffect: "Fireball", HardmodeGrantedEffect: "FireballRuthless", Tags: []string{"fire", "spell"}, Style: dataset.DefaultStyle},
				{ID: "FireballAltX", GrantedEffect: "FireballAltX", Tags: []string{"fire"}, Style: dataset.TransfiguredX},
			},
		},
		{
			ID: "Metadata/Items/Gems/SkillGemVaalFireball", Colour: dataset.Blue, IntelligencePercent: 100, Vaal: true,
			RegularGem: "Metadata/Items/Gems/SkillGemFireball",
			Effects:    []dataset.GemEffect{{ID: "VaalFireball", GrantedEffect: "VaalFireball", SecondaryGrantedEffect: "Fireball", Style: dataset.DefaultStyle}},
		},
		{
			ID: "Metadata/Items/Gems/SupportGemSpellTotem", Colour: dataset.Green, StrengthPercent: 50, DexterityPercent: 50, Support: true,
			Effects: []dataset.GemEffect{{ID: "SupportSpellTotem", GrantedEffect: "SupportSpellTotem", Tags: []string{"support"}, Style: dataset.DefaultStyle}},
		},
	}
}

func sampleGrantedEffects() []dataset.GrantedEffect {
	return []dataset.GrantedEffect{
		{
			ID: "Fireball", ActiveSkill: "fireball", CastTime: 750, StatSets: []string{"Fireball"},
			QualityStats: []dataset.StatValue{{Stat: 1, Value: 150}},
			Levels: []dataset.EffectLevel{
				{Level: 1, PlayerLevel: 1, Costs: []dataset.Cost{{Type: "Mana", Amount: 5}}},
				{Level: 20, PlayerLevel: 70, Costs: []dataset.Cost{{Type: "Mana", Amount: 23}}, Cooldown: 0, CooldownGroup: 36},
			},
		},
		{ID: "FireballAltX", ActiveSkill: "fireball_alt_x", StatSets: []string{"Fireball"}, Levels: []dataset.EffectLevel{{Level: 1}}},
		{ID: "FireballRuthless", ActiveSkill: "fireball", StatSets: []string{"Fireball"}},
		{
			ID: "MonsterSlam", ActiveSkill: "monster_slam", StatSets: []string{"MonsterSlam", "Fireball"},
			Levels: []dataset.EffectLevel{{Level: 1, AttackSpeedMultiplier: -20, AttackTime: 1000}},
		},
		{
			ID: "SupportSpellTotem", Support: true, StatSets: []string{"SupportSpellTotem"},
			AllowedTypes: []uint16{typeSpell, typeProjectile, typeOr}, ExcludedTypes: []uint16{typeTotem}, AddedTypes: []uint16{typeTotem},
			AddedMinionTypes: []uint16{typeSpell}, SupportWeaponRestrictions: []string{"Staff"}, SupportsGemsOnly: true,
			Levels: []dataset.EffectLevel{{Level: 1, CostMultiplier: 150, ManaReservationPercent: 2500, LifeReservationFlat: 10}},
		},
		{
			ID: "VaalFireball", ActiveSkill: "vaal_fireball", StatSets: []string{"Fireball"}, CannotBeSupported: true,
			Levels: []dataset.EffectLevel{{Level: 1, VaalSouls: 30, VaalStoredUses: 1, SoulGainPrevention: 4000, StoredUses: 1, Cooldown: 500}},
		},
	}
}

func TestSkillsRoundTrip(t *testing.T) {
	t.Parallel()
	want := withSkills()
	fsys := written(t, want)
	got, err := readAll(fsys)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	want.Manifest = got.Manifest
	if diff := gocmp.Diff(want, got, cmpopts.EquateEmpty()); diff != "" {
		t.Errorf("round trip mismatch (-want +got):\n%s", diff)
	}
	if !slices.ContainsFunc(got.Manifest.Files, func(f dataset.File) bool { return f.Name == "skills.fb" }) {
		t.Errorf("manifest: got files %v, want skills.fb among them", got.Manifest.Files)
	}
}

// A dataset without a skills group writes its text file as before the group existed.
func TestTextWithoutSkillNamesUnchanged(t *testing.T) {
	t.Parallel()
	plain, named := sample(), sample()
	named.Texts[0].GemNames = []dataset.Name{}
	a, b := written(t, plain), written(t, named)
	if !bytes.Equal(a["text-en.fb"].Data, b["text-en.fb"].Data) {
		t.Errorf("text-en.fb: bytes differ with empty gem names, want equal")
	}
}

// renumberedSkills returns withSkills with its stats reversed, as a later game version might
// number them.
func renumberedSkills() *dataset.Dataset { return reversedStats(withSkills()) }

// reversedStats returns d, which has a skills group, with its stats reversed.
func reversedStats(d *dataset.Dataset) *dataset.Dataset {
	n := uint32(len(d.Stats))
	h := func(x uint32) uint32 { return n - 1 - x }
	reversed := make([]dataset.Stat, n)
	for i, s := range d.Stats {
		reversed[h(uint32(i))] = s
	}
	d.Stats = reversed
	for _, c := range d.Characters {
		renumber(c.Stats, h)
		slices.SortFunc(c.Stats, func(a, b dataset.StatValue) int { return cmp.Compare(a.Stat, b.Stat) })
	}
	for _, q := range d.Quests {
		renumber(q.Stats, h)
	}
	for _, nd := range d.Trees[0].Nodes {
		renumber(nd.Stats, h)
		for _, b := range nd.Buffs {
			renumber(b, h)
		}
		for _, e := range nd.MasteryEffects {
			renumber(e.Stats, h)
		}
	}
	renumberSkills(d.Skills, h)
	if d.Monsters != nil {
		renumberMonsters(d.Monsters, h)
	}
	return d
}

func renumber(svs []dataset.StatValue, h func(uint32) uint32) {
	for i := range svs {
		svs[i].Stat = h(svs[i].Stat)
	}
}

func renumberHandles(hs []uint32, h func(uint32) uint32) {
	for i := range hs {
		hs[i] = h(hs[i])
	}
}

func renumberSkills(sk *dataset.Skills, h func(uint32) uint32) {
	for _, g := range sk.GrantedEffects {
		renumber(g.QualityStats, h)
	}
	for _, s := range sk.StatSets {
		renumber(s.ConstantStats, h)
		renumberHandles(s.ImplicitStats, h)
		for _, l := range s.Levels {
			for i := range l.FloatStats {
				l.FloatStats[i].Stat = h(l.FloatStats[i].Stat)
			}
			for i := range l.Stats {
				l.Stats[i].Stat = h(l.Stats[i].Stat)
			}
			renumberHandles(l.Flags, h)
		}
	}
}

func TestSkillsFileIndependentOfStatOrder(t *testing.T) {
	t.Parallel()
	a, b := written(t, withSkills()), written(t, renumberedSkills())
	if !bytes.Equal(a["skills.fb"].Data, b["skills.fb"].Data) {
		t.Errorf("skills.fb: bytes differ after renumbering stats, want equal")
	}
	got, err := readAll(b)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if diff := gocmp.Diff(renumberedSkills().Skills, got.Skills, cmpopts.EquateEmpty()); diff != "" {
		t.Errorf("round trip mismatch (-want +got):\n%s", diff)
	}
}

func TestSkillsWriteErrors(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name, want string
		change     func(s *dataset.Skills)
	}{
		{name: "skill type repeated", want: "skill type Spell repeated", change: func(s *dataset.Skills) { s.Types[1] = "Spell" }},
		{name: "skill type empty", want: "skill type 1: empty id", change: func(s *dataset.Skills) { s.Types[1] = "" }},
		{name: "too many skill types", want: "65537 skill types, the file holds at most 65536", change: func(s *dataset.Skills) {
			for i := range 65537 - len(s.Types) {
				s.Types = append(s.Types, "t"+strconv.Itoa(i))
			}
		}},
		{name: "gem repeated", want: "gem Metadata/Items/Gems/SkillGemFireball repeated", change: func(s *dataset.Skills) {
			s.Gems[2].ID = s.Gems[0].ID
		}},
		{name: "gem id empty", want: "gem 0: empty id", change: func(s *dataset.Skills) { s.Gems[0].ID = "" }},
		{name: "gem colour", want: "colour 5 out of range", change: func(s *dataset.Skills) { s.Gems[0].Colour = 5 }},
		{name: "unknown vaal gem", want: "vaal gem x not in skills", change: func(s *dataset.Skills) { s.Gems[0].VaalGem = "x" }},
		{name: "unknown regular gem", want: "regular gem x not in skills", change: func(s *dataset.Skills) { s.Gems[1].RegularGem = "x" }},
		{name: "unknown awakened gem", want: "awakened gem x not in skills", change: func(s *dataset.Skills) { s.Gems[2].AwakenedGem = "x" }},
		{name: "gem effect repeated", want: "effect Fireball repeated", change: func(s *dataset.Skills) { s.Gems[0].Effects[1].ID = "Fireball" }},
		{
			name: "gem effect without granted effect", want: "effect Fireball: no granted effect",
			change: func(s *dataset.Skills) { s.Gems[0].Effects[0].GrantedEffect = "" },
		},
		{name: "gem effect style", want: "effect Fireball: style 0 out of range", change: func(s *dataset.Skills) { s.Gems[0].Effects[0].Style = 0 }},
		{
			name: "unknown granted effect", want: "effect Fireball: granted effect x not in skills",
			change: func(s *dataset.Skills) { s.Gems[0].Effects[0].HardmodeGrantedEffect = "x" },
		},
		{name: "granted effect repeated", want: "granted effect Fireball repeated", change: func(s *dataset.Skills) {
			s.GrantedEffects[1].ID = "Fireball"
		}},
		{
			name: "unknown active skill", want: "granted effect Fireball: active skill x not in skills",
			change: func(s *dataset.Skills) { s.GrantedEffects[0].ActiveSkill = "x" },
		},
		{
			name: "unknown stat set", want: "granted effect Fireball: stat set x not in skills",
			change: func(s *dataset.Skills) { s.GrantedEffects[0].StatSets = []string{"x"} },
		},
		{name: "empty stat set", want: "granted effect Fireball: empty stat set", change: func(s *dataset.Skills) { s.GrantedEffects[0].StatSets = []string{""} }},
		{
			name: "quality stat out of range", want: "granted effect Fireball: quality: stat 9 out of range",
			change: func(s *dataset.Skills) { s.GrantedEffects[0].QualityStats[0].Stat = 9 },
		},
		{name: "skill type out of range", want: "granted effect SupportSpellTotem: skill type 4 out of range (4 types)", change: func(s *dataset.Skills) {
			s.GrantedEffects[4].ExcludedTypes = []uint16{4}
		}},
		{name: "effect levels unsorted", want: "granted effect Fireball: levels not sorted or repeated", change: func(s *dataset.Skills) {
			s.GrantedEffects[0].Levels[1].Level = 1
		}},
		{name: "cost without type", want: "granted effect Fireball: level 1: cost without type", change: func(s *dataset.Skills) {
			s.GrantedEffects[0].Levels[0].Costs[0].Type = ""
		}},
		{name: "stat set repeated", want: "stat set Fireball repeated", change: func(s *dataset.Skills) {
			s.StatSets[1].ID = "Fireball"
		}},
		{
			name: "implicit stat out of range", want: "stat set Fireball: implicit: stat 9 out of range",
			change: func(s *dataset.Skills) { s.StatSets[0].ImplicitStats[0] = 9 },
		},
		{
			name: "constant stat out of range", want: "stat set Fireball: constant: stat 9 out of range",
			change: func(s *dataset.Skills) { s.StatSets[0].ConstantStats[0].Stat = 9 },
		},
		{
			name: "stat set levels unsorted", want: "stat set Fireball: levels not sorted or repeated",
			change: func(s *dataset.Skills) { s.StatSets[0].Levels[1].Level = 1 },
		},
		{
			name: "float stat out of range", want: "stat set Fireball: level 1: stat 9 out of range",
			change: func(s *dataset.Skills) { s.StatSets[0].Levels[0].FloatStats[0].Stat = 9 },
		},
		{name: "float stat interpolation", want: "stat set Fireball: level 1: stat 3: interpolation 4 out of range", change: func(s *dataset.Skills) {
			s.StatSets[0].Levels[0].FloatStats[0].Interpolation = 4
		}},
		{name: "float stat without base", want: "stat set Fireball: level 1: float stat 3: no interpolation base", change: func(s *dataset.Skills) {
			s.StatSets[0].Levels[0].FloatStats[0].Base = ""
		}},
		{
			name: "level stat out of range", want: "stat set Fireball: level 1: stat 9 out of range",
			change: func(s *dataset.Skills) { s.StatSets[0].Levels[0].Stats[0].Stat = 9 },
		},
		{name: "level stat interpolation", want: "stat set Fireball: level 1: stat 1: interpolation 0 out of range", change: func(s *dataset.Skills) {
			s.StatSets[0].Levels[0].Stats[0].Interpolation = 0
		}},
		{
			name: "flag out of range", want: "stat set SupportSpellTotem: level 1: flags: stat 9 out of range",
			change: func(s *dataset.Skills) { s.StatSets[2].Levels[0].Flags[0] = 9 },
		},
		{name: "active skill repeated", want: "active skill fireball repeated", change: func(s *dataset.Skills) {
			s.ActiveSkills[2].ID = "fireball"
		}},
		{name: "operator without values", want: "granted effect SupportSpellTotem: skill types: OR needs 2 values, has 1", change: func(s *dataset.Skills) {
			s.GrantedEffects[4].AllowedTypes = []uint16{typeSpell, typeOr, typeProjectile}
		}},
		{name: "operator in excluded types", want: "granted effect SupportSpellTotem: skill types: OR needs 2 values, has 0", change: func(s *dataset.Skills) {
			s.GrantedEffects[4].ExcludedTypes = []uint16{typeOr}
		}},
		{name: "operator in added types", want: "granted effect SupportSpellTotem: skill types: operator OR outside allowed or excluded types",
			change: func(s *dataset.Skills) {
				s.GrantedEffects[4].AddedTypes = []uint16{typeSpell, typeOr}
			}},
		{name: "operator in skill types", want: "active skill fireball: skill types: operator OR outside allowed or excluded types", change: func(s *dataset.Skills) {
			s.ActiveSkills[0].Types = []uint16{typeOr}
		}},
		{name: "float stat NaN", want: "stat set Fireball: level 1: float stat 3: value NaN not finite", change: func(s *dataset.Skills) {
			s.StatSets[0].Levels[0].FloatStats[0].Value = float32(math.NaN())
		}},
		{name: "effectiveness infinite", want: "stat set Fireball: effectiveness +Inf, 0.0495 not finite", change: func(s *dataset.Skills) {
			s.StatSets[0].BaseEffectiveness = float32(math.Inf(1))
		}},
		{name: "effectiveness NaN", want: "stat set Fireball: effectiveness 2.921, NaN not finite", change: func(s *dataset.Skills) {
			s.StatSets[0].IncrementalEffectiveness = float32(math.NaN())
		}},
		{name: "shared effect differs", want: "gem Metadata/Items/Gems/SupportGemSpellTotem: effect Fireball differs from the effect of the same id in another gem",
			change: func(s *dataset.Skills) {
				s.Gems[2].Effects[0].ID = "Fireball"
			}},
		{
			name: "active skill type out of range", want: "active skill fireball: skill type 7 out of range",
			change: func(s *dataset.Skills) { s.ActiveSkills[0].Types[0] = 7 },
		},
		{
			name: "minion type out of range", want: "active skill monster_slam: skill type 7 out of range",
			change: func(s *dataset.Skills) { s.ActiveSkills[2].MinionTypes[0] = 7 },
		},
		{
			name: "unknown transfigure base", want: "active skill fireball_alt_x: transfigure base x not in skills",
			change: func(s *dataset.Skills) { s.ActiveSkills[1].TransfigureBase = "x" },
		},
		{name: "too many item classes", want: "65537 item classes, the file holds at most 65536", change: func(s *dataset.Skills) {
			for i := range 65535 {
				s.ActiveSkills[0].WeaponRestrictions = append(s.ActiveSkills[0].WeaponRestrictions, "c"+strconv.Itoa(i))
			}
		}},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			d := withSkills()
			testCase.change(d.Skills)
			_, err := dataset.Write(t.TempDir(), d)
			checkError(t, err, dataset.ErrInvalid, testCase.want)
		})
	}
}

// A gem effect the game shares between gems is accepted when it is the same in each.
func TestSkillsSharedGemEffect(t *testing.T) {
	t.Parallel()
	d := withSkills()
	d.Skills.Gems[2].Effects = append(d.Skills.Gems[2].Effects, d.Skills.Gems[0].Effects[1])
	if _, err := dataset.Write(t.TempDir(), d); err != nil {
		t.Errorf("Write: got %v, want no error", err)
	}
}

func TestSkillNamesWriteErrors(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name, want string
		change     func(d *dataset.Dataset)
	}{
		{name: "unknown gem", want: "text en: name for unknown gem x", change: func(d *dataset.Dataset) { d.Texts[0].GemNames[0].ID = "x" }},
		{name: "unknown skill", want: "text en: name for unknown skill x", change: func(d *dataset.Dataset) { d.Texts[0].SkillNames[1].ID = "x" }},
		{name: "unknown skill in second text", want: "text zh-hant: name for unknown skill x", change: func(d *dataset.Dataset) {
			d.Texts[1].SkillNames = []dataset.Name{{ID: "x", Name: "x"}}
		}},
		{name: "names without skills", want: "text en: name for unknown gem", change: func(d *dataset.Dataset) { d.Skills = nil }},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			d := withSkills()
			testCase.change(d)
			_, err := dataset.Write(t.TempDir(), d)
			checkError(t, err, dataset.ErrInvalid, testCase.want)
		})
	}
}

// skillsFile builds a skills file with the given active skills, each restricted to the item
// class index weapon, for layouts Write never produces.
func SkillsFile(actives []string, weapon uint16, classes []string) []byte {
	b := flatbuffers.NewBuilder(0)
	offs := make([]flatbuffers.UOffsetT, len(actives))
	for i, a := range actives {
		id := b.CreateString(a)
		fb.ActiveSkillStartWeaponRestrictionsVector(b, 1)
		b.PrependUint16(weapon)
		weapons := b.EndVector(1)
		fb.ActiveSkillStart(b)
		fb.ActiveSkillAddId(b, id)
		fb.ActiveSkillAddWeaponRestrictions(b, weapons)
		offs[i] = fb.ActiveSkillEnd(b)
	}
	vector := func(start func(*flatbuffers.Builder, int) flatbuffers.UOffsetT, offs []flatbuffers.UOffsetT) flatbuffers.UOffsetT {
		start(b, len(offs))
		for _, o := range slices.Backward(offs) {
			b.PrependUOffsetT(o)
		}
		return b.EndVector(len(offs))
	}
	strs := func(start func(*flatbuffers.Builder, int) flatbuffers.UOffsetT, ss []string) flatbuffers.UOffsetT {
		o := make([]flatbuffers.UOffsetT, len(ss))
		for i, s := range ss {
			o[i] = b.CreateString(s)
		}
		return vector(start, o)
	}
	activeVec := vector(fb.SkillsStartActiveSkillsVector, offs)
	classVec := strs(fb.SkillsStartItemClassesVector, classes)
	fb.SkillsStart(b)
	fb.SkillsAddActiveSkills(b, activeVec)
	fb.SkillsAddItemClasses(b, classVec)
	fb.FinishSkillsBuffer(b, fb.SkillsEnd(b))
	return b.FinishedBytes()
}

func TestSkillsReadErrors(t *testing.T) {
	t.Parallel()
	good := written(t, withSkills())
	replaced := func(name string, data []byte) fstest.MapFS {
		return rewritten(t, good, name, data)
	}
	skills := good["skills.fb"].Data
	fewStats := written(t, &dataset.Dataset{Stats: sample().Stats})["stats.fb"].Data
	testCases := []struct {
		name, want string
		fsys       fstest.MapFS
	}{
		{name: "truncated", want: "skills.fb: malformed", fsys: replaced("skills.fb", skills[:len(skills)/2])},
		{name: "wrong identifier", want: "skills.fb: wrong file identifier", fsys: replaced("skills.fb", good["constants.fb"].Data)},
		{name: "stat not in stats group", want: `skills.fb: unknown stat "display_minion_monster_type"`, fsys: replaced("stats.fb", fewStats)},
		{name: "index out of range", want: "skills.fb: item class 1 out of range (1)", fsys: replaced("skills.fb", SkillsFile([]string{"a"}, 1, []string{"Staff"}))},
		{
			name: "fails validation", want: "skills.fb: active skill a repeated",
			fsys: replaced("skills.fb", SkillsFile([]string{"a", "a"}, 0, []string{"Staff"})),
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			s, err := dataset.Open(testCase.fsys)
			if err != nil {
				t.Fatalf("Open: %v", err)
			}
			_, err = s.Skills()
			checkError(t, err, dataset.ErrInvalid, testCase.want)
		})
	}
}

// rewritten returns a copy of fsys with file name set to data and the manifest's size, hash
// and id updated to match.
func rewritten(tb testing.TB, fsys fstest.MapFS, name string, data []byte) fstest.MapFS {
	tb.Helper()
	m, err := dataset.ReadManifest(fsys)
	if err != nil {
		tb.Fatal(err)
	}
	out := maps.Clone(fsys)
	out[name] = &fstest.MapFile{Data: data}
	for i, f := range m.Files {
		if f.Name == name {
			sum := sha256.Sum256(data)
			m.Files[i] = dataset.File{Name: name, Size: len(data), SHA256: hex.EncodeToString(sum[:])}
		}
	}
	m.ID = dataset.ContentID(m)
	b, err := json.Marshal(m)
	if err != nil {
		tb.Fatal(err)
	}
	out[dataset.ManifestName] = &fstest.MapFile{Data: b}
	return out
}

// syntheticSkills returns a dataset whose skills group has about the row counts of the
// 3.29.3.3 game data: 12,829 granted effects with 80,600 effect levels, 12,763 stat sets with
// 79,927 levels holding 162,000 stats, 847 gems and 1,941 active skills.
func syntheticSkills() *dataset.Dataset {
	d := &dataset.Dataset{Stats: make([]dataset.Stat, 2000)}
	for i := range d.Stats {
		d.Stats[i].ID = fmt.Sprintf("stat_%04d", i)
	}
	sk := &dataset.Skills{Types: make([]string, 135), StatSets: syntheticStatSets(), GrantedEffects: syntheticGrantedEffects()}
	for i := range sk.Types {
		sk.Types[i] = fmt.Sprintf("Type%03d", i)
	}
	for i := range 1941 {
		sk.ActiveSkills = append(sk.ActiveSkills, dataset.ActiveSkill{ID: syntheticID("active", i), Types: []uint16{uint16(i % 135), 1, 7}, Totem: 22})
	}
	for i := range 847 {
		sk.Gems = append(sk.Gems, dataset.Gem{ID: syntheticID("gem", i), Colour: dataset.Blue, IntelligencePercent: 100, Effects: []dataset.GemEffect{
			{ID: syntheticID("gemeffect", i), GrantedEffect: syntheticID("effect", i), Tags: []string{"fire", "spell"}, Style: dataset.DefaultStyle},
		}})
	}
	d.Skills = sk
	return d
}

func syntheticID(kind string, i int) string { return fmt.Sprintf("%s%05d", kind, i) }

// syntheticStatSets returns 1,255 sets of 40 levels and the rest of 1 to 4, as in the game data.
func syntheticStatSets() []dataset.StatSet {
	out := make([]dataset.StatSet, 12763)
	for i := range out {
		set := dataset.StatSet{ID: syntheticID("set", i), ConstantStats: []dataset.StatValue{{Stat: uint32(i % 2000), Value: 25}}, BaseEffectiveness: 2.9}
		levels := 1 + i%4
		if i < 1255 {
			levels = 40
		}
		for l := range levels {
			set.Levels = append(set.Levels, dataset.StatSetLevel{
				Level: int32(l + 1), PlayerLevel: int32(l + 1), SpellCritChance: 600, DamageEffectiveness: 20000,
				FloatStats: []dataset.FloatStat{{
					Stat: uint32(i % 1999), Value: 0.8, Resolved: int32(l), Interpolation: dataset.EffectivenessInterpolation, Base: "DamageValue",
				}},
				Stats: []dataset.LevelStat{{Stat: uint32(l), Value: int32(i), Interpolation: dataset.NoInterpolation}},
			})
		}
		out[i] = set
	}
	return out
}

// syntheticGrantedEffects returns 1,600 effects of 40 levels, a tenth of the rest of 6 and the
// others of 1.
func syntheticGrantedEffects() []dataset.GrantedEffect {
	out := make([]dataset.GrantedEffect, 12829)
	for i := range out {
		ge := dataset.GrantedEffect{
			ID: syntheticID("effect", i), ActiveSkill: syntheticID("active", i%1941), StatSets: []string{syntheticID("set", i%12763)}, CastTime: 750,
		}
		levels := 1
		switch {
		case i < 1600:
			levels = 40
		case i%10 == 0:
			levels = 6
		}
		for l := range levels {
			ge.Levels = append(ge.Levels, dataset.EffectLevel{
				Level: int32(l + 1), PlayerLevel: int32(l + 1), Costs: []dataset.Cost{{Type: "Mana", Amount: int32(l)}}, CooldownGroup: 36,
			})
		}
		out[i] = ge
	}
	return out
}

// BenchmarkSkills opens a dataset and loads its skills group, which has about the row counts
// of the 3.29.3.3 game data (target: < 50 ms native).
func BenchmarkSkills(b *testing.B) {
	fsys := written(b, syntheticSkills())
	b.ReportAllocs()
	for b.Loop() {
		s, err := dataset.Open(fsys)
		if err != nil {
			b.Fatal(err)
		}
		if _, err := s.Skills(); err != nil {
			b.Fatal(err)
		}
	}
	b.ReportMetric(float64(len(fsys["skills.fb"].Data))/1e6, "file-MB")
}

func TestStoreSkillsMissing(t *testing.T) {
	t.Parallel()
	s, err := dataset.Open(written(t, sample()))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	_, err = s.Skills()
	checkError(t, err, dataset.ErrNotInDataset, "dataset 3.29.3.3 has no skills group")
	missing, ok := errors.AsType[*dataset.MissingGroupError](err)
	if !ok || *missing != (dataset.MissingGroupError{Version: "3.29.3.3", Group: "skills"}) {
		t.Errorf("got %#v, want a MissingGroupError for skills of 3.29.3.3", err)
	}
}

func TestStoreReadsSkillsOnUse(t *testing.T) {
	t.Parallel()
	want := withSkills()
	fsys := maps.Clone(written(t, want))
	for _, n := range []string{"characters.fb", "constants.fb", "quests.fb", "tree-standard.fb", "tree-ruthless.fb", "text-en.fb", "text-zh-hant.fb"} {
		fsys[n] = &fstest.MapFile{Data: []byte("x")}
	}
	s, err := dataset.Open(fsys)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	got, err := s.Skills()
	if err != nil {
		t.Fatalf("Skills: %v", err)
	}
	if diff := gocmp.Diff(want.Skills, got, cmpopts.EquateEmpty()); diff != "" {
		t.Errorf("Skills (-want +got):\n%s", diff)
	}
}
