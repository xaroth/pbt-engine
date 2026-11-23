package simulator

import (
	"cmp"
	"slices"

	"github.com/xaroth/pbt-engine/internal/resolve"
	"github.com/xaroth/pbt-engine/output"
)

// SkillRef addresses a skill of a build: its gem by index in build.Skills, from 0, and which of
// the gem's skills it is: 0 its first, 1 its second, such as a Vaal gem's regular skill.
type SkillRef struct {
	Group int `json:"group"`
	Gem   int `json:"gem"`
	Skill int `json:"skill"`
}

func refOf(sk *resolve.Skill) SkillRef {
	return SkillRef{Group: sk.Source.Group, Gem: sk.Source.Gem, Skill: sk.Which}
}

func compareRefs(a, b SkillRef) int {
	return cmp.Or(cmp.Compare(a.Group, b.Group), cmp.Compare(a.Gem, b.Gem), cmp.Compare(a.Skill, b.Skill))
}

// SkillStats is one skill's outputs of the skill group, read in its own context as the main
// skill's are.
type SkillStats struct {
	Ref SkillRef
	// GrantedEffect is the skill's GrantedEffect id.
	GrantedEffect string
	values        map[output.Key]float64
	keys          []output.Key
}

// Get returns the skill's value of output k, and false when the simulation did not compute it:
// an output not of the skill group, or one the skill does not have, such as a hit's for an aura.
func (s SkillStats) Get(k output.Key) (float64, bool) {
	v, ok := s.values[k]
	return v, ok
}

// Keys returns the skill's outputs computed, in the order of Stats.Keys.
func (s SkillStats) Keys() []output.Key { return slices.Clone(s.keys) }

// Skills returns the outputs of each skill the call asked for (EverySkill, RankBySkill) and of
// each skill counted in full DPS, the main skill first, then by group, gem and skill. A skill
// counted in full DPS also has poe1.TotalDPS: what it adds by its hits, its hit damage per
// second, or its minion's, times its group's count. A skill that did not resolve is left out
// and warned about.
func (st *Stats) Skills() []SkillStats { return slices.Clone(st.perSkill) }

// Skill returns the outputs of the skill at ref, none when the call did not compute them.
func (st *Stats) Skill(ref SkillRef) SkillStats {
	i := slices.IndexFunc(st.perSkill, func(s SkillStats) bool { return s.Ref == ref })
	if i < 0 {
		return SkillStats{Ref: ref}
	}
	return st.perSkill[i]
}

// paired reports whether a and b both computed the outputs of the skill at ref and it is the
// same skill on both: a ref is a position, so an edit that removes or adds a group or gem before
// it puts another skill there.
func paired(a, b *Stats, ref SkillRef) bool {
	i := slices.IndexFunc(a.perSkill, func(s SkillStats) bool { return s.Ref == ref })
	j := slices.IndexFunc(b.perSkill, func(s SkillStats) bool { return s.Ref == ref })
	return i >= 0 && j >= 0 && a.perSkill[i].GrantedEffect == b.perSkill[j].GrantedEffect
}

// GetSkill returns the change of output k of the skill at ref; Has is false unless both sides
// computed it for the same skill.
func (d *Diff) GetSkill(ref SkillRef, k output.Key) Change {
	before, okA := d.Before.Skill(ref).Get(k)
	after, okB := d.After.Skill(ref).Get(k)
	c := change(before, after)
	c.Has = okA && okB && paired(d.Before, d.After, ref)
	return c
}
