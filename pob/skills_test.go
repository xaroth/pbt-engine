package pob_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/pob"
)

// gemSpec is a gem of skillsRegistry: its name, whether it is a support, and its effects after
// the first, which is named as the gem.
type gemSpec struct {
	name    string
	support bool
	vaal    bool
	others  []string
}

// skillsRegistry is registry with a skills group of gems, each with one granted effect and
// active skill per effect, named in English. A gem's id is its name without spaces after
// Metadata/Items/Gems/SkillGem, plus Support for supports; an effect's id is its name without
// spaces.
func skillsRegistry(t *testing.T, gems ...gemSpec) *dataset.Registry {
	t.Helper()
	d := testDataset()
	d.Skills = &dataset.Skills{Types: []string{"Spell"}}
	for _, g := range gems {
		dg := dataset.Gem{ID: gemID(g), Colour: dataset.Red, Support: g.support, Vaal: g.vaal}
		for _, name := range append([]string{g.name}, g.others...) {
			id := strings.ReplaceAll(name, " ", "")
			e := dataset.GemEffect{ID: id, GrantedEffect: id, Style: dataset.DefaultStyle}
			ge := dataset.GrantedEffect{ID: id, Support: g.support}
			if g.vaal {
				e.SecondaryGrantedEffect = id
			}
			if !g.support {
				ge.ActiveSkill = strings.ToLower(id)
				d.Skills.ActiveSkills = append(d.Skills.ActiveSkills, dataset.ActiveSkill{ID: ge.ActiveSkill, Types: []uint16{0}})
				d.Texts[0].SkillNames = append(d.Texts[0].SkillNames, dataset.Name{ID: ge.ActiveSkill, Name: name})
			}
			dg.Effects = append(dg.Effects, e)
			d.Skills.GrantedEffects = append(d.Skills.GrantedEffects, ge)
		}
		name := g.name
		if g.support {
			name += " Support"
		}
		d.Skills.Gems = append(d.Skills.Gems, dg)
		d.Texts[0].GemNames = append(d.Texts[0].GemNames, dataset.Name{ID: dg.ID, Name: name})
	}
	return registryOf(t, d)
}

func gemID(g gemSpec) string {
	id := "Metadata/Items/Gems/SkillGem"
	if g.support {
		id += "Support"
	}
	return id + strings.ReplaceAll(g.name, " ", "")
}

func gem(name string, level, quality int) build.Gem {
	return build.Gem{ID: gemID(gemSpec{name: name}), Level: level, Quality: quality}
}

// support is a 20/20 support gem.
func support(name string) build.Gem {
	return build.Gem{ID: gemID(gemSpec{name: name, support: true}), Level: 20, Quality: 20, Support: true}
}

// TestFromXMLSkills imports the skill groups of a real build file.
func TestFromXMLSkills(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile(filepath.Join("testdata", "righteous-fire-skills.xml"))
	if err != nil {
		t.Fatal(err)
	}
	reg := skillsRegistry(t,
		gemSpec{name: "Righteous Fire"}, gemSpec{name: "Swift Affliction", support: true}, gemSpec{name: "Elemental Focus", support: true},
		gemSpec{name: "Burning Damage", support: true}, gemSpec{name: "Malevolence"}, gemSpec{name: "Eternal Blessing", support: true},
		gemSpec{name: "Blood Rage"}, gemSpec{name: "Shield Charge"}, gemSpec{name: "Frostblink"}, gemSpec{name: "Punishment"},
		gemSpec{name: "Enduring Cry"},
	)
	b, warnings, err := pob.FromXML(data, reg)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 0 {
		t.Errorf("warnings %v", warnings)
	}
	want := build.Skills{Groups: []build.SkillGroup{
		{Slot: build.SlotHelmet, Gems: []build.Gem{
			gem("Righteous Fire", 20, 20), support("Swift Affliction"), support("Elemental Focus"), support("Burning Damage"),
		}},
		{Slot: build.SlotGloves, Gems: []build.Gem{gem("Malevolence", 20, 20), support("Eternal Blessing"), gem("Blood Rage", 11, 0)}},
		{Slot: build.SlotBoots, Gems: []build.Gem{gem("Shield Charge", 20, 20), gem("Frostblink", 20, 20)}},
		{Slot: build.SlotWeapon, Gems: []build.Gem{gem("Punishment", 20, 20)}},
		{Slot: build.SlotOffhand, Gems: []build.Gem{gem("Enduring Cry", 20, 20)}},
	}}
	if diff := cmp.Diff(want, b.Skills); diff != "" {
		t.Errorf("skills (-want +got):\n%s", diff)
	}
}

