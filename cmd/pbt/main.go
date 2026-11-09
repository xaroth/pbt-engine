// Command pbt inspects pbt datasets.
//
//	pbt data show node <id> --version <v> [--variant standard]
//	pbt data show stat <id> --version <v>
//	pbt data show character <name> --version <v>
//	pbt data diff <version-a> <version-b> [--limit 20]
//	pbt build check <build.json>
//	pbt build resolve <build.json>
//	pbt semantics coverage --version <v> [--variant standard]
//	pbt eval <build.json> [--version v] [--set key=value]... [--explain output [--view tree]] [--warnings all]
//	pbt compare <a.json> <b.json> [--version v] [--version-b v] [--set key=value]... [--warnings all]
//	pbt power <build.json> [--by player.life] [--mode alone|path] [--max-distance n] [--top 20]
//
// Datasets are read from the subdirectories of --root (default datasets), with display text
// in --language (default en). Versions of --game (default poe1) resolve as
// dataset.Registry.Resolve does, so 3.29 is the newest 3.29 dataset; a version that resolves
// to another one is noted on stderr. Subdirectories whose dataset cannot be opened are skipped
// with a warning on stderr.
package main

import (
	"cmp"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/resolve"
	"github.com/xaroth/pbt-engine/semantics"
	"github.com/xaroth/pbt-engine/semantics/poe1"
	"github.com/xaroth/pbt-engine/statdesc"
)

var (
	errUnknownOption   = errors.New("unknown option")
	errNeedsValue      = errors.New("needs a value")
	errVersionRequired = errors.New("--version is required")
	errLimit           = errors.New("want a number >= 0")
	errUsage           = errors.New(usage)
	errNoSemantics     = errors.New("no semantics table for")
	errNotFound        = errors.New("not found")
)

const usage = `usage:
  pbt data show node <id> --version <v> [--variant standard]
  pbt data show stat <id> --version <v>
  pbt data show character <name> --version <v>
  pbt data diff <version-a> <version-b> [--limit 20]
  pbt build check <build.json>
  pbt build resolve <build.json>
  pbt semantics coverage --version <v> [--variant standard]
  pbt eval <build.json> [--version v] [--set key=value]... [--explain output [--view tree|proportional|marginal]] [--warnings all]
  pbt compare <a.json> <b.json> [--version v] [--version-b v] [--set key=value]... [--warnings all]
  pbt power <build.json> [--by player.life] [--mode alone|path] [--max-distance n] [--top 20]
options for all: [--root datasets] [--game poe1] [--language en]`

// Description scopes for passive node text, as named in the dataset.
const (
	treeScope = "Metadata/StatDescriptions/passive_skill_stat_descriptions.txt"
	auraScope = "Metadata/StatDescriptions/passive_skill_aura_stat_descriptions.txt"
)

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "pbt:", err)
		os.Exit(1)
	}
}

// parseArgs splits --name value and --name=value options from positional arguments.
func parseArgs(args []string, known map[string]string) ([]string, map[string]string, error) {
	opts := map[string]string{}
	maps.Copy(opts, known)
	var pos []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if !strings.HasPrefix(a, "-") {
			pos = append(pos, a)
			continue
		}
		name, val, hasVal := strings.Cut(strings.TrimLeft(a, "-"), "=")
		if _, ok := known[name]; !ok {
			return nil, nil, fmt.Errorf("%w %s", errUnknownOption, a)
		}
		if !hasVal {
			if i+1 >= len(args) {
				return nil, nil, fmt.Errorf("option %s %w", a, errNeedsValue)
			}
			i++
			val = args[i]
		}
		if name == "set" {
			val = opts[name] + val + "\n"
		}
		opts[name] = val
	}
	return pos, opts, nil
}

