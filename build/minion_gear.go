package build

// MinionSlots returns the item slots whose items the minion of the skill, a GrantedEffect id,
// uses, in Slots order; nil for a skill whose minion takes no gear. The game data does not mark
// these skills, so the list is Path of Building's: the skills it gives an item set
// (minionHasItemSet) with the slots they use (minionUses).
func MinionSlots(skill string) []Slot {
	guardian := []Slot{SlotWeapon, SlotOffhand, SlotHelmet, SlotBodyArmour, SlotGloves, SlotBoots}
	weapon := []Slot{SlotWeapon}
	return map[string][]Slot{
		"AnimateGuardian": guardian, "AnimateGuardianAltX": guardian, "AnimateGuardiansWeapon": weapon,
		"AnimateWeapon": weapon, "AnimateWeaponAltY": weapon, "VaalAnimateWeapon": weapon,
	}[skill]
}
