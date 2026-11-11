// Package items reads item text into the items a build holds, with one dataset's bases, mods
// and stat descriptions.
package items

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/statdesc"
)

// Errors of Parse; match them with errors.Is.
var (
	ErrFormat      = errors.New("items: not item text")
	ErrUnknownBase = errors.New("items: no base of that name")
)

// Warning codes of Parse.
const (
	// LineUnread is a mod line no stat description reads; the item keeps its text, no stats.
	LineUnread = build.ItemLineUnread
	// LineAmbiguous is a mod line read in more than one way the item could have; the first in
	// the game's order is kept.
	LineAmbiguous build.Code = "item-line-ambiguous"
	// RollAssumed is a mod line with a range of values, such as "(30-50)"; the highest is kept.
	RollAssumed build.Code = "item-roll-assumed"
)

// statScope is the description file mod lines are read with.
const statScope = "Metadata/StatDescriptions/stat_descriptions.txt"

// Reader reads item text with one dataset. It is safe for concurrent use.
type Reader struct {
	set    *statdesc.Set
	bases  map[string]*dataset.ItemBase
	byName map[string][]*dataset.ItemBase
	// mods are by stat id, each mod once per stat it has.
	mods      map[string][]modStat
	firstStat map[*dataset.Mod]string
	version   string
}

// modStat is a mod with the range of one of its stats.
type modStat struct {
	mod      *dataset.Mod
	min, max int32
}

// NewReader indexes the items group, the stats and the English stat descriptions of s. A
// dataset without the items group returns a *dataset.MissingGroupError.
func NewReader(s *dataset.Store) (*Reader, error) {
	it, err := s.Items()
	if err != nil {
		return nil, fmt.Errorf("items: %w", err)
	}
	mods, err := s.Mods()
	if err != nil {
		return nil, fmt.Errorf("items: %w", err)
	}
	stats, err := s.Stats()
	if err != nil {
		return nil, fmt.Errorf("items: %w", err)
	}
	text, err := s.Text("en")
	if err != nil {
		return nil, fmt.Errorf("items: %w", err)
	}
	set, err := statdesc.NewSet(text.Descriptions)
	if err != nil {
		return nil, fmt.Errorf("items: %w", err)
	}
	r := &Reader{
		set: set, bases: map[string]*dataset.ItemBase{}, byName: map[string][]*dataset.ItemBase{},
		mods: map[string][]modStat{}, firstStat: map[*dataset.Mod]string{}, version: s.Manifest().Version,
	}
	r.index(it.Bases, text.BaseNames, mods, stats)
	return r, nil
}

// Parse reads item text as Path of Building writes and copies it: "Rarity:", the name lines,
// properties such as "Quality: 20" up to "Implicits: N", then the mod lines, the first N of
// them implicit, each optionally tagged "{crafted}", "{fractured}" or "{enchant}". The base is
// found by its English name; a magic item's base is the longest base name in its name. Each mod
// line is read through the stat descriptions; a line that reads in several ways is read as one
// the item can have (see choose). A line nothing reads is a warning and stays without stats.
// Properties Path of Building adds for itself, such as "LevelReq" or "Prefix", are ignored. It
// fails with ErrFormat on text without a rarity, name or implicit count, and ErrUnknownBase.
func (r *Reader) Parse(text string) (build.Item, []build.Warning, error) {
	p, err := split(text)
	if err != nil {
		return build.Item{}, nil, err
	}
	base, err := r.base(p)
	if err != nil {
		return build.Item{}, nil, err
	}
	it := p.item
	it.Base = base.ID
	var warnings []build.Warning
	for i := 0; i < len(p.lines); {
		// A description spanning lines does not cross from one kind of line to another.
		end := i + 1
		for end < len(p.lines) && p.lines[end].kind == p.lines[i].kind {
			end++
		}
		texts := make([]string, end-i)
		for k := range texts {
			texts[k] = p.lines[i+k].text
		}
		line, ws, n, err := r.readLine(&it, base, p.lines[i].kind, texts)
		if err != nil {
			return build.Item{}, nil, err
		}
		it.Mods = append(it.Mods, line)
		warnings = append(warnings, ws...)
		i += n
	}
	return it, warnings, nil
}

