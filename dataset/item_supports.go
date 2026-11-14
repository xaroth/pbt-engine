package dataset

import (
	"slices"
)

// ItemSupports finds the supports an item stat grants to the gems socketed in the item, such as
// "Socketed Gems are Supported by Level 20 Spell Totem". The game grants it through the mod's
// granted effects; an item read from text keeps the stats of its lines, not its mods, so the
// supports are found by stat and value.
type ItemSupports struct {
	rolls map[string][]supportRoll
	// gemEffects are the effects gems grant.
	gemEffects map[string]bool
}

// supportRoll is a stat's range in a mod that grants supports, with the supports. A mod's
// supports share a level, such as the two of Cast On Critical Strike.
type supportRoll struct {
	min, max int32
	supports []GrantedSkill
}

// ItemSupports returns the index of the supports item mods grant, built once and shared. It
// fails as Mods and SkillIndex do.
func (s *Store) ItemSupports() (*ItemSupports, error) {
	return s.itemSupports.get(s.loadItemSupports)
}

func (s *Store) loadItemSupports() (*ItemSupports, error) {
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
	return newItemSupports(mods, skills, st.stats), nil
}

// gameModeRuthless is Mods.GameMode of a mod in Ruthless only.
const gameModeRuthless = 2

// newItemSupports indexes the stats of the mods that grant supports. A stat that a mod
// granting none also has, such as local physical damage, names no support.
func newItemSupports(mods []Mod, skills *SkillIndex, stats []Stat) *ItemSupports {
	x := &ItemSupports{rolls: map[string][]supportRoll{}, gemEffects: gemEffects(skills)}
	plain := map[uint32]bool{}
	for _, m := range mods {
		if m.GameMode == gameModeRuthless {
			continue
		}
		supports := modSupports(m, skills)
		if slices.ContainsFunc(supports, func(g GrantedSkill) bool { return g.Level != supports[0].Level }) {
			continue
		}
		for _, ms := range m.Stats {
			if len(supports) == 0 {
				plain[ms.Stat] = true
				continue
			}
			id := stats[ms.Stat].ID
			x.rolls[id] = append(x.rolls[id], supportRoll{min: ms.Min, max: ms.Max, supports: supports})
		}
	}
	for h := range plain {
		delete(x.rolls, stats[h].ID)
	}
	return x
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

// modSupports returns the supports m grants.
func modSupports(m Mod, skills *SkillIndex) []GrantedSkill {
	var out []GrantedSkill
	for _, g := range m.GrantedSkills {
		if ge := skills.GrantedEffect(g.Effect); ge != nil && ge.Support {
			out = append(out, g)
		}
	}
	return out
}

// Supports returns the supports an item stat with value grants: none when it grants none, or
// the stat is in mods granting different supports that value does not tell apart. Of those,
// supports gems grant are taken over those only items grant. The level is value when the stat
// carries it, as in "Supported by Level 20 Spell Totem", also at a level no mod rolls;
// otherwise the mod's.
func (x *ItemSupports) Supports(stat string, value int32) []GrantedSkill {
	in, beyond := x.rollsAt(stat, value)
	if !sameSupports(in) {
		in = slices.DeleteFunc(in, func(r supportRoll) bool {
			return slices.ContainsFunc(r.supports, func(g GrantedSkill) bool { return !x.gemEffects[g.Effect] })
		})
	}
	if len(in) == 0 || !sameSupports(in) {
		return nil
	}
	level := in[0].supports[0].Level
	switch {
	case beyond || slices.ContainsFunc(in, func(r supportRoll) bool { return r.supports[0].Level == value }):
		level = value
	case slices.ContainsFunc(in, func(r supportRoll) bool { return r.supports[0].Level != level }):
		return nil
	}
	out := slices.Clone(in[0].supports)
	for i := range out {
		out[i].Level = level
	}
	return out
}

// rollsAt returns the rolls of stat that value is in; when there are none and the stat carries
// the level in each, all of them, and beyond.
func (x *ItemSupports) rollsAt(stat string, value int32) (rolls []supportRoll, beyond bool) {
	for _, r := range x.rolls[stat] {
		if r.min <= value && value <= r.max {
			rolls = append(rolls, r)
		}
	}
	if len(rolls) == 0 && !slices.ContainsFunc(x.rolls[stat], func(r supportRoll) bool { return !carriesLevel(r) }) {
		return slices.Clone(x.rolls[stat]), true
	}
	return rolls, false
}

// carriesLevel reports whether r's stat is the level of its supports, as in "Supported by Level
// 20 Spell Totem".
func carriesLevel(r supportRoll) bool { l := r.supports[0].Level; return r.min == l && r.max == l }

// sameSupports reports whether rolls grant the same supports.
func sameSupports(rolls []supportRoll) bool {
	return !slices.ContainsFunc(rolls, func(r supportRoll) bool {
		return !slices.EqualFunc(r.supports, rolls[0].supports, func(a, b GrantedSkill) bool { return a.Effect == b.Effect })
	})
}
