package pob_test

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/config"
	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/internal/poe1"
	"github.com/xaroth/pbt-engine/pob"
)

const marauder = "Metadata/Characters/Str/Str"

// registry holds one poe1 dataset, 3.29.3.5, with the names FromXML reads: Marauder, and
// Chieftain as named in the standard and alternate trees.
func registry(t *testing.T) *dataset.Registry {
	t.Helper()
	d := &dataset.Dataset{
		Manifest:   dataset.Manifest{Game: "poe1", Version: "3.29.3.5"},
		Characters: []dataset.Character{{ID: marauder}},
		Trees:      []dataset.Tree{tree("standard"), tree("ruthless"), tree("alternate")},
		Texts: []dataset.Text{{
			Language:       "en",
			CharacterNames: []dataset.Name{{ID: marauder, Name: "Marauder"}},
			AscendancyNames: []dataset.AscendancyName{
				{Variant: "standard", ID: "Chieftain", Name: "Chieftain"},
				{Variant: "ruthless", ID: "Chieftain", Name: "Chieftain"},
				{Variant: "alternate", ID: "Chieftain", Name: "Antiquarian"},
			},
		}},
	}
	dir := t.TempDir()
	if _, err := dataset.Write(dir, d); err != nil {
		t.Fatal(err)
	}
	var reg dataset.Registry
	if _, err := reg.Add(os.DirFS(dir)); err != nil {
		t.Fatal(err)
	}
	return &reg
}

func tree(variant string) dataset.Tree {
	return dataset.Tree{
		Variant: variant,
		Classes: []dataset.Class{{Character: marauder, StartNode: 1, Ascendancies: []dataset.Ascendancy{{ID: "Chieftain"}}}},
		Nodes:   []dataset.Node{{ID: 1, Kind: dataset.ClassStart}},
	}
}

func values(t *testing.T, bandits poe1.BanditChoice, stage poe1.CampaignStage) config.Values {
	t.Helper()
	var v config.Values
	if err := config.Set(&v, poe1.Bandits, bandits); err != nil {
		t.Fatal(err)
	}
	if stage != "" {
		if err := config.Set(&v, poe1.Campaign, stage); err != nil {
			t.Fatal(err)
		}
	}
	return v
}

// The plan's example: one of everything FromXML leaves out.
const example = `<PathOfBuilding>
	<Build level="90" className="Marauder" ascendClassName="Chieftain" bandit="None" pantheonMajorGod="Arakaali" pantheonMinorGod="None"/>
	<Tree activeSpec="2">
		<Spec title="Leveling" treeVersion="3_29" nodes="4139"/>
		<Spec treeVersion="3_29" secondaryAscendClassId="2" nodes="4139,11505,65601,65602" masteryEffects="{4139,47642}">
			<Sockets><Socket nodeId="26725" itemId="3"/><Socket nodeId="61419" itemId="0"/></Sockets>
			<Overrides><Override nodeId="5152" dn="Tattoo of the Tasalio Shaman"/></Overrides>
		</Spec>
	</Tree>
	<Config activeConfigSet="1"><ConfigSet id="1">
		<Input name="resistancePenalty" number="-30"/>
		<Input name="usePowerCharges" boolean="true"/>
	</ConfigSet></Config>
	<Items>
		<Item id="1">Rarity: RARE
Doom Crown
Royal Burgonet</Item>
		<Item id="2">Rarity: UNIQUE
Kaom's Heart
Glorious Plate</Item>
		<Item id="3">Rarity: RARE
Corruption Spark
Crimson Jewel</Item>
	</Items>
	<Skills><SkillSet id="1"><Skill label="RF"><Gem nameSpec="Righteous Fire"/></Skill><Skill><Gem nameSpec="Flame Dash"/></Skill></SkillSet></Skills>
</PathOfBuilding>`

