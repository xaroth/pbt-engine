// Package items reads item text into the items a build holds, with one dataset's bases, mods
// and stat descriptions.
package items

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/statdesc"
)

// Errors of Parse; match them with errors.Is.
var (
	ErrFormat      = errors.New("items: not item text")
	ErrUnknownBase = errors.New("items: no base of that name")
	// ErrNoCrafting is a dataset whose items group does not hold the bases' mod domains, the
	// bench crafts and the essence mods (Items.Crafting false), as one written before them.
	ErrNoCrafting = errors.New("items: dataset without mod domains, bench crafts and essence mods")
)

// Warning codes of Parse.
const (
	// LineUnread is a mod line no stat description reads; the item keeps its text, no stats.
	LineUnread = build.ItemLineUnread
	// LineAmbiguous is a mod line read in more than one way the item could have; the first in
	// the game's order is kept.
	LineAmbiguous build.Code = "item-line-ambiguous"
	// RollOutOfRange is a mod line whose values no mod of its kind the item can have rolls; it
	// is read as the stats of such a mod.
	RollOutOfRange build.Code = "item-roll-out-of-range"
	// NotCorrupted is an implicit line only a corruption implicit gives, on an item not marked
	// Corrupted; it is read as that implicit.
	NotCorrupted build.Code = "item-not-corrupted"
	// ValueAssumed is a mod line whose text does not show a value, such as a curse's level, and
	// whose possible mods differ on it; the value most of them share is kept, the highest on a
	// tie.
	ValueAssumed build.Code = "item-value-assumed"
	// RollAssumed is a mod line with a range of values, such as "(30-50)"; the highest is kept.
	RollAssumed build.Code = "item-roll-assumed"
)

// statScope is the description file mod lines are read with.
const statScope = "Metadata/StatDescriptions/stat_descriptions.txt"

// Reader reads item text with one dataset. It is safe for concurrent use.
type Reader struct {
	set      *statdesc.Set
	bases    map[string]*dataset.ItemBase
	byName   map[string][]*dataset.ItemBase
	mods     modIndex
	crafting crafting
	version  string
}

// crafting is where bench crafts and essences put their mods: item classes by mod id.
type crafting struct {
	bench, essence map[string][]string
}

func newCrafting(it *dataset.Items) crafting {
	c := crafting{bench: map[string][]string{}, essence: map[string][]string{}}
	for _, m := range it.BenchCrafts {
		c.bench[m.Mod] = m.Classes
	}
	for _, m := range it.EssenceMods {
		c.essence[m.Mod] = m.Classes
	}
	return c
}

// NewReader indexes the items group, the stats and the English stat descriptions of s. A
// dataset without the items group returns a *dataset.MissingGroupError, and one whose items
// group lacks the mod domains, bench crafts and essence mods fails with ErrNoCrafting.
func NewReader(s *dataset.Store) (*Reader, error) {
	it, err := s.Items()
	if err != nil {
		return nil, fmt.Errorf("items: %w", err)
	}
	if !it.Crafting {
		return nil, fmt.Errorf("%w (%s); rebuild it", ErrNoCrafting, s.Manifest().Version)
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
		mods: indexMods(mods, stats), crafting: newCrafting(it), version: s.Manifest().Version,
	}
	r.index(it.Bases, text.BaseNames)
	return r, nil
}

// Parse reads item text as Path of Building writes and copies it: "Rarity:", the name lines,
// properties such as "Quality: 20" up to "Implicits: N", then the mod lines, the first N of
// them implicit, each optionally tagged "{crafted}", "{fractured}" or "{enchant}". The base is
// found by its English name; a magic item's base is the longest base name in its name; of bases
// of one name, the one whose implicits read the item's implicit lines. Each mod
// line is read through the stat descriptions; a line that reads in several ways is read as one
// the item can have (see choose). A line nothing reads is a warning and stays without stats.
// Properties Path of Building adds for itself, such as "LevelReq" or "Prefix", are ignored. A
// Relic reads as a unique. It fails with ErrFormat on text without a rarity, name or implicit
// count, and ErrUnknownBase.
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

// index indexes bases by id and English name.
func (r *Reader) index(bases []dataset.ItemBase, names []dataset.Name) {
	for i := range bases {
		r.bases[bases[i].ID] = &bases[i]
	}
	for _, n := range names {
		if b, ok := r.bases[n.ID]; ok {
			r.byName[n.Name] = append(r.byName[n.Name], b)
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
	rd := r.choose(it, base, kind, readings)
	line := build.ModLine{Kind: kind, Text: strings.Join(texts[:rd.Lines], "\n")}
	var warnings []build.Warning
	if len(rd.others) > 0 {
		warnings = append(warnings, build.Warning{
			Code: LineAmbiguous, Subject: line.Text,
			Message: fmt.Sprintf("read as %s, could also be %s", strings.Join(rd.Stats, ", "), strings.Join(rd.others, "; ")),
		})
	}
	for _, w := range rd.warnings {
		w.Subject = line.Text
		warnings = append(warnings, w)
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
		// Path of Building names bases of one name by their implicits, as in
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
// item's implicit lines with their values in range, then the most with any values; the first in
// the game's order on a tie. Bases of one name differ in their implicits, such as the three
// Two-Stone Rings, or are kept for another game mode.
func (r *Reader) byImplicits(bases []*dataset.ItemBase, p parsed) *dataset.ItemBase {
	var readings [][]statdesc.RangeReading
	for _, l := range p.lines {
		if l.kind != build.Implicit {
			continue
		}
		if rs, err := r.set.ReadRange(statScope, []string{l.text}); err == nil {
			readings = append(readings, rs)
		}
	}
	best, bestScore := bases[0], [2]int{-1, -1}
	for _, b := range bases {
		if score := r.implicitsRead(b, readings); score[0] > bestScore[0] || score[0] == bestScore[0] && score[1] > bestScore[1] {
			best, bestScore = b, score
		}
	}
	return best
}

// implicitsRead counts the lines, by their readings, that an implicit mod of b reads with the
// values in range, and with any values.
func (r *Reader) implicitsRead(b *dataset.ItemBase, readings [][]statdesc.RangeReading) [2]int {
	ofBase := func(m *dataset.Mod) bool { return slices.Contains(b.Implicits, m.ID) }
	var score [2]int
	for _, rs := range readings {
		if slices.ContainsFunc(rs, func(rd statdesc.RangeReading) bool { return r.gives(rd, true, ofBase) }) {
			score[0]++
		}
		if slices.ContainsFunc(rs, func(rd statdesc.RangeReading) bool { return r.gives(rd, false, ofBase) }) {
			score[1]++
		}
	}
	return score
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
