package dataset

import (
	"cmp"
	"fmt"
	"slices"

	"github.com/xaroth/pbt-engine/statdesc"
)

// maxDescriptionStats is what a handler's byte stat index can address.
const maxDescriptionStats = 256

// validate checks d. allTrees says every tree variant is loaded; text names of nodes are
// checked against the trees only then, since names are shared across variants.
func (d *Dataset) validate(allTrees bool) error {
	n := len(d.Stats)
	if err := validateCharacters(n, d.Characters); err != nil {
		return err
	}
	if err := validateConstants(d.Constants); err != nil {
		return err
	}
	variants := map[string]bool{}
	for _, t := range d.Trees {
		if variants[t.Variant] {
			return errVariant(t.Variant)
		}
		variants[t.Variant] = true
		if err := validateTree(n, d.Characters, t); err != nil {
			return err
		}
	}
	langs := map[string]bool{}
	for _, t := range d.Texts {
		if langs[t.Language] {
			return errLanguage(t.Language)
		}
		langs[t.Language] = true
		if err := validateText(d.Characters, t); err != nil {
			return err
		}
	}
	return validateNames(d.Texts, d.Trees, allTrees)
}

func errVariant(v string) error {
	return fmt.Errorf("tree variant %q: want a tag of a-z, 0-9 and -, used once: %w", v, ErrInvalid)
}

func errLanguage(l string) error {
	return fmt.Errorf("text language %q: want a tag of a-z, 0-9 and -, used once: %w", l, ErrInvalid)
}

func statsInRange(what string, svs []StatValue, n int) error {
	for _, sv := range svs {
		if int(sv.Stat) >= n {
			return fmt.Errorf("%s: stat %d out of range (%d stats): %w", what, sv.Stat, n, ErrInvalid)
		}
	}
	return nil
}

func sortedStats(what string, svs []StatValue, n int) error {
	for i := 1; i < len(svs); i++ {
		if svs[i-1].Stat >= svs[i].Stat {
			return fmt.Errorf("%s: stats not sorted or repeated: %w", what, ErrInvalid)
		}
	}
	return statsInRange(what, svs, n)
}

// validateCharacters checks characters against a stats group of n stats.
func validateCharacters(n int, chars []Character) error {
	for _, c := range chars {
		if err := sortedStats("character "+c.ID, c.Stats, n); err != nil {
			return err
		}
	}
	return nil
}

func validateConstants(cs []Constant) error {
	for _, c := range cs {
		if c.Per == 0 {
			return fmt.Errorf("constant %s: per is 0: %w", c.ID, ErrInvalid)
		}
	}
	return nil
}

// validateTree checks t against a stats group of n stats and the characters.
func validateTree(n int, chars []Character, t Tree) error {
	if !validTag(t.Variant) {
		return errVariant(t.Variant)
	}
	if !slices.IsSortedFunc(t.Nodes, func(a, b Node) int { return cmp.Compare(a.ID, b.ID) }) {
		return fmt.Errorf("tree %s: nodes not sorted: %w", t.Variant, ErrInvalid)
	}
	ids := make(map[uint32]bool, len(t.Nodes))
	for _, nd := range t.Nodes {
		if ids[nd.ID] {
			return fmt.Errorf("tree %s: node %d repeated: %w", t.Variant, nd.ID, ErrInvalid)
		}
		ids[nd.ID] = true
	}
	for _, nd := range t.Nodes {
		what := fmt.Sprintf("tree %s node %d", t.Variant, nd.ID)
		if err := sortedStats(what, nd.Stats, n); err != nil {
			return err
		}
		for _, b := range nd.Buffs {
			if err := statsInRange(what+" buff", b, n); err != nil {
				return err
			}
		}
		for _, e := range nd.MasteryEffects {
			if err := statsInRange(what+" mastery effect", e.Stats, n); err != nil {
				return err
			}
		}
		if !slices.IsSorted(nd.Neighbours) {
			return fmt.Errorf("%s: neighbours not sorted: %w", what, ErrInvalid)
		}
		for _, nb := range nd.Neighbours {
			if !ids[nb] {
				return fmt.Errorf("%s: neighbour %d not in tree: %w", what, nb, ErrInvalid)
			}
		}
	}
	for _, c := range t.Classes {
		if !ids[c.StartNode] {
			return fmt.Errorf("tree %s class %s: start node %d not in tree: %w", t.Variant, c.Character, c.StartNode, ErrInvalid)
		}
		if !slices.ContainsFunc(chars, func(ch Character) bool { return ch.ID == c.Character }) {
			return fmt.Errorf("tree %s: class character %s not in characters: %w", t.Variant, c.Character, ErrInvalid)
		}
	}
	for _, j := range t.JewelSlots {
		if !ids[j] {
			return fmt.Errorf("tree %s: jewel slot %d not in tree: %w", t.Variant, j, ErrInvalid)
		}
	}
	return nil
}

