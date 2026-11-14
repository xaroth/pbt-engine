package pob

import (
	"bytes"
	"cmp"
	"compress/zlib"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/config"
	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/internal/poe1"
)

// Errors of ToXML beside FromXML's: a build of another game than poe1, and a version that is
// not a PoB tree version.
var (
	ErrGame    = errors.New("pob: not a Path of Exile 1 build")
	ErrVersion = errors.New("pob: version not of the form major.minor")
)

// Warning codes of ToXML: each names something in the build that the file does not hold.
const (
	BloodlineNotExported build.Code = "pob-bloodline-not-exported"
	ConfigNotExported    build.Code = "pob-config-not-exported"
	ItemNotExported      build.Code = "pob-item-not-exported"
	OverrideNotExported  build.Code = "pob-override-not-exported"
)

const xmlTrue = "true"

// liveTargetVersion is the targetVersion PoB writes for PoE1 builds (GameVersions.lua).
const liveTargetVersion = "3_0"

var gameVersion = regexp.MustCompile(`^(\d+)\.(\d+)(?:\.|$)`)

// ToXML writes b as a PoB build file that FromXML reads back to b. Names come from the English
// text of the dataset reg resolves for b's version, or of the newest poe1 dataset when it has
// none (warning VersionFallback). The file holds one tree with b's class, ascendancy,
// bloodline, level, nodes, mastery choices and overrides; the bandit choice and the campaign
// stage as PoB's resistance penalty; and b's skill groups with their gems and main skill. A
// bandit choice PoB has no option for, a bloodline the tree does not list or PoB ignores, an
// override the text does not name, other config values and items give warnings. A gem the
// dataset lacks is written by id only, with warning GemUnknown. ErrGame, ErrClass,
// ErrAscendancy and ErrVersion are a build the file cannot name.
func ToXML(b *build.Build, reg *dataset.Registry) ([]byte, []build.Warning, error) {
	if b.Game != game {
		return nil, nil, fmt.Errorf("%w: %q", ErrGame, b.Game)
	}
	ex := &exporter{b: b}
	treeVersion, err := ex.treeVersion()
	if err != nil {
		return nil, nil, err
	}
	store, fallback, err := storeFor(reg, b.Version)
	if err != nil {
		return nil, nil, err
	}
	if fallback {
		ex.warnf(VersionFallback, b.Version, "no dataset of this version; names written with %s", store.Manifest().Version)
	}
	f := outFile{Build: outBuild{Level: b.Level, TargetVersion: liveTargetVersion}}
	if f.Tree, err = ex.tree(store, treeVersion, &f.Build); err != nil {
		return nil, nil, err
	}
	if err := ex.config(&f); err != nil {
		return nil, nil, err
	}
	if f.Skills, err = ex.skills(store); err != nil {
		return nil, nil, err
	}
	f.Build.MainSocketGroup = b.Skills.MainGroup + 1
	for _, s := range slices.Sorted(maps.Keys(b.Items)) {
		ex.warnf(ItemNotExported, string(s), "items are not exported")
	}
	out, err := xml.MarshalIndent(f, "", "\t")
	if err != nil {
		return nil, nil, fmt.Errorf("pob: %w", err)
	}
	return append([]byte(xml.Header), append(out, '\n')...), ex.warnings, nil
}

// EncodeCode returns the share code of a build file, as PoB writes it: the file compressed
// with zlib and base64-encoded with the URL alphabet.
func EncodeCode(file []byte) string {
	var buf bytes.Buffer
	// NewWriterLevel fails only on a bad level, and writes to a bytes.Buffer do not fail.
	w, _ := zlib.NewWriterLevel(&buf, zlib.BestCompression)
	_, _ = w.Write(file)
	_ = w.Close()
	return base64.URLEncoding.EncodeToString(buf.Bytes())
}

type exporter struct {
	b        *build.Build
	warnings []build.Warning
}

func (ex *exporter) warnf(code build.Code, subject, format string, args ...any) {
	ex.warnings = append(ex.warnings, build.Warning{Code: code, Subject: subject, Message: fmt.Sprintf(format, args...)})
}

// treeVersion is PoB's name of b's tree: 3_29, 3_29_ruthless, 3_29_alternate.
func (ex *exporter) treeVersion() (string, error) {
	m := gameVersion.FindStringSubmatch(ex.b.Version)
	if m == nil {
		return "", fmt.Errorf("%w: %q", ErrVersion, ex.b.Version)
	}
	v := m[1] + "_" + m[2]
	if variant := cmp.Or(ex.b.Variant, build.DefaultVariant); variant != build.DefaultVariant {
		v += "_" + variant
	}
	return v, nil
}

