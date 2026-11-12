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
	// id replaces the id made from the name.
	id      string
	name    string
	support bool
	vaal    bool
	others  []string
	// grants names the active skill a support grants as its second skill, such as Signal Prey.
	grants string
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
			if g.id != "" {
				id = strings.TrimPrefix(g.id, "Metadata/Items/Gems/")
			}
			e := dataset.GemEffect{ID: id, GrantedEffect: id, Style: dataset.DefaultStyle}
			ge := dataset.GrantedEffect{ID: id, Support: g.support}
			if g.vaal {
				e.SecondaryGrantedEffect = id
			}
			if g.grants != "" {
				granted := strings.ReplaceAll(g.grants, " ", "")
				e.SecondaryGrantedEffect = granted
				d.Skills.GrantedEffects = append(d.Skills.GrantedEffects, dataset.GrantedEffect{ID: granted, ActiveSkill: strings.ToLower(granted)})
				d.Skills.ActiveSkills = append(d.Skills.ActiveSkills, dataset.ActiveSkill{ID: strings.ToLower(granted), Types: []uint16{0}})
				d.Texts[0].SkillNames = append(d.Texts[0].SkillNames, dataset.Name{ID: strings.ToLower(granted), Name: g.grants})
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
	if g.id != "" {
		return g.id
	}
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

// skillResolution holds a gem with only a skill id, a name of two gems, and a main group an
// item grants.
const skillResolution = `<PathOfBuilding>
	<Build level="90" className="Marauder" ascendClassName="Chieftain" mainSocketGroup="2"/>
	<Tree activeSpec="1"><Spec treeVersion="3_29" nodes=""/></Tree>
	<Skills>
		<Skill><Gem skillId="FireballofSplitting" level="20"/><Gem nameSpec="Twin" level="1"/></Skill>
		<Skill source="Tree:12345"><Gem nameSpec="Frostblink" level="20"/></Skill>
	</Skills>
</PathOfBuilding>`

func TestFromXMLSkillResolution(t *testing.T) {
	t.Parallel()
	reg := skillsRegistry(t,
		gemSpec{name: "Fireball", others: []string{"Fireball of Splitting"}}, gemSpec{name: "Frostblink"},
		gemSpec{id: "Metadata/Items/Gems/SkillGemTwinA", name: "Twin"}, gemSpec{id: "Metadata/Items/Gems/SkillGemTwinB", name: "Twin"},
	)
	b, warnings, err := pob.FromXML([]byte(skillResolution), reg)
	if err != nil {
		t.Fatal(err)
	}
	splitting := gem("Fireball", 20, 0)
	splitting.Effect = "FireballofSplitting"
	twin := build.Gem{ID: "Metadata/Items/Gems/SkillGemTwinA", Level: 1}
	want := build.Skills{Groups: []build.SkillGroup{{Gems: []build.Gem{splitting, twin}}}}
	if diff := cmp.Diff(want, b.Skills); diff != "" {
		t.Errorf("skills (-want +got):\n%s", diff)
	}
	wantWarnings := []build.Warning{
		{Code: pob.GemAmbiguous, Subject: "Twin", Message: "also the gem of Metadata/Items/Gems/SkillGemTwinB; Metadata/Items/Gems/SkillGemTwinA imported"},
		{Code: pob.SkillIgnored, Subject: "Frostblink", Message: "granted by Tree:12345; skills an item or the tree grants are not imported"},
		{Code: pob.MainSkillIgnored, Subject: "Frostblink", Message: "the main skill group, granted by Tree:12345, is not imported; group 1 used"},
	}
	if diff := cmp.Diff(wantWarnings, warnings); diff != "" {
		t.Errorf("warnings (-want +got):\n%s", diff)
	}
}

// A main group past the last group is the last group; an unset one the first.
func TestFromXMLMainGroupClamped(t *testing.T) {
	t.Parallel()
	reg := skillsRegistry(t, gemSpec{name: "Frostblink"}, gemSpec{name: "Punishment"})
	for main, want := range map[string]int{"9": 1, "": 0, "0": 0} {
		file := strings.Replace(`<PathOfBuilding>
	<Build level="90" className="Marauder" ascendClassName="Chieftain" MAIN/>
	<Tree activeSpec="1"><Spec treeVersion="3_29" nodes=""/></Tree>
	<Skills><Skill><Gem nameSpec="Frostblink" level="1"/></Skill><Skill><Gem nameSpec="Punishment" level="1"/></Skill></Skills>
</PathOfBuilding>`, "MAIN", `mainSocketGroup="`+main+`"`, 1)
		b, warnings, err := pob.FromXML([]byte(file), reg)
		if err != nil || len(warnings) != 0 || b.Skills.MainGroup != want {
			t.Errorf("mainSocketGroup %q: main %d (%v, %v), want %d", main, b.Skills.MainGroup, warnings, err, want)
		}
	}
}

// As in PoB, a main active skill that is not a number, such as poe.ninja's "nil", is the
// group's first, and one past the group's last is its last.
func TestFromXMLMainActiveSkill(t *testing.T) {
	t.Parallel()
	reg := skillsRegistry(t, gemSpec{name: "Frostblink"}, gemSpec{name: "Punishment"})
	for main, want := range map[string]int{"nil": 0, "x": 0, "2": 1, "9": 1} {
		file := strings.Replace(`<PathOfBuilding>
	<Build level="90" className="Marauder" ascendClassName="Chieftain"/>
	<Tree activeSpec="1"><Spec treeVersion="3_29" nodes=""/></Tree>
	<Skills><Skill MAIN><Gem nameSpec="Frostblink" level="1"/><Gem nameSpec="Punishment" level="1"/></Skill></Skills>
</PathOfBuilding>`, "MAIN", `mainActiveSkill="`+main+`"`, 1)
		b, warnings, err := pob.FromXML([]byte(file), reg)
		if err != nil {
			t.Fatalf("mainActiveSkill %q: %v", main, err)
		}
		if len(warnings) != 0 || len(b.Skills.Groups) != 1 || b.Skills.Groups[0].Active != want {
			t.Errorf("mainActiveSkill %q: groups %+v, warnings %v, want active %d", main, b.Skills.Groups, warnings, want)
		}
	}
}

// A Vaal gem's two skills count apart for the main active skill, and the build keeps which of
// them is main; a disabled gem's skills do not count.
func TestFromXMLMainActiveSkillOfVaalGem(t *testing.T) {
	t.Parallel()
	reg := skillsRegistry(t, gemSpec{name: "Vaal Fireball", vaal: true}, gemSpec{name: "Punishment"})
	for main, want := range map[string][2]int{"1": {1, 0}, "2": {1, 1}, "3": {2, 0}} {
		file := strings.Replace(`<PathOfBuilding>
	<Build level="90" className="Marauder" ascendClassName="Chieftain"/>
	<Tree activeSpec="1"><Spec treeVersion="3_29" nodes=""/></Tree>
	<Skills><Skill MAIN>
		<Gem nameSpec="Punishment" level="1" enabled="false"/><Gem nameSpec="Vaal Fireball" level="1"/><Gem nameSpec="Punishment" level="1"/>
	</Skill></Skills>
</PathOfBuilding>`, "MAIN", `mainActiveSkill="`+main+`"`, 1)
		b, _, err := pob.FromXML([]byte(file), reg)
		if err != nil {
			t.Fatalf("mainActiveSkill %q: %v", main, err)
		}
		if g := b.Skills.Groups[0]; g.Active != want[0] || g.ActiveSkill != want[1] {
			t.Errorf("mainActiveSkill %q: gem %d skill %d, want gem %d skill %d", main, g.Active, g.ActiveSkill, want[0], want[1])
		}
	}
}

// A Vaal gem's skill that is switched off does not count, as in PoB: main active skill 1 is the
// regular skill when the Vaal skill is off.
func TestFromXMLMainActiveSkillSwitchedOff(t *testing.T) {
	t.Parallel()
	reg := skillsRegistry(t, gemSpec{name: "Vaal Fireball", vaal: true})
	file := `<PathOfBuilding>
	<Build level="90" className="Marauder" ascendClassName="Chieftain"/>
	<Tree activeSpec="1"><Spec treeVersion="3_29" nodes=""/></Tree>
	<Skills><Skill mainActiveSkill="1"><Gem nameSpec="Vaal Fireball" level="1" enableGlobal1="false" enableGlobal2="true"/></Skill></Skills>
</PathOfBuilding>`
	b, _, err := pob.FromXML([]byte(file), reg)
	if err != nil {
		t.Fatal(err)
	}
	if g := b.Skills.Groups[0]; g.Active != 0 || g.ActiveSkill != 1 {
		t.Errorf("gem %d skill %d, want gem 0 skill 1", g.Active, g.ActiveSkill)
	}
}

// An active skill a support grants counts for the main active skill in gem order, as in PoB,
// and the build keeps it as the support's second skill.
func TestFromXMLMainActiveSkillOfSupport(t *testing.T) {
	t.Parallel()
	reg := skillsRegistry(t, gemSpec{name: "Predator", support: true, grants: "Signal Prey"}, gemSpec{name: "Punishment"})
	for main, want := range map[string][2]int{"1": {0, 1}, "2": {1, 0}} {
		file := strings.Replace(`<PathOfBuilding>
	<Build level="90" className="Marauder" ascendClassName="Chieftain"/>
	<Tree activeSpec="1"><Spec treeVersion="3_29" nodes=""/></Tree>
	<Skills><Skill MAIN><Gem nameSpec="Predator" level="20"/><Gem nameSpec="Punishment" level="1"/></Skill></Skills>
</PathOfBuilding>`, "MAIN", `mainActiveSkill="`+main+`"`, 1)
		b, warnings, err := pob.FromXML([]byte(file), reg)
		if err != nil || len(warnings) != 0 {
			t.Fatalf("mainActiveSkill %q: %v, %v", main, warnings, err)
		}
		if g := b.Skills.Groups[0]; g.Active != want[0] || g.ActiveSkill != want[1] {
			t.Errorf("mainActiveSkill %q: gem %d skill %d, want gem %d skill %d", main, g.Active, g.ActiveSkill, want[0], want[1])
		}
	}
}
