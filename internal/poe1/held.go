package poe1

import (
	"fmt"
	"strings"

	"github.com/xaroth/pbt-engine/internal/core"
)

// Engine stats of what the character holds, set from its hands (SetHands): is_holding_shield,
// is_wielding_staff and is_dual_wielding are 1 while it holds a shield, wields a staff or holds
// a weapon in each hand, and the staff_ stats count the red and blue sockets of the staff it
// wields, 0 without one. Table modifiers hold on them ("while", "if") or scale with them ("per").
const (
	statIsHoldingShield  = "is_holding_shield"
	statIsWieldingStaff  = "is_wielding_staff"
	statIsDualWielding   = "is_dual_wielding"
	statStaffRedSockets  = "staff_red_sockets"
	statStaffBlueSockets = "staff_blue_sockets"
)

// heldMods are the sources and modifiers that hold those stats on a sheet.
type heldMods struct {
	shield, staff, dual core.SourceID
	red, blue           core.ModID
}

// addHeld adds the sources of what the character holds, all off and the counts 0.
func (sh *Sheet) addHeld() error {
	flags := []struct {
		stat, rule string
		src        *core.SourceID
	}{
		{statIsHoldingShield, "holding a shield", &sh.held.shield},
		{statIsWieldingStaff, "wielding a staff", &sh.held.staff},
		{statIsDualWielding, "dual wielding", &sh.held.dual},
	}
	for _, f := range flags {
		*f.src = sh.newSource(Source{RuleID: "held-" + f.stat, Rule: f.rule})
		if _, err := sh.session.AddModifier(core.Modifier{Stat: sh.stats[f.stat], Op: core.OpFlag, Value: 1, Source: *f.src}); err != nil {
			return fmt.Errorf("sheet: %s: %w", f.stat, err)
		}
		// The source is this session's, so this cannot fail.
		_ = sh.session.SetSourceEnabled(*f.src, false)
	}
	sockets := sh.newSource(Source{RuleID: "staff-sockets", Rule: "sockets of the wielded staff"})
	counts := []struct {
		stat string
		id   *core.ModID
	}{{statStaffRedSockets, &sh.held.red}, {statStaffBlueSockets, &sh.held.blue}}
	for _, c := range counts {
		var err error
		if *c.id, err = sh.session.AddModifier(core.Modifier{Stat: sh.stats[c.stat], Op: core.OpBase, Source: sockets}); err != nil {
			return fmt.Errorf("sheet: %s: %w", c.stat, err)
		}
	}
	return nil
}

// setHeld sets what the character holds from h. A staff is two handed, so it is in the main
// hand; a shield is in the off hand.
func (sh *Sheet) setHeld(h Hands) {
	staff := h.MainHand != nil && h.MainHand.Kinds&kinds(weaponStaff) != 0
	var red, blue int
	if staff {
		red, blue = strings.Count(h.MainHand.Sockets, "R"), strings.Count(h.MainHand.Sockets, "B")
	}
	// The sources and modifiers are this session's, so these cannot fail.
	_ = sh.session.SetSourceEnabled(sh.held.shield, h.Shield)
	_ = sh.session.SetSourceEnabled(sh.held.staff, staff)
	_ = sh.session.SetSourceEnabled(sh.held.dual, h.dualWielding())
	_ = sh.session.SetModifierValue(sh.held.red, float64(red))
	_ = sh.session.SetModifierValue(sh.held.blue, float64(blue))
}

func heldMechanic() mechanic {
	return mechanic{
		name:  "held",
		stats: []string{statIsHoldingShield, statIsWieldingStaff, statIsDualWielding, statStaffBlueSockets, statStaffRedSockets},
		def: func(def core.StatDef, _ map[string]core.StatID) core.StatDef {
			if def.Name == statIsHoldingShield || def.Name == statIsWieldingStaff || def.Name == statIsDualWielding {
				def.Flag = true
			}
			return def
		},
	}
}
