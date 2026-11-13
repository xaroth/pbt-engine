package items

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/xaroth/pbt-engine/build"
)

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
	if p.item.Rarity == "relic" {
		// A Relic is a unique of another look; its mods are the unique's.
		p.item.Rarity = build.Unique
	}
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
