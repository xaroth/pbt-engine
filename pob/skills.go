package pob

import (
	"cmp"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/pob/internal/pobxml"
)

// slots maps PoB's names of the item slots with sockets.
func slots() map[string]build.Slot {
	return map[string]build.Slot{
		"Weapon 1": build.SlotWeapon, "Weapon 2": build.SlotOffhand,
		"Weapon 1 Swap": build.SlotWeaponSwap, "Weapon 2 Swap": build.SlotOffhandSwap,
		"Helmet": build.SlotHelmet, "Body Armour": build.SlotBodyArmour, "Gloves": build.SlotGloves, "Boots": build.SlotBoots,
		"Amulet": build.SlotAmulet,
	}
}

// skills imports the active skill set's groups. A gem resolves by its gem id, variant and skill
// id when the dataset has them, else by its name; one that does not resolve is dropped. A group
// an item or the tree grants keeps its source, with the granted skill by its skill id
// (grantedSkill). PoB's group of enemies exploding is left out, and so is every group when the
// dataset has no skills group to resolve gems with. A disabled group's gems are disabled.
//
// The main group is PoB's mainSocketGroup, 1-based over every group of the set; PoB uses the
// first group when it is unset and the last when it is past the end (CalcSetup.lua). A main
// group that is left out warns, and group 1 is used.
func (im *importer) skills(store *dataset.Store) (build.Skills, error) {
	var out build.Skills
	if len(im.file.Skills) == 0 {
		return out, nil
	}
	data, err := store.SkillIndex()
	if _, missing := errors.AsType[*dataset.MissingGroupError](err); missing {
		for i, g := range im.file.Skills {
			im.warnf(SkillIgnored, groupSubject(g, i), "dataset %s has no skills group to read gems with; not imported", store.Manifest().Version)
		}
		return out, nil
	}
	if err != nil {
		return out, fmt.Errorf("pob: %w", err)
	}
	index := map[int]int{}
	for i, g := range im.file.Skills {
		var src *build.SkillSource
		if g.Source != "" {
			var ok bool
			if src, ok = im.skillSource(g, i); !ok {
				continue
			}
		}
		index[i] = len(out.Groups)
		group, err := im.group(g, i, store, data)
		if err != nil {
			return out, err
		}
		group.Source = src
		out.Groups = append(out.Groups, group)
	}
	im.mainGroup(&out, index)
	return out, nil
}

// mainGroup sets the main group, index mapping file groups to the groups imported.
func (im *importer) mainGroup(out *build.Skills, index map[int]int) {
	groups := im.file.Skills
	main := min(max(im.file.Build.MainSocketGroup, 1), len(groups)) - 1
	if i, ok := index[main]; ok {
		out.MainGroup = i
		return
	}
	im.warnf(MainSkillIgnored, groupSubject(groups[main], main),
		"the main skill group, from %s, is not imported; group 1 used", groups[main].Source)
}

// skillSource returns what grants group g, the i-th of the file, from PoB's source: "Item:<id>:<name>"
// with the item's slot as the group's, or "Tree:<node>". Others warn and report false, such as
// "Explode", PoB's group of the enemies a build's stats make explode, which nothing grants.
func (im *importer) skillSource(g pobxml.SkillGroup, i int) (*build.SkillSource, bool) {
	kind, rest, _ := strings.Cut(g.Source, ":")
	switch kind {
	case "Item":
		if s, ok := itemSlots()[g.Slot]; ok {
			return &build.SkillSource{Item: s}, true
		}
	case "Tree":
		if n, err := strconv.ParseUint(rest, 10, 32); err == nil {
			return &build.SkillSource{Node: uint32(n)}, true
		}
	}
	im.warnf(SkillIgnored, groupSubject(g, i), "from %s, which is neither an item in a slot nor a passive; not imported", g.Source)
	return nil, false
}

// pobSkillIDs maps the skill ids PoB gives some granted skills to the game's.
func pobSkillIDs() map[string]string {
	return map[string]string{"AssassinsMark": "AssassinsMarkTriggeredAssassin", "BloodRage": "JunTriggeredBloodRage"}
}

