package passives

import (
	"cmp"
	"fmt"
	"path"
	"slices"

	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/statdesc"
)

// Description files the passive tree's stats render with.
const (
	treeDescriptions = "Metadata/StatDescriptions/passive_skill_stat_descriptions.txt"
	auraDescriptions = "Metadata/StatDescriptions/passive_skill_aura_stat_descriptions.txt"
)

// NodeText is a passive node's name and stat lines as the game words them: its stats with the
// passive tree's descriptions, then each buff it grants with the passive aura descriptions.
// A mastery's effects are in Masteries, each with its own lines. Lines follow the description
// files' order. It leaves out the line for the passive skill points a node grants
// (dataset.Node.SkillPoints) and buff lines from stats without a value, which the dataset does
// not hold. A line holds a newline where the game breaks one description over several rows. Its
// lists are empty, not nil, so its JSON always holds them.
type NodeText struct {
	Name string `json:"name"`
	// Lines are the node's stats, then the stats of each buff it grants.
	Lines []string `json:"lines"`
	// Masteries are a mastery node's effects, in the dataset's order.
	Masteries []MasteryText `json:"masteries"`
}

// MasteryText is the stat lines of one mastery effect, by its effect id.
type MasteryText struct {
	Effect uint32   `json:"effect"`
	Lines  []string `json:"lines"`
}

// NodeText returns the name and stat lines of node id in the tree variant, such as
// build.DefaultVariant, in language, such as "en". A node without a name in the language has an
// empty Name. An unknown variant, language or node returns an error matching
// dataset.ErrNotInDataset.
func (p *Reader) NodeText(variant string, id uint32, language string) (NodeText, error) {
	tree, err := p.store.Tree(variant)
	if err != nil {
		return NodeText{}, fmt.Errorf("passives: %w", err)
	}
	i, ok := slices.BinarySearchFunc(tree.Nodes, id, func(n dataset.Node, id uint32) int { return cmp.Compare(n.ID, id) })
	if !ok {
		return NodeText{}, fmt.Errorf("passives: node %d in the %s tree: %w", id, variant, dataset.ErrNotInDataset)
	}
	t, err := textsOf(p.store, language)
	if err != nil {
		return NodeText{}, err
	}
	return t.node(&tree.Nodes[i])
}

// Load makes now the texts of language that NodeText makes on its first call in that language, so
// that call is as fast as later ones. Texts that cannot be made fail NodeText as they would have
// without Load.
func (p *Reader) Load(language string) {
	_, _ = textsOf(p.store, language)
}

// texts renders passive nodes in one language.
type texts struct {
	set   *statdesc.Set
	names []dataset.NodeName
	stats []dataset.Stat
}

// textsKey keys the texts of a language among a store's derived values.
type textsKey struct{ language string }

// textsOf returns the texts of language from s, made once per store.
func textsOf(s *dataset.Store, language string) (*texts, error) {
	//nolint:wrapcheck // the build function wraps its errors
	return dataset.Derived(s, textsKey{language}, func(s *dataset.Store) (*texts, error) {
		stats, err := s.Stats()
		if err != nil {
			return nil, fmt.Errorf("passives: %w", err)
		}
		text, err := s.Text(language)
		if err != nil {
			return nil, fmt.Errorf("passives: %w", err)
		}
		set, err := statdesc.NewSet(text.Descriptions)
		if err != nil {
			return nil, fmt.Errorf("passives: %s text: %w", language, err)
		}
		return &texts{set: set, names: text.NodeNames, stats: stats}, nil
	})
}

// node returns n's name and lines.
func (t *texts) node(n *dataset.Node) (NodeText, error) {
	out := NodeText{Lines: []string{}, Masteries: []MasteryText{}}
	if i, ok := slices.BinarySearchFunc(t.names, n.ID, func(nn dataset.NodeName, id uint32) int {
		return cmp.Compare(nn.ID, id)
	}); ok {
		out.Name = t.names[i].Name
	}
	lines, err := t.describe(treeDescriptions, n.Stats)
	if err != nil {
		return NodeText{}, fmt.Errorf("passives: node %d: %w", n.ID, err)
	}
	out.Lines = append(out.Lines, lines...)
	for _, b := range n.Buffs {
		lines, err := t.describe(auraDescriptions, b)
		if err != nil {
			return NodeText{}, fmt.Errorf("passives: node %d buff: %w", n.ID, err)
		}
		out.Lines = append(out.Lines, lines...)
	}
	for _, m := range n.MasteryEffects {
		lines, err := t.describe(treeDescriptions, m.Stats)
		if err != nil {
			return NodeText{}, fmt.Errorf("passives: node %d mastery effect %d: %w", n.ID, m.ID, err)
		}
		out.Masteries = append(out.Masteries, MasteryText{Effect: m.ID, Lines: lines})
	}
	return out, nil
}

func (t *texts) describe(scope string, svs []dataset.StatValue) ([]string, error) {
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