func TestFromXML(t *testing.T) {
	t.Parallel()
	b, warnings, err := pob.FromXML([]byte(example), registry(t))
	if err != nil {
		t.Fatal(err)
	}
	want := &build.Build{
		Format: build.Format, Game: "poe1", Version: "3.29", Variant: "standard",
		Class: marauder, Ascendancy: "Chieftain", Level: 90,
		Nodes: []uint32{4139, 11505}, Masteries: map[uint32]uint32{4139: 47642},
		Config: values(t, poe1.BanditsKillAll, poe1.CampaignActs6To10),
	}
	if diff := cmp.Diff(want, b); diff != "" {
		t.Errorf("build (-want +got):\n%s", diff)
	}
	wantWarnings := []build.Warning{
		{Code: pob.ClusterNodeDropped, Subject: "nodes 65601,65602", Message: "2 cluster jewel nodes dropped; cluster jewels are not imported"},
		{Code: pob.ConfigIgnored, Subject: "usePowerCharges", Message: "config input not modelled (true)"},
		{Code: pob.SpecIgnored, Subject: "Leveling", Message: "only the active tree is imported"},
		{Code: pob.AscendancyIgnored, Subject: "2", Message: "secondary ascendancy not imported"},
		{Code: pob.PantheonIgnored, Subject: "Arakaali", Message: "pantheon not imported"},
		{Code: pob.OverrideIgnored, Subject: "node 5152", Message: `node override "Tattoo of the Tasalio Shaman" not imported`},
		{Code: pob.ItemIgnored, Subject: "Doom Crown", Message: "items are not imported"},
		{Code: pob.ItemIgnored, Subject: "Kaom's Heart", Message: "items are not imported"},
		{Code: pob.JewelIgnored, Subject: "node 26725", Message: `jewel "Corruption Spark" not imported`},
		{Code: pob.SkillIgnored, Subject: "RF", Message: "skills are not imported (Righteous Fire)"},
		{Code: pob.SkillIgnored, Subject: "Flame Dash", Message: "skills are not imported (Flame Dash)"},
	}
	if diff := cmp.Diff(wantWarnings, warnings); diff != "" {
		t.Errorf("warnings (-want +got):\n%s", diff)
	}
}

func fromXML(t *testing.T, buildAttrs, specAttrs, config string) (*build.Build, []build.Warning) {
	t.Helper()
	b, w, err := pob.FromXML([]byte(xmlOf(buildAttrs, specAttrs, config)), registry(t))
	if err != nil {
		t.Fatal(err)
	}
	return b, w
}

func xmlOf(buildAttrs, specAttrs, config string) string {
	return `<PathOfBuilding><Build ` + buildAttrs + `/><Tree><Spec ` + specAttrs + `/></Tree><Config>` + config + `</Config></PathOfBuilding>`
}

func codes(w []build.Warning) []build.Code {
	out := make([]build.Code, 0, len(w))
	for _, x := range w {
		out = append(out, x.Code)
	}
	return out
}

func TestFromXMLVariants(t *testing.T) {
	t.Parallel()
	for _, testCase := range []struct{ tree, ascendancy, variant string }{
		{"3_29", "Chieftain", "standard"},
		{"3_29_ruthless", "Chieftain", "ruthless"},
		{"3_29_alternate", "Antiquarian", "alternate"},
	} {
		b, w := fromXML(t, `className="Marauder" ascendClassName="`+testCase.ascendancy+`"`, `treeVersion="`+testCase.tree+`"`, "")
		if b.Version != "3.29" || b.Variant != testCase.variant || b.Ascendancy != "Chieftain" || len(w) != 0 {
			t.Errorf("%s: version %s, variant %s, ascendancy %s, warnings %v", testCase.tree, b.Version, b.Variant, b.Ascendancy, w)
		}
	}
	b, _ := fromXML(t, `className="Marauder" ascendClassName="None"`, `treeVersion="3_29"`, "")
	if b.Ascendancy != "" {
		t.Errorf("ascendancy None: got %q", b.Ascendancy)
	}
}