// groupGem imports gem pg of group g: the skill g's source grants, a gem with a skill id and no
// gem id, or else a gem as gem does.
func (im *importer) groupGem(g pobxml.SkillGroup, pg pobxml.Gem, store *dataset.Store, data *dataset.SkillIndex) (build.Gem, bool) {
	if g.Source != "" && pg.GemID == "" && pg.SkillID != "" {
		return grantedSkill(pg), true
	}
	return im.gem(pg, store, data)
}

// grantedSkill imports the skill a group's source grants, by its skill id in the game's name.
func grantedSkill(g pobxml.Gem) build.Gem {
	id := g.SkillID
	if game, ok := pobSkillIDs()[id]; ok {
		id = game
	}
	return build.Gem{Skill: id, Level: g.Level, Quality: g.Quality, Disabled: !g.Enabled}
}

// group imports group g, the i-th of the file, with its count, full DPS flag and the gear of
// its main skill's minion (groupMinion).
func (im *importer) group(g pobxml.SkillGroup, i int, store *dataset.Store, data *dataset.SkillIndex) (build.SkillGroup, error) {
	subject := groupSubject(g, i)
	out := build.SkillGroup{Slot: slots()[g.Slot], FullDPS: g.IncludeInFullDPS}
	if out.Slot == "" && g.Slot != "" && g.Source == "" {
		im.warnf(SlotIgnored, subject, "slot %q has no sockets; group imported without a slot", g.Slot)
	}
	from := im.groupGems(&out, g, store, data)
	if len(out.Gems) == 0 {
		return out, nil
	}
	return out, im.groupMinion(&out, subject, g, from[out.Active], store, data)
}

// groupGems imports the gems of g into out with its main active skill, and returns the file's
// gem of each gem imported.
func (im *importer) groupGems(out *build.SkillGroup, g pobxml.SkillGroup, store *dataset.Store, data *dataset.SkillIndex) []pobxml.Gem {
	var from []pobxml.Gem
	var skills []gemSkill // each active skill PoB counts for the main active skill
	first := gemSkill{gem: -1}
	for _, pg := range g.Gems {
		gem, ok := im.groupGem(g, pg, store, data)
		if !ok {
			continue
		}
		gem.Disabled = gem.Disabled || !g.Enabled
		active := build.ActiveSkills(data, gem)
		if first.gem < 0 && len(active) > 0 {
			first = gemSkill{gem: len(out.Gems), skill: active[0]}
		}
		if !gem.Disabled {
			skills = append(skills, countedSkills(pg, len(out.Gems), active)...)
		}
		out.Gems = append(out.Gems, gem)
		from = append(from, pg)
	}
	main := activeSkill(skills, first, g.MainActiveSkill)
	out.Active, out.ActiveSkill = main.gem, main.skill
	return from
}

// groupMinion sets the count of out, imported from g, and the gear of its main skill's minion
// (minionGear) from pg, that skill's gem. The count is PoB's groupCount, else pg's count
// (Calcs.lua getActiveSkillCount).
func (im *importer) groupMinion(
	out *build.SkillGroup, subject string, g pobxml.SkillGroup, pg pobxml.Gem, store *dataset.Store, data *dataset.SkillIndex,
) error {
	if n := cmp.Or(g.GroupCount, pg.Count); n > 1 {
		out.Count = n
	}
	gem := out.Gems[out.Active]
	if len(build.ActiveSkills(data, gem)) == 0 {
		return nil
	}
	var err error
	out.MinionItems, err = im.minionGear(subject, build.GemSkill(data, gem, out.ActiveSkill), pg, itemReader(store))
	return err
}

// countedSkills returns the active skills of gem g, at index gem, that PoB counts for the main
// active skill, in order. PoB leaves out a skill switched off (CalcSetup.lua), and lets only a
// Vaal gem's skills, the ones with global effects, be switched.
func countedSkills(g pobxml.Gem, gem int, active []int) []gemSkill {
	var out []gemSkill
	for _, i := range active {
		if len(active) < 2 || []bool{g.EnableGlobal1, g.EnableGlobal2}[i] {
			out = append(out, gemSkill{gem: gem, skill: i})
		}
	}
	return out
}

// gemSkill is skill of the gem at index gem: 0 its first, 1 its second.
type gemSkill struct{ gem, skill int }

