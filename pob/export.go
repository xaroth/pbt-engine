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
	"github.com/xaroth/pbt-engine/dataset"
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

// clusterHashFormatVersion marks the spec's cluster jewel node ids as the current format; PoB
// reads a spec without it as the legacy one and renumbers them.
const clusterHashFormatVersion = 2

var gameVersion = regexp.MustCompile(`^(\d+)\.(\d+)(?:\.|$)`)

// ToXML writes b as a PoB build file that FromXML reads back to b. Names come from the English
// text of the dataset reg resolves for b's version, or of the newest poe1 dataset when it has
// none (warning VersionFallback). The file holds one tree with b's class, ascendancy,
// bloodline, level, nodes, mastery choices and overrides; the bandit choice and the campaign
// stage as PoB's resistance penalty; b's items in the active item set, with their abyss jewels,
// and its jewels in their tree sockets; and b's skill groups with their gems and main skill, a
// group an item grants with that item as its source and the gear of a group's minion in an
// item set of its own. A bandit choice PoB has no option for, a bloodline the tree does not
// list or PoB ignores, an override the text does not name, other config values and an item
// whose base has no English name give warnings. A gem the dataset lacks is written by id only,
// with warning GemUnknown. ErrGame, ErrClass, ErrAscendancy and ErrVersion are a build the file
// cannot name.
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
	if err := ex.itemsAndSkills(store, &f); err != nil {
		return nil, nil, err
	}
	f.Build.MainSocketGroup = b.Skills.MainGroup + 1
	out, err := xml.MarshalIndent(f, "", "\t")
	if err != nil {
		return nil, nil, fmt.Errorf("pob: %w", err)
	}
	return []byte(xml.Header + pobEscapes(string(out)) + "\n"), ex.warnings, nil
}

// pobEscapes rewrites the numeric references encoding/xml writes for text PoB's XML reader
// decodes only as &lt; &gt; &amp; &apos; &quot; or not at all. A literal "&#xA;" in a value is
// written as "&amp;#xA;", so no reference here is the value's own. A carriage return keeps its
// reference: written raw, it would read back as a newline.
func pobEscapes(s string) string {
	return strings.NewReplacer("&#xA;", "\n", "&#x9;", "\t", "&#39;", "&apos;", "&#34;", "&quot;").Replace(s)
}

// itemsAndSkills writes b's items, its jewels in f's spec and its skill groups, with the item
// sets of their minions' gear.
func (ex *exporter) itemsAndSkills(store *dataset.Store, f *outFile) error {
	text, err := store.Text("en")
	if err != nil {
		return fmt.Errorf("pob: %w", err)
	}
	items := ex.newItemWriter(text)
	f.Tree.Specs[0].Sockets = items.character()
	if f.Skills, err = ex.skills(store, items); err != nil {
		return err
	}
	if len(items.out.Items) > 0 || len(items.out.Sets) > 1 {
		f.Items = &items.out
	}
	return nil
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

// warnNodef warns about node n.
func (ex *exporter) warnNodef(code build.Code, n uint32, format string, args ...any) {
	ex.warnings = append(ex.warnings, nodeWarning(code, n, format, args...))
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
	spec := outSpec{
		TreeVersion: treeVersion, ClusterHash: clusterHashFormatVersion, ClassID: classID,
		Nodes: joinNodes(b.Nodes), Masteries: joinMasteries(b.Masteries),
	}
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
			ex.warnNodef(OverrideNotExported, node, "override %q has no English name; not exported", id)
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

type outFile struct {
	XMLName xml.Name   `xml:"PathOfBuilding"`
	Build   outBuild   `xml:"Build"`
	Tree    outTree    `xml:"Tree"`
	Items   *outItems  `xml:"Items"`
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
	ClusterHash   int           `xml:"clusterHashFormatVersion,attr"`
	ClassID       int           `xml:"classId,attr"`
	AscendClassID int           `xml:"ascendClassId,attr"`
	SecondaryID   int           `xml:"secondaryAscendClassId,attr,omitempty"`
	Nodes         string        `xml:"nodes,attr"`
	Masteries     string        `xml:"masteryEffects,attr"`
	Overrides     []outOverride `xml:"Overrides>Override,omitempty"`
	Sockets       []outSocket   `xml:"Sockets>Socket,omitempty"`
}

type outOverride struct {
	Node uint32 `xml:"nodeId,attr"`
	Name string `xml:"dn,attr"`
}