// run writes results to w and notes about how they were found to notes.
func run(args []string, w, notes io.Writer) error {
	pos, opts, err := parseArgs(args, map[string]string{
		"version": "", "variant": build.DefaultVariant, "root": "datasets", "limit": "20", "language": "en", "game": "poe1",
		"set": "", "explain": "", "view": "tree", "version-b": "", "warnings": "short",
		"by": "player.life", "mode": modeAlone, "max-distance": "", "top": "20",
	})
	if err != nil {
		return err
	}
	commands := map[string]struct {
		args int
		run  func(invocation) error
	}{
		"data show":          {4, dataShow},
		"data diff":          {4, dataDiff},
		"build check":        {3, buildCheck},
		"build resolve":      {3, buildResolve},
		"semantics coverage": {2, semanticsCoverage},
		"eval":               {2, eval},
		"compare":            {3, compare},
		"power":              {2, nodePower},
	}
	const commandWords = 2
	cmd, ok := commands[strings.Join(pos[:min(len(pos), commandWords)], " ")]
	if !ok && len(pos) > 0 {
		cmd, ok = commands[pos[0]]
	}
	if !ok || len(pos) != cmd.args {
		return errUsage
	}
	return cmd.run(invocation{pos: pos, opts: opts, w: w, notes: notes})
}

// invocation is a parsed command line and where its output goes.
type invocation struct {
	pos      []string
	opts     map[string]string
	w, notes io.Writer
}

func dataShow(in invocation) error {
	if in.opts["version"] == "" {
		return errVersionRequired
	}
	var variants []string // stats and characters need no tree
	if in.pos[2] == "node" {
		variants = []string{in.opts["variant"]}
	}
	res, err := resolveVersions(in.notes, in.opts["root"], in.opts["game"], in.opts["version"])
	if err != nil {
		return err
	}
	d, err := read(res[0].Store, variants, in.opts["language"])
	if err != nil {
		return err
	}
	switch in.pos[2] {
	case "node":
		return showNode(in.w, d, in.opts["variant"], in.pos[3])
	case "stat":
		return showStat(in.w, d, in.pos[3])
	case "character":
		return showCharacter(in.w, d, in.pos[3])
	}
	return errUsage
}

func dataDiff(in invocation) error {
	limit, err := strconv.Atoi(in.opts["limit"])
	if err != nil || limit < 0 {
		return fmt.Errorf("--limit %q: %w", in.opts["limit"], errLimit)
	}
	res, err := resolveVersions(in.notes, in.opts["root"], in.opts["game"], in.pos[2], in.pos[3])
	if err != nil {
		return err
	}
	sa, sb := res[0].Store, res[1].Store
	a, err := read(sa, sa.Manifest().Trees(), in.opts["language"])
	if err != nil {
		return err
	}
	b, err := read(sb, sb.Manifest().Trees(), in.opts["language"])
	if err != nil {
		return err
	}
	return diff(in.w, a, b, limit)
}

func buildCheck(in invocation) error {
	c, err := checkBuild(in.notes, in.opts["root"], in.pos[2])
	if err != nil {
		return err
	}
	p := &printer{w: in.w}
	printWarnings(p, c.Warnings())
	b := c.Build()
	p.printf("ok: %s %s (dataset %s), %s tree, %s %s, level %d, %d nodes\n", b.Game, b.Version, c.Store().Manifest().Version,
		b.Variant, b.Class, cmp.Or(b.Ascendancy, "no ascendancy"), b.Level, len(b.Nodes))
	return p.err
}

func buildResolve(in invocation) error {
	c, err := checkBuild(in.w, in.opts["root"], in.pos[2])
	if err != nil {
		return err
	}
	p := &printer{w: in.w}
	printWarnings(p, c.Warnings())
	if p.err != nil {
		return p.err
	}
	r, err := resolve.Resolve(c)
	if err != nil {
		return fmt.Errorf("%s: %w", in.pos[2], err)
	}
	if err := r.Dump(in.w); err != nil {
		return fmt.Errorf("dump: %w", err)
	}
	return nil
}

