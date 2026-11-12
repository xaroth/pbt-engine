package simulator

import (
	"cmp"
	"fmt"
	"slices"

	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/internal/poe1"
	"github.com/xaroth/pbt-engine/internal/resolve"
	"github.com/xaroth/pbt-engine/statdesc"
)

// Node is a passive tree node as its tooltip shows it, in English.
type Node struct {
	ID   uint32
	Name string
	Kind dataset.NodeKind
	// Ascendancy is the ascendancy the node belongs to, with its name; zero for other nodes.
	Ascendancy dataset.Name
	// Stats are the node's stat lines in the order of the stat description files, then those
	// of the auras it grants. A line holds a newline where one description renders as more
	// than one line.
	Stats []string
	// SkillPoints are the passive skill points the node grants. The tooltip's line for them,
	// "Grants 1 Passive Skill Point", has no stat description, so Stats leaves it out.
	SkillPoints int32
	// Effects are a mastery's effects, in the tree's order.
	Effects []MasteryEffect
}

// MasteryEffect is one effect a mastery offers, with its stat lines.
type MasteryEffect struct {
	ID    uint32
	Stats []string
}

// Class is a class of a tree variant with its ascendancies, in the tree's order, by id and
// English name.
type Class struct {
	// ID is the class's dataset.Character ID.
	ID           string
	Name         string
	Ascendancies []dataset.Name
}

// nodeText is what renders nodes: stat ids by handle, the English text and its descriptions.
type nodeText struct {
	stats []dataset.Stat
	text  *dataset.Text
	set   *statdesc.Set
	names map[uint32]string
}

func newNodeText(s *dataset.Store) (*nodeText, error) {
	stats, err := s.Stats()
	if err != nil {
		return nil, fmt.Errorf("stats: %w", err)
	}
	text, err := s.Text("en")
	if err != nil {
		return nil, fmt.Errorf("text: %w", err)
	}
	set, err := statdesc.NewSet(text.Descriptions)
	if err != nil {
		return nil, fmt.Errorf("stat descriptions: %w", err)
	}
	names := make(map[uint32]string, len(text.NodeNames))
	for _, n := range text.NodeNames {
		names[n.ID] = n.Name
	}
	return &nodeText{stats: stats, text: text, set: set, names: names}, nil
}

// describe renders stats as the passive tree does in scope.
func (t *nodeText) describe(scope resolve.Scope, stats []dataset.StatValue) ([]string, error) {
	values := make(map[string]int32, len(stats))
	for _, sv := range stats {
		values[t.stats[sv.Stat].ID] = sv.Value
	}
	lines, err := t.set.Describe(poe1.DescriptionFiles()[scope], values)
	if err != nil {
		return nil, fmt.Errorf("describe: %w", err)
	}
	var out []string
	for _, l := range lines {
		out = append(out, l.Text)
	}
	return out, nil
}

func (t *nodeText) ascendancy(variant, id string) dataset.Name {
	i := slices.IndexFunc(t.text.AscendancyNames, func(a dataset.AscendancyName) bool { return a.Variant == variant && a.ID == id })
	if i < 0 {
		return dataset.Name{ID: id}
	}
	return dataset.Name{ID: id, Name: t.text.AscendancyNames[i].Name}
}

// Node returns node id of the tree variant, such as "standard", with its English name and
// text rendered through the passive tree's stat descriptions, without reminder text. A node
// without a name, such as a class start, has an empty Name. A variant the dataset lacks or a
// node its tree lacks returns dataset.ErrNotInDataset; a dataset without English text returns
// that too.
func (d *Data) Node(variant string, id uint32) (Node, error) {
	tree, err := d.store.Tree(variant)
	if err != nil {
		return Node{}, fmt.Errorf("simulator: %w", err)
	}
	i, ok := slices.BinarySearchFunc(tree.Nodes, id, func(n dataset.Node, id uint32) int { return cmp.Compare(n.ID, id) })
	if !ok {
		return Node{}, fmt.Errorf("simulator: node %d: %w", id, dataset.ErrNotInDataset)
	}
	t, err := d.text()
	if err != nil {
		return Node{}, fmt.Errorf("simulator: %w", err)
	}
	n := &tree.Nodes[i]
	out := Node{ID: n.ID, Name: t.names[n.ID], Kind: n.Kind, SkillPoints: n.SkillPoints}
	if n.Ascendancy != "" {
		out.Ascendancy = t.ascendancy(variant, n.Ascendancy)
	}
	if out.Stats, err = t.describe(resolve.Global, n.Stats); err != nil {
		return Node{}, fmt.Errorf("simulator: node %d: %w", id, err)
	}
	for _, b := range n.Buffs {
		lines, err := t.describe(resolve.Buff, b)
		if err != nil {
			return Node{}, fmt.Errorf("simulator: node %d aura: %w", id, err)
		}
		out.Stats = append(out.Stats, lines...)
	}
	for _, e := range n.MasteryEffects {
		lines, err := t.describe(resolve.Global, e.Stats)
		if err != nil {
			return Node{}, fmt.Errorf("simulator: node %d effect %d: %w", id, e.ID, err)
		}
		out.Effects = append(out.Effects, MasteryEffect{ID: e.ID, Stats: lines})
	}
	return out, nil
}

// Classes returns the classes of the tree variant with their ascendancies, by id and English
// name. A variant the dataset lacks, or a dataset without English text, returns
// dataset.ErrNotInDataset.
func (d *Data) Classes(variant string) ([]Class, error) {
	tree, err := d.store.Tree(variant)
	if err != nil {
		return nil, fmt.Errorf("simulator: %w", err)
	}
	t, err := d.text()
	if err != nil {
		return nil, fmt.Errorf("simulator: %w", err)
	}
	out := make([]Class, len(tree.Classes))
	for i, c := range tree.Classes {
		out[i].ID = c.Character
		if j := slices.IndexFunc(t.text.CharacterNames, func(n dataset.Name) bool { return n.ID == c.Character }); j >= 0 {
			out[i].Name = t.text.CharacterNames[j].Name
		}
		for _, a := range c.Ascendancies {
			out[i].Ascendancies = append(out[i].Ascendancies, t.ascendancy(variant, a.ID))
		}
	}
	return out, nil
}
