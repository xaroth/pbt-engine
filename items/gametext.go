package items

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/dataset"
)

// gameSeparator is the line between the sections of game item text.
const gameSeparator = "--------"

// isGameText reports whether text is item text as the game copies it: sections between
// separator lines.
func isGameText(text string) bool {
	for l := range strings.Lines(text) {
		if strings.TrimSpace(l) == gameSeparator {
			return true
		}
	}
	return false
}

// gameClass returns the item class name the game's text starts with, such as "Body Armours".
func gameClass(text string) (string, bool) {
	for l := range strings.Lines(text) {
		if l = strings.TrimSpace(l); l != "" {
			return strings.CutPrefix(l, "Item Class: ")
		}
	}
	return "", false
}

// unusable is the line the game puts after the rarity when the character cannot use the item.
const unusable = "You cannot use this item. Its stats will be ignored"

// splitGame splits item text as the game copies it, with Ctrl+C or with the mod headers of
// Ctrl+Alt+C. The first section holds the item class, the rarity and the names; the sections up
// to the item level hold properties; after it come the enchant, implicit and explicit
// sections and the flags. Mod lines are told apart by their headers, such as "{ Implicit
// Modifier }", or else by suffixes such as " (implicit)"; without headers the first other
// section after the item level holds the explicit mods. Sections after the mods, such as flavour
// text, are left out.
func splitGame(text string) (parsed, error) {
	var p parsed
	sections, err := p.gameHeader(gameSections(text))
	if err != nil {
		return p, err
	}
	headers := slices.ContainsFunc(sections, func(s []string) bool {
		return slices.ContainsFunc(s, isModHeader)
	})
	levelSeen, modsSeen := false, false
	for _, s := range sections {
		switch {
		case p.markedSection(s):
		case headers && isModHeader(s[0]), !headers && levelSeen && !modsSeen:
			p.gameMods(s, build.Explicit)
			modsSeen = true
		case !levelSeen:
			levelSeen = p.properties(s)
		}
	}
	return p, nil
}

// properties reads a section of property lines and reports whether it holds the item level.
func (p *parsed) properties(s []string) bool {
	level := false
	for _, l := range s {
		p.property(l)
		level = level || strings.HasPrefix(l, "Item Level: ")
	}
	return level
}

// markedSection reads a section whose every line says what it is: flags, enchants or
// implicits. It reports whether s was such a section.
func (p *parsed) markedSection(s []string) bool {
	switch {
	case allFlags(s):
		p.item.Flags = append(p.item.Flags, s...)
	case allSuffixed(s, " (enchant)"):
		p.gameMods(s, build.Enchant)
	case allSuffixed(s, " (implicit)"):
		p.gameMods(s, build.Implicit)
	default:
		return false
	}
	return true
}

// gameSections splits text into its sections' non-empty lines; empty sections are left out.
func gameSections(text string) [][]string {
	sections := [][]string{nil}
	for l := range strings.Lines(text) {
		switch l = strings.TrimSpace(l); l {
		case "":
		case gameSeparator:
			sections = append(sections, nil)
		default:
			sections[len(sections)-1] = append(sections[len(sections)-1], l)
		}
	}
	return slices.DeleteFunc(sections, func(s []string) bool { return len(s) == 0 })
}

// gameHeader reads the rarity and names from the first sections and returns the sections after
// them.
func (p *parsed) gameHeader(sections [][]string) ([][]string, error) {
	if len(sections) == 0 {
		return nil, fmt.Errorf("%w: no rarity line", ErrFormat)
	}
	header := slices.DeleteFunc(slices.Clone(sections[0]), func(l string) bool {
		return strings.HasPrefix(l, "Item Class: ") || l == unusable
	})
	sections = sections[1:]
	if len(header) == 1 && len(sections) > 0 {
		// The names follow the unusable line in a section of their own.
		header = append(header, sections[0]...)
		sections = sections[1:]
	}
	if len(header) == 2 && (strings.EqualFold(header[0], "Rarity: Rare") || strings.EqualFold(header[0], "Rarity: Unique")) {
		// An unidentified rare or unique shows its base only.
		header = []string{header[0], "", header[1]}
	}
	_, err := p.names(header)
	return sections, err
}