func semanticsCoverage(in invocation) error {
	res, err := resolveVersions(in.notes, in.opts["root"], in.opts["game"], in.opts["version"])
	if err != nil {
		return err
	}
	return coverage(in.w, res[0].Store, in.opts["variant"])
}

// resolveVersions resolves versions of game among the datasets under root and notes on notes
// each dataset skipped and each version that resolves to another one.
func resolveVersions(notes io.Writer, root, game string, versions ...string) ([]dataset.Resolution, error) {
	reg, err := dataset.OpenRoot(os.DirFS(root))
	if reg == nil {
		return nil, fmt.Errorf("open datasets: %w", err)
	}
	p := &printer{w: notes}
	if err != nil {
		for line := range strings.SplitSeq(err.Error(), "\n") {
			p.printf("pbt: skipped %s/%s\n", root, line)
		}
	}
	var out []dataset.Resolution
	for _, version := range versions {
		res, err := reg.Resolve(game, version)
		if err != nil {
			return nil, fmt.Errorf("resolve version: %w", err)
		}
		out = append(out, res)
	}
	for _, res := range out {
		name := res.Store.Manifest().Version
		//exhaustive:ignore // an exact match needs no note
		switch res.Match {
		case dataset.MatchNewest:
			p.printf("(%s %s resolves to %s)\n", game, res.Requested, name)
		case dataset.MatchEarlierPatch:
			p.printf("(%s %s has no dataset; using %s, the newest earlier patch of its league)\n", game, res.Requested, name)
		case dataset.MatchLaterPatch:
			p.printf("(%s %s has no dataset or earlier patch; using %s, the newest patch of its league)\n", game, res.Requested, name)
		}
	}
	return out, p.err
}

// coverage prints how far the edition's semantics table covers the stats of s that feed the
// character sheet.
func coverage(w io.Writer, s *dataset.Store, variant string) error {
	if g := s.Manifest().Game; g != "poe1" {
		return fmt.Errorf("%w %s", errNoSemantics, g)
	}
	t, err := poe1.Load()
	if err != nil {
		return fmt.Errorf("load semantics: %w", err)
	}
	feeds, err := poe1.FeedsSheet(s)
	if err != nil {
		return fmt.Errorf("sheet stats: %w", err)
	}
	c, err := semantics.Cover(s, variant, t, feeds)
	if err != nil {
		return fmt.Errorf("coverage: %w", err)
	}
	if err := c.WriteText(w); err != nil {
		return fmt.Errorf("write coverage: %w", err)
	}
	return nil
}

// read reads the given tree variants and the text of one language from s.
func read(s *dataset.Store, variants []string, language string) (*dataset.Dataset, error) {
	d, err := s.Dataset(dataset.Options{Trees: variants, Languages: []string{language}})
	if err != nil {
		return nil, fmt.Errorf("dataset %s: %w", s.Manifest().Version, err)
	}
	return d, nil
}

// printWarnings prints one line per warning.
func printWarnings(p *printer, ws []build.Warning) {
	for _, w := range ws {
		p.printf("warning %s\n", w)
	}
}

// checkBuild reads a build file and checks it against the dataset its version resolves to
// under --root.
func checkBuild(notes io.Writer, root, path string) (*build.Checked, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read build: %w", err)
	}
	b, err := build.Parse(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	res, err := resolveVersions(notes, root, b.Game, b.Version)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	c, _, err := build.Check(b, res[0].Store)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return c, nil
}

// texter renders stat values and names of a dataset loaded with one language.
type texter struct {
	d     *dataset.Dataset
	text  dataset.Text
	set   *statdesc.Set
	nodes map[uint32]string
}

func newTexter(d *dataset.Dataset) (texter, error) {
	text := d.Texts[0] // read reads exactly one language
	set, err := statdesc.NewSet(text.Descriptions)
	nodes := make(map[uint32]string, len(text.NodeNames))
	for _, n := range text.NodeNames {
		nodes[n.ID] = n.Name
	}
	if err != nil {
		return texter{d, text, set, nodes}, fmt.Errorf("description set: %w", err)
	}
	return texter{d, text, set, nodes}, nil
}

