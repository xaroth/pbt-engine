package simulator

import (
	"cmp"
	"fmt"
	"slices"
	"sync"

	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/internal/poe1"
)

// NodeText is a passive node's name and stat lines as the game words them: its stats with the
// passive tree's descriptions, then each buff it grants with the passive aura descriptions.
// A mastery's effects are in Masteries, each with its own lines. A line holds a newline where
// the game breaks one description over several rows. Its lists are empty, not nil, so its JSON
// always holds them.
type NodeText = poe1.NodeText

// MasteryText is the stat lines of one mastery effect, by its effect id.
type MasteryText = poe1.MasteryText

// nodeTexts renders nodes per language, each made on first use.
type nodeTexts struct {
	mu   sync.Mutex
	byLg map[string]func() (*poe1.NodeTexts, error)
}

func (n *nodeTexts) get(store *dataset.Store, language string) (*poe1.NodeTexts, error) {
	n.mu.Lock()
	f, ok := n.byLg[language]
	if !ok {
		if n.byLg == nil {
			n.byLg = map[string]func() (*poe1.NodeTexts, error){}
		}
		f = sync.OnceValues(func() (*poe1.NodeTexts, error) { return poe1.NewNodeTexts(store, language) })
		n.byLg[language] = f
	}
	n.mu.Unlock()
	return f()
}

// NodeText returns the name and stat lines of node id in the tree variant, such as
// build.DefaultVariant, in language, such as "en". An unknown variant, language or node
// returns an error matching dataset.ErrNotInDataset.
func (s *Sim) NodeText(variant string, id uint32, language string) (NodeText, error) {
	tree, err := s.store.Tree(variant)
	if err != nil {
		return NodeText{}, fmt.Errorf("simulator: %w", err)
	}
	i, ok := slices.BinarySearchFunc(tree.Nodes, id, func(n dataset.Node, id uint32) int { return cmp.Compare(n.ID, id) })
	if !ok {
		return NodeText{}, fmt.Errorf("simulator: node %d in the %s tree: %w", id, variant, dataset.ErrNotInDataset)
	}
	t, err := s.nodeTexts.get(s.store, language)
	if err != nil {
		return NodeText{}, fmt.Errorf("simulator: %w", err)
	}
	nt, err := t.Node(&tree.Nodes[i])
	if err != nil {
		return NodeText{}, fmt.Errorf("simulator: %w", err)
	}
	return nt, nil
}
