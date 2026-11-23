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
	// SkillNeedsShield is a skill that requires a shield in a build whose off hand holds none:
	// the skill is off, it reserves nothing, gives no buff and no hit.
	SkillNeedsShield build.Code = "skill-needs-shield"
)

// typeRequiresShield is the skill type of a skill that cannot be used without a shield.
const typeRequiresShield = "RequiresShield"

// SheetSkills are the skills a build puts in a sheet's skill slots (SetSkills).
type SheetSkills struct {
	// Main is the main skill, nil when the build has none.
	Main *resolve.Skill
	// Skills are in slot order: the main skill first, if any, then the other skills that
	// reserve life or mana, grant you a buff or apply a curse, then the other skills, as many
	// as the sheet holds. Every skill is on the sheet so the conditions it causes can be derived
	// (Sheet.DeriveConditions).
	Skills []*resolve.Skill
	// Every says the outputs of every skill are read, not only the main skill's.
	Every bool
	// Kept is how many of Skills, from the first, are read and noted: the main skill, those
	// that reserve, buff or curse and, with Every, the rest. The others are on the sheet only to derive the
	// conditions they cause (Sheet.DeriveOnly).
	Kept int
	// AnimatedWeapons is the number of Animated Weapons the build's skills allow.
	AnimatedWeapons int
	Warnings        []build.Warning
	// off are the skills left out because they need a shield and the hands hold none.
	off []*resolve.Skill
	// store names the skills in warnings.
	store *dataset.Store
}

