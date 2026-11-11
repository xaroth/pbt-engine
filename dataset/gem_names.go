package dataset

import (
	"fmt"
	"slices"
	"strings"
)

// supportSuffix ends the item name of a support gem, which the game leaves out where it
// names the support in a group.
const supportSuffix = " Support"

// supersededSuffix ends the id of a gem effect the game has replaced and keeps in its files,
// such as ConvocationOld beside Convocation.
const supersededSuffix = "Old"

// GemMatch is a gem and the effect of it that a name or granted effect means.
type GemMatch struct {
	Gem    *Gem
	Effect *GemEffect
	// Others are the ids of the other gems it could be, when choosing between gems of one name
	// or granted effect could not decide; Gem is then the first in the game's order.
	Others []string
}

// GemByName returns the gem whose English name is name, and the effect of it the name means:
// a gem's own name means its first effect, a transfigured skill's name its effect. A support
// matches with or without " Support". A dataset without the skills group returns a
// *MissingGroupError; an unknown name ErrNotInDataset.
//
// The game's files hold more than one gem for some names: the gems of the Royale mode, which
// take no quality, and older items it has replaced. Among such gems the lookup keeps, in turn,
// the ones whose effect takes quality, then the ones whose effect id does not end in "Old".
// Gems left that share one gem effect are the same skill, such as the two Arctic Armour items,
// and the first in the game's order is returned; gems left with different effects are listed
// in GemMatch.Others.
func (s *Store) GemByName(name string) (GemMatch, error) {
	sk, err := s.Skills()
	if err != nil {
		return GemMatch{}, err
	}
	text, err := s.Text("en")
	if err != nil {
		return GemMatch{}, err
	}
	if m, ok := sk.choose(sk.gemsNamed(text.GemNames, name)); ok {
		return m, nil
	}
	actives := map[string]bool{}
	for _, n := range text.SkillNames {
		if n.Name == name {
			actives[n.ID] = true
		}
	}
	if m, ok := sk.choose(sk.gemsGranting(func(ge *GrantedEffect) bool { return actives[ge.ActiveSkill] })); ok {
		return m, nil
	}
	return GemMatch{}, fmt.Errorf("gem %q: %w", name, ErrNotInDataset)
}

// GemByGrantedEffect returns the gem whose effect grants granted effect id, chosen among
// several as GemByName chooses. An unknown id returns ErrNotInDataset.
func (s *Skills) GemByGrantedEffect(id string) (GemMatch, error) {
	if m, ok := s.choose(s.gemsGranting(func(ge *GrantedEffect) bool { return ge.ID == id })); ok {
		return m, nil
	}
	return GemMatch{}, fmt.Errorf("granted effect %q: %w", id, ErrNotInDataset)
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

// gemsNamed returns the first effect of each gem with effects whose name in names is name, or
// name and " Support" for a support, in the game's order.
func (s *Skills) gemsNamed(names []Name, name string) []GemMatch {
	exact, suffixed := map[string]bool{}, map[string]bool{}
	for _, n := range names {
		exact[n.ID] = exact[n.ID] || n.Name == name
		suffixed[n.ID] = suffixed[n.ID] || n.Name == name+supportSuffix
	}
	var out []GemMatch
	for i := range s.Gems {
		g := &s.Gems[i]
		if len(g.Effects) > 0 && (exact[g.ID] || suffixed[g.ID] && g.Support) {
			out = append(out, GemMatch{Gem: g, Effect: &g.Effects[0]})
		}
	}
	return out
}

// gemsGranting returns each gem effect whose granted effect matches, in the game's order.
func (s *Skills) gemsGranting(match func(*GrantedEffect) bool) []GemMatch {
	granted := map[string]bool{}
	for i := range s.GrantedEffects {
		if match(&s.GrantedEffects[i]) {
			granted[s.GrantedEffects[i].ID] = true
		}
	}
	var out []GemMatch
	for i := range s.Gems {
		g := &s.Gems[i]
		for j := range g.Effects {
			if granted[g.Effects[j].GrantedEffect] {
				out = append(out, GemMatch{Gem: g, Effect: &g.Effects[j]})
			}
		}
	}
	return out
}

// choose picks among candidates as GemByName describes, and reports false for none.
func (s *Skills) choose(candidates []GemMatch) (GemMatch, bool) {
	if len(candidates) == 0 {
		return GemMatch{}, false
	}
	quality := map[string]bool{}
	for _, ge := range s.GrantedEffects {
		quality[ge.ID] = len(ge.QualityStats) > 0
	}
	candidates = narrow(candidates, func(c GemMatch) bool { return quality[c.Effect.GrantedEffect] })
	candidates = narrow(candidates, func(c GemMatch) bool { return !strings.HasSuffix(c.Effect.ID, supersededSuffix) })
	m := candidates[0]
	for _, c := range candidates[1:] {
		// Effects of the first gem, such as a Vaal gem's variants of one granted effect, are
		// its own; another gem with the same gem effect is the same skill.
		if c.Gem != m.Gem && c.Effect.ID != m.Effect.ID && !slices.Contains(m.Others, c.Gem.ID) {
			m.Others = append(m.Others, c.Gem.ID)
		}
	}
	return m, true
}

// narrow returns the candidates keep holds for, or all of them when it holds for none.
func narrow(candidates []GemMatch, keep func(GemMatch) bool) []GemMatch {
	kept := slices.DeleteFunc(slices.Clone(candidates), func(c GemMatch) bool { return !keep(c) })
	if len(kept) == 0 {
		return candidates
	}
	return kept
}
