package pobxml_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/xaroth/pbt-engine/pob/internal/pobxml"
)

// full holds every element Parse reads, as PoB writes them.
const full = `<?xml version="1.0" encoding="UTF-8"?>
<PathOfBuilding>
	<Build level="90" targetVersion="3_0" className="Marauder" ascendClassName="Chieftain" bandit="Alira"
		pantheonMajorGod="Arakaali" pantheonMinorGod="None" mainSocketGroup="1"/>
	<Tree activeSpec="2">
		<Spec title="Leveling" treeVersion="3_28" classId="1" ascendClassId="0" nodes="" masteryEffects=""/>
		<Spec treeVersion="3_29" classId="1" ascendClassId="3" secondaryAscendClassId="0" nodes="4139,11505,65601,4139" masteryEffects="{4139,47642},{11505,36313}">
			<URL>https://www.pathofexile.com/passive-skill-tree/AAAA</URL>
			<Sockets>
				<Socket nodeId="26725" itemId="3"/>
				<Socket nodeId="61419" itemId="0"/>
			</Sockets>
			<Overrides>
				<Override nodeId="5152" dn="Tattoo of the Tasalio Shaman">+4 to Strength</Override>
			</Overrides>
		</Spec>
	</Tree>
	<Config activeConfigSet="2">
		<ConfigSet id="1" title="Default"><Input name="resistancePenalty" number="0"/></ConfigSet>
		<ConfigSet id="2" title="Mapping">
			<Input name="resistancePenalty" number="-60"/>
			<Input name="usePowerCharges" boolean="true"/>
			<Input name="enemyIsBoss" string="Pinnacle"/>
			<Placeholder name="enemyLevel" number="84"/>
		</ConfigSet>
	</Config>
	<Items activeItemSet="1">
		<Item id="1">
			Rarity: RARE
			Doom Crown
			Royal Burgonet
		</Item>
		<Item id="2">Rarity: MAGIC
Seething Divine Life Flask of Staunching</Item>
		<Item id="3">
Rarity: RARE
Corruption Spark
Crimson Jewel
</Item>
		<ItemSet id="1"><Slot name="Helmet" itemId="1"/></ItemSet>
	</Items>
	<Skills activeSkillSet="1">
		<SkillSet id="1">
			<Skill label="RF" enabled="true" slot="Body Armour"><Gem nameSpec="Righteous Fire"/><Gem nameSpec="Burning Damage"/></Skill>
			<Skill label="" enabled="false"><Gem nameSpec="Flame Dash"/></Skill>
		</SkillSet>
	</Skills>
	<Notes>notes</Notes>
	<TreeView zoomLevel="3"/>
</PathOfBuilding>`

func TestParse(t *testing.T) {
	t.Parallel()
	f, err := pobxml.Parse([]byte(full))
	if err != nil {
		t.Fatal(err)
	}
	want := &pobxml.File{
		Build: pobxml.Build{Level: 90, Class: "Marauder", Ascendancy: "Chieftain", Bandit: "Alira", PantheonMajor: "Arakaali", PantheonMinor: "None"},
		Specs: []pobxml.Spec{
			{Title: "Leveling", TreeVersion: "3_28", ClassID: 1},
			{
				TreeVersion: "3_29", ClassID: 1, AscendClassID: 3,
				Nodes:     []uint32{4139, 11505, 65601},
				Masteries: []pobxml.Mastery{{4139, 47642}, {11505, 36313}},
				Sockets:   []pobxml.Socket{{26725, 3}, {61419, 0}},
				Overrides: []pobxml.Override{{5152, "Tattoo of the Tasalio Shaman"}},
			},
		},
		Active: 1,
		Config: []pobxml.Input{
			{"resistancePenalty", "number", "-60"},
			{"usePowerCharges", "boolean", "true"},
			{"enemyIsBoss", "string", "Pinnacle"},
		},
		Items: []pobxml.Item{{1, "Doom Crown"}, {2, "Seething Divine Life Flask of Staunching"}, {3, "Corruption Spark"}},
		Skills: []pobxml.SkillGroup{
			{Label: "RF", Enabled: true, Gems: []string{"Righteous Fire", "Burning Damage"}},
			{Enabled: false, Gems: []string{"Flame Dash"}},
		},
	}
	if !reflect.DeepEqual(f, want) {
		t.Errorf("got\n%+v\nwant\n%+v", f, want)
	}
}