// A version without a dataset reads names with the newest dataset and keeps its own version.
func TestFromXMLVersionFallback(t *testing.T) {
	t.Parallel()
	b, w := fromXML(t, `className="Marauder"`, `treeVersion="3_10"`, "")
	if b.Version != "3.10" || b.Class != marauder {
		t.Errorf("got version %s, class %s", b.Version, b.Class)
	}
	want := build.Warning{Code: pob.VersionFallback, Subject: "3.10", Message: "no dataset of this version; class names read with 3.29.3.5"}
	if len(w) != 1 || w[0] != want {
		t.Errorf("warnings %v, want %v", w, want)
	}
}

func TestFromXMLConfig(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name, bandit, config string
		bandits              poe1.BanditChoice
		stage                poe1.CampaignStage
		warnings             []build.Code
	}{
		{"defaults", "", "", poe1.BanditsKillAll, "", nil},
		{"help oak, acts 1 to 5", `bandit="Oak"`, `<Input name="resistancePenalty" number="0"/>`, poe1.BanditsHelpOak, poe1.CampaignActs1To5, nil},
		{"complete, flat inputs", `bandit="Alira"`, `<Input name="resistancePenalty" number="-60"/>`, poe1.BanditsHelpAlira, poe1.CampaignComplete, nil},
		{"eramir, mirrored inputs", `bandit="Eramir"`, `<Input name="bandit" string="Eramir"/><Input name="pantheonMajorGod" string="Lunaris"/>`,
			poe1.BanditsKillAll, "", nil},
		{"unknown penalty", `bandit="Kraityn"`, `<Input name="resistancePenalty" number="-45"/>`, poe1.BanditsHelpKraityn, "", []build.Code{pob.ConfigIgnored}},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			b, w := fromXML(t, `className="Marauder" `+testCase.bandit, `treeVersion="3_29"`, testCase.config)
			if diff := cmp.Diff(values(t, testCase.bandits, testCase.stage), b.Config); diff != "" {
				t.Errorf("config (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(testCase.warnings, codes(w), cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("warnings (-want +got):\n%s", diff)
			}
		})
	}
	b, w := fromXML(t, `className="Marauder" bandit="Kitava"`, `treeVersion="3_29"`, "")
	if _, ok := b.Config[string(poe1.Bandits)]; ok || len(w) != 1 || w[0].Code != pob.ConfigIgnored || w[0].Subject != "bandit" {
		t.Errorf("unknown bandit: config %v, warnings %v", b.Config, w)
	}
}

func TestFromXMLErrors(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name, xml string
		want      error
	}{
		{"not xml", "eNrtfWlz2zq", pob.ErrNotPoB},
		{"no spec", "<PathOfBuilding><Build/><Tree/></PathOfBuilding>", pob.ErrNotPoB},
		{"poe2", "<PathOfBuilding2><Build/></PathOfBuilding2>", pob.ErrPoE2},
		{"tree version", xmlOf(`className="Marauder"`, `treeVersion="3.29"`, ""), pob.ErrTreeVersion},
		{"class", xmlOf(`className="Exile"`, `treeVersion="3_29"`, ""), pob.ErrClass},
		{"ascendancy", xmlOf(`className="Marauder" ascendClassName="Antiquarian"`, `treeVersion="3_29"`, ""), pob.ErrAscendancy},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			if _, _, err := pob.FromXML([]byte(testCase.xml), registry(t)); !errors.Is(err, testCase.want) {
				t.Errorf("got %v, want %v", err, testCase.want)
			}
		})
	}
	_, _, err := pob.FromXML([]byte(xmlOf(`className="Marauder"`, `treeVersion="3_29"`, "")), &dataset.Registry{})
	if !errors.Is(err, pob.ErrNoDataset) || !strings.Contains(err.Error(), "3.29") {
		t.Errorf("empty registry: got %v", err)
	}
}
