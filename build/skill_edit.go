package build

import (
	"fmt"
	"slices"
)

// Skill edit ops.
const (
	OpAddSkillGroup    EditOp = "add-skill-group"
	OpRemoveSkillGroup EditOp = "remove-skill-group"
	OpSetSkillSlot     EditOp = "set-skill-slot"
	OpSetMainSkill     EditOp = "set-main-skill"
	OpAddGem           EditOp = "add-gem"
	OpRemoveGem        EditOp = "remove-gem"
	OpSetActiveGem     EditOp = "set-active-gem"
	OpSetGemLevel      EditOp = "set-gem-level"
	OpSetGemQuality    EditOp = "set-gem-quality"
	OpSetGemEnabled    EditOp = "set-gem-enabled"
	OpSetGroupCount    EditOp = "set-group-count"
	OpSetFullDPS       EditOp = "set-full-dps"
	OpEquipMinion      EditOp = "equip-minion"
	OpUnequipMinion    EditOp = "unequip-minion"
)

// SkillAt addresses a skill group by index, and a gem in it for the edits of one gem. Skill is
// which of the gem's skills, for set-active-gem.
type SkillAt struct {
	Group int `json:"group"`
	Gem   int `json:"gem"`
	Skill int `json:"skill,omitzero"`
}

// AddSkillGroup appends group g.
func AddSkillGroup(g SkillGroup) Edit {
	c := g.cloned()
	return Edit{Op: OpAddSkillGroup, SkillGroup: &c}
}

// RemoveSkillGroup removes group i; later groups move down by one, and the main skill stays
// on its group, or becomes group 0 when it was group i.
func RemoveSkillGroup(i int) Edit { return Edit{Op: OpRemoveSkillGroup, Skill: &SkillAt{Group: i}} }

// SetSkillSlot sets the slot group i is socketed in; empty for none.
func SetSkillSlot(i int, s Slot) Edit {
	return Edit{Op: OpSetSkillSlot, Skill: &SkillAt{Group: i}, Slot: s}
}

// SetMainSkill makes group i the build's main skill.
func SetMainSkill(i int) Edit { return Edit{Op: OpSetMainSkill, Skill: &SkillAt{Group: i}} }

// AddGem appends gem g to group i.
func AddGem(i int, g Gem) Edit { return Edit{Op: OpAddGem, Skill: &SkillAt{Group: i}, Gem: &g} }

// RemoveGem removes a gem from group; later gems move down by one, and the group's active skill
// stays on its gem, or becomes the first skill of gem 0 when it was this one.
func RemoveGem(group, gem int) Edit {
	return Edit{Op: OpRemoveGem, Skill: &SkillAt{Group: group, Gem: gem}}
}

// SetActiveGem makes a gem's first skill its group's main active skill.
func SetActiveGem(group, gem int) Edit { return SetActiveGemSkill(group, gem, 0) }

// SetActiveGemSkill makes skill of a gem its group's main active skill: 0 its first, 1 its
// second, such as a Vaal gem's regular skill.
func SetActiveGemSkill(group, gem, skill int) Edit {
	return Edit{Op: OpSetActiveGem, Skill: &SkillAt{Group: group, Gem: gem, Skill: skill}}
}

// SetGemLevel sets a gem's level.
func SetGemLevel(group, gem, level int) Edit {
	return Edit{Op: OpSetGemLevel, Skill: &SkillAt{Group: group, Gem: gem}, Level: level}
}

// SetGemQuality sets a gem's quality.
func SetGemQuality(group, gem, quality int) Edit {
	return Edit{Op: OpSetGemQuality, Skill: &SkillAt{Group: group, Gem: gem}, Quality: quality}
}

// SetGemEnabled enables or disables a gem.
func SetGemEnabled(group, gem int, enabled bool) Edit {
	return Edit{Op: OpSetGemEnabled, Skill: &SkillAt{Group: group, Gem: gem}, Enabled: enabled}
}

// SetGroupCount sets how many of group i's main active skill are in use; 0 is 1.
func SetGroupCount(i, n int) Edit {
	return Edit{Op: OpSetGroupCount, Skill: &SkillAt{Group: i}, Count: n}
}

// SetFullDPS sets whether group i counts towards full DPS.
func SetFullDPS(i int, on bool) Edit {
	return Edit{Op: OpSetFullDPS, Skill: &SkillAt{Group: i}, Enabled: on}
}

