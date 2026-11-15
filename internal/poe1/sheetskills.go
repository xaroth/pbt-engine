package poe1

import (
	"errors"
	"fmt"
	"slices"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/internal/resolve"
)

// Warning codes of SheetSkillsOf.
const (
	// SkillsNotSimulated is a build with skill groups whose hits the sheet does not give: outputs
	// leave them out until those skills are simulated.
	SkillsNotSimulated build.Code = "skills-not-simulated"
	// SkillUnresolved is a skill asked for that the dataset does not resolve, such as one without
	// levels: it has no outputs.
	SkillUnresolved build.Code = "skill-unresolved"
)

// SheetSkills are the skills a build puts in a sheet's skill slots (SetSkills).
type SheetSkills struct {
	// Main is the main skill, nil when the build has none.
	Main *resolve.Skill
	// Skills are in slot order: the main skill first, if any, then the other skills that
	// reserve life or mana, then, when every skill was asked for, the other skills.
	Skills []*resolve.Skill
	// Every says Skills holds every skill of the build the sheet can hold.
	Every    bool
	Warnings []build.Warning
}

// SheetSkillsOf resolves the skills of c to put on a sheet with hands: the main skill, the
// default attack when the build's main group has no active gem (resolve.SkillResolver.Default),
// as when it has no groups, none when the dataset has no skills group or lacks the main gem
// (the build check warns about that), the other skills that reserve life or mana and, with
// every, each other skill of the build (resolve.SkillResolver.All), so the sheet gives each
// skill's outputs. It warns about the gems whose hits the sheet does not give and, with every,
// about the skills that do not resolve or do not fit the sheet.
func SheetSkillsOf(c *build.Checked, hands Hands, every bool) (SheetSkills, error) {
	out := SheetSkills{Every: every}
	sk := c.Build().Skills
	skills, err := resolve.NewSkillResolver(c, GemRules())
	switch {
	case errors.Is(err, dataset.ErrNotInDataset):
		skills = nil
	case err != nil:
		return out, fmt.Errorf("sheet skills: %w", err)
	default:
		main, warnings, err := skills.Group(sk.MainGroup)
		if errors.Is(err, resolve.ErrNoActiveSkill) {
			main, err = skills.Default()
		}
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
	if skills != nil {
		out.addOthers(skills.All())
	}
	if w, ok := notSimulated(sk, out.Read(), hands); ok {
		out.Warnings = append(out.Warnings, w)
	}
	return out, nil
}

// Read returns the skills whose outputs are read: the main skill, or with Every each skill.
func (s *SheetSkills) Read() []*resolve.Skill {
	if s.Every {
		return s.Skills
	}
	if s.Main == nil {
		return nil
	}
	return s.Skills[:1]
}

// addOthers adds the skills of all other than the main skill: those that reserve, then with
// Every the rest, warning about those that do not resolve or do not fit.
func (s *SheetSkills) addOthers(all []resolve.SkillWarnings) {
	var rest []resolve.SkillWarnings
	for _, sw := range all {
		switch {
		case sw.Skill != nil && sw.Skill == s.Main:
		case sw.Err != nil:
			if s.Every {
				s.Warnings = append(s.Warnings, build.Warning{
					Code: SkillUnresolved, Subject: sw.Source.String(), Message: fmt.Sprintf("skill %d: %v; no outputs", sw.Which+1, sw.Err),
				})
			}
		case Reserves(sw.Skill):
			s.Skills = append(s.Skills, sw.Skill)
			s.Warnings = append(s.Warnings, sw.Warnings...)
		case s.Every:
			rest = append(rest, sw)
		}
	}
	for i, sw := range rest {
		if len(s.Skills) == maxSkills {
			msg := fmt.Sprintf("%d skills beyond the sheet's %d not simulated", len(rest)-i, maxSkills)
			s.Warnings = append(s.Warnings, build.Warning{Code: SkillsNotSimulated, Subject: "skills", Message: msg})
			return
		}
		s.Skills = append(s.Skills, sw.Skill)
		s.Warnings = append(s.Warnings, sw.Warnings...)
	}
}

// notSimulated warns about the gems of s outside the groups of read whose hit the sheet gives
// with hands.
func notSimulated(s build.Skills, read []*resolve.Skill, hands Hands) (build.Warning, bool) {
	gems, groups := 0, 0
	for i, g := range s.Groups {
		if len(g.Gems) == 0 || slices.ContainsFunc(read, func(sk *resolve.Skill) bool { return sk.Source.Group == i && Hit(sk, hands) }) {
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