func (t texter) characterName(id string) string {
	for _, n := range t.text.CharacterNames {
		if n.ID == id {
			return n.Name
		}
	}
	return id
}

func (t texter) ascendancyName(variant, id string) string {
	for _, n := range t.text.AscendancyNames {
		if n.Variant == variant && n.ID == id {
			return n.Name
		}
	}
	return id
}

// lines renders stats in scope, one entry per text line.
func (t texter) lines(scope string, stats map[string]int32) []string {
	ls, err := t.set.Describe(scope, stats)
	if err != nil {
		return []string{"(" + err.Error() + ")"}
	}
	var out []string
	for _, l := range ls {
		out = append(out, strings.Split(l.Text, "\n")...)
	}
	return out
}

func showNode(w io.Writer, d *dataset.Dataset, variant, idArg string) error {
	p := &printer{w: w}
	n, err := findNode(d, variant, idArg)
	if err != nil {
		return err
	}
	t, err := newTexter(d)
	if err != nil {
		return err
	}
	kind := n.Kind.String()
	if n.Ascendancy != "" {
		kind += ", " + t.ascendancyName(variant, n.Ascendancy)
	}
	p.printf("%d %s (%s)\n", n.ID, t.nodes[n.ID], kind)
	for _, l := range t.lines(treeScope, d.StatMap(n.Stats)) {
		p.printf("  %s\n", l)
	}
	for _, b := range n.Buffs {
		for _, l := range t.lines(auraScope, d.StatMap(b)) {
			p.printf("  %s (aura)\n", l)
		}
	}
	if n.SkillPoints > 0 {
		p.printf("  grants %d passive skill points\n", n.SkillPoints)
	}
	for _, e := range n.MasteryEffects {
		p.printf("  mastery effect %d: %s\n", e.ID, strings.Join(t.lines(treeScope, d.StatMap(e.Stats)), " / "))
	}
	var nb []string
	for _, x := range n.Neighbours {
		nb = append(nb, strconv.FormatUint(uint64(x), 10))
	}
	p.printf("  neighbours: %s\n", strings.Join(nb, " "))
	return p.err
}

func findNode(d *dataset.Dataset, variant, idArg string) (dataset.Node, error) {
	id, err := strconv.ParseUint(idArg, 10, 32)
	if err != nil {
		return dataset.Node{}, fmt.Errorf("node id %q: %w", idArg, err)
	}
	ti := slices.IndexFunc(d.Trees, func(t dataset.Tree) bool { return t.Variant == variant })
	if ti < 0 {
		return dataset.Node{}, fmt.Errorf("no %s tree in %s: %w", variant, d.Manifest.Version, errNotFound)
	}
	nodes := d.Trees[ti].Nodes
	ni, ok := slices.BinarySearchFunc(nodes, uint32(id), func(n dataset.Node, id uint32) int { return int(n.ID) - int(id) })
	if !ok {
		return dataset.Node{}, fmt.Errorf("no node %d in the %s %s tree: %w", id, d.Manifest.Version, variant, errNotFound)
	}
	return nodes[ni], nil
}

func showStat(w io.Writer, d *dataset.Dataset, id string) error {
	p := &printer{w: w}
	i := slices.IndexFunc(d.Stats, func(s dataset.Stat) bool { return s.ID == id })
	if i < 0 {
		return fmt.Errorf("no stat %s in %s: %w", id, d.Manifest.Version, errNotFound)
	}
	p.printf("%s (handle %d of %d)\n", id, i, len(d.Stats))
	return p.err
}

