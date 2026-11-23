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
	// ErrNotEquipable is game text of an item class no slot takes, such as a map or currency.
	ErrNotEquipable = errors.New("items: not an item to equip")
	// ErrNotSupported is game text of an item class the reader does not read yet: flasks.
	ErrNotSupported = errors.New("items: item class not supported yet")
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
	// RollAssumed is a mod line with a range of values, such as "(30-50)", and no roll; the
	// highest is kept.
	RollAssumed build.Code = "item-roll-assumed"
)

// statScope is the description file mod lines are read with.
const statScope = "Metadata/StatDescriptions/stat_descriptions.txt"

// Reader reads item text with one dataset. It is safe for concurrent use.
type Reader struct {
	set      *statdesc.Set
	bases    map[string]*dataset.ItemBase
	byName   map[string][]*dataset.ItemBase // by folded name
	mods     modIndex
	crafting crafting
	classes  classes
	version  string
	// passiveStats are the stats a line shows as a passive's name. Each item has its own
	// passive, so the value is not a roll in its mod's range.
	passiveStats map[string]bool
	// passiveNames names nodes by id; ascendancyPassives gives the ascendancy passive, not a
	// start, of a name, which a line of build.AscendancyPassiveStats names where nodes of other
	// kinds share its name.
	passiveNames       map[int32]string
	ascendancyPassives map[string]int32
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

// NewReader indexes the items group, the stats, the English stat descriptions and the
// ascendancy passives of the standard tree of s. A dataset without the items group returns a
// *dataset.MissingGroupError, and one whose items group lacks the mod domains, bench crafts and
// essence mods fails with ErrNoCrafting.
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
	names := passiveNames(text.NodeNames)
	ascendancy, err := ascendancyPassives(s, names)
	if err != nil {
		return nil, err
	}
	timeless, err := timelessNames(s, text)
	if err != nil {
		return nil, err
	}
	set, err := statdesc.NewSet(text.Descriptions, append(timeless, statdesc.WithNames(passiveHandler, names))...)
	if err != nil {
		return nil, fmt.Errorf("items: %w", err)
	}
	r := &Reader{
		set: set, bases: map[string]*dataset.ItemBase{}, byName: map[string][]*dataset.ItemBase{},
		mods: indexMods(mods, stats, it.Bases), crafting: newCrafting(it), version: s.Manifest().Version,
		passiveStats: passiveStats(text.Descriptions), passiveNames: names, ascendancyPassives: ascendancy,
	}
	r.index(it.Bases, text.BaseNames)
	r.classes = newClasses(it.Bases, text.ItemClassNames)
	set.IndexTexts(statScope)
	return r, nil
}

type readerKey struct{}

// ReaderOf returns the reader of s, made by NewReader once per store and shared after, its
// error included.
func ReaderOf(s *dataset.Store) (*Reader, error) {
	//nolint:wrapcheck // NewReader wraps its errors
	return dataset.Derived(s, readerKey{}, NewReader)
}

// passiveHandler is the stat description handler that shows a passive's name for its node id,
// as in "Passive Skills in Radius of Supreme Ego can be Allocated".
const passiveHandler = "passive_hash"

// passiveStats returns the stats some description shows through passiveHandler.
func passiveStats(scopes []statdesc.Scope) map[string]bool {
	out := map[string]bool{}
	for _, sc := range scopes {
		for _, d := range sc.Descriptions {
			for _, v := range d.Variants {
				for _, h := range v.Handlers {
					if h.Name == passiveHandler {
						out[d.Stats[h.Stat]] = true
					}
				}
			}
		}
	}
	return out
}

func passiveNames(nodes []dataset.NodeName) map[int32]string {
	out := make(map[int32]string, len(nodes))
	for _, n := range nodes {
		out[int32(n.ID)] = n.Name //nolint:gosec // node ids are 16-bit hashes
	}
	return out
}

