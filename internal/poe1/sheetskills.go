package poe1

import (
	"errors"
	"fmt"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/internal/resolve"
)

// SkillsNotSimulated is a build with skill groups beyond the main skill's hit: outputs leave
// them out until those skills are simulated.
const SkillsNotSimulated build.Code = "skills-not-simulated"

// SheetSkills are the skills a build puts in a sheet's skill slots (SetSkills).
type SheetSkills struct {
	// Main is the main skill, nil when the build has none.
	Main *resolve.Skill
	// Skills are in slot order: the main skill first, if any, then the other skills that
	// reserve life or mana.
	Skills   []*resolve.Skill
	Warnings []build.Warning
}

// SheetSkillsOf resolves the skills of c to put on a sheet with hands: the main skill, none
// when the build has no group with an active gem to resolve or the dataset has no skills group
// (the build check warns about that), and the other skills that reserve life or mana. It warns
// about the gems whose hits the sheet does not give.
func SheetSkillsOf(c *build.Checked, hands Hands) (SheetSkills, error) {
	var out SheetSkills
	sk := c.Build().Skills
	if len(sk.Groups) == 0 {
		return out, nil
	}
	skills, err := resolve.NewSkillResolver(c, GemRules())
	switch {
	case errors.Is(err, dataset.ErrNotInDataset):
		skills = nil
	case err != nil:
		return out, fmt.Errorf("sheet skills: %w", err)
	default:
		main, warnings, err := skills.Group(sk.MainGroup)
		switch {
		case errors.Is(err, resolve.ErrNoActiveSkill), errors.Is(err, dataset.ErrNotInDataset):
		case err != nil:
			return out, fmt.Errorf("sheet skills: %w", err)
		default:
			out.Main = main
			out.Skills = append(out.Skills, main)
			out.Warnings = append(out.Warnings, warnings...)
		}
	}
	if w, ok := notSimulated(sk, out.Main, hands); ok {
		out.Warnings = append(out.Warnings, w)
	}
	if skills != nil {
		out.addReserving(skills.All())
	}
	return out, nil
}

// addReserving adds the skills of all other than the main skill that reserve life or mana.
func (s *SheetSkills) addReserving(all []resolve.SkillWarnings) {
	for _, sw := range all {
		if sw.Skill == s.Main || !Reserves(sw.Skill) {
			continue
		}
		s.Skills = append(s.Skills, sw.Skill)
		s.Warnings = append(s.Warnings, sw.Warnings...)
	}
}

// notSimulated warns about the gems of s outside the main skill's group whose hit the sheet
// gives with hands (main, nil for none).
func notSimulated(s build.Skills, main *resolve.Skill, hands Hands) (build.Warning, bool) {
	gems, groups := 0, 0
	for i, g := range s.Groups {
		if len(g.Gems) == 0 || main != nil && i == main.Source.Group && Hit(main, hands) {
			continue
		}
		gems += len(g.Gems)
		groups++
	}
	if gems == 0 {
		return build.Warning{}, false
	}
	msg := fmt.Sprintf("%d gems in %d groups not simulated yet; outputs leave them out", gems, groups)
	return build.Warning{Code: SkillsNotSimulated, Subject: "skills", Message: msg}, true
}
