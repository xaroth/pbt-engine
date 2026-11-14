package pob_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/config"
	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/internal/poe1"
	"github.com/xaroth/pbt-engine/internal/realdata"
	"github.com/xaroth/pbt-engine/pob"
)

const (
	marauder = "Metadata/Characters/Str/Str"
	scion    = "Metadata/Characters/StrDexInt/StrDexInt"
)

// registry holds one poe1 dataset, 3.29.3.5, with the names FromXML reads: Marauder,
// Chieftain as named in the standard and alternate trees, a tattoo, a runegraft, and two
// overrides that share a name. Its trees list the legacy bloodlines and Trialmaster, fourth as
// in the game's trees.
func registry(t *testing.T) *dataset.Registry {
	t.Helper()
	return registryOf(t, testDataset())
}

func testDataset() *dataset.Dataset {
	return &dataset.Dataset{
		Manifest:   dataset.Manifest{Game: "poe1", Version: "3.29.3.5"},
		Characters: []dataset.Character{{ID: scion}, {ID: marauder}},
		Trees:      []dataset.Tree{tree("standard"), tree("ruthless"), tree("alternate")},
		Texts: []dataset.Text{{
			Language:       "en",
			CharacterNames: []dataset.Name{{ID: marauder, Name: "Marauder"}},
			AscendancyNames: []dataset.AscendancyName{
				{Variant: "standard", ID: "Chieftain", Name: "Chieftain"},
				{Variant: "ruthless", ID: "Chieftain", Name: "Chieftain"},
				{Variant: "alternate", ID: "Chieftain", Name: "Antiquarian"},
			},
			OverrideNames: []dataset.Name{
				{ID: "Twin1", Name: "Tattoo of the Twins"}, {ID: "Twin2", Name: "Tattoo of the Twins"},
				{ID: "Valako4", Name: "Tattoo of the Valako Shieldbearer"}, {ID: "runegraft_8", Name: "Runegraft of the Warp"},
			},
		}},
	}
}

func registryOf(t *testing.T, d *dataset.Dataset) *dataset.Registry {
	t.Helper()
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

// tree has the Marauder second and the Chieftain third, as PoB numbers them in classId and
// ascendClassId.
func tree(variant string) dataset.Tree {
	return dataset.Tree{
		Variant: variant,
		Classes: []dataset.Class{
			{Character: scion, StartNode: 2},
			{Character: marauder, StartNode: 1, Ascendancies: []dataset.Ascendancy{{ID: "Juggernaut"}, {ID: "Berserker"}, {ID: "Chieftain"}}},
		},
		AlternateAscendancies: []dataset.Ascendancy{{ID: "Warden"}, {ID: "Warlock"}, {ID: "Primalist"}, {ID: "Trialmaster"}},
		Nodes:                 []dataset.Node{{ID: 1, Kind: dataset.ClassStart}, {ID: 2, Kind: dataset.ClassStart}},
		Overrides: []dataset.NodeOverride{
			{ID: "Twin1", Icon: "twin.png"}, {ID: "Twin2", Icon: "twin.png"},
			{ID: "Valako4", Icon: "shield.png", Target: dataset.SmallAttribute},
			{ID: "runegraft_8", Kind: dataset.Runegraft, Icon: "warp.png", Target: dataset.MasteryTarget},
		},
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

// example holds one of everything FromXML leaves out.
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
		<Item id="4">Rarity: MAGIC
Seething Divine Life Flask of Staunching</Item>
		<ItemSet id="1"><Slot name="Body Armour" itemId="2"/><Slot name="Flask 1" itemId="4"/></ItemSet>
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
		Nodes: []uint32{4139, 11505, 65601, 65602}, Masteries: map[uint32]uint32{4139: 47642},
		Config: values(t, poe1.BanditsKillAll, poe1.CampaignActs6To10),
	}
	if diff := cmp.Diff(want, b); diff != "" {
		t.Errorf("build (-want +got):\n%s", diff)
	}
	wantWarnings := []build.Warning{
		{Code: pob.AscendancyIgnored, Subject: "Warlock", Message: "legacy secondary ascendancy 2, which PoB hides and ignores; not imported"},
		{Code: pob.OverrideIgnored, Subject: "node 5152", Message: `no override named "Tattoo of the Tasalio Shaman" in dataset 3.29.3.5; not imported`},
		{Code: pob.ConfigIgnored, Subject: "usePowerCharges", Message: "config input not modelled (true)"},
		{Code: pob.SkillIgnored, Subject: "RF", Message: "dataset 3.29.3.5 has no skills group to read gems with; not imported"},
		{Code: pob.SkillIgnored, Subject: "Flame Dash", Message: "dataset 3.29.3.5 has no skills group to read gems with; not imported"},
		{Code: pob.SpecIgnored, Subject: "Leveling", Message: "only the active tree is imported"},
		{Code: pob.PantheonIgnored, Subject: "Arakaali", Message: "pantheon not imported"},
		{Code: pob.ItemIgnored, Subject: "Doom Crown", Message: "not equipped in the active item set; not imported"},
		{Code: pob.ItemIgnored, Subject: "Kaom's Heart", Message: "not read: items: dataset 3.29.3.5 has no items group; not imported"},
		{Code: pob.JewelIgnored, Subject: "node 26725", Message: `jewel "Corruption Spark" not read: items: dataset 3.29.3.5 has no items group; not imported`},
		{Code: pob.ItemIgnored, Subject: "Seething Divine Life Flask of Staunching", Message: `slot "Flask 1" is not an equipment slot; not imported`},
	}
	if diff := cmp.Diff(wantWarnings, warnings); diff != "" {
		t.Errorf("warnings (-want +got):\n%s", diff)
	}
}

// TestFromXMLBloodlineReal imports a 3.29 Gladiator with the Trialmaster bloodline, PoB's
// fourth secondary ascendancy, and two of its nodes, on the pinned dataset, and exports it back.
func TestFromXMLBloodlineReal(t *testing.T) {
	t.Parallel()
	reg := &dataset.Registry{}
	if _, err := reg.Add(realdata.Record(t, realdata.Current)); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join("testdata", "trialmaster-bloodline.xml"))
	if err != nil {
		t.Fatal(err)
	}
	b, warnings, err := pob.FromXML(data, reg)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) > 0 {
		t.Errorf("warnings %v", warnings)
	}
	if b.Bloodline != "Trialmaster" {
		t.Errorf("bloodline %q, want Trialmaster", b.Bloodline)
	}
	file, warnings, err := pob.ToXML(b, reg)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) > 0 {
		t.Errorf("export warnings %v", warnings)
	}
	back, _, err := pob.FromXML(file, reg)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(b, back, cmpopts.EquateEmpty(), cmpopts.SortSlices(func(a, b uint32) bool { return a < b })); diff != "" {
		t.Errorf("round trip (-want +got):\n%s", diff)
	}
	c, checked, err := build.Check(b, reg.Stores("poe1")[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(checked) > 0 {
		t.Errorf("check: %v", checked)
	}
	if got := c.Build().Nodes; len(got) != 4 {
		t.Errorf("nodes %v, want the Gladiator start, the Trialmaster start and two Trialmaster nodes", got)
	}
}

func TestFromXMLBloodline(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		secondary, want string
		warnings        []build.Warning
	}{
		{"4", "Trialmaster", nil},
		{"3", "", []build.Warning{
			{Code: pob.AscendancyIgnored, Subject: "Primalist", Message: "legacy secondary ascendancy 3, which PoB hides and ignores; not imported"},
		}},
		{"5", "", []build.Warning{
			{Code: pob.AscendancyIgnored, Subject: "5", Message: "no secondary ascendancy 5 in the standard tree of dataset 3.29.3.5; not imported"},
		}},
	}
	for _, testCase := range testCases {
		b, w := fromXML(t, `className="Marauder" ascendClassName="None"`, `treeVersion="3_29" secondaryAscendClassId="`+testCase.secondary+`"`, "")
		if b.Bloodline != testCase.want {
			t.Errorf("%s: bloodline %q, want %q", testCase.secondary, b.Bloodline, testCase.want)
		}
		if diff := cmp.Diff(testCase.warnings, w); diff != "" {
			t.Errorf("%s: warnings (-want +got):\n%s", testCase.secondary, diff)
		}
	}
}

