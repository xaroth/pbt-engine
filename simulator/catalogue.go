package simulator

import (
	"encoding/json"
	"fmt"

	"github.com/xaroth/pbt-engine/internal/core"
	"github.com/xaroth/pbt-engine/internal/poe1"
	"github.com/xaroth/pbt-engine/internal/resolve"
)

// The names below alias the game's types so hosts can read results without importing the
// rules. Explanations get types of their own when their game-neutral part leaves the rules.

// Output describes one output a simulation reports: its key, label, unit, decimal
// places, group and display order.
type Output = poe1.Output

// Outputs returns the outputs of the sim's game, in display order. Consumers render from it
// instead of naming outputs themselves.
func (s *Sim) Outputs() []Output { return poe1.Outputs() }

// ConfigKey describes one config key a sim takes, with values as SetRaw takes them.
type ConfigKey struct {
	Name    string
	Default json.RawMessage
	// Values are the values the key takes, in display order; nil when any value of its type
	// does.
	Values []json.RawMessage
}

// ConfigKeys returns the config keys of the sim's game.
func (s *Sim) ConfigKeys() ([]ConfigKey, error) {
	specs := poe1.ConfigSpecs()
	out := make([]ConfigKey, 0, len(specs))
	for _, sp := range specs {
		k := ConfigKey{Name: sp.Name}
		var err error
		if k.Default, err = json.Marshal(sp.Default); err != nil {
			return nil, fmt.Errorf("simulator: config key %s: %w", sp.Name, err)
		}
		for _, v := range sp.Values {
			raw, err := json.Marshal(v)
			if err != nil {
				return nil, fmt.Errorf("simulator: config key %s: %w", sp.Name, err)
			}
			k.Values = append(k.Values, raw)
		}
		out = append(out, k)
	}
	return out, nil
}

// View selects how Stats.Explain attributes an output to its sources.
type View = poe1.View

// The views of an explanation.
const (
	// Tree shows how the value is built: terms per stat, nodes read, caps and rounding.
	Tree = poe1.Tree
	// Proportional splits the total among sources; the lines sum to Amount.
	Proportional = poe1.Proportional
	// Marginal lists what the output loses without each source.
	Marginal = poe1.Marginal
)

// Explanation is an output explained in one view.
type Explanation = poe1.Explanation

// Line is one source's share of a proportional or marginal explanation.
type Line = poe1.Line

// LinePart is the share of a line from one kind of modifier.
type LinePart = poe1.LinePart

// TreeNode is one stat of a tree explanation, with the terms it sums and the stats it reads.
type TreeNode = poe1.TreeNode

// TreeTerm is one modifier of a tree explanation's stat.
type TreeTerm = poe1.TreeTerm

// Source is what a value is credited to. Bundle.Source is one of CharacterSource,
// LevelSource, NodeSource, QuestSource, ItemSource or JewelSource; a rule's own source has no
// Bundle and names itself in Rule.
type Source = poe1.Source

// The kinds of bundle source.
type (
	// CharacterSource is a class's base stats; ID is the Character.ID.
	CharacterSource = resolve.CharacterSource
	// LevelSource is the character level.
	LevelSource = resolve.LevelSource
	// NodeSource is a passive node, or with Effect one of a mastery's effects.
	NodeSource = resolve.NodeSource
	// QuestSource is a quest reward.
	QuestSource = resolve.QuestSource
	// ItemSource is an equipped item by slot, with its name.
	ItemSource = resolve.ItemSource
	// JewelSource is a jewel by its socket node, with its name.
	JewelSource = resolve.JewelSource
)

// Op is how a modifier applies.
type Op = core.Op

// The ops of explanation terms and parts.
const (
	OpBase     = core.OpBase
	OpInc      = core.OpInc
	OpMore     = core.OpMore
	OpOverride = core.OpOverride
	OpFlag     = core.OpFlag
)
