package dataset

import (
	"fmt"
	"slices"
)

// validateGrantedSkills checks that every skill a mod or tree node grants names an effect of
// the skills group at one of its levels. The Store does not repeat it, so reading mods or a
// tree does not read the skills group.
func (d *Dataset) validateGrantedSkills() error {
	grants := d.grants()
	if len(grants) == 0 {
		return nil
	}
	if d.Skills == nil {
		return fmt.Errorf("%s: granted skill without skills: %w", grants[0].owner, ErrInvalid)
	}
	levels := make(map[string][]EffectLevel, len(d.Skills.GrantedEffects))
	for _, e := range d.Skills.GrantedEffects {
		levels[e.ID] = e.Levels
	}
	for _, g := range grants {
		ls, ok := levels[g.skill.Effect]
		if !ok {
			return fmt.Errorf("%s: granted effect %q not in skills: %w", g.owner, g.skill.Effect, ErrInvalid)
		}
		if _, found := slices.BinarySearchFunc(ls, g.skill.Level, func(l EffectLevel, v int32) int { return int(l.Level - v) }); !found {
			return fmt.Errorf("%s: granted effect %q has no level %d: %w", g.owner, g.skill.Effect, g.skill.Level, ErrInvalid)
		}
	}
	return nil
}

type grant struct {
	owner string
	skill GrantedSkill
}

func (d *Dataset) grants() []grant {
	var grants []grant
	for _, m := range d.Mods {
		for _, g := range m.GrantedSkills {
			grants = append(grants, grant{"mod " + m.ID, g})
		}
	}
	for _, t := range d.Trees {
		for _, n := range t.Nodes {
			if n.GrantedSkill != nil {
				grants = append(grants, grant{fmt.Sprintf("tree %s node %d", t.Variant, n.ID), *n.GrantedSkill})
			}
		}
	}
	return grants
}
