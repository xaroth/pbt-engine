package poe1

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/xaroth/pbt-engine/internal/core"
	"github.com/xaroth/pbt-engine/internal/semantics"
)

// Context dimensions. A value is read in a context: the sheet's values in the empty one, a
// skill's values in a context of its types, its slot and, for a part of its damage, the damage
// types of that part's chain and the form of the damage. A table modifier with a semantics.ContextFilter applies only in the contexts
// it matches, so it never reaches a sheet value.
const (
	// dimType holds the skill's ActiveSkillType ids that skillTypes lists, those its supports
	// add included. They are the skill's tags: Fireball has type.Fire whatever damage it deals.
	dimType = "type"
	// dimDamage holds the damage types of the part of a skill's damage being read: its type and
	// every type it was converted from, since converted damage takes the modifiers of each type
	// in its chain. A modifier needs one of them per term, so it applies once.
	dimDamage = "damage"
	// dimSkill holds the sheet's slot of the skill, one bit. Every modifier from the skill's
	// own stats (stat set, quality, supports) requires it, so they reach that skill only.
	// Tables never name it.
	dimSkill = "skill"
	// dimForm holds the form of the damage being read: a hit, damage over time or an ailment.
	// A modifier to "damage with hits" requires the hit; one to damage in general none.
	dimForm = "form"
	// dimWeapon holds what an attack is made with: the kinds of the weapon of the hand read.
	// Spells and a skill that is not an attack read none.
	dimWeapon = "weapon"
	// dimHand holds the hand an attack is read with, and whether the character dual wields.
	// Every attack reads one hand: the main hand, and the off hand too when the skill attacks
	// with both. A weapon's own values require its hand.
	dimHand = "hand"
	// dimAilment holds the ailment whose damage or chance is read, with the ailment
	// form for its damage. A modifier to "damage with ignites" requires ailment.ignite.
	dimAilment = "ailment"
	// dimOffHandWeapon holds the off hand weapon's kinds in the context of an attack with both
	// hands read with the main hand: the attack's context, from which its off hand's is made
	// (handBits). Tables never name it.
	dimOffHandWeapon = "off_hand_weapon"
)

// hand is a value of the hand dimension.
type hand uint8

const (
	handMain hand = iota
	handOff
	// handDualWielding is set with either hand while the character holds a weapon in each.
	handDualWielding
)

func handNames() []string {
	return []string{handMain: "main_hand", handOff: "off_hand", handDualWielding: "dual_wielding"}
}

// damageForm is a form of damage, a value of the form dimension; formNone reads none.
type damageForm uint8

const (
	formNone damageForm = iota
	formHit
	formDot
	formAilment
)

func damageFormNames() []string { return []string{"hit", "dot", "ailment"} }

// ailment is an ailment a hit inflicts, a value of the ailment dimension: the damaging ones,
// whose damage the sheet gives, and shock and chill, whether a hit inflicts which decides a
// condition (DeriveConditions).
type ailment uint8

const (
	ailmentIgnite ailment = iota
	ailmentBleed
	ailmentPoison
	ailmentShock
	ailmentChill
)

func ailmentNames() []string {
	return []string{ailmentIgnite: "ignite", ailmentBleed: "bleed", ailmentPoison: "poison", ailmentShock: "shock", ailmentChill: "chill"}
}

// maxSkills is how many skills a sheet holds at once, each with its own dimSkill bit.
const maxSkills = 32

// DamageType is a type of damage, a value of the damage dimension.
type DamageType uint8

// Damage types, in the game's conversion order.
const (
	Physical DamageType = iota + 1
	Fire
	Cold
	Lightning
	Chaos
)

func damageTypeNames() []string {
	return []string{Physical: "physical", Fire: "fire", Cold: "cold", Lightning: "lightning", Chaos: "chaos"}
}

func (d DamageType) String() string {
	if names := damageTypeNames(); d >= Physical && int(d) < len(names) {
		return names[d]
	}
	return "damage?"
}

// skillTypes are the ActiveSkillType ids a table modifier can require, as the game names
// them. A skill type not listed adds nothing to a skill's context; listing one costs a context
// bit.
func skillTypes() []string {
	return []string{
		"Attack", "Spell", "Projectile", "Area", "Duration", "Melee", "Minion", "Channel", "Brand",
		"Warcry", "Aura", "Herald", "Golem", "Guard", "Travel", "Movement", "Vaal", "Triggered",
		"Trapped", "RemoteMined", "SummonsTotem", "AppliesCurse", "Hex", "Mark", "Slam", "Nova",
		"Orb", "Banner", "Chains", "DamageOverTime", "Physical", "Fire", "Cold", "Lightning", "Chaos",
		"Instant", "Cooldown", "Stance", "Steel", "Blessing", "Offering", "Retaliation", "Rain",
		"Link", "Arcane",
	}
}

