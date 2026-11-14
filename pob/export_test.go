package pob_test

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/internal/poe1"
	"github.com/xaroth/pbt-engine/pob"
)

func exportRegistry(t *testing.T) *dataset.Registry {
	t.Helper()
	return skillsRegistry(t,
		gemSpec{name: "Fireball", others: []string{"Fireball of Splitting"}}, gemSpec{name: "Vaal Fireball", vaal: true},
		gemSpec{name: "Inspiration", support: true}, gemSpec{name: "Frostblink"}, gemSpec{name: "Punishment"},
		gemSpec{name: "Predator", support: true, grants: "Signal Prey"},
	)
}

// chieftain is a build with one of everything ToXML writes.
func chieftain(t *testing.T) *build.Build {
	t.Helper()
	splitting := gem("Fireball", 21, 23)
	splitting.Effect = "FireballofSplitting"
	disabled := func(g build.Gem) build.Gem { g.Disabled = true; return g }
	return &build.Build{
		Format: build.Format, Game: "poe1", Version: "3.29", Variant: build.DefaultVariant,
		Class: marauder, Ascendancy: "Chieftain", Level: 90,
		Nodes:     []uint32{4139, 11505, 1731},
		Masteries: map[uint32]uint32{11505: 36313, 4139: 47642},
		Overrides: map[uint32]string{1731: "Valako4"},
		Config:    values(t, poe1.BanditsHelpAlira, poe1.CampaignActs6To10),
		Skills: build.Skills{Groups: []build.SkillGroup{
			// The disabled gem's skill does not count towards the main active skill.
			{Slot: build.SlotGloves, Gems: []build.Gem{disabled(gem("Punishment", 20, 0)), gem("Frostblink", 1, 0), gem("Punishment", 1, 0)}, Active: 1},
			// The Vaal gem's two skills come before the main active skill.
			{Slot: build.SlotBodyArmour, Gems: []build.Gem{gem("Vaal Fireball", 20, 0), support("Inspiration"), splitting}, Active: 2},
			{Gems: []build.Gem{disabled(gem("Frostblink", 5, 0))}},
			// The Vaal gem's regular skill is the main one.
			{Slot: build.SlotHelmet, Gems: []build.Gem{gem("Vaal Fireball", 20, 0), gem("Punishment", 1, 0)}, ActiveSkill: 1},
			// The skill Predator grants counts before the main gem, and can be the main skill.
			{Slot: build.SlotBoots, Gems: []build.Gem{support("Predator"), gem("Punishment", 1, 0)}, Active: 1},
			{Gems: []build.Gem{support("Predator"), gem("Frostblink", 1, 0)}, ActiveSkill: 1},
		}, MainGroup: 1},
	}
}

// TestToXMLRoundTrip exports builds to a share code and imports them back unchanged.
func TestToXMLRoundTrip(t *testing.T) {
	t.Parallel()
	reg := exportRegistry(t)
	ruthless := &build.Build{
		Format: build.Format, Game: "poe1", Version: "3.29", Variant: "ruthless", Class: marauder, Level: 1,
		Config: values(t, poe1.BanditsKillAll, poe1.CampaignActs1To5),
	}
	alternate := chieftain(t)
	alternate.Variant, alternate.Config = "alternate", values(t, poe1.BanditsHelpOak, poe1.CampaignComplete)
	bloodline := chieftain(t)
	bloodline.Bloodline, bloodline.Nodes = "Trialmaster", append(bloodline.Nodes, 32730, 17297)
	kraityn := chieftain(t)
	kraityn.Config, kraityn.Skills = values(t, poe1.BanditsHelpKraityn, ""), build.Skills{}
	testCases := []struct {
		name string
		b    *build.Build
	}{
		{"chieftain", chieftain(t)},
		{"ruthless without ascendancy or skills", ruthless},
		{"alternate", alternate},
		{"bloodline with its nodes", bloodline},
		{"campaign stage unset", kraityn},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			file, warnings, err := pob.ToXML(testCase.b, reg)
			if err != nil {
				t.Fatal(err)
			}
			if len(warnings) != 0 {
				t.Errorf("export warnings %v", warnings)
			}
			data, err := pob.DecodeCode(pob.EncodeCode(file))
			if err != nil {
				t.Fatal(err)
			}
			got, warnings, err := pob.FromXML(data, reg)
			if err != nil {
				t.Fatal(err)
			}
			if len(warnings) != 0 {
				t.Errorf("import warnings %v", warnings)
			}
			want := testCase.b.Clone()
			slices.Sort(want.Nodes)
			if diff := cmp.Diff(want, got, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("build (-want +got):\n%s", diff)
			}
		})
	}
}

