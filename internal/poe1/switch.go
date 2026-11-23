package poe1

import (
	"errors"
	"fmt"
	"slices"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/config"
	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/internal/resolve"
)

// Switch is what a sheet is switched to for one checked build (Sheet.Switch): its level,
// nodes, masteries, items, hands, skills, config and enemy, with its point budget.
type Switch struct {
	Checked *build.Checked
	Config  config.Resolved
	Items   []Item
	// Hands is what the character attacks with.
	Hands  Hands
	Skills SheetSkills
	Budget Budget
	// Enemy is what the main skill's hits are measured against.
	Enemy EnemyStats
	// Totems gives the base life of the skills' totems.
	Totems TotemLives
	// Minions are the minions of the skills whose outputs are read, by slot (minionsOf).
	Minions []*Minion
	// Warnings are the skills' (SheetSkillsOf), then the budget's (PointBudget).
	Warnings []build.Warning
}

// SwitchOf returns what a sheet is switched to for c under cfg with its compiled items and its
// minions' gear; with every, each skill of the build goes on the sheet (SheetSkillsOf). The
// enemy is the default one unless the sheet gives the hit of a skill whose outputs are read or
// of its minion; it reads the monsters
// group of c's dataset only when its kind takes a normal monster's armour and evasion, for the
// totem life of a totem skill whose outputs are read, or for the minion of such a skill.
func SwitchOf(c *build.Checked, cfg config.Resolved, items []Item, gear MinionGear, every bool) (Switch, error) {
	sw := Switch{Checked: c, Config: cfg, Items: items, Hands: HandsOf(items, c.Character())}
	var err error
	if sw.Skills, err = SheetSkillsOf(c, sw.Hands, every); err != nil {
		return sw, err
	}
	sw.Warnings = append(sw.Warnings, sw.Skills.Warnings...)
	if sw.Minions, err = minionsOf(&sw, gear); err != nil {
		return sw, err
	}
	if w, ok := sw.notSimulated(); ok {
		sw.Warnings = append(sw.Warnings, w)
	}
	var over []build.Warning
	if sw.Budget, over, err = PointBudget(c, cfg); err != nil {
		return sw, err
	}
	sw.Warnings = append(sw.Warnings, over...)
	if sw.Enemy, err = enemyOf(&sw); err != nil {
		return sw, err
	}
	if sw.Totems, err = totemsOf(&sw); err != nil {
		return sw, err
	}
	return sw, nil
}

// totemsOf returns the base life of totems from the monsters group of sw's dataset when a skill
// whose outputs are read is a totem's; none otherwise or without the group.
func totemsOf(sw *Switch) (TotemLives, error) {
	if !slices.ContainsFunc(sw.Skills.Read(), func(sk *resolve.Skill) bool { return hasTypes(sk, typeSummonsTotem) }) {
		return TotemLives{}, nil
	}
	m, err := sw.Checked.Store().Monsters()
	switch {
	case errors.Is(err, dataset.ErrNotInDataset):
		return TotemLives{}, nil
	case err != nil:
		return TotemLives{}, err //nolint:wrapcheck // wrapped by the caller
	}
	return TotemLivesOf(m), nil
}

// enemyOf resolves the enemy of sw's config when the sheet gives the hit of a skill whose
// outputs are read or of its minion, the default enemy otherwise. Only an enemy kind with a
// normal monster's armour and evasion reads them from the dataset's monsters group, and fails
// without it.
func enemyOf(sw *Switch) (EnemyStats, error) {
	if !sw.hits() {
		return DefaultEnemy(), nil
	}
	var levels []dataset.MonsterLevel
	if enemyReadsMonsters(sw.Config) {
		m, err := sw.Checked.Store().Monsters()
		switch {
		case err == nil:
			levels = m.Levels
		case !errors.Is(err, dataset.ErrNotInDataset):
			return EnemyStats{}, err //nolint:wrapcheck // wrapped by the caller
		}
	}
	return EnemyOf(sw.Config, sw.Checked.Build().Level, levels)
}

// Minion returns the minion of the skill in slot whose outputs are read, nil for none.
func (sw *Switch) Minion(slot int) *Minion {
	if slot < 0 || slot >= len(sw.Minions) {
		return nil
	}
	return sw.Minions[slot]
}

// hits reports whether the sheet gives the hit of a skill whose outputs are read or of its minion.
func (sw *Switch) hits() bool {
	for i, sk := range sw.Skills.Read() {
		if Hit(sk, sw.Hands) || sw.Minion(i).Hit() {
			return true
		}
	}
	return false
}

// notSimulated warns about the gems outside the groups of the skills whose outputs are read
// whose hit, or whose minion's hit, the sheet gives. A group with a skill the hands lack a
// shield for is left out: its own warning says why.
func (sw *Switch) notSimulated() (build.Warning, bool) {
	read := sw.Skills.Read()
	gems, groups := 0, 0
	for i, g := range sw.Checked.Build().Skills.Groups {
		inGroup := func(sk *resolve.Skill) bool { return sk.Source.Group == i }
		hit := false
		for slot, sk := range read {
			hit = hit || inGroup(sk) && (Hit(sk, sw.Hands) || sw.Minion(slot).Hit())
		}
		if len(g.Gems) == 0 || slices.ContainsFunc(sw.Skills.off, inGroup) || hit {
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

// Switch switches sh to sw's level, nodes, masteries, items, hands, totem lives, skills, their
// minions, the skills counted in full DPS, config and enemy.
func (sh *Sheet) Switch(sw *Switch) error {
	cb := sw.Checked.Build()
	if err := sh.SetLevel(cb.Level); err != nil {
		return err
	}
	sh.SetNodes(cb.Nodes, cb.Masteries)
	if err := sh.SetItems(sw.Items); err != nil {
		return err
	}
	if err := sh.SetHands(sw.Hands); err != nil {
		return err
	}
	sh.SetTotemLives(sw.Totems)
	sh.SetAnimatedWeaponsAllowed(sw.Skills.AnimatedWeapons)
	if err := sh.SetSkills(sw.Skills.Skills); err != nil {
		return err
	}
	if err := sh.SetMinions(sw.Minions); err != nil {
		return err
	}
	sh.SetFullDPS(sw.Skills.FullDPS)
	sh.DeriveOnly(sw.Skills.Kept)
	sh.SetConfig(sw.Config)
	sh.SetEnemy(sw.Enemy)
	return nil
}
