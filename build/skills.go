package build

import (
	"errors"
	"fmt"
	"slices"
)

// ErrEditTarget is an edit addressing a skill group or gem the build does not have.
var ErrEditTarget = errors.New("build: no such skill group or gem")

// Skills are a build's skill groups, as plain data by game id.
type Skills struct {
	Groups []SkillGroup `json:"groups,omitzero"`
	// MainGroup is the index in Groups of the build's main skill.
	MainGroup int `json:"main,omitzero"`
}

// SkillGroup is a group of linked gems.
type SkillGroup struct {
	// Slot is the item slot the gems are socketed in; empty when they are in none.
	Slot Slot `json:"slot,omitzero"`
	// Gems keep their order. A gem can be in several groups, and the same support more than
	// once in one.
	Gems []Gem `json:"gems,omitzero"`
	// Active is the index in Gems of the group's main active skill.
	Active int `json:"active,omitzero"`
	// ActiveSkill is which skill of the active gem is the main one: 0 its first, 1 its second,
	// such as a Vaal gem's regular skill.
	ActiveSkill int `json:"activeSkill,omitzero"`
}

// GemID names a gem: the dataset Gem.ID, and the GemEffect.ID of the variant when it is not
// the gem's first, such as a transfigured gem.
type GemID struct {
	ID     string `json:"id"`
	Effect string `json:"effect,omitzero"`
}

// Gem is a gem in a skill group.
type Gem struct {
	GemID

	Level   int  `json:"level"`
	Quality int  `json:"quality,omitzero"`
	Support bool `json:"support,omitzero"`
	// Disabled gems stay in the group and do nothing.
	Disabled bool `json:"disabled,omitzero"`
}

// Slot is an item slot gems can be socketed in.
type Slot string

// Slots with sockets.
const (
	SlotWeapon      Slot = "weapon"
	SlotOffhand     Slot = "offhand"
	SlotWeaponSwap  Slot = "weapon-swap"
	SlotOffhandSwap Slot = "offhand-swap"
	SlotHelmet      Slot = "helmet"
	SlotBodyArmour  Slot = "body-armour"
	SlotGloves      Slot = "gloves"
	SlotBoots       Slot = "boots"
)

// Swap reports whether s is a weapon-swap slot, whose items and gems are not in use.
func (s Slot) Swap() bool { return s == SlotWeaponSwap || s == SlotOffhandSwap }

// Slots returns the slots with sockets. An amulet has one when its base does, such as an Unset
// Amulet.
func Slots() []Slot {
	return []Slot{SlotWeapon, SlotOffhand, SlotWeaponSwap, SlotOffhandSwap, SlotHelmet, SlotBodyArmour, SlotGloves, SlotBoots, SlotAmulet}
}

// Group returns a handle on group i, whose methods apply edits to s.
func (s *Skills) Group(i int) *Group { return &Group{skills: s, index: i} }

// Main returns a handle on the build's main skill group.
func (s *Skills) Main() *Group { return s.Group(s.MainGroup) }

// AddGroup appends g and returns a handle on it.
func (s *Skills) AddGroup(g SkillGroup) *Group {
	h := s.Group(len(s.Groups))
	h.err = s.apply(AddSkillGroup(g))
	return h
}

func (s *Skills) clone() Skills {
	c := *s
	c.Groups = slices.Clone(s.Groups)
	for i := range c.Groups {
		c.Groups[i].Gems = slices.Clone(c.Groups[i].Gems)
	}
	return c
}

func (s *Skills) hasGroup(i int) bool { return i >= 0 && i < len(s.Groups) }

// Group is a handle on one skill group. Each method applies one edit; the first that fails,
// such as one on a group the build does not have, stops the later ones, and Err returns it.
type Group struct {
	skills *Skills
	index  int
	err    error
}

// Index returns the group's index in Skills.Groups.
func (g *Group) Index() int { return g.index }

// Err returns the error of the first edit that failed.
func (g *Group) Err() error { return g.err }

// Gem returns a handle on the group's first gem named id.
func (g *Group) Gem(id GemID) *GemRef {
	h := &GemRef{group: g.index, skills: g.skills, err: g.err}
	if h.err != nil {
		return h
	}
	if !g.skills.hasGroup(g.index) {
		h.err = fmt.Errorf("%w: group %d", ErrEditTarget, g.index)
		return h
	}
	h.gem = slices.IndexFunc(g.skills.Groups[g.index].Gems, func(x Gem) bool { return x.GemID == id })
	if h.gem < 0 {
		h.err = fmt.Errorf("%w: gem %s in group %d", ErrEditTarget, id.ID, g.index)
	}
	return h
}

// AddGem appends an active skill gem.
func (g *Group) AddGem(id GemID, level, quality int) *GemRef {
	return g.add(Gem{GemID: id, Level: level, Quality: quality})
}

// AddSupport appends a support gem.
func (g *Group) AddSupport(id GemID, level, quality int) *GemRef {
	return g.add(Gem{GemID: id, Level: level, Quality: quality, Support: true})
}

// SetSlot sets the slot the group's gems are socketed in; empty for none.
func (g *Group) SetSlot(s Slot) *Group { return g.apply(SetSkillSlot(g.index, s)) }

// SetMain makes the group the build's main skill.
func (g *Group) SetMain() *Group { return g.apply(SetMainSkill(g.index)) }

// Remove removes the group; later groups move down by one.
func (g *Group) Remove() *Group { return g.apply(RemoveSkillGroup(g.index)) }

func (g *Group) apply(e Edit) *Group {
	if g.err == nil {
		g.err = g.skills.apply(e)
	}
	return g
}

func (g *Group) add(gem Gem) *GemRef {
	h := &GemRef{group: g.index, skills: g.skills}
	if g.skills.hasGroup(g.index) {
		h.gem = len(g.skills.Groups[g.index].Gems)
	}
	h.err = g.apply(AddGem(g.index, gem)).err
	return h
}

// GemRef is a handle on one gem of a group, with errors as Group's.
type GemRef struct {
	skills     *Skills
	group, gem int
	err        error
}

// Err returns the error of the first edit that failed.
func (h *GemRef) Err() error { return h.err }

// SetLevel sets the gem's level.
func (h *GemRef) SetLevel(level int) *GemRef { return h.apply(SetGemLevel(h.group, h.gem, level)) }

// SetQuality sets the gem's quality.
func (h *GemRef) SetQuality(quality int) *GemRef {
	return h.apply(SetGemQuality(h.group, h.gem, quality))
}

// SetEnabled enables or disables the gem.
func (h *GemRef) SetEnabled(enabled bool) *GemRef {
	return h.apply(SetGemEnabled(h.group, h.gem, enabled))
}

// SetActive makes the gem its group's main active skill.
func (h *GemRef) SetActive() *GemRef { return h.apply(SetActiveGem(h.group, h.gem)) }

// Remove removes the gem; later gems move down by one.
func (h *GemRef) Remove() *GemRef { return h.apply(RemoveGem(h.group, h.gem)) }

func (h *GemRef) apply(e Edit) *GemRef {
	if h.err == nil {
		h.err = h.skills.apply(e)
	}
	return h
}