// ascendancyPassives returns the lowest ascendancy passive of each name in s's standard tree,
// other than the starts, by name; none without the tree. An Ascendant passive shares its name
// with an ascendancy's start, such as Guardian, or with a cluster jewel notable, such as
// Saboteur.
func ascendancyPassives(s *dataset.Store, names map[int32]string) (map[string]int32, error) {
	out := map[string]int32{}
	t, err := s.Tree(build.DefaultVariant)
	if errors.Is(err, dataset.ErrNotInDataset) {
		return out, nil
	}
	if err != nil {
		return nil, fmt.Errorf("items: %w", err)
	}
	for _, n := range t.Nodes {
		if n.Ascendancy == "" || n.Kind == dataset.AscendancyStart {
			continue
		}
		id := int32(n.ID) //nolint:gosec // node ids are 16-bit hashes
		if name, ok := names[id]; ok {
			if old, seen := out[name]; !seen || id < old {
				out[name] = id
			}
		}
	}
	return out, nil
}

// Parse reads item text as Path of Building writes and copies it, or as the game copies it
// with Ctrl+C or Ctrl+Alt+C.
//
// Path of Building's text is "Rarity:", the name lines, properties such as "Quality: 20" up to
// "Implicits: N", then the mod lines, the first N of them implicit, each optionally tagged
// "{crafted}", "{fractured}" or "{enchant}". A unique's lines tagged "{variant:n,...}" are read
// only when one of n is its "Selected Variant" or a selected alt variant, and a line tagged
// "{range:r}" has its ranges set at r (see withRoll). Properties Path of Building adds for itself,
// such as "LevelReq" or "Prefix", are ignored.
//
// The game's text is sections between "--------" lines: the item class, rarity and names, the
// properties, then the mods, whose kind its mod headers or line suffixes such as " (implicit)"
// give. The ranges Ctrl+Alt+C shows, reminder text in parentheses and flavour text are left
// out. Item classes no slot takes, such as maps and currency, fail with ErrNotEquipable, and
// flasks with ErrNotSupported.
//
// A Relic reads as a unique. The base is found by its English name, accents aside, as Path of
// Building drops them ("Maelstrom Staff"); a magic item's base is the longest base name in its
// name; of bases of one name, the one whose implicits read the item's implicit lines. Each mod
// line is read through the stat descriptions; a line that reads in several ways is read as one
// the item can have (see choose). A line nothing reads is a warning and stays without stats. It
// fails with ErrFormat on text without a rarity, name or, in Path of Building's text, implicit
// count, and ErrUnknownBase.
func (r *Reader) Parse(text string) (build.Item, []build.Warning, error) {
	p, err := r.split(text)
	if err != nil {
		return build.Item{}, nil, err
	}
	p.scaling.addMagnitudes(p.lines)
	base, err := r.base(p)
	if err != nil {
		return build.Item{}, nil, err
	}
	it := p.item
	it.Base = base.ID
	if c := p.scaling.catalyst; c != nil {
		it.Catalyst, it.CatalystQuality = c.name, p.scaling.catalystQuality()
	}
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
		sc := p.scaling
		if p.lines[i].unscalable {
			sc = scaling{}
		}
		line, ws, n, err := r.readLine(&it, base, p.lines[i].kind, texts, sc.scaler(p.lines[i].kind))
		if err != nil {
			return build.Item{}, nil, err
		}
		it.Mods = append(it.Mods, line)
		warnings = append(warnings, ws...)
		i += n
	}
	warnings = append(warnings, rollDefences(&it, base, p.game)...)
	return it, warnings, nil
}

// split splits text in either format.
func (r *Reader) split(text string) (parsed, error) {
	if !isGameText(text) {
		return split(text)
	}
	if class, ok := gameClass(text); ok {
		if err := r.classes.check(class); err != nil {
			return parsed{}, err
		}
	}
	return splitGame(text)
}

// index indexes bases by id and English name.
func (r *Reader) index(bases []dataset.ItemBase, names []dataset.Name) {
	for i := range bases {
		r.bases[bases[i].ID] = &bases[i]
	}
	for _, n := range names {
		if b, ok := r.bases[n.ID]; ok {
			r.byName[fold(n.Name)] = append(r.byName[fold(n.Name)], b)
		}
	}
}

