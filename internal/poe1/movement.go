package poe1

import (
	"fmt"

	"github.com/xaroth/pbt-engine/internal/core"
)

// Engine stats of movement. movement_speed is the character's movement speed as a percentage of
// the base speed: 100 before modifiers. armour_movement_penalties_are_ignored is a flag that
// lifts the movement speed penalties of armour bases.
const (
	statArmourMovementPenaltiesAreIgnored = "armour_movement_penalties_are_ignored"
	statMovementSpeed                     = "movement_speed"
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

func movementMechanic() mechanic {
	return mechanic{
		name: "movement", stats: []string{
			statArmourMovementPenaltiesAreIgnored,
			statMovementSpeed,
		},
		def: func(def core.StatDef, _ map[string]core.StatID) core.StatDef {
			if def.Name == statArmourMovementPenaltiesAreIgnored {
				def.Flag = true
			}
			return def
		},
	}
}
