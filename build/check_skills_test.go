package build_test

import (
	"slices"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/dataset"
)

func vaalFireball() build.GemID { return build.GemID{ID: "Metadata/Items/Gems/SkillGemVaalFireball"} }

func predator() build.GemID { return build.GemID{ID: "Metadata/Items/Gems/SupportGemDeathmark"} }

// skillsStore is testStore with a skills group: Fireball (levels 1 to 40) with its
// transfigured variant FireballAltX (1 to 20), Vaal Fireball (1 to 20) whose second skill is
// Fireball, the support Inspiration (1 to 20), and the support Predator (1 to 20) whose second
// skill is the active Signal Prey.
func skillsStore(t *testing.T) *dataset.Store {
	t.Helper()
	d := testDataset()
	levels := func(top int32) []dataset.EffectLevel { return []dataset.EffectLevel{{Level: 1}, {Level: top}} }
	d.Skills = &dataset.Skills{
		Types: []string{"Spell"},
		Gems: []dataset.Gem{
			{ID: fireball().ID, Colour: dataset.Blue, Effects: []dataset.GemEffect{
				{ID: "Fireball", GrantedEffect: "Fireball", Style: dataset.DefaultStyle},
				{ID: "FireballAltX", GrantedEffect: "FireballAltX", Style: dataset.TransfiguredX},
			}},
			{ID: vaalFireball().ID, Colour: dataset.Blue, Vaal: true, Effects: []dataset.GemEffect{
				{ID: "VaalFireball", GrantedEffect: "VaalFireball", SecondaryGrantedEffect: "Fireball", Style: dataset.DefaultStyle},
			}},
			{ID: predator().ID, Colour: dataset.Green, Support: true, Effects: []dataset.GemEffect{
				{ID: "SupportPredator", GrantedEffect: "SupportPredator", SecondaryGrantedEffect: "SignalPrey", Style: dataset.DefaultStyle},
			}},
			{ID: inspiration().ID, Colour: dataset.Blue, Support: true, Effects: []dataset.GemEffect{
				{ID: "SupportInspiration", GrantedEffect: "SupportInspiration", Style: dataset.DefaultStyle},
			}},
		},
		GrantedEffects: []dataset.GrantedEffect{
			{ID: "Fireball", ActiveSkill: "fireball", Levels: levels(40)},
			{ID: "FireballAltX", ActiveSkill: "fireball_alt_x", Levels: levels(20)},
			{ID: "VaalFireball", ActiveSkill: "vaal_fireball", Levels: levels(20)},
			{ID: "SupportInspiration", Support: true, Levels: levels(20)},
			{ID: "SupportPredator", Support: true, Levels: levels(20)},
			{ID: "SignalPrey", ActiveSkill: "signal_prey", Levels: levels(20)},
		},
		ActiveSkills: []dataset.ActiveSkill{
			{ID: "fireball", Types: []uint16{0}}, {ID: "fireball_alt_x", Types: []uint16{0}}, {ID: "vaal_fireball", Types: []uint16{0}},
			{ID: "signal_prey", Types: []uint16{0}},
		},
	}
	return openStore(t, d)
}

