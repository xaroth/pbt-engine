package poe1

import (
	"fmt"
	"slices"
	"strings"

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
	// weaponUnarmed is an attack made without a weapon: it sets no other kind.
	weaponUnarmed
	weaponKindCount
)

// bow is the bow weapon kind and the gem tag of bow skills.
const bow = "bow"

// unarmed is the weapon kind of an attack made without a weapon; unarmedClass is how a skill's
// weapon restrictions name it.
const (
	unarmed      = "unarmed"
	unarmedClass = "Unarmed"
)

func weaponKindNames() []string {
	return []string{
		weaponAxe: "axe", weaponBow: bow, weaponClaw: "claw", weaponDagger: "dagger", weaponMace: "mace",
		weaponSceptre: "sceptre", weaponStaff: "staff", weaponSword: "sword", weaponWand: "wand",
		weaponOneHanded: "one_handed", weaponTwoHanded: "two_handed", weaponMelee: "melee", weaponUnarmed: unarmed,
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

// Weapon is a weapon's own values, its local stats and quality applied, or the values of the
// class's unarmed attacks.
type Weapon struct {
	// Name is the item's name, empty for unarmed attacks.
	Name string
	// Class is the item's ItemClass id, the way skills' weapon restrictions name it: "Unarmed"
	// for unarmed attacks.
	Class string
	// Kinds are what the weapon counts as for modifiers that name a kind of weapon.
	Kinds weaponKinds
	// Damage is the weapon's damage range per type, indexed by DamageType; 0 to 0 for a type
	// it does not deal.
	Damage [Chaos + 1]DamageRange
	// AttackRate is attacks per second; CriticalChance is in percent.
	AttackRate, CriticalChance float64
	// Accuracy and IncreasedAccuracy are the accuracy rating the weapon adds to its attacks and
	// the percentage it increases theirs by; with CannotBeEvaded its attacks always hit.
	Accuracy, IncreasedAccuracy float64
	CannotBeEvaded              bool
	// Sockets are the item's sockets as its text writes them, such as "R-B-B".
	Sockets string
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
	localAccuracy           = "local_accuracy_rating"
	localAccuracyInc        = "local_accuracy_rating_+%"
	localAlwaysHit          = "local_always_hit"
)

// localWeaponStats are the stats that change a weapon's own values.
func localWeaponStats() map[string]bool {
	local := map[string]bool{
		localPhysicalDamageInc: true, localElementalDamageInc: true, localAttackSpeedInc: true,
		localCriticalChance: true, localCriticalChanceInc: true, localNoPhysicalDamage: true,
		localAccuracy: true, localAccuracyInc: true, localAlwaysHit: true,
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
// addition, times the local increase, rounded to hundredths. Its accuracy stats apply to its
// attacks only, as PoB makes them local (Item.lua). Order and roundings follow PoB
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
	w := &Weapon{Name: it.Name, Class: base.Class, Kinds: ks, Damage: weaponDamage(it, b, sums), Sockets: it.Sockets}
	w.AttackRate = roundHundredths(attackRate(b.AttackTime) * (1 + sums[localAttackSpeedInc]/percentScale))
	crit := float64(b.CriticalChance)/critPermyriadToPercent + sums[localCriticalChance]/critPermyriadToPercent
	w.CriticalChance = roundHundredths(crit * (1 + sums[localCriticalChanceInc]/percentScale))
	w.Accuracy, w.IncreasedAccuracy, w.CannotBeEvaded = sums[localAccuracy], sums[localAccuracyInc], sums[localAlwaysHit] != 0
	return w
}

// attackRate returns the attacks per second of an attack time in milliseconds, rounded to
// hundredths; 0 for none.
func attackRate(ms int32) float64 {
	if ms <= 0 {
		return 0
	}
	return roundHundredths(millisecondsPerSecond / float64(ms))
}

// UnarmedWeapon returns the values of the unarmed attacks of class ch: its physical damage and
// attacks per second, no critical strike chance; nil when the dataset has no unarmed attack
// time for it. The game data has no critical strike chance for unarmed attacks; PoB gives them
// 0 (Data.lua unarmedWeaponData).
func UnarmedWeapon(ch *dataset.Character) *Weapon {
	if ch.UnarmedAttackTime <= 0 {
		return nil
	}
	w := &Weapon{Class: unarmedClass, Kinds: kinds(weaponUnarmed), AttackRate: attackRate(ch.UnarmedAttackTime)}
	if d := ch.UnarmedDamage; d.Min > 0 && d.Max > 0 {
		w.Damage[Physical] = DamageRange{Min: float64(d.Min), Max: float64(d.Max)}
	}
	return w
}

// usableWith reports whether a skill with weapon restrictions can be used with w: w's class is
// among them, or there are none.
func (w *Weapon) usableWith(restrictions []string) bool {
	return len(restrictions) == 0 || slices.Contains(restrictions, w.Class)
}

// label is how a rule names w.
func (w *Weapon) label() string {
	if w.Class == unarmedClass {
		return "unarmed"
	}
	return w.Name
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

// weaponKey is a weapon by hand and content.
type weaponKey struct {
	hand hand
	key  string
}

// SetHands puts h in the character's hands: each hand's weapon's damage, attacks per second
// and critical strike chance become the base of the attacks read with that hand, and what it
// holds sets the stats of a held shield or staff (setHeld). A weapon not seen yet in its hand
// gets its source added once; adding is refused while a checkpoint is open (ErrCheckpointOpen).
func (sh *Sheet) SetHands(h Hands) error {
	held := map[hand]*Weapon{handMain: h.MainHand, handOff: h.OffHand}
	want := map[weaponKey]bool{}
	for hd, w := range held {
		if w != nil {
			want[weaponKey{hd, fmt.Sprint(*w)}] = true
		}
	}
	for k, src := range sh.weapons {
		sh.setSources([]core.SourceID{src}, want[k])
	}
	sh.hands = h
	sh.setHeld(h)
	for _, hd := range []hand{handMain, handOff} {
		w := held[hd]
		if w == nil {
			continue
		}
		k := weaponKey{hd, fmt.Sprint(*w)}
		if _, ok := sh.weapons[k]; ok {
			continue
		}
		if sh.marked {
			return fmt.Errorf("%w: %s weapon %s", ErrCheckpointOpen, handLabel(hd), w.label())
		}
		src, err := sh.addWeapon(w, hd)
		if err != nil {
			return err
		}
		if sh.weapons == nil {
			sh.weapons = map[weaponKey]core.SourceID{}
		}
		sh.weapons[k] = src
	}
	return nil
}

func handLabel(h hand) string {
	if h == handOff {
		return "off hand"
	}
	return "main hand"
}

// weaponMod is one of a weapon's values as a modifier, in the contexts of req besides attacks.
type weaponMod struct {
	stat  string
	op    core.Op
	value float64
	req   []core.Mask
}

// mods returns w's values as modifiers, damage in the context of its type, with dim the damage
// dimension.
func (w *Weapon) mods(dim core.Dimension) []weaponMod {
	var out []weaponMod
	for d := Physical; d <= Chaos; d++ {
		bit := []core.Mask{damageBit(dim, d)}
		out = append(out,
			weaponMod{statBaseMinimumDamage, core.OpBase, w.Damage[d].Min, bit},
			weaponMod{statBaseMaximumDamage, core.OpBase, w.Damage[d].Max, bit},
		)
	}
	out = append(out,
		weaponMod{statAttackRate, core.OpBase, w.AttackRate, nil},
		weaponMod{statCriticalStrikeChance, core.OpBase, w.CriticalChance, nil},
		weaponMod{statAccuracyRating, core.OpBase, w.Accuracy, nil},
		weaponMod{statAccuracyRating, core.OpInc, w.IncreasedAccuracy, nil},
	)
	if w.CannotBeEvaded {
		out = append(out, weaponMod{statHitsCannotBeEvaded, core.OpFlag, 1, nil})
	}
	return out
}

// addWeapon adds w's values as a source whose modifiers reach the attacks read with hand hd
// only.
func (sh *Sheet) addWeapon(w *Weapon, hd hand) (core.SourceID, error) {
	label := handLabel(hd)
	src := sh.newSource(Source{RuleID: strings.ReplaceAll(label, " ", "-") + "-weapon", Rule: label + " weapon " + w.label()})
	md := sh.session.Model()
	attack, ok := md.Bit(dimType + "." + typeAttack)
	if !ok {
		panic("poe1: no attack context")
	}
	held, ok := md.Bit(dimHand + "." + handNames()[hd])
	if !ok {
		panic("poe1: no hand context")
	}
	for _, m := range w.mods(sh.ctx.damage) {
		if m.value == 0 {
			continue
		}
		req := append([]core.Mask{core.MaskOf(attack), core.MaskOf(held)}, m.req...)
		mod := core.Modifier{Stat: sh.stats[m.stat], Op: m.op, Value: m.value, Source: src, Pred: core.Predicate{Req: req}}
		if _, err := sh.session.AddModifier(mod); err != nil {
			return 0, fmt.Errorf("sheet: %s weapon: %w", label, err)
		}
	}
	return src, nil
}