// readLine reads the mod line at the start of texts, all of kind, its values scaled from its
// mods' ranges by scale, and returns it with how many lines it spans.
func (r *Reader) readLine(
	it *build.Item, base *dataset.ItemBase, kind build.ModKind, texts []string, scale scaler,
) (build.ModLine, []build.Warning, int, error) {
	readings, err := r.set.ReadRange(statScope, texts)
	if err != nil {
		return build.ModLine{}, nil, 0, fmt.Errorf("items: %w", err)
	}
	if len(readings) == 0 {
		w := build.Warning{Code: LineUnread, Subject: texts[0], Message: "no stat description reads it; left out"}
		return build.ModLine{Kind: kind, Text: texts[0]}, []build.Warning{w}, 1, nil
	}
	rd := r.choose(it, base, kind, readings, scale)
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
		line.Stats = append(line.Stats, build.StatValue{Stat: st, Value: r.ascendancyPassive(st, rd.Max[k])})
	}
	if ranged {
		warnings = append(warnings, build.Warning{Code: RollAssumed, Subject: line.Text, Message: "a range of values; the highest used"})
	}
	return line, warnings, rd.Lines, nil
}

// ascendancyPassive returns the ascendancy passive of the name the reading gave as node v, for a
// stat that names one, else v.
func (r *Reader) ascendancyPassive(stat string, v int32) int32 {
	if !slices.Contains(build.AscendancyPassiveStats(), stat) {
		return v
	}
	if id, ok := r.ascendancyPassives[r.passiveNames[v]]; ok {
		return id
	}
	return v
}

// base returns the item's base by the name in its text.
func (r *Reader) base(p parsed) (*dataset.ItemBase, error) {
	name := fold(strings.TrimPrefix(p.baseName, "Superior "))
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
// item's implicit lines with their values in range, then the most with any values, then that has
// the most of the defences the text prints; the first in the game's order on a tie. Bases of one
// name differ in their implicits, such as the three Two-Stone Rings, or in their defences too,
// such as the three Two-Toned Boots, or are kept for another game mode.
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
	best, bestScore := bases[0], [3]int{-1, -1, -1}
	scale := p.scaling.scaler(build.Implicit)
	for _, b := range bases {
		read := r.implicitsRead(b, readings, scale)
		if score := [3]int{read[0], read[1], printedDefences(b, &p.item)}; slices.Compare(score[:], bestScore[:]) > 0 {
			best, bestScore = b, score
		}
	}
	return best
}

// implicitsRead counts the lines, by their readings, that an implicit mod of b reads with the
// values in range, scaled by scale, and with any values.
func (r *Reader) implicitsRead(b *dataset.ItemBase, readings [][]statdesc.RangeReading, scale scaler) [2]int {
	ofBase := func(m *dataset.Mod) bool { return slices.Contains(b.Implicits, m.ID) }
	var score [2]int
	for _, rs := range readings {
		if slices.ContainsFunc(rs, func(rd statdesc.RangeReading) bool { return r.gives(rd, true, ofBase, scale) }) {
			score[0]++
		}
		if slices.ContainsFunc(rs, func(rd statdesc.RangeReading) bool { return r.gives(rd, false, ofBase, nil) }) {
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

// latin1ASCII is the ASCII letter of each Latin-1 letter from U+00C0, or 0 for one that has none.
const latin1ASCII = "AAAAAA\x00CEEEEIIIIDNOOOOO\x00OUUUUY\x00\x00aaaaaa\x00ceeeeiiiidnooooo\x00ouuuuy\x00y"

// fold drops the accents of Latin letters, as Path of Building writes the game's names: the
// game's "Maelström Staff" is its "Maelstrom Staff".
func fold(s string) string {
	return strings.Map(func(c rune) rune {
		if c >= 0xC0 && c <= 0xFF && latin1ASCII[c-0xC0] != 0 {
			return rune(latin1ASCII[c-0xC0])
		}
		return c
	}, s)
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