func showCharacter(w io.Writer, d *dataset.Dataset, name string) error {
	p := &printer{w: w}
	t, err := newTexter(d)
	if err != nil {
		return err
	}
	i := slices.IndexFunc(d.Characters, func(c dataset.Character) bool { return strings.EqualFold(t.characterName(c.ID), name) })
	if i < 0 {
		return fmt.Errorf("no character %s in %s: %w", name, d.Manifest.Version, errNotFound)
	}
	c := d.Characters[i]
	p.printf("%s (%s)\n  strength %d, dexterity %d, intelligence %d\n  base life %d, base mana %d\n",
		t.characterName(c.ID), c.ID, c.BaseStrength, c.BaseDexterity, c.BaseIntelligence, c.BaseMaxLife, c.BaseMaxMana)
	m := d.StatMap(c.Stats)
	for _, k := range slices.Sorted(maps.Keys(m)) {
		p.printf("  %s = %d\n", k, m[k])
	}
	return p.err
}

func percent(kept, total int) string {
	if total == 0 {
		return "n/a"
	}
	return fmt.Sprintf("%.2f%%", 100*float64(kept)/float64(total))
}

func diff(w io.Writer, a, b *dataset.Dataset, limit int) error {
	p := &printer{w: w}
	ta, err := newTexter(a)
	if err != nil {
		return err
	}
	tb, err := newTexter(b)
	if err != nil {
		return err
	}
	d := dataset.Compare(a, b)
	p.printf("%s -> %s\n", a.Manifest.Version, b.Manifest.Version)
	p.printf("stats: %d -> %d, %d added, %d removed; ids kept: %s\n",
		len(a.Stats), len(b.Stats), len(d.StatsAdded), len(d.StatsRemoved), percent(len(a.Stats)-len(d.StatsRemoved), len(a.Stats)))
	p.printf("characters changed: %s\n", list(d.CharactersChanged))
	p.printf("constants added: %s; removed: %s; changed: %s\n", list(d.ConstantsAdded), list(d.ConstantsRemoved), list(d.ConstantsChanged))
	for _, x := range d.Texts {
		p.printf("text %s: description scopes changed: %s; names changed: %d nodes, %d characters, %d ascendancies\n",
			x.Language, list(x.ScopesChanged), len(x.NodeNamesChanged), len(x.CharacterNamesChanged), len(x.AscendancyNamesChanged))
	}
	if len(d.TreeVariantsAdded)+len(d.TreeVariantsRemoved) > 0 {
		p.printf("tree variants added: %s; removed: %s\n", list(d.TreeVariantsAdded), list(d.TreeVariantsRemoved))
	}
	for _, t := range d.Trees {
		total := t.NodesCompared + len(t.Removed)
		p.printf("tree %s: %d nodes added, %d removed, %d changed; node ids kept: %s; mastery effects %d added, %d removed\n",
			t.Variant, len(t.Added), len(t.Removed), len(t.Changed), percent(t.NodesCompared, total), len(t.EffectsAdded), len(t.EffectsRemoved))
		printChangedNodes(p, t, ta, tb, limit)
	}
	return p.err
}

func printChangedNodes(p *printer, t dataset.TreeDiff, ta, tb texter, limit int) {
	for i, n := range t.Changed {
		if i == limit {
			p.printf("  ... %d more\n", len(t.Changed)-limit)
			return
		}
		p.printf("  %d %s [%s]\n", n.ID, tb.nodes[n.ID], strings.Join(n.Fields, ", "))
		if n.StatsBefore != nil {
			printStatChanges(p, ta.lines(treeScope, n.StatsBefore), tb.lines(treeScope, n.StatsAfter))
		}
	}
}

func printStatChanges(p *printer, before, after []string) {
	for _, l := range before {
		if !slices.Contains(after, l) {
			p.printf("    - %s\n", l)
		}
	}
	for _, l := range after {
		if !slices.Contains(before, l) {
			p.printf("    + %s\n", l)
		}
	}
}

// printer writes formatted output and keeps the first write error.
type printer struct {
	w   io.Writer
	err error
}

func (p *printer) printf(format string, args ...any) {
	if p.err == nil {
		_, p.err = fmt.Fprintf(p.w, format, args...)
	}
}

func list(s []string) string {
	if len(s) == 0 {
		return "none"
	}
	return strings.Join(s, ", ")
}