// skillCases holds what the real file does not: gems by name only, a transfigured gem by its
// variant and by its name, a Vaal gem whose skills count twice for the main active skill and
// one of which is switched off, a disabled group, a disabled gem before the group's first
// enabled active gem, an unknown gem, a slot without sockets, a group an item grants, and the
// main group after it.
const skillCases = `<PathOfBuilding>
	<Build level="90" className="Marauder" ascendClassName="Chieftain" mainSocketGroup="4"/>
	<Tree activeSpec="1"><Spec treeVersion="3_29" nodes=""/></Tree>
	<Skills activeSkillSet="2">
		<SkillSet id="1"><Skill><Gem nameSpec="Punishment"/></Skill></SkillSet>
		<SkillSet id="2">
			<Skill label="main" slot="Body Armour" mainActiveSkill="3">
				<Gem nameSpec="Vaal Fireball" gemId="Metadata/Items/Gems/SkillGemVaalFireball" level="20" enableGlobal1="false"/>
				<Gem nameSpec="Inspiration" level="20" quality="20"/>
				<Gem nameSpec="Fireball of Splitting" gemId="Metadata/Items/Gems/SkillGemFireball" skillId="FireballofSplitting" level="21" quality="23"/>
				<Gem nameSpec="Firestorm of Pelting" level="1"/>
			</Skill>
			<Skill enabled="false" slot="Amulet"><Gem nameSpec="Frostblink" level="5"/><Gem nameSpec="Blink Arrow" level="5"/></Skill>
			<Skill source="Item:3:Doom Crown"><Gem nameSpec="Envy" level="15"/></Skill>
			<Skill slot="Gloves"><Gem nameSpec="Punishment" level="20" enabled="false"/><Gem nameSpec="Fireball of Splitting" level="2"/></Skill>
		</SkillSet>
	</Skills>
</PathOfBuilding>`

func TestFromXMLSkillCases(t *testing.T) {
	t.Parallel()
	reg := skillsRegistry(t,
		gemSpec{name: "Fireball", others: []string{"Fireball of Splitting"}}, gemSpec{name: "Vaal Fireball", vaal: true},
		gemSpec{name: "Inspiration", support: true}, gemSpec{name: "Frostblink"}, gemSpec{name: "Punishment"},
	)
	b, warnings, err := pob.FromXML([]byte(skillCases), reg)
	if err != nil {
		t.Fatal(err)
	}
	splitting := gem("Fireball", 21, 23)
	splitting.Effect = "FireballofSplitting"
	splitting2 := gem("Fireball", 2, 0)
	splitting2.Effect = "FireballofSplitting"
	disabled := func(g build.Gem) build.Gem { g.Disabled = true; return g }
	want := build.Skills{Groups: []build.SkillGroup{
		{Slot: build.SlotBodyArmour, Gems: []build.Gem{gem("Vaal Fireball", 20, 0), support("Inspiration"), splitting}, Active: 2},
		{Gems: []build.Gem{disabled(gem("Frostblink", 5, 0))}},
		{Slot: build.SlotGloves, Gems: []build.Gem{disabled(gem("Punishment", 20, 0)), splitting2}, Active: 1},
	}, MainGroup: 2}
	if diff := cmp.Diff(want, b.Skills); diff != "" {
		t.Errorf("skills (-want +got):\n%s", diff)
	}
	wantWarnings := []build.Warning{
		{Code: pob.GemToggleIgnored, Subject: "Vaal Fireball", Message: "one of the Vaal gem's two skills is switched off; the switch is not imported"},
		{Code: pob.GemUnknown, Subject: "Firestorm of Pelting", Message: "gem not in dataset 3.29.3.5; dropped"},
		{Code: pob.SlotIgnored, Subject: "Frostblink", Message: `slot "Amulet" has no sockets; group imported without a slot`},
		{Code: pob.GemUnknown, Subject: "Blink Arrow", Message: "gem not in dataset 3.29.3.5; dropped"},
		{Code: pob.SkillIgnored, Subject: "Envy", Message: "granted by Item:3:Doom Crown; skills an item or the tree grants are not imported"},
	}
	if diff := cmp.Diff(wantWarnings, warnings); diff != "" {
		t.Errorf("warnings (-want +got):\n%s", diff)
	}
}
