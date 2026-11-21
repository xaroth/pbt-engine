package dataset

import (
	"slices"
)

// ItemGrants finds what an item stat grants: the supports it grants to the gems socketed in the
// item, such as "Socketed Gems are Supported by Level 20 Spell Totem", and the skills it grants
// the character, such as "Grants Level 30 Herald of the Hive Skill". The game grants both through
// the mod's granted effects; an item read from text keeps the stats of its lines, not its mods,
// so they are found by stat and value.
type ItemGrants struct {
	supports map[string][]grantRoll
	// skills are the rolls of the mods granting active skills, in standard and in Ruthless.
	skills [2]map[string][]grantRoll
	// gemEffects are the effects gems grant.
	gemEffects map[string]bool
	// none is set when no mod grants anything, as in a dataset from before granted skills.
	none bool
}

// grantRoll is a stat's range in a mod that grants supports or skills, with what it grants. A
// mod's grants of one kind share a level, such as the two supports of Cast On Critical Strike.
type grantRoll struct {
	min, max int32
	grants   []GrantedSkill
}

// ItemGrants returns the index of what item mods grant, built once and shared. It fails as Mods
// and SkillIndex do.
func (s *Store) ItemGrants() (*ItemGrants, error) {
	return s.itemGrants.get(s.loadItemGrants)
}

func (s *Store) loadItemGrants() (*ItemGrants, error) {
	mods, err := s.Mods()
	if err != nil {
		return nil, err
	}
	skills, err := s.SkillIndex()
	if err != nil {
		return nil, err
	}
	st, err := s.statsGroup()
	if err != nil {
		return nil, err
	}
	return newItemGrants(mods, skills, st.stats), nil
}

// Mods.GameMode of a mod in standard only and in Ruthless only.
const (
	gameModeStandard = 1
	gameModeRuthless = 2
)

// newItemGrants indexes the stats of the mods that grant supports or skills. A stat that a mod
// granting none of the kind also has, such as local physical damage, names none of it.
func newItemGrants(mods []Mod, skills *SkillIndex, stats []Stat) *ItemGrants {
	x := &ItemGrants{gemEffects: gemEffects(skills), none: !slices.ContainsFunc(mods, func(m Mod) bool { return len(m.GrantedSkills) > 0 })}
	x.supports = indexGrants(mods, stats, func(m Mod) ([]GrantedSkill, bool) {
		return grantsOf(m.GrantedSkills, skills, true), m.GameMode != gameModeRuthless
	})
	for i, ruthless := range []bool{false, true} {
		x.skills[i] = indexGrants(mods, stats, func(m Mod) ([]GrantedSkill, bool) {
			in := m.GameMode != gameModeRuthless && !ruthless || m.GameMode != gameModeStandard && ruthless
			return grantsOf(m.Grants(ruthless), skills, false), in
		})
	}
	return x
}

// indexGrants returns the rolls of the stats of the mods grants returns some for, by stat id.
// It leaves out the mods grants reports are not in the game mode.
func indexGrants(mods []Mod, stats []Stat, grants func(Mod) ([]GrantedSkill, bool)) map[string][]grantRoll {
	out := map[string][]grantRoll{}
	plain := map[uint32]bool{}
	for _, m := range mods {
		gs, in := grants(m)
		if !in || slices.ContainsFunc(gs, func(g GrantedSkill) bool { return g.Level != gs[0].Level }) {
			continue
		}
		for _, ms := range m.Stats {
			if len(gs) == 0 {
				plain[ms.Stat] = true
				continue
			}
			id := stats[ms.Stat].ID
			out[id] = append(out[id], grantRoll{min: ms.Min, max: ms.Max, grants: gs})
		}
	}
	for h := range plain {
		delete(out, stats[h].ID)
	}
	return out
}