// gem imports gem g, or warns and reports false.
func (im *importer) gem(g pobxml.Gem, store *dataset.Store, data *dataset.SkillIndex) (build.Gem, bool) {
	m, ok := resolveGem(g, store, data)
	if !ok {
		im.warnf(GemUnknown, cmp.Or(g.Name, g.GemID, g.SkillID), "gem not in dataset %s; dropped", store.Manifest().Version)
		return build.Gem{}, false
	}
	if len(m.Others) > 0 {
		im.warnf(GemAmbiguous, cmp.Or(g.Name, g.SkillID), "also the gem of %s; %s imported", strings.Join(m.Others, ", "), m.Gem.ID)
	}
	dg, effect := m.Gem, m.Effect
	if dg.Vaal && (!g.EnableGlobal1 || !g.EnableGlobal2) {
		im.warnf(GemToggleIgnored, g.Name, "one of the Vaal gem's two skills is switched off; the switch is not imported")
	}
	out := build.Gem{ID: dg.ID, Level: g.Level, Quality: g.Quality, Support: dg.Support, Disabled: !g.Enabled}
	if effect != &dg.Effects[0] {
		out.Effect = effect.ID
	}
	im.skillPart(g, &out, data)
	return out, true
}

// skillPart sets gem's stat set and stages from PoB's skill part and stage count. PoB's parts
// are its own list per skill; part n is taken as the skill's nth stat set, as for Lightning
// Strike's melee hit and projectiles, when each active skill of the gem has that many sets. Its
// first part is the primary set. A later part with no such set has no counterpart in the game
// data and warns, and its stages go with it.
func (im *importer) skillPart(g pobxml.Gem, gem *build.Gem, data *dataset.SkillIndex) {
	if g.SkillPart > 1 {
		for _, which := range build.ActiveSkills(data, *gem) {
			if sets := len(skillSets(data, *gem, which)); g.SkillPart > sets {
				im.warnf(SkillPartIgnored, g.Name, "PoB's skill part %d has no stat set of %s, which has %d; its primary set and no stages used",
					g.SkillPart, build.GemSkill(data, *gem, which), sets)
				return
			}
		}
		gem.StatSet = g.SkillPart - 1
	}
	gem.Stages = max(g.StageCount, 0)
}

// skillSets returns the stat set ids of skill which of gem.
func skillSets(data *dataset.SkillIndex, gem build.Gem, which int) []string {
	if ge := data.GrantedEffect(build.GemSkill(data, gem, which)); ge != nil {
		return ge.StatSets
	}
	return nil
}

// activeSkill returns PoB's 1-based main active skill n among skills, the first when n is below
// 1 and the last when it is past the end, as PoB caps it (CalcSetup.lua); without skills,
// first, the first active skill of the group, else gem 0's first skill.
func activeSkill(skills []gemSkill, first gemSkill, n int) gemSkill {
	switch {
	case len(skills) > 0:
		return skills[min(max(n, 1), len(skills))-1]
	case first.gem >= 0:
		return first
	}
	return gemSkill{}
}

// resolveGem returns the dataset gem and effect of g, trying in turn: its gem id with its
// variant id, then with its skill id, then its first effect; the gem whose effect grants its
// skill id; its name. This is the order PoB loads gems in (SkillsTab.lua, LoadSkill).
func resolveGem(g pobxml.Gem, store *dataset.Store, data *dataset.SkillIndex) (dataset.GemMatch, bool) {
	if dg := data.Gem(g.GemID); dg != nil && len(dg.Effects) > 0 {
		m := dataset.GemMatch{Gem: dg, Effect: &dg.Effects[0]}
		if e := dg.Effect(g.VariantID); e != nil {
			m.Effect = e
		} else if i := slices.IndexFunc(dg.Effects, func(e dataset.GemEffect) bool { return e.GrantedEffect == g.SkillID }); i >= 0 {
			m.Effect = &dg.Effects[i]
		}
		return m, true
	}
	if m, err := data.GemByGrantedEffect(g.SkillID); g.SkillID != "" && err == nil {
		return m, true
	}
	if m, err := store.GemByName(g.Name); err == nil {
		return m, true
	}
	return dataset.GemMatch{}, false
}

// groupSubject names a group by its label, else its first gem, else its position.
func groupSubject(g pobxml.SkillGroup, i int) string {
	if g.Label != "" {
		return g.Label
	}
	if len(g.Gems) > 0 {
		return cmp.Or(g.Gems[0].Name, g.Gems[0].GemID)
	}
	return "skill group " + strconv.Itoa(i+1)
}