// FromXML keeps a tattoo and a runegraft by the ids their names map to; a name two overrides
// share is left out.
func TestFromXMLOverrides(t *testing.T) {
	t.Parallel()
	const file = `<PathOfBuilding>
	<Build level="90" className="Marauder" ascendClassName="None" bandit="None"/>
	<Tree activeSpec="1"><Spec treeVersion="3_29" nodes="4139,11505,26196">
		<Overrides>
			<Override nodeId="11505" dn="Tattoo of the Valako Shieldbearer" icon="x.png"/>
			<Override nodeId="4139" dn="Runegraft of the Warp"/>
			<Override nodeId="26196" dn="Tattoo of the Twins"/>
		</Overrides>
	</Spec></Tree>
</PathOfBuilding>`
	b, warnings, err := pob.FromXML([]byte(file), registry(t))
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(map[uint32]string{11505: "Valako4", 4139: "runegraft_8"}, b.Overrides); diff != "" {
		t.Errorf("overrides (-want +got):\n%s", diff)
	}
	want := []build.Warning{{Code: pob.OverrideIgnored, Subject: "node 26196", Message: `2 overrides are named "Tattoo of the Twins"; not imported`}}
	if diff := cmp.Diff(want, warnings); diff != "" {
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

func TestFileVersion(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name, xml, version, variant string
		err                         error
	}{
		{"standard", xmlOf(`className="Marauder"`, `treeVersion="3_29"`, ""), "3.29", "standard", nil},
		{"ruthless", xmlOf(`className="Exile"`, `treeVersion="3_28_ruthless"`, ""), "3.28", "ruthless", nil},
		{"not xml", "eNrtfWlz2zq", "", "", pob.ErrNotPoB},
		{"poe2", "<PathOfBuilding2><Build/></PathOfBuilding2>", "", "", pob.ErrPoE2},
		{"tree version", xmlOf(`className="Marauder"`, `treeVersion="3.29"`, ""), "", "", pob.ErrTreeVersion},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			version, variant, err := pob.FileVersion([]byte(testCase.xml))
			if version != testCase.version || variant != testCase.variant || !errors.Is(err, testCase.err) {
				t.Errorf("got %q, %q, %v, want %q, %q, %v", version, variant, err, testCase.version, testCase.variant, testCase.err)
			}
		})
	}
}
