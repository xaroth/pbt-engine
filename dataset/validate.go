package dataset

import (
	"cmp"
	"fmt"
	"slices"

	"github.com/xaroth/pbt-engine/statdesc"
)

func (d *Dataset) validate() error {
	n := uint32(len(d.Stats))
	inRange := func(what string, svs []StatValue) error {
		for _, sv := range svs {
			if sv.Stat >= n {
				return fmt.Errorf("%s: stat %d out of range (%d stats)", what, sv.Stat, n)
			}
		}
		return nil
	}
	sortedStats := func(what string, svs []StatValue) error {
		for i := 1; i < len(svs); i++ {
			if svs[i-1].Stat >= svs[i].Stat {
				return fmt.Errorf("%s: stats not sorted or repeated", what)
			}
		}
		return inRange(what, svs)
	}

	for _, c := range d.Characters {
		if err := sortedStats("character "+c.ID, c.Stats); err != nil {
			return err
		}
	}
	for _, c := range d.Constants {
		if c.Per == 0 {
			return fmt.Errorf("constant %s: per is 0", c.ID)
		}
	}

	variants := map[string]bool{}
	for _, t := range d.Trees {
		if !validTag(t.Variant) || variants[t.Variant] {
			return fmt.Errorf("tree variant %q: want a tag of a-z, 0-9 and -, used once", t.Variant)
		}
		variants[t.Variant] = true
		if !slices.IsSortedFunc(t.Nodes, func(a, b Node) int { return cmp.Compare(a.ID, b.ID) }) {
			return fmt.Errorf("tree %s: nodes not sorted", t.Variant)
		}
		ids := make(map[uint32]bool, len(t.Nodes))
		for _, nd := range t.Nodes {
			if ids[nd.ID] {
				return fmt.Errorf("tree %s: node %d repeated", t.Variant, nd.ID)
			}
			ids[nd.ID] = true
		}
		has := func(id uint32) bool { return ids[id] }
		for _, nd := range t.Nodes {
			what := fmt.Sprintf("tree %s node %d", t.Variant, nd.ID)
			if err := sortedStats(what, nd.Stats); err != nil {
				return err
			}
			for _, b := range nd.Buffs {
				if err := inRange(what+" buff", b); err != nil {
					return err
				}
			}
			for _, e := range nd.MasteryEffects {
				if err := inRange(what+" mastery effect", e.Stats); err != nil {
					return err
				}
			}
			if !slices.IsSorted(nd.Neighbours) {
				return fmt.Errorf("%s: neighbours not sorted", what)
			}
			for _, nb := range nd.Neighbours {
				if !has(nb) {
					return fmt.Errorf("%s: neighbour %d not in tree", what, nb)
				}
			}
		}
		for _, c := range t.Classes {
			if !has(c.StartNode) {
				return fmt.Errorf("tree %s class %s: start node %d not in tree", t.Variant, c.Character, c.StartNode)
			}
			if !slices.ContainsFunc(d.Characters, func(ch Character) bool { return ch.ID == c.Character }) {
				return fmt.Errorf("tree %s: class character %s not in characters", t.Variant, c.Character)
			}
		}
		for _, j := range t.JewelSlots {
			if !has(j) {
				return fmt.Errorf("tree %s: jewel slot %d not in tree", t.Variant, j)
			}
		}
	}
	return d.validateTexts()
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

func (d *Dataset) validateTexts() error {
	nodes := map[uint32]bool{}
	ascendancies := map[string]bool{} // "<variant>/<id>"
	for _, t := range d.Trees {
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
	characters := map[string]bool{}
	for _, c := range d.Characters {
		characters[c.ID] = true
	}
	langs := map[string]bool{}
	for _, t := range d.Texts {
		if !validTag(t.Language) || langs[t.Language] {
			return fmt.Errorf("text language %q: want a tag of a-z, 0-9 and -, used once", t.Language)
		}
		langs[t.Language] = true
		if _, err := statdesc.NewSet(t.Descriptions); err != nil {
			return fmt.Errorf("text %s descriptions: %w", t.Language, err)
		}
		for _, sc := range t.Descriptions {
			for _, desc := range sc.Descriptions {
				if len(desc.Stats) > 256 {
					return fmt.Errorf("text %s scope %s: %d stats in one description, the file holds at most 256", t.Language, sc.Name, len(desc.Stats))
				}
			}
		}
		for i, n := range t.NodeNames {
			if i > 0 && t.NodeNames[i-1].ID >= n.ID {
				return fmt.Errorf("text %s: node names not sorted or repeated", t.Language)
			}
			if !nodes[n.ID] {
				return fmt.Errorf("text %s: name for unknown node %d", t.Language, n.ID)
			}
		}
		for _, n := range t.CharacterNames {
			if !characters[n.ID] {
				return fmt.Errorf("text %s: name for unknown character %s", t.Language, n.ID)
			}
		}
		for _, n := range t.AscendancyNames {
			if !ascendancies[n.Variant+"/"+n.ID] {
				return fmt.Errorf("text %s: name for unknown ascendancy %s in tree %s", t.Language, n.ID, n.Variant)
			}
		}
	}
	return nil
}