// gemEffects returns the effects gems grant.
func gemEffects(skills *SkillIndex) map[string]bool {
	out := map[string]bool{}
	for _, g := range skills.Gems {
		for _, e := range g.Effects {
			out[e.GrantedEffect] = true
			out[e.SecondaryGrantedEffect] = true
		}
	}
	return out
}

// grantsOf returns the supports among gs, or with support false the active skills.
func grantsOf(gs []GrantedSkill, skills *SkillIndex, support bool) []GrantedSkill {
	var out []GrantedSkill
	for _, g := range gs {
		if ge := skills.GrantedEffect(g.Effect); ge != nil && ge.Support == support {
			out = append(out, g)
		}
	}
	return out
}

// None reports whether no mod grants anything, as in a dataset from before granted skills.
func (x *ItemGrants) None() bool { return x.none }

// Supports returns the supports an item stat with value grants: none when it grants none, or
// the stat is in mods granting different supports that value does not tell apart. Of those,
// supports gems grant are taken over those only items grant. The level is value when the stat
// carries it, as in "Supported by Level 20 Spell Totem", also at a level no mod rolls;
// otherwise the mod's.
func (x *ItemGrants) Supports(stat string, value int32) []GrantedSkill {
	in, beyond := rollsAt(x.supports[stat], value)
	if !sameGrants(in) {
		in = slices.DeleteFunc(in, func(r grantRoll) bool {
			return slices.ContainsFunc(r.grants, func(g GrantedSkill) bool { return !x.gemEffects[g.Effect] })
		})
	}
	return grantsAt(in, beyond, value)
}

// Skills returns the active skills an item stat with value grants the character, in standard or
// in Ruthless: none when it grants none, or the stat is in mods granting different skills that
// value does not tell apart. The level is as for Supports, as in "Grants Level 30 Herald of the
// Hive Skill", and the mod's for a stat that is a chance, as in "15% chance to Trigger Level 1
// Blood Rage when you Kill an Enemy".
func (x *ItemGrants) Skills(stat string, value int32, ruthless bool) []GrantedSkill {
	mode := 0
	if ruthless {
		mode = 1
	}
	in, beyond := rollsAt(x.skills[mode][stat], value)
	return grantsAt(in, beyond, value)
}

// grantsAt returns what rolls in grant at value, with beyond as rollsAt gives it.
func grantsAt(in []grantRoll, beyond bool, value int32) []GrantedSkill {
	if len(in) == 0 || !sameGrants(in) {
		return nil
	}
	level := in[0].grants[0].Level
	switch {
	case beyond || slices.ContainsFunc(in, func(r grantRoll) bool { return r.grants[0].Level == value }):
		level = value
	case slices.ContainsFunc(in, func(r grantRoll) bool { return r.grants[0].Level != level }):
		return nil
	}
	out := slices.Clone(in[0].grants)
	for i := range out {
		out[i].Level = level
	}
	return out
}

// rollsAt returns the rolls that value is in; when there are none and the stat carries the
// level in each, all of them, and beyond.
func rollsAt(rolls []grantRoll, value int32) (in []grantRoll, beyond bool) {
	for _, r := range rolls {
		if r.min <= value && value <= r.max {
			in = append(in, r)
		}
	}
	if len(in) == 0 && !slices.ContainsFunc(rolls, func(r grantRoll) bool { return !carriesLevel(r) }) {
		return slices.Clone(rolls), true
	}
	return in, false
}

// carriesLevel reports whether r's stat is the level of its grants, as in "Supported by Level
// 20 Spell Totem".
func carriesLevel(r grantRoll) bool { l := r.grants[0].Level; return r.min == l && r.max == l }

// sameGrants reports whether rolls grant the same effects.
func sameGrants(rolls []grantRoll) bool {
	return !slices.ContainsFunc(rolls, func(r grantRoll) bool {
		return !slices.EqualFunc(r.grants, rolls[0].grants, func(a, b GrantedSkill) bool { return a.Effect == b.Effect })
	})
}
