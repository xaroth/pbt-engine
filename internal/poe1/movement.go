package poe1

import (
	"fmt"

	"github.com/xaroth/pbt-engine/internal/core"
)

// baseMovementSpeed is movement_speed before modifiers: the base speed, 100%.
const baseMovementSpeed = 100

// addBaseMovementSpeed adds the base movement speed as a rule's source, so increases and more
// multipliers apply to it by the default rule. Action speed, which also scales how fast the
// character moves, is not part of it.
func (sh *Sheet) addBaseMovementSpeed() error {
	src := sh.newSource(Source{RuleID: "base-movement-speed", Rule: "base movement speed (100%)"})
	mod := core.Modifier{Stat: sh.stats[statMovementSpeed], Op: core.OpBase, Value: baseMovementSpeed, Source: src}
	if _, err := sh.session.AddModifier(mod); err != nil {
		return fmt.Errorf("sheet: base movement speed: %w", err)
	}
	return nil
}