// validTag reports whether s is safe as part of a file name: a-z, 0-9 and inner dashes.
func validTag(s string) bool {
	if s == "" || s[0] == '-' || s[len(s)-1] == '-' {
		return false
	}
	for _, c := range s {
		if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '-' {
			return false
		}
	}
	return true
}

// validateText checks t on its own and against the characters; validateNames checks it
// against trees.
func validateText(chars []Character, t Text) error {
	if !validTag(t.Language) {
		return errLanguage(t.Language)
	}
	if _, err := statdesc.NewSet(t.Descriptions); err != nil {
		return fmt.Errorf("text %s descriptions: %w", t.Language, err)
	}
	for _, sc := range t.Descriptions {
		for _, desc := range sc.Descriptions {
			if len(desc.Stats) > maxDescriptionStats {
				return fmt.Errorf("text %s scope %s: %d stats in one description, the file holds at most %d: %w", t.Language, sc.Name, len(desc.Stats), maxDescriptionStats, ErrInvalid)
			}
		}
	}
	for i, n := range t.NodeNames {
		if i > 0 && t.NodeNames[i-1].ID >= n.ID {
			return fmt.Errorf("text %s: node names not sorted or repeated: %w", t.Language, ErrInvalid)
		}
	}
	for _, n := range t.CharacterNames {
		if !slices.ContainsFunc(chars, func(c Character) bool { return c.ID == n.ID }) {
			return fmt.Errorf("text %s: name for unknown character %s: %w", t.Language, n.ID, ErrInvalid)
		}
	}
	return nil
}

// validateNames checks text names of ascendancies against the trees given, and names of nodes
// only when allTrees says those are every tree of the dataset.
func validateNames(texts []Text, trees []Tree, allTrees bool) error {
	nodes := map[uint32]bool{}
	ascendancies := map[string]bool{} // "<variant>/<id>"
	variants := map[string]bool{}
	for _, t := range trees {
		variants[t.Variant] = true
		for _, n := range t.Nodes {
			nodes[n.ID] = true
		}
		for _, c := range t.Classes {
			for _, a := range c.Ascendancies {
				ascendancies[t.Variant+"/"+a.ID] = true
			}
		}
		for _, a := range t.AlternateAscendancies {
			ascendancies[t.Variant+"/"+a.ID] = true
		}
	}
	for _, t := range texts {
		if allTrees {
			for _, n := range t.NodeNames {
				if !nodes[n.ID] {
					return fmt.Errorf("text %s: name for unknown node %d: %w", t.Language, n.ID, ErrInvalid)
				}
			}
		}
		for _, n := range t.AscendancyNames {
			if !allTrees && !variants[n.Variant] {
				continue // tree not loaded
			}
			if !ascendancies[n.Variant+"/"+n.ID] {
				return fmt.Errorf("text %s: name for unknown ascendancy %s in tree %s: %w", t.Language, n.ID, n.Variant, ErrInvalid)
			}
		}
	}
	return nil
}