// TestToXMLSpec checks what FromXML does not read back but PoB needs: the class and
// ascendancy ids and the target version.
func TestToXMLSpec(t *testing.T) {
	t.Parallel()
	file, _, err := pob.ToXML(chieftain(t), exportRegistry(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`<Build level="90" targetVersion="3_0" className="Marauder" ascendClassName="Chieftain" bandit="Alira" mainSocketGroup="2">`,
		`<Spec treeVersion="3_29" classId="1" ascendClassId="3" nodes="1731,4139,11505" masteryEffects="{4139,47642},{11505,36313}">`,
		`<Input name="bandit" string="Alira"></Input>`,
		`<Input name="resistancePenalty" number="-30">`,
		`<Gem nameSpec="Inspiration" gemId="Metadata/Items/Gems/SkillGemSupportInspiration" variantId="Inspiration" skillId="Inspiration" ` +
			`level="20" quality="20" enabled="true"`,
		`<Skill enabled="true" slot="Helmet" mainActiveSkill="2">`,
	} {
		if !strings.Contains(string(file), want) {
			t.Errorf("file lacks %s:\n%s", want, file)
		}
	}
}

func TestToXMLWarnings(t *testing.T) {
	t.Parallel()
	b := chieftain(t)
	b.Version = "3.28"
	b.Config = values(t, poe1.BanditsNotCompleted, "")
	b.Config["power-charges"] = json.RawMessage("3")
	b.Skills.Groups[0].Gems[1].ID = "Metadata/Items/Gems/SkillGemFlameDash"
	b.Items = build.Items{build.SlotRing1: {Rarity: build.Rare, Base: "Ruby Ring"}, build.SlotBelt: {Rarity: build.Normal, Base: "Heavy Belt"}}
	b.Jewels = build.Jewels{26725: {Rarity: build.Rare, Base: "Crimson Jewel"}}
	b.Overrides[2] = "Unnamed1"
	_, warnings, err := pob.ToXML(b, exportRegistry(t))
	if err != nil {
		t.Fatal(err)
	}
	want := []build.Warning{
		{Code: pob.VersionFallback, Subject: "3.28", Message: "no dataset of this version; names written with 3.29.3.5"},
		{Code: pob.OverrideNotExported, Subject: "node 2", Message: `override "Unnamed1" has no English name; not exported`},
		{Code: pob.ConfigNotExported, Subject: "power-charges", Message: "config value not exported"},
		{Code: pob.ConfigNotExported, Subject: "bandits", Message: "PoB has no choice for the bandit quest not completed; its default, kill all, applies"},
		{Code: pob.GemUnknown, Subject: "Metadata/Items/Gems/SkillGemFlameDash", Message: "gem not in dataset; written by id only"},
		{Code: pob.ItemNotExported, Subject: "belt", Message: "items are not exported"},
		{Code: pob.ItemNotExported, Subject: "ring-1", Message: "items are not exported"},
		{Code: pob.ItemNotExported, Subject: "node 26725", Message: "jewels are not exported"},
	}
	if diff := cmp.Diff(want, warnings); diff != "" {
		t.Errorf("warnings (-want +got):\n%s", diff)
	}
}

func TestToXMLBloodline(t *testing.T) {
	t.Parallel()
	reg := exportRegistry(t)
	testCases := []struct {
		bloodline, spec string
		warnings        []build.Warning
	}{
		{"Trialmaster", `ascendClassId="3" secondaryAscendClassId="4" nodes=`, nil},
		{"Warden", `ascendClassId="3" nodes=`, []build.Warning{
			{Code: pob.BloodlineNotExported, Subject: "Warden", Message: "legacy secondary ascendancy, which PoB hides and ignores; not exported"},
		}},
		{"Oshabi", `ascendClassId="3" nodes=`, []build.Warning{
			{Code: pob.BloodlineNotExported, Subject: "Oshabi", Message: `no secondary ascendancy "Oshabi" in the standard tree of dataset 3.29.3.5; not exported`},
		}},
	}
	for _, testCase := range testCases {
		b := chieftain(t)
		b.Bloodline = testCase.bloodline
		file, warnings, err := pob.ToXML(b, reg)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(file), testCase.spec) {
			t.Errorf("%s: file lacks %s:\n%s", testCase.bloodline, testCase.spec, file)
		}
		if diff := cmp.Diff(testCase.warnings, warnings); diff != "" {
			t.Errorf("%s: warnings (-want +got):\n%s", testCase.bloodline, diff)
		}
	}
}

func TestToXMLErrors(t *testing.T) {
	t.Parallel()
	reg := exportRegistry(t)
	testCases := []struct {
		name string
		edit func(*build.Build)
		reg  *dataset.Registry
		want error
	}{
		{"class not in the tree", func(b *build.Build) { b.Class = "Metadata/Characters/Dex/Dex" }, reg, pob.ErrClass},
		{"class without a name", func(b *build.Build) { b.Class, b.Ascendancy = scion, "" }, reg, pob.ErrClass},
		{"ascendancy of another class", func(b *build.Build) { b.Ascendancy = "Ascendant" }, reg, pob.ErrAscendancy},
		{"ascendancy without a name", func(b *build.Build) { b.Ascendancy = "Berserker" }, reg, pob.ErrAscendancy},
		{"game", func(b *build.Build) { b.Game = "poe2" }, reg, pob.ErrGame},
		{"version", func(b *build.Build) { b.Version = "latest" }, reg, pob.ErrVersion},
		{"no dataset", func(*build.Build) {}, &dataset.Registry{}, pob.ErrNoDataset},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			b := chieftain(t)
			testCase.edit(b)
			if _, _, err := pob.ToXML(b, testCase.reg); !errors.Is(err, testCase.want) {
				t.Errorf("got %v, want %v", err, testCase.want)
			}
		})
	}
}