// TestCheckSkills: each skill check warns and fixes as its message says.
func TestCheckSkills(t *testing.T) {
	t.Parallel()
	s := skillsStore(t)
	if _, warnings, err := build.Check(withSkillsChecked(), s); err != nil || len(warnings) != 0 {
		t.Fatalf("valid skills: %v, %v", warnings, err)
	}
	testCases := []struct {
		name  string
		edit  func(sk *build.Skills)
		codes []build.Code
		want  func(sk *build.Skills)
	}{
		{
			"unknown gem dropped", func(sk *build.Skills) {
				sk.Groups[0].Gems = slices.Insert(sk.Groups[0].Gems, 0, build.Gem{ID: "Metadata/Items/Gems/Nothing", Level: 1})
				sk.Groups[0].Active = 1
			},
			[]build.Code{build.GemUnknown}, func(*build.Skills) {},
		},
		{
			"unknown variant cleared", func(sk *build.Skills) { sk.Groups[0].Gems[0].Effect = "FireballAltZ" },
			[]build.Code{build.GemEffectUnknown}, func(*build.Skills) {},
		},
		{
			"first variant named", func(sk *build.Skills) { sk.Groups[0].Gems[0].Effect = "Fireball" },
			nil, func(*build.Skills) {},
		},
		{
			"level over the variant's top", func(sk *build.Skills) { sk.Groups[0].Gems[0].Effect = "FireballAltX"; sk.Groups[0].Gems[0].Level = 21 },
			[]build.Code{build.GemLevelOutOfRange},
			func(sk *build.Skills) { sk.Groups[0].Gems[0].Effect = "FireballAltX"; sk.Groups[0].Gems[0].Level = 20 },
		},
		{
			"level under 1", func(sk *build.Skills) { sk.Groups[0].Gems[1].Level = 0 },
			[]build.Code{build.GemLevelOutOfRange}, func(sk *build.Skills) { sk.Groups[0].Gems[1].Level = 1 },
		},
		{
			"quality over 23", func(sk *build.Skills) { sk.Groups[0].Gems[0].Quality = 30 },
			[]build.Code{build.GemQualityOutOfRange}, func(sk *build.Skills) { sk.Groups[0].Gems[0].Quality = 23 },
		},
		{
			"negative quality", func(sk *build.Skills) { sk.Groups[0].Gems[1].Quality = -1 },
			[]build.Code{build.GemQualityOutOfRange}, func(sk *build.Skills) { sk.Groups[0].Gems[1].Quality = 0 },
		},
		{
			"support flag corrected", func(sk *build.Skills) { sk.Groups[0].Gems[0].Support = true; sk.Groups[0].Gems[2].Support = false },
			[]build.Code{build.GemRoleMismatch, build.GemRoleMismatch}, func(*build.Skills) {},
		},
		{
			"active gem a support", func(sk *build.Skills) { sk.Groups[0].Active = 1 },
			[]build.Code{build.ActiveGemInvalid}, func(*build.Skills) {},
		},
		{
			"active gem dropped", func(sk *build.Skills) {
				sk.Groups[1].Gems[0].ID = "Metadata/Items/Gems/Nothing"
				sk.Groups[1].Gems[2] = build.Gem{GemID: fireball(), Level: 1}
			},
			[]build.Code{build.GemUnknown, build.ActiveGemInvalid},
			func(sk *build.Skills) {
				sk.Groups[1].Gems = []build.Gem{sk.Groups[1].Gems[1], {GemID: fireball(), Level: 1}}
				sk.Groups[1].Active = 1
			},
		},
		{
			"second skill of a Vaal gem", func(sk *build.Skills) {
				sk.Groups[0].Gems[0] = build.Gem{GemID: vaalFireball(), Level: 20}
				sk.Groups[0].ActiveSkill = 1
			},
			nil, func(sk *build.Skills) {
				sk.Groups[0].Gems[0] = build.Gem{GemID: vaalFireball(), Level: 20}
				sk.Groups[0].ActiveSkill = 1
			},
		},
		{
			"a support's active skill", func(sk *build.Skills) {
				sk.Groups[0].Gems[1] = build.Gem{GemID: predator(), Level: 20, Support: true}
				sk.Groups[0].Active, sk.Groups[0].ActiveSkill = 1, 1
			},
			nil, func(sk *build.Skills) {
				sk.Groups[0].Gems[1] = build.Gem{GemID: predator(), Level: 20, Support: true}
				sk.Groups[0].Active, sk.Groups[0].ActiveSkill = 1, 1
			},
		},
		{
			"a support's own skill", func(sk *build.Skills) {
				sk.Groups[0].Gems[1] = build.Gem{GemID: predator(), Level: 20, Support: true}
				sk.Groups[0].Active = 1
			},
			[]build.Code{build.ActiveGemInvalid}, func(sk *build.Skills) {
				sk.Groups[0].Gems[1] = build.Gem{GemID: predator(), Level: 20, Support: true}
				sk.Groups[0].Active, sk.Groups[0].ActiveSkill = 1, 1
			},
		},
		{
			"second skill of a gem with one", func(sk *build.Skills) { sk.Groups[0].ActiveSkill = 1 },
			[]build.Code{build.ActiveGemInvalid}, func(*build.Skills) {},
		},
		{
			"active skill of a replaced active gem", func(sk *build.Skills) { sk.Groups[0].Active, sk.Groups[0].ActiveSkill = 1, 1 },
			[]build.Code{build.ActiveGemInvalid}, func(*build.Skills) {},
		},
		{
			"active gem out of range", func(sk *build.Skills) { sk.Groups[0].Active = 9 },
			[]build.Code{build.ActiveGemInvalid}, func(*build.Skills) {},
		},
		{
			"supports only", func(sk *build.Skills) { sk.Groups[1].Gems = sk.Groups[1].Gems[1:]; sk.Groups[1].Active = 3 },
			nil, func(sk *build.Skills) { sk.Groups[1].Gems = sk.Groups[1].Gems[1:] },
		},
		{
			"slot without sockets", func(sk *build.Skills) { sk.Groups[1].Slot = "ring-1" },
			[]build.Code{build.SlotUnknown}, func(*build.Skills) {},
		},
		{
			"main skill out of range", func(sk *build.Skills) { sk.MainGroup = 2 },
			[]build.Code{build.MainSkillOutOfRange}, func(sk *build.Skills) { sk.MainGroup = 0 },
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			b := withSkillsChecked()
			testCase.edit(&b.Skills)
			c, warnings, err := build.Check(b, s)
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(testCase.codes, codesOf(warnings)); diff != "" {
				t.Errorf("codes (-want +got):\n%s\n%v", diff, warnings)
			}
			want := withSkillsChecked().Skills
			testCase.want(&want)
			if diff := cmp.Diff(want, c.Build().Skills); diff != "" {
				t.Errorf("checked skills (-want +got):\n%s", diff)
			}
		})
	}
}