// gameMods reads the lines of a mod section, of kind unless a header or a suffix says otherwise.
func (p *parsed) gameMods(lines []string, kind build.ModKind) {
	reminder := false
	for _, l := range lines {
		switch {
		case reminder || strings.HasPrefix(l, "("):
			// Reminder text, as "(Maximum Resistances cannot be raised above 90%)", may wrap.
			reminder = !strings.HasSuffix(l, ")")
		case isModHeader(l):
			kind = headerKind(l)
		case slices.Contains(flagLines(), l):
			p.item.Flags = append(p.item.Flags, l)
		default:
			k, text := suffixed(l, kind)
			p.lines = append(p.lines, modText{kind: k, text: withoutRolls(text)})
		}
	}
}

// isModHeader reports whether l is a mod header of Ctrl+Alt+C, such as
// `{ Prefix Modifier "Virile" (Tier: 2) — Life }`.
func isModHeader(l string) bool {
	return strings.HasPrefix(l, "{ ") && strings.HasSuffix(l, " }")
}

// headerKind is the kind of the mod lines under header.
func headerKind(header string) build.ModKind {
	switch {
	case strings.Contains(header, "Implicit Modifier"):
		return build.Implicit
	case strings.HasPrefix(header, "{ Master Crafted "):
		return build.Crafted
	case strings.HasPrefix(header, "{ Fractured "):
		return build.Fractured
	}
	return build.Explicit
}

// suffixed strips the suffixes the game puts after a mod line and returns the line's kind by
// them, kind when none says.
func suffixed(l string, kind build.ModKind) (build.ModKind, string) {
	l = strings.TrimSuffix(l, " — Unscalable Value")
	for suffix, k := range map[string]build.ModKind{
		" (implicit)": build.Implicit, " (crafted)": build.Crafted, " (fractured)": build.Fractured, " (enchant)": build.Enchant,
	} {
		if text, ok := strings.CutSuffix(l, suffix); ok {
			return k, text
		}
	}
	return kind, l
}

// withoutRolls strips the range Ctrl+Alt+C shows after each value, as in "168(150-200)%".
func withoutRolls(l string) string {
	var b strings.Builder
	for {
		i := strings.IndexByte(l, '(')
		if i < 0 {
			b.WriteString(l)
			return b.String()
		}
		j := strings.IndexByte(l[i:], ')')
		if i > 0 && '0' <= l[i-1] && l[i-1] <= '9' && j > 0 && isRange(l[i+1:i+j]) {
			b.WriteString(l[:i])
			l = l[i+j+1:]
			continue
		}
		b.WriteString(l[:i+1])
		l = l[i+1:]
	}
}

// isRange reports whether s is a range of two numbers, such as "150-200" or "-10--5".
func isRange(s string) bool {
	if s == "" {
		return false
	}
	k := strings.IndexByte(s[1:], '-') + 1
	if k < 1 {
		return false
	}
	_, errA := strconv.ParseFloat(s[:k], 64)
	_, errB := strconv.ParseFloat(s[k+1:], 64)
	return errA == nil && errB == nil
}

func allFlags(s []string) bool {
	return !slices.ContainsFunc(s, func(l string) bool { return !slices.Contains(flagLines(), l) })
}

func allSuffixed(s []string, suffix string) bool {
	return !slices.ContainsFunc(s, func(l string) bool { return !strings.HasSuffix(l, suffix) })
}

// classes are the game's item class names a reader takes, and those it refuses for now.
type classes struct {
	equipable   map[string]bool
	unsupported map[string]bool
}

// check refuses an item class the game's text names that cannot be equipped or that the reader
// does not support yet.
func (c classes) check(name string) error {
	switch {
	case c.unsupported[name]:
		return fmt.Errorf("%w: %s", ErrNotSupported, name)
	case !c.equipable[name]:
		return fmt.Errorf("%w: %s", ErrNotEquipable, name)
	}
	return nil
}

// newClasses names the classes of bases as the game's text does; flask classes are not
// supported yet.
func newClasses(bases []dataset.ItemBase, names []dataset.Name) classes {
	flask := map[string]bool{}
	for _, b := range bases {
		flask[b.Class] = flask[b.Class] || b.Flask != nil
	}
	c := classes{equipable: map[string]bool{}, unsupported: map[string]bool{}}
	for _, n := range names {
		c.equipable[n.Name] = true
		c.unsupported[n.Name] = flask[n.ID]
	}
	return c
}
