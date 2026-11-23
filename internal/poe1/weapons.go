package poe1

import (
	"fmt"
	"math"
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
	// ConvertedToRandomElement and GainedAsColdOrLightning are the percentages of its attacks'
	// physical damage converted to a random element and gained as cold or lightning.
	ConvertedToRandomElement, GainedAsColdOrLightning float64
	// FireOfCharacterLife is the fire damage its attacks add, in percent of the character's
	// maximum life, whoever wields it.
	FireOfCharacterLife float64
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
	localAttackSpeedPer8Q   = "local_attack_speed_+%_per_8%_quality"
	localCriticalPer4Q      = "local_critical_strike_chance_+%_per_4%_quality"
	localNoQualityPhysical  = "local_quality_does_not_increase_physical_damage"
	localCriticalChance     = "local_critical_strike_chance"
	localCriticalChanceInc  = "local_critical_strike_chance_+%"
	localNoPhysicalDamage   = "local_weapon_no_physical_damage"
	localAccuracy           = "local_accuracy_rating"
	localAccuracyInc        = "local_accuracy_rating_+%"
	localAlwaysHit          = "local_always_hit"
	localToRandomElement    = "local_physical_damage_%_to_convert_to_a_random_element"
	localAsColdOrLightning  = "local_physical_damage_%_to_gain_as_cold_or_lightning"
	localFireOfLife         = "local_fire_damage_from_life_%"
)

