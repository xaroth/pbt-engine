package poe1

import (
	"cmp"
	"fmt"
	"path"
	"slices"

	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/statdesc"
)

// NodeText is a passive node's name and stat lines as the game words them. A line can hold
// newlines where the game breaks one description over several rows.
type NodeText struct {
	Name string `json:"name"`
	// Lines are the node's stats, then the stats of each buff it grants.
	Lines []string `json:"lines"`
	// Masteries are a mastery node's effects, in the dataset's order.
	Masteries []MasteryText `json:"masteries"`
}

// MasteryText is the stat lines of one mastery effect.
type MasteryText struct {
	Effect uint32   `json:"effect"`
	Lines  []string `json:"lines"`
}

// NodeTexts renders passive nodes in one language.
type NodeTexts struct {
	set   *statdesc.Set
	names []dataset.NodeName
	stats []dataset.Stat
}

// NewNodeTexts reads the stats and the text of language from s.
func NewNodeTexts(s *dataset.Store, language string) (*NodeTexts, error) {
	stats, err := s.Stats()
	if err != nil {
		return nil, fmt.Errorf("poe1: %w", err)
	}
	text, err := s.Text(language)
	if err != nil {
		return nil, fmt.Errorf("poe1: %w", err)
	}
	set, err := statdesc.NewSet(text.Descriptions)
	if err != nil {
		return nil, fmt.Errorf("poe1: %s text: %w", language, err)
	}
	return &NodeTexts{set: set, names: text.NodeNames, stats: stats}, nil
}

// Node returns n's name and lines. A node without a name in the language has an empty Name.
func (t *NodeTexts) Node(n *dataset.Node) (NodeText, error) {
	out := NodeText{Lines: []string{}, Masteries: []MasteryText{}}
	if i, ok := slices.BinarySearchFunc(t.names, n.ID, func(nn dataset.NodeName, id uint32) int {
		return cmp.Compare(nn.ID, id)
	}); ok {
		out.Name = t.names[i].Name
	}
	lines, err := t.describe(treeDescriptions, n.Stats)
	if err != nil {
		return NodeText{}, fmt.Errorf("poe1: node %d: %w", n.ID, err)
	}
	out.Lines = append(out.Lines, lines...)
	for _, b := range n.Buffs {
		lines, err := t.describe(auraDescriptions, b)
		if err != nil {
			return NodeText{}, fmt.Errorf("poe1: node %d buff: %w", n.ID, err)
		}
		out.Lines = append(out.Lines, lines...)
	}
	for _, m := range n.MasteryEffects {
		lines, err := t.describe(treeDescriptions, m.Stats)
		if err != nil {
			return NodeText{}, fmt.Errorf("poe1: node %d mastery effect %d: %w", n.ID, m.ID, err)
		}
		out.Masteries = append(out.Masteries, MasteryText{Effect: m.ID, Lines: lines})
	}
	return out, nil
}

func (t *NodeTexts) describe(scope string, svs []dataset.StatValue) ([]string, error) {
	if len(svs) == 0 {
		return []string{}, nil
	}
	vals := make(map[string]int32, len(svs))
	for _, sv := range svs {
		vals[t.stats[sv.Stat].ID] = sv.Value
	}
	ls, err := t.set.Describe(scope, vals)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path.Base(scope), err)
	}
	out := make([]string, len(ls))
	for i, l := range ls {
		out[i] = l.Text
	}
	return out, nil
}
