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
	if _, err := statdesc.NewSet(d.Descriptions); err != nil {
		return fmt.Errorf("descriptions: %w", err)
	}
	for _, sc := range d.Descriptions {
		for _, desc := range sc.Descriptions {
			if len(desc.Stats) > 256 {
				return fmt.Errorf("descriptions %s: %d stats in one description, the file holds at most 256", sc.Name, len(desc.Stats))
			}
		}
	}
	for _, c := range d.Constants {
		if c.Per == 0 {
			return fmt.Errorf("constant %s: per is 0", c.ID)
		}
	}

	variants := map[string]bool{}
	for _, t := range d.Trees {
		if t.Variant == "" || variants[t.Variant] {
			return fmt.Errorf("tree variant %q empty or repeated", t.Variant)
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
				return fmt.Errorf("tree %s class %s: start node %d not in tree", t.Variant, c.Name, c.StartNode)
			}
		}
		for _, j := range t.JewelSlots {
			if !has(j) {
				return fmt.Errorf("tree %s: jewel slot %d not in tree", t.Variant, j)
			}
		}
	}
	return nil
}
