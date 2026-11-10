package simulator

import "github.com/xaroth/pbt-engine/rules/poe1"

// Output describes one output a simulation reports: its key, label, unit and decimal places.
type Output = poe1.Output

// Outputs returns the outputs of the sim's game, in display order. Consumers render from it
// instead of naming outputs themselves.
func (s *Sim) Outputs() []Output { return poe1.Outputs() }

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
