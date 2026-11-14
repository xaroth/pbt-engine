package poe1

import (
	"fmt"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/internal/core"
)

// weaponKind is a value of the weapon dimension: one thing an attack can be made with.
type weaponKind uint8

// Weapon kinds. Each class sets one kind of its own and its handedness; a melee weapon also
// sets weaponMelee, so a modifier to one handed melee weapons needs both.
const (
	weaponAxe weaponKind = iota
	weaponBow
	weaponClaw
	weaponDagger
	weaponMace
	weaponSceptre
	weaponStaff
	weaponSword
	weaponWand
	weaponOneHanded
	weaponTwoHanded
	weaponMelee
	weaponKindCount
)

// bow is the bow weapon kind and the gem tag of bow skills.
const bow = "bow"

func weaponKindNames() []string {
	return []string{
		weaponAxe: "axe", weaponBow: bow, weaponClaw: "claw", weaponDagger: "dagger", weaponMace: "mace",
		weaponSceptre: "sceptre", weaponStaff: "staff", weaponSword: "sword", weaponWand: "wand",
		weaponOneHanded: "one_handed", weaponTwoHanded: "two_handed", weaponMelee: "melee",
	}
}

// weaponKinds is a set of weapon kinds, bit k for kind k.
type weaponKinds uint16

func kinds(ks ...weaponKind) weaponKinds {
	var s weaponKinds
	for _, k := range ks {
		s |= 1 << k
	}
	return s
}

// weaponClasses maps the ItemClass ids of weapons to their kinds. A thrusting sword is a sword,
// a rune dagger a dagger and a warstaff a staff, as the game's text for modifiers to them reads.
func weaponClasses() map[string]weaponKinds {
	one, two := kinds(weaponOneHanded, weaponMelee), kinds(weaponTwoHanded, weaponMelee)
	return map[string]weaponKinds{
		"One Hand Axe":             one | kinds(weaponAxe),
		"One Hand Mace":            one | kinds(weaponMace),
		"Sceptre":                  one | kinds(weaponSceptre),
		"One Hand Sword":           one | kinds(weaponSword),
		"Thrusting One Hand Sword": one | kinds(weaponSword),
		"Claw":                     one | kinds(weaponClaw),
		"Dagger":                   one | kinds(weaponDagger),
		"Rune Dagger":              one | kinds(weaponDagger),
		"Wand":                     kinds(weaponOneHanded, weaponWand),
		"Two Hand Axe":             two | kinds(weaponAxe),
		"Two Hand Mace":            two | kinds(weaponMace),
		"Two Hand Sword":           two | kinds(weaponSword),
		"Staff":                    two | kinds(weaponStaff),
		"Warstaff":                 two | kinds(weaponStaff),
		"Bow":                      kinds(weaponTwoHanded, weaponBow),
	}
}

// Weapon is a weapon's own values, its local stats and quality applied.
type Weapon struct {
	// Kinds are what the weapon counts as for modifiers that name a kind of weapon.
	Kinds weaponKinds
	// Damage is the weapon's damage range per type, indexed by DamageType; 0 to 0 for a type
	// it does not deal.
	Damage [Chaos + 1]DamageRange
	// AttackRate is attacks per second; CriticalChance is in percent.
	AttackRate, CriticalChance float64
}

// DamageRange is a minimum and maximum damage.
type DamageRange struct {
	Min, Max float64
}

// localAdded are the local stats that add damage of a type to a weapon, by DamageType.
func localAdded() [Chaos + 1][2]string {
	var out [Chaos + 1][2]string
	for d := Physical; d <= Chaos; d++ {
		out[d] = [2]string{"local_minimum_added_" + d.String() + "_damage", "local_maximum_added_" + d.String() + "_damage"}
	}
	return out
}

// Local weapon stats other than added damage.
const (
	localPhysicalDamageInc  = "local_physical_damage_+%"
	localElementalDamageInc = "local_elemental_damage_+%"
	localAttackSpeedInc     = "local_attack_speed_+%"
	localCriticalChance     = "local_critical_strike_chance"
	localCriticalChanceInc  = "local_critical_strike_chance_+%"
	localNoPhysicalDamage   = "local_weapon_no_physical_damage"
)

// localWeaponStats are the stats that change a weapon's own values.
func localWeaponStats() map[string]bool {
	local := map[string]bool{
		localPhysicalDamageInc: true, localElementalDamageInc: true, localAttackSpeedInc: true,
		localCriticalChance: true, localCriticalChanceInc: true, localNoPhysicalDamage: true,
	}
	for _, ids := range localAdded() {
		for _, id := range ids {
			if id != "" {
				local[id] = true
			}
		}
	}
	return local
}