// index indexes bases by id and English name, and mods by stat id.
func (r *Reader) index(bases []dataset.ItemBase, names []dataset.Name, mods []dataset.Mod, stats []dataset.Stat) {
	for i := range bases {
		r.bases[bases[i].ID] = &bases[i]
	}
	for _, n := range names {
		if b, ok := r.bases[n.ID]; ok {
			r.byName[n.Name] = append(r.byName[n.Name], b)
		}
	}
	for i := range mods {
		if len(mods[i].Stats) > 0 {
			r.firstStat[&mods[i]] = stats[mods[i].Stats[0].Stat].ID
		}
		for _, st := range mods[i].Stats {
			id := stats[st.Stat].ID
			r.mods[id] = append(r.mods[id], modStat{mod: &mods[i], min: min(st.Min, st.Max), max: max(st.Min, st.Max)})
		}
	}
}

// readLine reads the mod line at the start of texts, all of kind, and returns it with how many
// lines it spans.
func (r *Reader) readLine(it *build.Item, base *dataset.ItemBase, kind build.ModKind, texts []string) (build.ModLine, []build.Warning, int, error) {
	readings, err := r.set.ReadRange(statScope, texts)
	if err != nil {
		return build.ModLine{}, nil, 0, fmt.Errorf("items: %w", err)
	}
	if len(readings) == 0 {
		w := build.Warning{Code: LineUnread, Subject: texts[0], Message: "no stat description reads it; left out"}
		return build.ModLine{Kind: kind, Text: texts[0]}, []build.Warning{w}, 1, nil
	}
	rd, others := r.choose(it, base, kind, readings)
	line := build.ModLine{Kind: kind, Text: strings.Join(texts[:rd.Lines], "\n")}
	var warnings []build.Warning
	if len(others) > 0 {
		warnings = append(warnings, build.Warning{
			Code: LineAmbiguous, Subject: line.Text,
			Message: fmt.Sprintf("read as %s, could also be %s", strings.Join(rd.Stats, ", "), strings.Join(others, "; ")),
		})
	}
	ranged := false
	for k, st := range rd.Stats {
		if rd.Max[k] == 0 {
			continue
		}
		ranged = ranged || rd.Min[k] != rd.Max[k]
		line.Stats = append(line.Stats, build.StatValue{Stat: st, Value: rd.Max[k]})
	}
	if ranged {
		warnings = append(warnings, build.Warning{Code: RollAssumed, Subject: line.Text, Message: "a range of values; the highest used"})
	}
	return line, warnings, rd.Lines, nil
}

// base returns the item's base by the name in its text.
func (r *Reader) base(p parsed) (*dataset.ItemBase, error) {
	name := strings.TrimPrefix(p.baseName, "Superior ")
	if p.item.Rarity == build.Magic {
		name = r.longestBaseName(name)
	}
	bases := r.byName[name]
	if len(bases) == 0 {
		// Path of Building tells bases of one name apart by their implicits, as in
		// "Two-Stone Ring (Fire/Cold)".
		if i := strings.LastIndex(name, " ("); i > 0 && strings.HasSuffix(name, ")") {
			bases = r.byName[name[:i]]
		}
	}
	if len(bases) == 0 {
		return nil, fmt.Errorf("%w: %q in %s", ErrUnknownBase, p.baseName, r.version)
	}
	return r.byImplicits(bases, p), nil
}

// byImplicits returns the base among bases of one name whose implicit mods read the most of the
// item's implicit lines, the first in the game's order on a tie.
func (r *Reader) byImplicits(bases []*dataset.ItemBase, p parsed) *dataset.ItemBase {
	best, most := bases[0], 0
	for _, b := range bases[1:] {
		if n := r.implicitsRead(b, p); n > most {
			best, most = b, n
		}
	}
	if most > 0 && r.implicitsRead(bases[0], p) >= most {
		return bases[0]
	}
	return best
}

// implicitsRead counts the item's implicit lines that read as an implicit mod of b.
func (r *Reader) implicitsRead(b *dataset.ItemBase, p parsed) int {
	ofBase := func(m *dataset.Mod) bool { return slices.Contains(b.Implicits, m.ID) }
	n := 0
	for _, l := range p.lines {
		if l.kind != build.Implicit {
			continue
		}
		rs, err := r.set.ReadRange(statScope, []string{l.text})
		if err == nil && slices.ContainsFunc(rs, func(rd statdesc.RangeReading) bool { return r.fits(rd, true, ofBase) }) {
			n++
		}
	}
	return n
}

// longestBaseName returns the longest base name in a magic item's name, or "".
func (r *Reader) longestBaseName(name string) string {
	best := ""
	for n := range r.byName {
		if n != "" && containsWord(name, n) && (len(n) > len(best) || len(n) == len(best) && n < best) {
			best = n
		}
	}
	return best
}

