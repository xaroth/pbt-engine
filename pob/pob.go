// Package pob imports Path of Building builds (PoE1). FromXML reads a build file into a
// build.Build with what the engine models, and a warning for each thing it leaves out.
package pob

import (
	"cmp"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/config"
	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/pob/internal/pobxml"
	"github.com/xaroth/pbt-engine/rules/poe1"
)

// Errors FromXML returns; match them with errors.Is. A file that is not a PoE1 PoB build
// matches ErrNotPoB.
var (
	ErrNotPoB      = errors.New("pob: not a usable Path of Building build")
	ErrPoE2        = errors.New("pob: Path of Exile 2 builds are not supported")
	ErrTreeVersion = errors.New("pob: unknown tree version")
	ErrNoDataset   = errors.New("pob: no dataset to read names with")
	ErrClass       = errors.New("pob: unknown class")
	ErrAscendancy  = errors.New("pob: unknown ascendancy")
)

// Warning codes of FromXML: each names something in the file that the build does not hold.
const (
	VersionFallback    build.Code = "pob-version-fallback"
	SpecIgnored        build.Code = "pob-spec-ignored"
	ClusterNodeDropped build.Code = "pob-cluster-node-dropped"
	JewelIgnored       build.Code = "pob-jewel-ignored"
	OverrideIgnored    build.Code = "pob-override-ignored"
	ConfigIgnored      build.Code = "pob-config-ignored"
	ItemIgnored        build.Code = "pob-item-ignored"
	SkillIgnored       build.Code = "pob-skill-ignored"
	PantheonIgnored    build.Code = "pob-pantheon-ignored"
	AscendancyIgnored  build.Code = "pob-ascendancy-ignored"
)

const (
	game = "poe1"
	// maxTreeNode is the largest id of a tree node; PoB gives the nodes it generates for
	// cluster jewels larger ids.
	maxTreeNode = 0xFFFF
	// none is PoB's value for no choice.
	none = "None"
	// penaltyInput is PoB's resistance penalty input, the only config input imported.
	penaltyInput = "resistancePenalty"
)

// treeVersion matches PoB's tree versions: 3_29, 3_29_ruthless, 3_29_alternate.
var treeVersion = regexp.MustCompile(`^(\d+)_(\d+)(?:_(ruthless|alternate))?$`)

// FromXML reads a PoB build file. The active tree spec gives the version, variant, nodes and
// mastery choices; the class and ascendancy names map to ids through the English text of the
// dataset reg resolves for that version, or of the newest poe1 dataset when it has none
// (warning VersionFallback; the build keeps the file's version). Config: the bandit choice,
// and the campaign stage from PoB's resistance penalty. Everything else in the file gives a
// warning: other specs, cluster jewel nodes (dropped), jewels, node overrides, config inputs,
// items, skill groups, pantheon and a secondary ascendancy.
func FromXML(data []byte, reg *dataset.Registry) (*build.Build, []build.Warning, error) {
	f, err := pobxml.Parse(data)
	switch {
	case errors.Is(err, pobxml.ErrPoE2):
		return nil, nil, ErrPoE2
	case err != nil:
		return nil, nil, fmt.Errorf("%w: %w", ErrNotPoB, err)
	}
	im := &importer{file: f, spec: f.Specs[f.Active]}
	version, variant, err := versionOf(im.spec.TreeVersion)
	if err != nil {
		return nil, nil, err
	}
	b := &build.Build{Format: build.Format, Game: game, Version: version, Variant: variant, Level: f.Build.Level}
	store, err := im.store(reg, version)
	if err != nil {
		return nil, nil, err
	}
	if b.Class, b.Ascendancy, err = names(store, variant, f.Build.Class, f.Build.Ascendancy); err != nil {
		return nil, nil, err
	}
	b.Nodes = im.nodes()
	b.Masteries = im.masteries()
	if b.Config, err = im.config(); err != nil {
		return nil, nil, err
	}
	im.leftOut()
	return b, im.warnings, nil
}