// tree writes the spec and the class and ascendancy names into bl. PoB's classId is the
// class's index in the tree and ascendClassId the 1-based index of the ascendancy in it.
func (ex *exporter) tree(s *dataset.Store, treeVersion string, bl *outBuild) (outTree, error) {
	b := ex.b
	variant := cmp.Or(b.Variant, build.DefaultVariant)
	tree, err := s.Tree(variant)
	if err != nil {
		return outTree{}, fmt.Errorf("pob: %w", err)
	}
	text, err := s.Text("en")
	if err != nil {
		return outTree{}, fmt.Errorf("pob: %w", err)
	}
	classID := slices.IndexFunc(tree.Classes, func(c dataset.Class) bool { return c.Character == b.Class })
	i := slices.IndexFunc(text.CharacterNames, func(n dataset.Name) bool { return n.ID == b.Class })
	if classID < 0 || i < 0 {
		return outTree{}, fmt.Errorf("%w %q", ErrClass, b.Class)
	}
	bl.Class, bl.Ascendancy = text.CharacterNames[i].Name, none
	spec := outSpec{TreeVersion: treeVersion, ClassID: classID, Nodes: joinNodes(b.Nodes), Masteries: joinMasteries(b.Masteries)}
	if b.Ascendancy != "" {
		a := slices.IndexFunc(tree.Classes[classID].Ascendancies, func(a dataset.Ascendancy) bool { return a.ID == b.Ascendancy })
		j := slices.IndexFunc(text.AscendancyNames, func(n dataset.AscendancyName) bool {
			return n.Variant == variant && n.ID == b.Ascendancy
		})
		if a < 0 || j < 0 {
			return outTree{}, fmt.Errorf("%w %q of %s in the %s tree", ErrAscendancy, b.Ascendancy, b.Class, variant)
		}
		spec.AscendClassID, bl.Ascendancy = a+1, text.AscendancyNames[j].Name
	}
	spec.SecondaryID = ex.bloodline(tree, s.Manifest().Version)
	spec.Overrides = ex.overrides(text)
	return outTree{Active: 1, Specs: []outSpec{spec}}, nil
}

// bloodline is PoB's secondary ascendancy of b's bloodline: the 1-based index of the
// alternate ascendancy in the tree, or 0 with a warning when PoB cannot hold it.
func (ex *exporter) bloodline(tree *dataset.Tree, version string) int {
	id := ex.b.Bloodline
	if id == "" {
		return 0
	}
	if slices.Contains(legacyBloodlines, id) {
		ex.warnf(BloodlineNotExported, id, "legacy secondary ascendancy, which PoB hides and ignores; not exported")
		return 0
	}
	i := slices.IndexFunc(tree.AlternateAscendancies, func(a dataset.Ascendancy) bool { return a.ID == id })
	if i < 0 {
		ex.warnf(BloodlineNotExported, id, "no secondary ascendancy %q in the %s tree of dataset %s; not exported", id, tree.Variant, version)
		return 0
	}
	return i + 1
}

// overrides names b's overrides as PoB does, by their English names.
func (ex *exporter) overrides(text *dataset.Text) []outOverride {
	var out []outOverride
	for _, node := range slices.Sorted(maps.Keys(ex.b.Overrides)) {
		id := ex.b.Overrides[node]
		i := slices.IndexFunc(text.OverrideNames, func(n dataset.Name) bool { return n.ID == id })
		if i < 0 {
			ex.warnf(OverrideNotExported, nodeSubject(node), "override %q has no English name; not exported", id)
			continue
		}
		out = append(out, outOverride{Node: node, Name: text.OverrideNames[i].Name})
	}
	return out
}

func joinNodes(nodes []uint32) string {
	out := make([]string, 0, len(nodes))
	for _, n := range slices.Sorted(slices.Values(nodes)) {
		out = append(out, strconv.FormatUint(uint64(n), 10))
	}
	return strings.Join(out, ",")
}

func joinMasteries(m map[uint32]uint32) string {
	out := make([]string, 0, len(m))
	for _, n := range slices.Sorted(maps.Keys(m)) {
		out = append(out, fmt.Sprintf("{%d,%d}", n, m[n]))
	}
	return strings.Join(out, ",")
}