// Older files keep config inputs and skill groups without sets, and may omit activeSpec.
func TestParseWithoutSets(t *testing.T) {
	t.Parallel()
	f, err := pobxml.Parse([]byte(`<PathOfBuilding>
		<Build level="12" className="Witch" ascendClassName="None"/>
		<Tree><Spec treeVersion="3_10" nodes="1,2"/></Tree>
		<Config><Input name="resistancePenalty" number="-30"/></Config>
		<Skills><Skill><Gem nameSpec="Fireball"/></Skill></Skills>
	</PathOfBuilding>`))
	if err != nil {
		t.Fatal(err)
	}
	if f.Active != 0 || !reflect.DeepEqual(f.Specs[0].Nodes, []uint32{1, 2}) {
		t.Errorf("spec: active %d, %+v", f.Active, f.Specs)
	}
	if want := []pobxml.Input{{"resistancePenalty", "number", "-30"}}; !reflect.DeepEqual(f.Config, want) {
		t.Errorf("config %+v, want %+v", f.Config, want)
	}
	if want := []pobxml.SkillGroup{{Enabled: true, Gems: []string{"Fireball"}}}; !reflect.DeepEqual(f.Skills, want) {
		t.Errorf("skills %+v, want %+v", f.Skills, want)
	}
}

// Without an active config set, the first set applies.
func TestParseConfigFirstSet(t *testing.T) {
	t.Parallel()
	f, err := pobxml.Parse([]byte(`<PathOfBuilding><Build/><Tree><Spec/></Tree>
		<Config><ConfigSet id="7"><Input name="a" number="1"/></ConfigSet><ConfigSet id="8"/></Config>
	</PathOfBuilding>`))
	if err != nil {
		t.Fatal(err)
	}
	if want := []pobxml.Input{{"a", "number", "1"}}; !reflect.DeepEqual(f.Config, want) {
		t.Errorf("config %+v, want %+v", f.Config, want)
	}
}

func TestParseErrors(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name, xml string
		want      error
	}{
		{"not xml", "eNrtfWlz2zq", pobxml.ErrNotPoB},
		{"other root", "<Build/>", pobxml.ErrNotPoB},
		{"poe2", "<PathOfBuilding2><Build/></PathOfBuilding2>", pobxml.ErrPoE2},
		{"no build", "<PathOfBuilding><Tree><Spec/></Tree></PathOfBuilding>", pobxml.ErrMissing},
		{"no tree", "<PathOfBuilding><Build/></PathOfBuilding>", pobxml.ErrMissing},
		{"no spec", "<PathOfBuilding><Build/><Tree/></PathOfBuilding>", pobxml.ErrMissing},
		{"active spec past the end", `<PathOfBuilding><Build/><Tree activeSpec="2"><Spec/></Tree></PathOfBuilding>`, pobxml.ErrMissing},
		{"bad node", `<PathOfBuilding><Build/><Tree><Spec nodes="1,x"/></Tree></PathOfBuilding>`, pobxml.ErrMalformed},
		{"bad mastery", `<PathOfBuilding><Build/><Tree><Spec masteryEffects="{1}"/></Tree></PathOfBuilding>`, pobxml.ErrMalformed},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			if _, err := pobxml.Parse([]byte(testCase.xml)); !errors.Is(err, testCase.want) {
				t.Errorf("got %v, want %v", err, testCase.want)
			}
		})
	}
}