// SheetSkillsOf resolves the skills of c to put on a sheet with hands: the main skill, the
// default attack when the build's main group has no active gem (resolve.SkillResolver.Default),
// as when it has no groups, none when the dataset has no skills group or lacks the main gem
// (the build check warns about that), the other skills that reserve life or mana, grant you a
// buff or apply a curse and each other skill of the build (resolve.SkillResolver.All). With every, the sheet's outputs of each
// skill are read. A skill that requires a shield is left out, with a warning, while hands hold
// none, as the game does not let it be used. It warns about the gems whose hits the sheet does
// not give and, with every, about the skills that do not resolve or do not fit the sheet. It
// gives the number of Animated Weapons the skills allow and warns about each group counted as
// more.
func SheetSkillsOf(c *build.Checked, hands Hands, every bool) (SheetSkills, error) {
	out := SheetSkills{Every: every, store: c.Store()}
	sk := c.Build().Skills
	skills, err := resolve.NewSkillResolver(c, GemRules())
	switch {
	case errors.Is(err, dataset.ErrNotInDataset):
		skills = nil
	case err != nil:
		return out, fmt.Errorf("sheet skills: %w", err)
	default:
		if err := out.addMain(skills, hands, sk.MainGroup); err != nil {
			return out, err
		}
	}
	if skills != nil {
		all := skills.All()
		out.addOthers(all, hands, sk.Groups)
		out.AnimatedWeapons = animatedWeaponsAllowed(all)
		out.Warnings = append(out.Warnings, countWarnings(all, sk.Groups, out.AnimatedWeapons)...)
	}
	if w, ok := notSimulated(sk, out.Read(), out.off, hands); ok {
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

// addMain makes the skill of group the main skill, unless the dataset lacks it or it needs a
// shield hands lack.
func (s *SheetSkills) addMain(skills *resolve.SkillResolver, hands Hands, group int) error {
	main, warnings, err := skills.Group(group)
	if errors.Is(err, resolve.ErrNoActiveSkill) {
		main, err = skills.Default()
	}
	switch {
	case errors.Is(err, resolve.ErrNoActiveSkill), errors.Is(err, dataset.ErrNotInDataset):
		return nil
	case err != nil:
		return fmt.Errorf("sheet skills: %w", err)
	}
	s.Warnings = append(s.Warnings, warnings...)
	if !s.needsShield(main, hands) {
		s.Main = main
		s.Skills = append(s.Skills, main)
	}
	return nil
}

// addOthers adds the skills of all other than the main skill: those that reserve or whose buff
// affects you (buffList), then those that curse (curseList), then the rest, warning with Every about those that do not resolve or
// do not fit and with the rest's own warnings, whose outputs are then read. Without Every the
// rest are only those whose hit the sheet gives with hands, the skills that can cause a
// condition.
func (s *SheetSkills) addOthers(all []resolve.SkillWarnings, hands Hands, groups []build.SkillGroup) {
	var rest []resolve.SkillWarnings
	var curses curseList
	all = s.usable(all, hands)
	buffs := s.buffList(all)
	for _, sw := range all {
		switch {
		case sw.Skill != nil && sw.Skill == s.Main:
		case sw.Err != nil:
			s.unresolved(sw)
		case keeps(sw.Skill, buffs):
			s.keep(sw)
		case curses.take(sw):
		case s.Every || Hit(sw.Skill, hands):
			rest = append(rest, sw)
		}
	}
	s.keep(curses.ordered(groups)...)
	s.Kept = len(s.Skills)
	s.addRest(rest)
	if s.Every {
		s.Kept = len(s.Skills)
	}
}

// usable returns all without the skills that need a shield hands lack.
func (s *SheetSkills) usable(all []resolve.SkillWarnings, hands Hands) []resolve.SkillWarnings {
	return slices.DeleteFunc(slices.Clone(all), func(sw resolve.SkillWarnings) bool {
		return sw.Skill != nil && s.needsShield(sw.Skill, hands)
	})
}

// needsShield reports whether sk requires a shield that hands lack. Such a skill is left out
// of the sheet's skills, and warned about once.
func (s *SheetSkills) needsShield(sk *resolve.Skill, hands Hands) bool {
	if hands.Shield || !slices.Contains(sk.Types, typeRequiresShield) {
		return false
	}
	// The main skill comes again with the others.
	if slices.ContainsFunc(s.off, func(o *resolve.Skill) bool { return o.Source == sk.Source && o.Which == sk.Which }) {
		return true
	}
	s.off = append(s.off, sk)
	s.Warnings = append(s.Warnings, build.Warning{
		Code: SkillNeedsShield, Subject: sk.Source.String(), Message: s.skillName(sk) + " needs a shield; it is off",
	})
	return true
}

// skillName is the English name of sk's active skill, its id when the dataset has no name.
func (s *SheetSkills) skillName(sk *resolve.Skill) string {
	if text, err := s.store.Text("en"); err == nil {
		if i := slices.IndexFunc(text.SkillNames, func(n dataset.Name) bool { return n.ID == sk.ActiveSkill }); i >= 0 {
			return text.SkillNames[i].Name
		}
	}
	return sk.ActiveSkill
}

// keeps reports whether sk is kept beside the main skill for itself: it reserves life or mana
// or its buff affects you (buffs).
func keeps(sk *resolve.Skill, buffs *buffList) bool { return buffs.take(sk) || Reserves(sk) }

// buffList returns the list of the buffs of all, in slot order, with the main skill taken.
func (s *SheetSkills) buffList(all []resolve.SkillWarnings) *buffList {
	skills := slices.Clone(s.Skills)
	for _, sw := range all {
		if sw.Err == nil && sw.Skill != s.Main {
			skills = append(skills, sw.Skill)
		}
	}
	l := newBuffList(skillBuffs(), skills)
	for _, sk := range s.Skills {
		l.take(sk)
	}
	return l
}

// keep adds sws with their warnings.
func (s *SheetSkills) keep(sws ...resolve.SkillWarnings) {
	for _, sw := range sws {
		s.Skills = append(s.Skills, sw.Skill)
		s.Warnings = append(s.Warnings, sw.Warnings...)
	}
}

// unresolved warns with Every about sw, which does not resolve.
func (s *SheetSkills) unresolved(sw resolve.SkillWarnings) {
	if s.Every {
		s.Warnings = append(s.Warnings, build.Warning{
			Code: SkillUnresolved, Subject: sw.Source.String(), Message: fmt.Sprintf("skill %d: %v; no outputs", sw.Which+1, sw.Err),
		})
	}
}

// addRest adds the skills of rest as long as the sheet has slots, with Every with their
// warnings and one about those that do not fit.
func (s *SheetSkills) addRest(rest []resolve.SkillWarnings) {
	for i, sw := range rest {
		if len(s.Skills) == maxSkills {
			if s.Every {
				msg := fmt.Sprintf("%d skills beyond the sheet's %d not simulated", len(rest)-i, maxSkills)
				s.Warnings = append(s.Warnings, build.Warning{Code: SkillsNotSimulated, Subject: "skills", Message: msg})
			}
			return
		}
		s.Skills = append(s.Skills, sw.Skill)
		if s.Every {
			s.Warnings = append(s.Warnings, sw.Warnings...)
		}
	}
}

// notSimulated warns about the gems of s outside the groups of read whose hit the sheet gives
// with hands. A group with a skill in off is left out: its own warning says why.
func notSimulated(s build.Skills, read, off []*resolve.Skill, hands Hands) (build.Warning, bool) {
	gems, groups := 0, 0
	inGroup := func(i int) func(*resolve.Skill) bool {
		return func(sk *resolve.Skill) bool { return sk.Source.Group == i }
	}
	for i, g := range s.Groups {
		if len(g.Gems) == 0 || slices.ContainsFunc(off, inGroup(i)) ||
			slices.ContainsFunc(read, func(sk *resolve.Skill) bool { return inGroup(i)(sk) && Hit(sk, hands) }) {
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