// config writes the bandit choice into f's <Build> and its config set, the campaign stage as
// the resistance penalty, and the enemy keys as PoB's enemy inputs. Without a bandit choice
// PoB's default, kill all, applies.
func (ex *exporter) config(f *outFile) error {
	v := ex.b.Config
	enemy, enemyKeys, unwritable, err := enemyInputs(v)
	if err != nil {
		return err
	}
	ex.warnConfigLeftOut(v, enemyKeys)
	for _, name := range unwritable {
		ex.warnf(ConfigNotExported, name, "PoB reads 0 in this input as empty and uses its boss preset; not exported")
	}
	var inputs []outInput
	bandit, ok, err := config.Get(v, poe1.Bandits)
	if err != nil {
		return fmt.Errorf("pob: %w", err)
	}
	if ok {
		if f.Build.Bandit, ok = pobBandit(bandit); ok {
			// PoB reads the <Build> attribute only when the config set does not replace it.
			inputs = append(inputs, outInput{Name: banditInput, String: f.Build.Bandit})
		} else {
			ex.warnf(ConfigNotExported, string(poe1.Bandits), "PoB has no choice for the bandit quest not completed; its default, kill all, applies")
		}
	}
	stage, ok, err := config.Get(v, poe1.Campaign)
	if err != nil {
		return fmt.Errorf("pob: %w", err)
	}
	if ok {
		inputs = append(inputs, outInput{Name: penaltyInput, Number: pobPenalty(stage)})
	}
	inputs = append(inputs, enemy...)
	if len(inputs) > 0 {
		f.Config = &outConfig{Active: "1", Sets: []outConfigSet{{ID: "1", Inputs: inputs}}}
	}
	return nil
}

// warnConfigLeftOut warns of each config key of v that config does not export.
func (ex *exporter) warnConfigLeftOut(v config.Values, enemyKeys map[string]bool) {
	for _, name := range v.Names() {
		if name != string(poe1.Bandits) && name != string(poe1.Campaign) && !enemyKeys[name] {
			ex.warnf(ConfigNotExported, name, "config value not exported")
		}
	}
}

// pobBandit is the inverse of banditOf; PoB has no choice for the quest not completed.
func pobBandit(b poe1.BanditChoice) (string, bool) {
	switch b {
	case poe1.BanditsKillAll:
		return none, true
	case poe1.BanditsHelpAlira:
		return "Alira", true
	case poe1.BanditsHelpKraityn:
		return "Kraityn", true
	case poe1.BanditsHelpOak:
		return "Oak", true
	case poe1.BanditsNotCompleted:
	}
	return "", false
}

// pobPenalty is the inverse of stageOf; config.Get rejects other stages.
func pobPenalty(s poe1.CampaignStage) string {
	switch s {
	case poe1.CampaignActs6To10:
		return "-30"
	case poe1.CampaignActs1To5:
		return "0"
	case poe1.CampaignComplete:
	}
	return "-60"
}

// skills writes b's groups as one skill set. Gems are named by gem id, variant and skill id,
// the order PoB loads them in, so a name PoB spells differently does not matter.
func (ex *exporter) skills(store *dataset.Store) (*outSkills, error) {
	groups := ex.b.Skills.Groups
	out := &outSkills{Active: "1", Sets: []outSkillSet{{ID: "1"}}}
	if len(groups) == 0 {
		return out, nil
	}
	data, err := store.Skills()
	if _, missing := errors.AsType[*dataset.MissingGroupError](err); missing {
		data = nil
	} else if err != nil {
		return nil, fmt.Errorf("pob: %w", err)
	}
	text, err := store.Text("en")
	if err != nil {
		return nil, fmt.Errorf("pob: %w", err)
	}
	for _, g := range groups {
		out.Sets[0].Groups = append(out.Sets[0].Groups, ex.group(g, data, text))
	}
	return out, nil
}

func (ex *exporter) group(g build.SkillGroup, data *dataset.Skills, text *dataset.Text) outSkill {
	out := outSkill{Enabled: xmlTrue, Slot: pobSlot(g.Slot)}
	// PoB's main active skill counts the active skills of enabled gems in order, 1-based; both
	// switches of every gem are written on. See countedSkills.
	main := 0
	for i, gem := range g.Gems {
		out.Gems = append(out.Gems, ex.gem(gem, data, text))
		active := activeSkillsOf(data, gem)
		switch {
		case i == g.Active:
			main += max(slices.Index(active, g.ActiveSkill), 0) + 1
		case i < g.Active && !gem.Disabled:
			main += len(active)
		}
	}
	out.MainActive = strconv.Itoa(max(main, 1))
	return out
}

// activeSkillsOf is build.ActiveSkills, or an active gem's first skill without a skills group.
func activeSkillsOf(data *dataset.Skills, gem build.Gem) []int {
	switch {
	case data != nil:
		return build.ActiveSkills(data, gem)
	case !gem.Support:
		return []int{0}
	}
	return nil
}

