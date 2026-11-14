package dataset

// SkillIndex is a skills group with its rows indexed by id. Gem, GrantedEffect, StatSet and
// ActiveSkill find a row without searching the group. The group must not change once indexed.
type SkillIndex struct {
	*Skills

	gems, effects, sets, actives map[string]int
}

// NewSkillIndex indexes s. A Store indexes its skills group once; see Store.SkillIndex.
func NewSkillIndex(s *Skills) *SkillIndex {
	return &SkillIndex{
		Skills:  s,
		gems:    positions(s.Gems, gemID),
		effects: positions(s.GrantedEffects, grantedEffectID),
		sets:    positions(s.StatSets, statSetID),
		actives: positions(s.ActiveSkills, activeSkillID),
	}
}

// positions maps the id of each row of list to its index, the first row's for a repeated id.
func positions[T any](list []T, id func(T) string) map[string]int {
	out := make(map[string]int, len(list))
	for i := range list {
		if _, ok := out[id(list[i])]; !ok {
			out[id(list[i])] = i
		}
	}
	return out
}

func rowOf[T any](list []T, index map[string]int, id string) *T {
	if i, ok := index[id]; ok {
		return &list[i]
	}
	return nil
}

// Gem returns the gem with ID id, or nil.
func (x *SkillIndex) Gem(id string) *Gem { return rowOf(x.Gems, x.gems, id) }

// GrantedEffect returns the granted effect with ID id, or nil.
func (x *SkillIndex) GrantedEffect(id string) *GrantedEffect {
	return rowOf(x.GrantedEffects, x.effects, id)
}

// StatSet returns the stat set with ID id, or nil.
func (x *SkillIndex) StatSet(id string) *StatSet { return rowOf(x.StatSets, x.sets, id) }

// ActiveSkill returns the active skill with ID id, or nil.
func (x *SkillIndex) ActiveSkill(id string) *ActiveSkill {
	return rowOf(x.ActiveSkills, x.actives, id)
}