// modelContexts returns the context dimensions of a sheet model: addContexts registers them
// the same way on every builder, so their bits are the same in every model.
func modelContexts() contexts {
	var b core.ModelBuilder
	return addContexts(&b)
}

// typeMask returns skill type t's bit among c's.
func typeMask(c contexts, t string) core.Mask {
	return core.MaskOf(c.types.Bit(slices.Index(c.types.Values, t)))
}

// contexts are the context dimensions of a sheet model.
type contexts struct {
	types, damage, skill, form, weapon, hand, offHandWeapon, ailment core.Dimension
}

// addContexts registers the context dimensions on b.
func addContexts(b *core.ModelBuilder) contexts {
	slots := make([]string, maxSkills)
	for i := range slots {
		slots[i] = strconv.Itoa(i)
	}
	return contexts{
		types:  mustDimension(b, dimType, skillTypes()),
		damage: mustDimension(b, dimDamage, damageTypeNames()[Physical:]),
		skill:  mustDimension(b, dimSkill, slots),
		form:   mustDimension(b, dimForm, damageFormNames()),
		weapon: mustDimension(b, dimWeapon, weaponKindNames()),
		hand:   mustDimension(b, dimHand, handNames()),
		// The same values as the weapon dimension's, so a kind's index is its bit in either.
		offHandWeapon: mustDimension(b, dimOffHandWeapon, weaponKindNames()),
		ailment:       mustDimension(b, dimAilment, ailmentNames()),
	}
}

func mustDimension(b *core.ModelBuilder, name string, values []string) core.Dimension {
	d, err := b.AddDimension(name, values...)
	if err != nil {
		panic(err)
	}
	return d
}

// isTableContext reports whether name is a context value a table modifier may require.
func isTableContext(name string) bool {
	dim, value, ok := strings.Cut(name, ".")
	switch {
	case !ok:
		return false
	case dim == dimType:
		return slices.Contains(skillTypes(), value)
	case dim == dimDamage:
		return slices.Contains(damageTypeNames()[Physical:], value)
	case dim == dimForm:
		return slices.Contains(damageFormNames(), value)
	case dim == dimWeapon:
		return slices.Contains(weaponKindNames(), value)
	case dim == dimHand:
		return slices.Contains(handNames(), value)
	case dim == dimAilment:
		return slices.Contains(ailmentNames(), value)
	}
	return false
}

// wield is what an attack is read with: the hand, the kinds of that hand's weapon, the off
// hand weapon's kinds when the skill attacks with both hands and the main hand is read, and
// whether the character dual wields. The zero value is a skill that is not an attack.
type wield struct {
	attack            bool
	hand              hand
	weapon, offWeapon weaponKinds
	dualWielding      bool
}

// skillMask returns the context of the skill in slot with types, made with w, reading a part of
// its damage whose chain is damage, or no part when damage is empty, in form, or none.
func (c contexts) skillMask(slot int, types []string, w wield, damage []DamageType, form damageForm) core.Mask {
	m := core.MaskOf(c.skill.Bit(slot))
	for k := range weaponKindCount {
		if w.weapon&(1<<k) != 0 {
			m = m.Set(c.weapon.Bit(int(k)))
		}
		if w.offWeapon&(1<<k) != 0 {
			m = m.Set(c.offHandWeapon.Bit(int(k)))
		}
	}
	if w.attack {
		m = m.Set(c.hand.Bit(int(w.hand)))
		if w.dualWielding {
			m = m.Set(c.hand.Bit(int(handDualWielding)))
		}
	}
	for _, t := range types {
		if i := slices.Index(c.types.Values, t); i >= 0 {
			m = m.Set(c.types.Bit(i))
		}
	}
	for _, d := range damage {
		m = m.Set(c.damage.Bit(int(d) - 1))
	}
	if form != formNone {
		m = m.Set(c.form.Bit(int(form) - 1))
	}
	return m
}

// predicate returns the core predicate of a table context, and of slot's skill when slot is
// not negative.
func predicate(m *core.Model, c semantics.ContextFilter, cs contexts, slot int) (core.Predicate, error) {
	var p core.Predicate
	for _, term := range c.In {
		var req core.Mask
		for _, n := range term {
			b, ok := m.Bit(n)
			if !ok {
				return core.Predicate{}, fmt.Errorf("%w: context %q", ErrUnknownStat, n)
			}
			req = req.Set(b)
		}
		p.Req = append(p.Req, req)
	}
	for _, n := range c.NotIn {
		b, ok := m.Bit(n)
		if !ok {
			return core.Predicate{}, fmt.Errorf("%w: context %q", ErrUnknownStat, n)
		}
		p.Excl = p.Excl.Set(b)
	}
	if slot >= 0 {
		p.Req = append(p.Req, core.MaskOf(cs.skill.Bit(slot)))
	}
	return p, nil
}