// Against a dataset without a skills group, a build with gems gets one warning and keeps them.
func TestCheckSkillsWithoutGroup(t *testing.T) {
	t.Parallel()
	b := withSkillsChecked()
	b.Skills.Groups[0].Gems[0].Level = 99
	c, warnings, err := build.Check(b, testStore(t))
	if err != nil {
		t.Fatal(err)
	}
	want := []build.Warning{{Code: build.SkillsUnchecked, Subject: "skills", Message: "dataset 3.29.3.3 has no skills group; gems kept unchecked"}}
	if diff := cmp.Diff(want, warnings); diff != "" {
		t.Errorf("warnings (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff(b.Skills, c.Build().Skills); diff != "" {
		t.Errorf("skills changed (-want +got):\n%s", diff)
	}
	empty := juggernaut()
	empty.Skills.Groups = []build.SkillGroup{{}}
	if _, warnings, _ := build.Check(empty, testStore(t)); len(warnings) != 0 {
		t.Errorf("groups without gems: %v", warnings)
	}
}

// withSkillsChecked is juggernaut with Fireball and Inspiration in the helmet and a group of
// Fireball of the X variant with two Inspirations; every value fits skillsStore.
func withSkillsChecked() *build.Build {
	b := juggernaut()
	b.Skills = build.Skills{Groups: []build.SkillGroup{
		{Slot: build.SlotHelmet, Gems: []build.Gem{
			{GemID: fireball(), Level: 21, Quality: 23},
			{GemID: inspiration(), Level: 20, Support: true},
			{GemID: inspiration(), Level: 1, Support: true, Disabled: true},
		}},
		{Gems: []build.Gem{
			{GemID: fireballAlt(), Level: 20},
			{GemID: inspiration(), Level: 20, Support: true},
			{GemID: inspiration(), Level: 20, Support: true},
		}},
	}, MainGroup: 1}
	return b
}

func codesOf(warnings []build.Warning) []build.Code {
	if len(warnings) == 0 {
		return nil
	}
	out := make([]build.Code, 0, len(warnings))
	for _, w := range warnings {
		out = append(out, w.Code)
	}
	return out
}
