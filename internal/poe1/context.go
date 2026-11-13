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
// types of that part's chain. A table modifier with a semantics.ContextFilter applies only in the contexts
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
)

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

// contexts are the context dimensions of a sheet model.
type contexts struct {
	types, damage, skill core.Dimension
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
	}
	return false
}

// skillMask returns the context of the skill in slot with types, reading a part of its damage
// whose chain is damage, or no part when damage is empty.
func (c contexts) skillMask(slot int, types []string, damage []DamageType) core.Mask {
	m := core.MaskOf(c.skill.Bit(slot))
	for _, t := range types {
		if i := slices.Index(c.types.Values, t); i >= 0 {
			m = m.Set(c.types.Bit(i))
		}
	}
	for _, d := range damage {
		m = m.Set(c.damage.Bit(int(d) - 1))
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