// EquipMinion puts it in slot s of the gear of group i's minion, replacing what is there.
// Whether the minion uses the slot is not checked here; Check warns.
func EquipMinion(i int, s Slot, it Item) Edit {
	return Edit{Op: OpEquipMinion, Skill: &SkillAt{Group: i}, Slot: s, Item: it.Clone()}
}

// UnequipMinion empties slot s of the gear of group i's minion.
func UnequipMinion(i int, s Slot) Edit {
	return Edit{Op: OpUnequipMinion, Skill: &SkillAt{Group: i}, Slot: s}
}

func skillOps() []EditOp {
	return []EditOp{
		OpAddSkillGroup, OpRemoveSkillGroup, OpSetSkillSlot, OpSetMainSkill, OpAddGem, OpRemoveGem,
		OpSetActiveGem, OpSetGemLevel, OpSetGemQuality, OpSetGemEnabled, OpSetGroupCount, OpSetFullDPS,
		OpEquipMinion, OpUnequipMinion,
	}
}

// apply applies skill edit e, or fails with ErrEditTarget when it addresses a group or gem s
// does not have, leaving s unchanged. The main skill and a group's active gem are indexes like
// any other value: setting one out of range is not an error, Check judges it.
func (s *Skills) apply(e Edit) error {
	switch {
	case e.Op == OpAddSkillGroup && e.SkillGroup != nil:
		s.Groups = append(s.Groups, e.SkillGroup.cloned())
		return nil
	case e.Op == OpAddSkillGroup:
		return fmt.Errorf("%w: %s without a group", ErrEditTarget, e.Op)
	case e.Skill == nil:
		return fmt.Errorf("%w: %s without a group index", ErrEditTarget, e.Op)
	case e.Op == OpSetMainSkill:
		s.MainGroup = e.Skill.Group
		return nil
	case !s.hasGroup(e.Skill.Group):
		return fmt.Errorf("%w: %s: group %d", ErrEditTarget, e.Op, e.Skill.Group)
	}
	return s.applyGroup(e)
}

// applyGroup applies an edit of an existing group or of one of its gems.
func (s *Skills) applyGroup(e Edit) error {
	at, g := e.Skill, &s.Groups[e.Skill.Group]
	switch e.Op {
	case OpRemoveSkillGroup:
		s.Groups = slices.Delete(s.Groups, at.Group, at.Group+1)
		s.MainGroup = shifted(s.MainGroup, at.Group)
	case OpSetSkillSlot:
		g.Slot = e.Slot
	case OpSetActiveGem:
		g.Active, g.ActiveSkill = at.Gem, at.Skill
	case OpAddGem:
		if e.Gem == nil {
			return fmt.Errorf("%w: %s without a gem", ErrEditTarget, e.Op)
		}
		g.Gems = append(g.Gems, *e.Gem)
	case OpSetGroupCount:
		g.Count = e.Count
	case OpSetFullDPS:
		g.FullDPS = e.Enabled
	case OpEquipMinion, OpUnequipMinion:
		g.applyMinionItem(e)
	default:
		return s.applyGem(e)
	}
	return nil
}

// applyGem applies an edit of one existing gem.
func (s *Skills) applyGem(e Edit) error {
	at := e.Skill
	if at.Gem < 0 || at.Gem >= len(s.Groups[at.Group].Gems) {
		return fmt.Errorf("%w: %s: group %d gem %d", ErrEditTarget, e.Op, at.Group, at.Gem)
	}
	g := &s.Groups[at.Group]
	switch e.Op {
	case OpRemoveGem:
		g.removeGem(at.Gem)
	case OpSetGemLevel:
		g.Gems[at.Gem].Level = e.Level
	case OpSetGemQuality:
		g.Gems[at.Gem].Quality = e.Quality
	case OpSetGemEnabled:
		g.Gems[at.Gem].Disabled = !e.Enabled
	default:
		return fmt.Errorf("%w %q", ErrEditOp, e.Op)
	}
	return nil
}

// applyMinionItem applies e, an equip-minion or unequip-minion edit, to g's minion gear.
func (g *SkillGroup) applyMinionItem(e Edit) {
	if e.Op == OpUnequipMinion {
		delete(g.MinionItems, e.Slot)
		if len(g.MinionItems) == 0 {
			g.MinionItems = nil
		}
		return
	}
	if g.MinionItems == nil {
		g.MinionItems = Items{}
	}
	var it Item
	if e.Item != nil {
		it = *e.Item.Clone()
	}
	g.MinionItems[e.Slot] = it
}

