package poe1

import (
	"fmt"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/internal/core"
)

// Engine stats of what the equipped items are, set from them (SetItems), each 1 while it holds:
// no_life_modifiers_on_body_armour while the body armour has no Life Modifier, an empty body
// armour slot included, and armour_on_helmet_body_armour_gloves_and_boots while the helmet, body
// armour, gloves and boots all have armour of their own. Table modifiers hold on them ("while").
const (
	statNoLifeModifiersOnBodyArmour = "no_life_modifiers_on_body_armour"
	statArmourOnArmourSlots         = "armour_on_helmet_body_armour_gloves_and_boots"
)

// armourSlots are the slots statArmourOnArmourSlots looks at.
func armourSlots() []build.Slot {
	return []build.Slot{build.SlotHelmet, build.SlotBodyArmour, build.SlotGloves, build.SlotBoots}
}

// equipmentMods are the sources that hold those stats on a sheet.
type equipmentMods struct {
	noLife, armour core.SourceID
}

// addEquipment adds the sources of what the equipped items are, as without items: the body
// armour slot empty, so without Life Modifiers, and no armour.
func (sh *Sheet) addEquipment() error {
	flags := []struct {
		stat, rule string
		src        *core.SourceID
		on         bool
	}{
		{statNoLifeModifiersOnBodyArmour, "no Life Modifiers on the body armour", &sh.equipment.noLife, true},
		{statArmourOnArmourSlots, "armour on the helmet, body armour, gloves and boots", &sh.equipment.armour, false},
	}
	for _, f := range flags {
		*f.src = sh.newSource(Source{RuleID: "equipment-" + f.stat, Rule: f.rule})
		if _, err := sh.session.AddModifier(core.Modifier{Stat: sh.stats[f.stat], Op: core.OpFlag, Value: 1, Source: *f.src}); err != nil {
			return fmt.Errorf("sheet: %s: %w", f.stat, err)
		}
		// The source is this session's, so this cannot fail.
		_ = sh.session.SetSourceEnabled(*f.src, f.on)
	}
	return nil
}

// setEquipment sets what the equipped items are from items. An abyss jewel shares its item's
// slot without its Life Modifiers or armour, so the item's own count whatever the jewel has.
func (sh *Sheet) setEquipment(items []Item) {
	life := false
	armour := map[build.Slot]bool{}
	for _, it := range items {
		life = life || it.Slot == build.SlotBodyArmour && it.LifeModifiers
		armour[it.Slot] = armour[it.Slot] || it.Armour > 0
	}
	all := true
	for _, s := range armourSlots() {
		all = all && armour[s]
	}
	// The sources are this session's, so these cannot fail.
	_ = sh.session.SetSourceEnabled(sh.equipment.noLife, !life)
	_ = sh.session.SetSourceEnabled(sh.equipment.armour, all)
}

func equipmentMechanic() mechanic {
	return mechanic{
		name:  "equipment",
		stats: []string{statArmourOnArmourSlots, statNoLifeModifiersOnBodyArmour},
		def: func(def core.StatDef, _ map[string]core.StatID) core.StatDef {
			if def.Name == statArmourOnArmourSlots || def.Name == statNoLifeModifiersOnBodyArmour {
				def.Flag = true
			}
			return def
		},
	}
}
