// Command pbt inspects pbt datasets.
//
//	pbt data show node <id> --version <v> [--variant standard]
//	pbt data show stat <id> --version <v>
//	pbt data show character <name> --version <v>
//	pbt data diff <version-a> <version-b> [--limit 20]
//	pbt build check <build.json>
//	pbt build resolve <build.json>
//	pbt semantics coverage --version <v> [--variant standard]
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
		opts[name] = val
	}
	return pos, opts, nil
}

// run writes results to w and notes about how they were found to notes.
func run(args []string, w, notes io.Writer) error {
	pos, opts, err := parseArgs(args, map[string]string{
		"version": "", "variant": build.DefaultVariant, "root": "datasets", "limit": "20", "language": "en", "game": "poe1",
	})
	if err != nil {
		return err
	}
	switch {
	case len(pos) == 4 && pos[0] == "data" && pos[1] == "show":
		if opts["version"] == "" {
			return errVersionRequired
		}
		var variants []string // stats and characters need no tree
		if pos[2] == "node" {
			variants = []string{opts["variant"]}
		}
		res, err := resolveVersions(notes, opts["root"], opts["game"], opts["version"])
		if err != nil {
			return err
		}
		d, err := read(res[0].Store, variants, opts["language"])
		if err != nil {
			return err
		}
		switch pos[2] {
		case "node":
			return showNode(w, d, opts["variant"], pos[3])
		case "stat":
			return showStat(w, d, pos[3])
		case "character":
			return showCharacter(w, d, pos[3])
		}
	case len(pos) == 4 && pos[0] == "data" && pos[1] == "diff":
		limit, err := strconv.Atoi(opts["limit"])
		if err != nil || limit < 0 {
			return fmt.Errorf("--limit %q: %w", opts["limit"], errLimit)
		}
		res, err := resolveVersions(notes, opts["root"], opts["game"], pos[2], pos[3])
		if err != nil {
			return err
		}
		sa, sb := res[0].Store, res[1].Store
		a, err := read(sa, sa.Manifest().Trees(), opts["language"])
		if err != nil {
			return err
		}
		b, err := read(sb, sb.Manifest().Trees(), opts["language"])
		if err != nil {
			return err
		}
		return diff(w, a, b, limit)
	case len(pos) == 3 && pos[0] == "build" && pos[1] == "check":
		c, err := checkBuild(notes, opts["root"], pos[2])
		if err != nil {
			return err
		}
		p := &printer{w: w}
		b := c.Build()
		p.printf("ok: %s %s (dataset %s), %s tree, %s %s, level %d, %d nodes\n", b.Game, b.Version, c.Store().Manifest().Version,
			b.Variant, b.Class, cmp.Or(b.Ascendancy, "no ascendancy"), b.Level, len(b.Nodes))
		return p.err
	case len(pos) == 3 && pos[0] == "build" && pos[1] == "resolve":
		c, err := checkBuild(w, opts["root"], pos[2])
		if err != nil {
			return err
		}
		r, err := resolve.Resolve(c)
		if err != nil {
			return fmt.Errorf("%s: %w", pos[2], err)
		}
		if err := r.Dump(w); err != nil {
			return fmt.Errorf("dump: %w", err)
		}
		return nil
	case len(pos) == 2 && pos[0] == "semantics" && pos[1] == "coverage":
		res, err := resolveVersions(notes, opts["root"], opts["game"], opts["version"])
		if err != nil {
			return err
		}
		return coverage(w, res[0].Store, opts["variant"])
	}
	return errUsage
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
	c, err := build.Check(b, res[0].Store)
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
	id, err := strconv.ParseUint(idArg, 10, 32)
	if err != nil {
		return fmt.Errorf("node id %q: %w", idArg, err)
	}
	ti := slices.IndexFunc(d.Trees, func(t dataset.Tree) bool { return t.Variant == variant })
	if ti < 0 {
		return fmt.Errorf("no %s tree in %s: %w", variant, d.Manifest.Version, errNotFound)
	}
	nodes := d.Trees[ti].Nodes
	ni, ok := slices.BinarySearchFunc(nodes, uint32(id), func(n dataset.Node, id uint32) int { return int(n.ID) - int(id) })
	if !ok {
		return fmt.Errorf("no node %d in the %s %s tree: %w", id, d.Manifest.Version, variant, errNotFound)
	}
	n := nodes[ni]
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
		for i, n := range t.Changed {
			if i == limit {
				p.printf("  ... %d more\n", len(t.Changed)-limit)
				break
			}
			p.printf("  %d %s [%s]\n", n.ID, tb.nodes[n.ID], strings.Join(n.Fields, ", "))
			if n.StatsBefore == nil {
				continue
			}
			before := ta.lines(treeScope, n.StatsBefore)
			after := tb.lines(treeScope, n.StatsAfter)
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
	}
	return p.err
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