// containsWord reports whether s contains w with no letter on either side.
func containsWord(s, w string) bool {
	for i := 0; ; {
		j := strings.Index(s[i:], w)
		if j < 0 {
			return false
		}
		start, end := i+j, i+j+len(w)
		if (start == 0 || s[start-1] == ' ') && (end == len(s) || s[end] == ' ') {
			return true
		}
		i = start + 1
	}
}

// parsed is item text split into its parts.
type parsed struct {
	item     build.Item
	baseName string
	lines    []modText
}

type modText struct {
	kind build.ModKind
	text string
}

// flagLines are lines that name a property of the item rather than a mod.
func flagLines() []string {
	return []string{"Corrupted", "Mirrored", "Split", "Unidentified", "Synthesised Item", "Fractured Item"}
}

func split(text string) (parsed, error) {
	var lines []string
	for l := range strings.Lines(text) {
		if l = strings.TrimSpace(l); l != "" {
			lines = append(lines, l)
		}
	}
	var p parsed
	names, err := p.names(lines)
	if err != nil {
		return p, err
	}
	implicits := -1
	i := 1 + names
	for ; i < len(lines) && implicits < 0; i++ {
		implicits = p.property(lines[i])
	}
	if implicits < 0 {
		return p, fmt.Errorf("%w: no implicit count", ErrFormat)
	}
	p.modLines(lines[i:], implicits)
	return p, nil
}

// names reads the rarity and the name lines, and returns how many name lines there are.
func (p *parsed) names(lines []string) (int, error) {
	first := ""
	if len(lines) > 0 {
		first = lines[0]
	}
	rarity, ok := strings.CutPrefix(first, "Rarity: ")
	if !ok {
		return 0, fmt.Errorf("%w: no rarity line", ErrFormat)
	}
	p.item.Rarity = build.Rarity(strings.ToLower(rarity))
	names := 1
	switch p.item.Rarity {
	case build.Rare, build.Unique:
		names = 2
	case build.Magic, build.Normal:
	default:
		return 0, fmt.Errorf("%w: rarity %q", ErrFormat, rarity)
	}
	if len(lines) < 1+names {
		return 0, fmt.Errorf("%w: no name", ErrFormat)
	}
	p.baseName = lines[names]
	if p.item.Rarity != build.Normal {
		p.item.Name = lines[1]
	}
	return names, nil
}

// modLines reads the lines after the implicit count, the first implicits of them implicit.
func (p *parsed) modLines(lines []string, implicits int) {
	for k, l := range lines {
		if slices.Contains(flagLines(), l) {
			p.item.Flags = append(p.item.Flags, l)
			continue
		}
		kind, text := tagged(l)
		if k < implicits && kind == build.Explicit {
			kind = build.Implicit
		}
		p.lines = append(p.lines, modText{kind: kind, text: text})
	}
}

// property reads one property line into the item and returns the implicit count when the line
// is "Implicits: N", else -1.
func (p *parsed) property(l string) int {
	key, val, ok := strings.Cut(l, ": ")
	if !ok {
		if strings.HasSuffix(l, " Item") {
			p.item.Flags = append(p.item.Flags, l)
		}
		return -1
	}
	n, _ := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(val, "+"), "%"))
	if key == "Implicits" {
		return max(n, 0)
	}
	if key == "Sockets" {
		p.item.Sockets = val
	}
	fields := map[string]*int{
		"Item Level": &p.item.ItemLevel, "Quality": &p.item.Quality, "Armour": &p.item.Armour,
		"Evasion": &p.item.Evasion, "Energy Shield": &p.item.EnergyShield, "Ward": &p.item.Ward,
	}
	if f, ok := fields[key]; ok {
		*f = n
	}
	return -1
}

// tagged strips the leading {tag} markers of a mod line and returns its kind by them.
func tagged(l string) (build.ModKind, string) {
	kind := build.Explicit
	for strings.HasPrefix(l, "{") {
		end := strings.IndexByte(l, '}')
		if end < 0 {
			break
		}
		switch tag, _, _ := strings.Cut(l[1:end], ":"); tag {
		case "crafted":
			kind = build.Crafted
		case "fractured":
			kind = build.Fractured
		case "enchant":
			kind = build.Enchant
		}
		l = l[end+1:]
	}
	return kind, l
}
