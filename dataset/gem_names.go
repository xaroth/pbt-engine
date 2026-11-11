package dataset

import (
	"fmt"
	"slices"
)

// supportSuffix ends the item name of a support gem, which the game leaves out where it
// names the support in a group.
const supportSuffix = " Support"

// GemByName returns the gem whose English name is name, and the effect of it that the name
// means: a gem's own name means its first effect, a transfigured skill's name its effect. A
// support matches with or without " Support". A dataset without the skills group returns a
// *MissingGroupError; an unknown name ErrNotInDataset.
func (s *Store) GemByName(name string) (*Gem, *GemEffect, error) {
	sk, err := s.Skills()
	if err != nil {
		return nil, nil, err
	}
	text, err := s.Text("en")
	if err != nil {
		return nil, nil, err
	}
	if g := sk.gemNamed(text.GemNames, name); g != nil {
		return g, &g.Effects[0], nil
	}
	for _, n := range text.SkillNames {
		if n.Name != name {
			continue
		}
		if g, e := sk.gemOfSkill(n.ID); g != nil {
			return g, e, nil
		}
	}
	return nil, nil, fmt.Errorf("gem %q: %w", name, ErrNotInDataset)
}

// gemNamed returns the gem with effects whose name in names is name, or name and " Support"
// for a support, or nil.
func (s *Skills) gemNamed(names []Name, name string) *Gem {
	for _, n := range names {
		if n.Name != name && n.Name != name+supportSuffix {
			continue
		}
		if g := s.Gem(n.ID); g != nil && len(g.Effects) > 0 && (n.Name == name || g.Support) {
			return g
		}
	}
	return nil
}

// Gem returns the gem with ID id, or nil.
func (s *Skills) Gem(id string) *Gem {
	if i := slices.IndexFunc(s.Gems, func(g Gem) bool { return g.ID == id }); i >= 0 {
		return &s.Gems[i]
	}
	return nil
}

// Effect returns the gem's effect with ID id, or nil.
func (g *Gem) Effect(id string) *GemEffect {
	if i := slices.IndexFunc(g.Effects, func(e GemEffect) bool { return e.ID == id }); i >= 0 {
		return &g.Effects[i]
	}
	return nil
}

// gemOfSkill returns the gem and effect whose granted effect is active skill id.
func (s *Skills) gemOfSkill(id string) (*Gem, *GemEffect) {
	granted := map[string]bool{}
	for _, ge := range s.GrantedEffects {
		if ge.ActiveSkill == id {
			granted[ge.ID] = true
		}
	}
	for i := range s.Gems {
		g := &s.Gems[i]
		for j := range g.Effects {
			if granted[g.Effects[j].GrantedEffect] {
				return g, &g.Effects[j]
			}
		}
	}
	return nil, nil
}