type importer struct {
	file     *pobxml.File
	spec     pobxml.Spec
	warnings []build.Warning
}

func (im *importer) warnf(code build.Code, subject, format string, args ...any) {
	im.warnings = append(im.warnings, build.Warning{Code: code, Subject: subject, Message: fmt.Sprintf(format, args...)})
}

func versionOf(tree string) (version, variant string, err error) {
	m := treeVersion.FindStringSubmatch(tree)
	if m == nil {
		return "", "", fmt.Errorf("%w %q", ErrTreeVersion, tree)
	}
	variant = build.DefaultVariant
	if m[3] != "" {
		variant = m[3]
	}
	return m[1] + "." + m[2], variant, nil
}

// store is the dataset of version, else the newest of the game with a warning.
func (im *importer) store(reg *dataset.Registry, version string) (*dataset.Store, error) {
	if res, err := reg.Resolve(game, version); err == nil {
		return res.Store, nil
	}
	stores := reg.Stores(game)
	if len(stores) == 0 {
		return nil, fmt.Errorf("%w for %s %s", ErrNoDataset, game, version)
	}
	s := stores[len(stores)-1]
	im.warnf(VersionFallback, version, "no dataset of this version; class names read with %s", s.Manifest().Version)
	return s, nil
}

// names maps PoB's class and ascendancy names to ids through the dataset's English text.
func names(s *dataset.Store, variant, class, ascendancy string) (string, string, error) {
	text, err := s.Text("en")
	if err != nil {
		return "", "", fmt.Errorf("pob: %w", err)
	}
	i := slices.IndexFunc(text.CharacterNames, func(n dataset.Name) bool { return n.Name == class })
	if i < 0 {
		return "", "", fmt.Errorf("%w %q", ErrClass, class)
	}
	if ascendancy == "" || ascendancy == none {
		return text.CharacterNames[i].ID, "", nil
	}
	j := slices.IndexFunc(text.AscendancyNames, func(n dataset.AscendancyName) bool {
		return n.Variant == variant && n.Name == ascendancy
	})
	if j < 0 {
		return "", "", fmt.Errorf("%w %q in the %s tree", ErrAscendancy, ascendancy, variant)
	}
	return text.CharacterNames[i].ID, text.AscendancyNames[j].ID, nil
}

// nodes are the spec's nodes without the cluster jewel nodes PoB generates.
func (im *importer) nodes() []uint32 {
	var out []uint32
	var dropped []string
	for _, n := range im.spec.Nodes {
		if n > maxTreeNode {
			dropped = append(dropped, strconv.FormatUint(uint64(n), 10))
			continue
		}
		out = append(out, n)
	}
	if len(dropped) > 0 {
		im.warnf(ClusterNodeDropped, "nodes "+strings.Join(dropped, ","),
			"%d cluster jewel nodes dropped; cluster jewels are not imported", len(dropped))
	}
	return out
}

func (im *importer) masteries() map[uint32]uint32 {
	if len(im.spec.Masteries) == 0 {
		return nil
	}
	out := make(map[uint32]uint32, len(im.spec.Masteries))
	for _, m := range im.spec.Masteries {
		out[m.Node] = m.Effect
	}
	return out
}

// config is the bandit choice and the campaign stage; other inputs give warnings.
func (im *importer) config() (config.Values, error) {
	var v config.Values
	if bandit, ok := banditOf(im.file.Build.Bandit); ok {
		if err := config.Set(&v, poe1.Bandits, bandit); err != nil {
			return nil, fmt.Errorf("pob: %w", err)
		}
	} else {
		im.warnf(ConfigIgnored, "bandit", "unknown bandit choice %q; bandits left unset", im.file.Build.Bandit)
	}
	for _, in := range im.file.Config {
		if mirrorsBuild(in.Name) {
			continue
		}
		if in.Name != penaltyInput {
			im.warnf(ConfigIgnored, in.Name, "config input not modelled (%s)", in.Value)
			continue
		}
		stage, ok := stageOf(in.Value)
		if !ok {
			im.warnf(ConfigIgnored, in.Name, "unknown resistance penalty %q; campaign stage left at its default", in.Value)
			continue
		}
		if err := config.Set(&v, poe1.Campaign, stage); err != nil {
			return nil, fmt.Errorf("pob: %w", err)
		}
	}
	return v, nil
}