// localWeaponStats are the stats that change a weapon's own values.
func localWeaponStats() map[string]bool {
	local := map[string]bool{
		localPhysicalDamageInc: true, localElementalDamageInc: true, localAttackSpeedInc: true, localAttackSpeedPer8Q: true,
		localCriticalChance: true, localCriticalChanceInc: true, localCriticalPer4Q: true,
		localNoPhysicalDamage: true, localNoQualityPhysical: true,
		localAccuracy: true, localAccuracyInc: true, localAlwaysHit: true,
		localToRandomElement: true, localAsColdOrLightning: true, localFireOfLife: true,
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
// increase; each rounded half up; a weapon whose quality does not increase physical damage has
// no quality factor. Attacks per second are the base's, rounded to hundredths, times the local
// increase, rounded again; the increase per 8% quality counts whole steps of 8%, rounded down
// after multiplying; critical strike chance is the base's plus the local addition, times the
// local increase plus that per 4% quality, counted the same way, rounded to hundredths. Its
// accuracy stats apply to its attacks only, as PoB makes them local (Item.lua). Order and
// roundings follow PoB (Item.lua, BuildModListForSlotNum; the base's attacks per second from
// Export/Scripts/bases.lua);
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
	w.AttackRate = roundHundredths(attackRate(b.AttackTime) * (1 + localAttackSpeed(it, sums)/percentScale))
	crit := float64(b.CriticalChance)/critPermyriadToPercent + sums[localCriticalChance]/critPermyriadToPercent
	w.CriticalChance = roundHundredths(crit * (1 + localCriticalInc(it, sums)/percentScale))
	w.Accuracy, w.IncreasedAccuracy, w.CannotBeEvaded = sums[localAccuracy], sums[localAccuracyInc], sums[localAlwaysHit] != 0
	w.ConvertedToRandomElement, w.GainedAsColdOrLightning = sums[localToRandomElement], sums[localAsColdOrLightning]
	w.FireOfCharacterLife = sums[localFireOfLife]
	return w
}

// Quality per step of a weapon's attack speed per 8% quality and of its critical strike chance
// per 4% quality.
const (
	qualityStep         = 8
	criticalQualityStep = 4
)

// localCriticalInc returns the local increased critical strike chance of it, given sums of its
// stats: its own and that per 4% quality, rounded down after scaling by quality / 4 (Item.lua,
// AlternateQualityLocalCritChancePer4Quality).
func localCriticalInc(it *build.Item, sums map[string]float64) float64 {
	return sums[localCriticalChanceInc] + math.Floor(float64(it.Quality)/criticalQualityStep*sums[localCriticalPer4Q])
}

// localAttackSpeed returns the local increased attack speed of it, given sums of its stats: its
// own and that per 8% quality, which PoB rounds down after scaling by quality / 8 (Item.lua,
// AlternateQualityLocalAttackSpeedPer8Quality).
func localAttackSpeed(it *build.Item, sums map[string]float64) float64 {
	return sums[localAttackSpeedInc] + math.Floor(float64(it.Quality)/qualityStep*sums[localAttackSpeedPer8Q])
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
			f := (1 + sums[localPhysicalDamageInc]/percentScale) * (1 + physicalQuality(it, sums)/percentScale)
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

// physicalQuality returns the quality that increases the physical damage of it, given sums of its
// stats: none when its quality does not increase it (Item.lua, AlternateQualityWeapon).
func physicalQuality(it *build.Item, sums map[string]float64) float64 {
	if sums[localNoQualityPhysical] > 0 {
		return 0
	}
	return float64(it.Quality)
}

// weaponKey is a weapon by hand and content.
type weaponKey struct {
	hand   hand
	weapon Weapon
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
			want[weaponKey{hd, *w}] = true
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
		k := weaponKey{hd, *w}
		if _, ok := sh.weapons[k]; ok {
			continue
		}
		if sh.marked {
			return fmt.Errorf("%w: %s weapon %s", ErrCheckpointOpen, handLabel(hd), w.label())
		}
		src, err := sh.addWeapon(w, hd, playerActor, core.Mask{})
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

// weaponMod is one of a weapon's values as a modifier, in the contexts of req besides attacks;
// with ofLife, in percent of the character's maximum life.
type weaponMod struct {
	stat   string
	op     core.Op
	value  float64
	req    []core.Mask
	ofLife bool
}

// mods returns w's values as modifiers, damage in the context of its type, with dim the damage
// dimension.
func (w *Weapon) mods(dim core.Dimension) []weaponMod {
	var out []weaponMod
	for d := Physical; d <= Chaos; d++ {
		bit := []core.Mask{damageBit(dim, d)}
		out = append(out,
			weaponMod{stat: statBaseMinimumDamage, op: core.OpBase, value: w.Damage[d].Min, req: bit},
			weaponMod{stat: statBaseMaximumDamage, op: core.OpBase, value: w.Damage[d].Max, req: bit},
		)
	}
	out = append(out,
		weaponMod{stat: statAttackRate, op: core.OpBase, value: w.AttackRate},
		weaponMod{stat: statCriticalStrikeChance, op: core.OpBase, value: w.CriticalChance},
		weaponMod{stat: statAccuracyRating, op: core.OpBase, value: w.Accuracy},
		weaponMod{stat: statAccuracyRating, op: core.OpInc, value: w.IncreasedAccuracy},
	)
	if w.CannotBeEvaded {
		out = append(out, weaponMod{stat: statHitsCannotBeEvaded, op: core.OpFlag, value: 1})
	}
	if v := w.FireOfCharacterLife; v != 0 {
		fire := []core.Mask{damageBit(dim, Fire)}
		out = append(out,
			weaponMod{stat: statBaseMinimumDamage, op: core.OpBase, value: v, req: fire, ofLife: true},
			weaponMod{stat: statBaseMaximumDamage, op: core.OpBase, value: v, req: fire, ofLife: true},
		)
	}
	return append(out, w.randomElementMods(dim)...)
}

// randomElementMods returns w's physical damage converted to a random element and gained as
// cold or lightning as conversion and gain shared evenly between the elements it may become,
// as PoB's default "Average" random element mode does (CalcOffence.lua physMode); the game
// picks one per hit.
func (w *Weapon) randomElementMods(dim core.Dimension) []weaponMod {
	physical := []core.Mask{damageBit(dim, Physical)}
	var out []weaponMod
	if v, to := w.ConvertedToRandomElement, []DamageType{Fire, Cold, Lightning}; v != 0 {
		for _, d := range to {
			out = append(out, weaponMod{stat: conversionStat(d), op: core.OpBase, value: v / float64(len(to)), req: physical})
		}
	}
	if v, to := w.GainedAsColdOrLightning, []DamageType{Cold, Lightning}; v != 0 {
		for _, d := range to {
			out = append(out, weaponMod{stat: gainStat(d), op: core.OpBase, value: v / float64(len(to)), req: physical})
		}
	}
	return out
}

// addWeapon adds w's values as a source whose modifiers reach actor's attacks read with hand hd
// only, in the context of ctx too when it is not empty. Its fire damage of the character's life
// reads the character's maximum life as the sheet gives it, unrounded, as PoB's PercentStat does.
func (sh *Sheet) addWeapon(w *Weapon, hd hand, actor core.ActorID, ctx core.Mask) (core.SourceID, error) {
	label := handLabel(hd)
	if actor == minionActor {
		label = "minion " + label
	}
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
		if !ctx.IsZero() {
			req = append(req, ctx)
		}
		mod := core.Modifier{Actor: actor, Stat: sh.stats[m.stat], Op: m.op, Value: m.value, Source: src, Pred: core.Predicate{Req: req}}
		if m.ofLife {
			mod.Scales = []core.Scale{{Ref: characterRef(sh.stats[statMaximumLife]), Step: percentScale, Ratio: true}}
		}
		if _, err := sh.session.AddModifier(mod); err != nil {
			return 0, fmt.Errorf("sheet: %s weapon: %w", label, err)
		}
	}
	return src, nil
}