// compileWeapon returns the weapon it is on base, with sums of its stats; nil when base is not a
// weapon. Physical damage is the base's plus local added damage, times the local increase and,
// separately, the quality; elemental damage is the local added damage times the local elemental
// increase; each rounded half up. Attacks per second are the base's, rounded to hundredths,
// times the local increase, rounded again; critical strike chance is the base's plus the local
// addition, times the local increase, rounded to hundredths. Order and roundings follow PoB
// (Item.lua, BuildModListForSlotNum; the base's attacks per second from Export/Scripts/bases.lua);
// the game text gives none of them.
func compileWeapon(it *build.Item, base *dataset.ItemBase, sums map[string]float64) *Weapon {
	if base.Weapon == nil {
		return nil
	}
	ks, ok := weaponClasses()[base.Class]
	if !ok {
		return nil
	}
	b := base.Weapon
	w := &Weapon{Kinds: ks, Damage: weaponDamage(it, b, sums)}
	if b.AttackTime > 0 {
		rate := roundHundredths(millisecondsPerSecond / float64(b.AttackTime))
		w.AttackRate = roundHundredths(rate * (1 + sums[localAttackSpeedInc]/percentScale))
	}
	crit := float64(b.CriticalChance)/critPermyriadToPercent + sums[localCriticalChance]/critPermyriadToPercent
	w.CriticalChance = roundHundredths(crit * (1 + sums[localCriticalChanceInc]/percentScale))
	return w
}

// weaponDamage returns the damage per type of it on a weapon base with values b, given sums of
// its stats, as compileWeapon says; a weapon with "No Physical Damage" has none.
func weaponDamage(it *build.Item, b *dataset.WeaponValues, sums map[string]float64) [Chaos + 1]DamageRange {
	var out [Chaos + 1]DamageRange
	for d, ids := range localAdded() {
		if d == 0 {
			continue
		}
		lo, hi := sums[ids[0]], sums[ids[1]]
		switch DamageType(d) {
		case Physical:
			lo += float64(b.PhysicalDamage.Min)
			hi += float64(b.PhysicalDamage.Max)
			f := (1 + sums[localPhysicalDamageInc]/percentScale) * (1 + float64(it.Quality)/percentScale)
			lo, hi = roundHalf(lo*f), roundHalf(hi*f)
		case Fire, Cold, Lightning:
			f := 1 + sums[localElementalDamageInc]/percentScale
			lo, hi = roundHalf(lo*f), roundHalf(hi*f)
		case Chaos:
		}
		if lo > 0 && hi > 0 && (DamageType(d) != Physical || sums[localNoPhysicalDamage] == 0) {
			out[d] = DamageRange{Min: lo, Max: hi}
		}
	}
	return out
}

// setMainHand makes w the main hand weapon, nil for none: its damage, attacks per second and
// critical strike chance become the base of every attack's. A weapon not seen yet gets its
// source added once; adding is refused while a checkpoint is open (ErrCheckpointOpen).
func (sh *Sheet) setMainHand(w *Weapon, name string) error {
	key := ""
	if w != nil {
		key = fmt.Sprint(*w)
	}
	for k, src := range sh.weapons {
		sh.setSources([]core.SourceID{src}, k == key)
	}
	sh.mainHand = 0
	if w == nil {
		return nil
	}
	sh.mainHand = w.Kinds
	if _, ok := sh.weapons[key]; ok {
		return nil
	}
	if sh.marked {
		return fmt.Errorf("%w: main hand weapon %s", ErrCheckpointOpen, name)
	}
	src, err := sh.addWeapon(w, name)
	if err != nil {
		return err
	}
	if sh.weapons == nil {
		sh.weapons = map[string]core.SourceID{}
	}
	sh.weapons[key] = src
	return nil
}

// addWeapon adds w's values as a source whose modifiers reach attacks only.
func (sh *Sheet) addWeapon(w *Weapon, name string) (core.SourceID, error) {
	src := sh.newSource(Source{RuleID: "main-hand-weapon", Rule: "main hand weapon " + name})
	attack, ok := sh.session.Model().Bit(dimType + "." + typeAttack)
	if !ok {
		panic("poe1: no attack context")
	}
	attackMask := core.MaskOf(attack)
	add := func(stat string, v float64, req ...core.Mask) error {
		if v == 0 {
			return nil
		}
		mod := core.Modifier{Stat: sh.stats[stat], Op: core.OpBase, Value: v, Source: src, Pred: core.Predicate{Req: append([]core.Mask{attackMask}, req...)}}
		if _, err := sh.session.AddModifier(mod); err != nil {
			return fmt.Errorf("sheet: main hand weapon: %w", err)
		}
		return nil
	}
	for d := Physical; d <= Chaos; d++ {
		bit := damageBit(sh.ctx.damage, d)
		if err := add(statBaseMinimumDamage, w.Damage[d].Min, bit); err != nil {
			return 0, err
		}
		if err := add(statBaseMaximumDamage, w.Damage[d].Max, bit); err != nil {
			return 0, err
		}
	}
	if err := add(statAttackRate, w.AttackRate); err != nil {
		return 0, err
	}
	if err := add(statCriticalStrikeChance, w.CriticalChance); err != nil {
		return 0, err
	}
	return src, nil
}