// mirrorsBuild reports a config input that repeats a <Build> attribute, read from there.
func mirrorsBuild(input string) bool {
	return input == "bandit" || input == "pantheonMajorGod" || input == "pantheonMinorGod"
}

// banditOf maps PoB's bandit choice. None is PoB's kill all, and so is no attribute. Eramir,
// who rewards killing all three, is the GGG character API's value for it; files imported from
// a character carry it (PoB maps it to None in ImportTab.lua).
func banditOf(pob string) (poe1.BanditChoice, bool) {
	switch pob {
	case "", none, "Eramir":
		return poe1.BanditsKillAll, true
	case "Alira":
		return poe1.BanditsHelpAlira, true
	case "Kraityn":
		return poe1.BanditsHelpKraityn, true
	case "Oak":
		return poe1.BanditsHelpOak, true
	}
	return "", false
}

// stageOf maps PoB's resistance penalty to the campaign stage that gives it.
func stageOf(penalty string) (poe1.CampaignStage, bool) {
	switch penalty {
	case "-60":
		return poe1.CampaignComplete, true
	case "-30":
		return poe1.CampaignActs6To10, true
	case "0":
		return poe1.CampaignActs1To5, true
	}
	return "", false
}

// leftOut warns about everything else in the file that the build does not hold.
func (im *importer) leftOut() {
	im.treeLeftOut()
	im.itemsLeftOut()
	for i, g := range im.file.Skills {
		subject := cmp.Or(g.Label, "skill group "+strconv.Itoa(i+1))
		if g.Label == "" && len(g.Gems) > 0 {
			subject = g.Gems[0]
		}
		im.warnf(SkillIgnored, subject, "skills are not imported (%s)", strings.Join(g.Gems, ", "))
	}
}

func (im *importer) treeLeftOut() {
	f := im.file
	for i, s := range f.Specs {
		if i != f.Active {
			im.warnf(SpecIgnored, specName(s, i), "only the active tree is imported")
		}
	}
	if id := im.spec.SecondaryAscendClassID; id != 0 {
		im.warnf(AscendancyIgnored, strconv.Itoa(id), "secondary ascendancy not imported")
	}
	for _, god := range []string{f.Build.PantheonMajor, f.Build.PantheonMinor} {
		if god != "" && god != none {
			im.warnf(PantheonIgnored, god, "pantheon not imported")
		}
	}
	for _, o := range im.spec.Overrides {
		im.warnf(OverrideIgnored, nodeSubject(o.Node), "node override %q not imported", o.Name)
	}
}

// itemsLeftOut warns per item; an item in a jewel socket of the active tree is a jewel.
func (im *importer) itemsLeftOut() {
	jewels := map[int]uint32{}
	for _, s := range im.spec.Sockets {
		if s.Item != 0 {
			jewels[s.Item] = s.Node
		}
	}
	for _, it := range im.file.Items {
		if node, ok := jewels[it.ID]; ok {
			im.warnf(JewelIgnored, nodeSubject(node), "jewel %q not imported", it.Name)
			continue
		}
		im.warnf(ItemIgnored, cmp.Or(it.Name, "item "+strconv.Itoa(it.ID)), "items are not imported")
	}
}

func specName(s pobxml.Spec, i int) string {
	return cmp.Or(s.Title, "tree "+strconv.Itoa(i+1))
}

func nodeSubject(n uint32) string { return "node " + strconv.FormatUint(uint64(n), 10) }