// gem writes gem.
func (ex *exporter) gem(gem build.Gem, data *dataset.Skills, text *dataset.Text) outGem {
	out := outGem{
		GemID: gem.ID, VariantID: gem.Effect, Level: gem.Level, Quality: gem.Quality,
		Enabled: strconv.FormatBool(!gem.Disabled), EnableGlobal1: xmlTrue, EnableGlobal2: xmlTrue,
	}
	if data == nil {
		return out
	}
	dg := data.Gem(gem.ID)
	if dg == nil || len(dg.Effects) == 0 {
		ex.warnf(GemUnknown, gem.ID, "gem not in dataset; written by id only")
		return out
	}
	effect := &dg.Effects[0]
	if e := dg.Effect(gem.Effect); e != nil {
		effect = e
	}
	// PoB takes the first variant granting the skill id when variantId is missing, and a Vaal
	// gem shares its skill id with its transfigured Vaal variants.
	out.VariantID, out.SkillID = effect.ID, effect.GrantedEffect
	if i := slices.IndexFunc(text.GemNames, func(n dataset.Name) bool { return n.ID == gem.ID }); i >= 0 {
		// PoB names supports without the suffix (SkillsTab.lua).
		out.Name = strings.TrimSuffix(text.GemNames[i].Name, " Support")
	}
	return out
}

// pobSlot is the inverse of slots.
func pobSlot(s build.Slot) string {
	for name, slot := range slots() {
		if slot == s {
			return name
		}
	}
	return ""
}

type outFile struct {
	XMLName xml.Name   `xml:"PathOfBuilding"`
	Build   outBuild   `xml:"Build"`
	Tree    outTree    `xml:"Tree"`
	Skills  *outSkills `xml:"Skills"`
	Config  *outConfig `xml:"Config"`
}

type outBuild struct {
	Level           int    `xml:"level,attr"`
	TargetVersion   string `xml:"targetVersion,attr"`
	Class           string `xml:"className,attr"`
	Ascendancy      string `xml:"ascendClassName,attr"`
	Bandit          string `xml:"bandit,attr,omitempty"`
	MainSocketGroup int    `xml:"mainSocketGroup,attr"`
}

type outTree struct {
	Active int       `xml:"activeSpec,attr"`
	Specs  []outSpec `xml:"Spec"`
}

type outSpec struct {
	TreeVersion   string        `xml:"treeVersion,attr"`
	ClassID       int           `xml:"classId,attr"`
	AscendClassID int           `xml:"ascendClassId,attr"`
	SecondaryID   int           `xml:"secondaryAscendClassId,attr,omitempty"`
	Nodes         string        `xml:"nodes,attr"`
	Masteries     string        `xml:"masteryEffects,attr"`
	Overrides     []outOverride `xml:"Overrides>Override,omitempty"`
}

type outOverride struct {
	Node uint32 `xml:"nodeId,attr"`
	Name string `xml:"dn,attr"`
}

type outConfig struct {
	Active string         `xml:"activeConfigSet,attr"`
	Sets   []outConfigSet `xml:"ConfigSet"`
}

type outConfigSet struct {
	ID     string     `xml:"id,attr"`
	Inputs []outInput `xml:"Input"`
}

type outInput struct {
	Name   string `xml:"name,attr"`
	Number string `xml:"number,attr,omitempty"`
	String string `xml:"string,attr,omitempty"`
}

type outSkills struct {
	Active string        `xml:"activeSkillSet,attr"`
	Sets   []outSkillSet `xml:"SkillSet"`
}

type outSkillSet struct {
	ID     string     `xml:"id,attr"`
	Groups []outSkill `xml:"Skill"`
}

type outSkill struct {
	Enabled    string   `xml:"enabled,attr"`
	Slot       string   `xml:"slot,attr,omitempty"`
	MainActive string   `xml:"mainActiveSkill,attr"`
	Gems       []outGem `xml:"Gem"`
}

type outGem struct {
	Name          string `xml:"nameSpec,attr,omitempty"`
	GemID         string `xml:"gemId,attr"`
	VariantID     string `xml:"variantId,attr,omitempty"`
	SkillID       string `xml:"skillId,attr,omitempty"`
	Level         int    `xml:"level,attr"`
	Quality       int    `xml:"quality,attr"`
	Enabled       string `xml:"enabled,attr"`
	EnableGlobal1 string `xml:"enableGlobal1,attr"`
	EnableGlobal2 string `xml:"enableGlobal2,attr"`
}