// shifted returns index i after removing index removed: one lower above it, 0 in its place.
func shifted(i, removed int) int {
	switch {
	case i > removed:
		return i - 1
	case i == removed:
		return 0
	}
	return i
}

// removeGem removes gem j, keeping the active skill on its gem, or making it the first skill
// of gem 0 when it was on gem j.
func (g *SkillGroup) removeGem(j int) {
	g.Gems = slices.Delete(g.Gems, j, j+1)
	if g.Active == j {
		g.ActiveSkill = 0
	}
	g.Active = shifted(g.Active, j)
}

func (g *SkillGroup) cloned() SkillGroup {
	c := *g
	c.Gems = slices.Clone(g.Gems)
	c.MinionItems = cloneItems(g.MinionItems)
	if g.Source != nil {
		src := *g.Source
		c.Source = &src
	}
	return c
}

// skillsDiff returns the edits that turn a into b: groups b does not have removed from the
// last, then per shared group its slot, count, full DPS flag and minion gear, its gems (the gems after the first that differs in id
// or role removed from the last and b's added, the others' level, quality and enabled set)
// and its active gem and skill, then b's further groups added, then the main skill. Groups are
// shared up to the first whose source differs.
func skillsDiff(a, b Skills) []Edit {
	var out []Edit
	shared := 0
	for shared < min(len(a.Groups), len(b.Groups)) && sameSource(a.Groups[shared].Source, b.Groups[shared].Source) {
		shared++
	}
	for i := len(a.Groups) - 1; i >= shared; i-- {
		out = append(out, RemoveSkillGroup(i))
	}
	for i := range shared {
		out = append(out, groupDiff(i, a.Groups[i], b.Groups[i])...)
	}
	for _, g := range b.Groups[shared:] {
		out = append(out, AddSkillGroup(g))
	}
	// Removals move the main skill, so compare with where the edits so far leave it.
	got := a.clone()
	for _, e := range out {
		_ = got.apply(e)
	}
	if got.MainGroup != b.MainGroup {
		out = append(out, SetMainSkill(b.MainGroup))
	}
	return out
}

// sameGem reports whether a and b are the same gem or granted skill in the same role.
func sameGem(a, b Gem) bool {
	return a.GemID == b.GemID && a.Skill == b.Skill && a.Support == b.Support
}

func sameSource(a, b *SkillSource) bool {
	return a == nil && b == nil || a != nil && b != nil && *a == *b
}

func groupDiff(i int, a, b SkillGroup) []Edit {
	var out []Edit
	if a.Slot != b.Slot {
		out = append(out, SetSkillSlot(i, b.Slot))
	}
	out = append(out, minionDiff(i, a, b)...)
	same := 0
	for same < min(len(a.Gems), len(b.Gems)) && sameGem(a.Gems[same], b.Gems[same]) {
		same++
	}
	// Removals move the active gem, so compare with where they leave it.
	at := a.cloned()
	for j := len(a.Gems) - 1; j >= same; j-- {
		out = append(out, RemoveGem(i, j))
		at.removeGem(j)
	}
	for j := range same {
		out = append(out, gemDiff(i, j, a.Gems[j], b.Gems[j])...)
	}
	for _, g := range b.Gems[same:] {
		out = append(out, AddGem(i, g))
	}
	if at.Active != b.Active || at.ActiveSkill != b.ActiveSkill {
		out = append(out, SetActiveGemSkill(i, b.Active, b.ActiveSkill))
	}
	return out
}

// minionDiff returns the edits that turn group i's count, full DPS flag and minion gear from
// a's into b's.
func minionDiff(i int, a, b SkillGroup) []Edit {
	var out []Edit
	if a.Count != b.Count {
		out = append(out, SetGroupCount(i, b.Count))
	}
	if a.FullDPS != b.FullDPS {
		out = append(out, SetFullDPS(i, b.FullDPS))
	}
	return append(out, itemsDiff(a.MinionItems, b.MinionItems,
		func(s Slot) Edit { return UnequipMinion(i, s) },
		func(s Slot, it Item) Edit { return EquipMinion(i, s, it) })...)
}

// gemDiff returns the edits that turn gem j of group i from a into b, of the same id and role.
func gemDiff(i, j int, a, b Gem) []Edit {
	var out []Edit
	if a.Level != b.Level {
		out = append(out, SetGemLevel(i, j, b.Level))
	}
	if a.Quality != b.Quality {
		out = append(out, SetGemQuality(i, j, b.Quality))
	}
	if a.Disabled != b.Disabled {
		out = append(out, SetGemEnabled(i, j, !b.Disabled))
	}
	return out
}
