package pob

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/dataset"
)

// skills writes b's groups as one skill set, with their counts, full DPS flags and minion skills.
// Gems are named by gem id, variant and skill id, the order PoB loads them in, so a name PoB
// spells differently does not matter.
func (ex *exporter) skills(store *dataset.Store, items *itemWriter) (*outSkills, error) {
	groups := ex.b.Skills.Groups
	out := &outSkills{Active: "1", Sets: []outSkillSet{{ID: "1"}}}
	if len(groups) == 0 {
		return out, nil
	}
	data, err := store.SkillIndex()
	if _, missing := errors.AsType[*dataset.MissingGroupError](err); missing {
		data = nil
	} else if err != nil {
		return nil, fmt.Errorf("pob: %w", err)
	}
	text, err := store.Text("en")
	if err != nil {
		return nil, fmt.Errorf("pob: %w", err)
	}
	for i, g := range groups {
		out.Sets[0].Groups = append(out.Sets[0].Groups, ex.group(i, g, data, text, items))
	}
	return out, nil
}

// group writes g, the i-th group, with the item that grants it as its source, its minion's
// skill, and its minion's gear in an item set of its own.
func (ex *exporter) group(i int, g build.SkillGroup, data *dataset.SkillIndex, text *dataset.Text, items *itemWriter) outSkill {
	out := outSkill{Enabled: xmlTrue, Slot: pobSlot(g.Slot)}
	if g.FullDPS {
		out.FullDPS = xmlTrue
	}
	if g.Count > 0 {
		out.GroupCount = strconv.Itoa(g.Count)
	}
	if g.Source != nil {
		groupSource(&out, *g.Source, items)
	}
	for _, gem := range g.Gems {
		out.Gems = append(out.Gems, ex.gem(gem, data, text))
	}
	out.MainActive = strconv.Itoa(mainActive(g, data))
	if g.Active < len(out.Gems) && (len(g.MinionItems) > 0 || takesGear(data, g)) {
		// A minion without a set of its own takes the character's items.
		out.Gems[g.Active].MinionItemSet = items.minionSet(i, g.MinionItems)
	}
	if g.Active < len(out.Gems) && g.MinionSkill > 0 {
		out.Gems[g.Active].MinionSkill = strconv.Itoa(g.MinionSkill + 1)
	}
	return out
}

// mainActive is PoB's main active skill of g. PoB counts the active skills of enabled gems in
// order, 1-based; both switches of every gem are written on. See countedSkills.
func mainActive(g build.SkillGroup, data *dataset.SkillIndex) int {
	main := 0
	for i, gem := range g.Gems {
		active := activeSkillsOf(data, gem)
		switch {
		case i == g.Active:
			main += max(slices.Index(active, g.ActiveSkill), 0) + 1
		case i < g.Active && !gem.Disabled:
			main += len(active)
		}
	}
	return max(main, 1)
}

// groupSource writes what grants a group into out.
func groupSource(out *outSkill, src build.SkillSource, items *itemWriter) {
	if src.Node != 0 {
		// PoB matches the group to the grant it finds on the node.
		out.Source = "Tree:" + strconv.FormatUint(uint64(src.Node), 10)
		return
	}
	// Without its item, which warns, the group is written as one of the player's.
	if s, ok := items.source(src.Item); ok {
		out.Source, out.Slot = s, pobItemSlot(src.Item)
	}
}

// takesGear reports whether the minion of g's main skill, which g has, uses gear.
func takesGear(data *dataset.SkillIndex, g build.SkillGroup) bool {
	return data != nil && build.MinionSlots(build.GemSkill(data, g.Gems[g.Active], g.ActiveSkill)) != nil
}

// activeSkillsOf is build.ActiveSkills, or an active gem's first skill without a skills group.
func activeSkillsOf(data *dataset.SkillIndex, gem build.Gem) []int {
	switch {
	case data != nil:
		return build.ActiveSkills(data, gem)
	case !gem.Support:
		return []int{0}
	}
	return nil
}

// gem writes gem.
func (ex *exporter) gem(gem build.Gem, data *dataset.SkillIndex, text *dataset.Text) outGem {
	if gem.Skill != "" {
		return grantedGem(gem)
	}
	out := outGem{
		GemID: gem.ID, VariantID: gem.Effect, Level: gem.Level, Quality: gem.Quality,
		Enabled: strconv.FormatBool(!gem.Disabled), EnableGlobal1: xmlTrue, EnableGlobal2: xmlTrue,
	}
	if data == nil {
		return out
	}
	dg := data.Gem(gem.ID)
	if dg == nil || len(dg.Effects) == 0 {
		ex.warnf(GemUnknown, gem.ID, "gem not in dataset; written by id only")
		return out
	}
	effect := &dg.Effects[0]
	if e := dg.Effect(gem.Effect); e != nil {
		effect = e
	}
	// PoB takes the first variant granting the skill id when variantId is missing, and a Vaal
	// gem shares its skill id with its transfigured Vaal variants.
	out.VariantID, out.SkillID = effect.ID, effect.GrantedEffect
	if i := slices.IndexFunc(text.GemNames, func(n dataset.Name) bool { return n.ID == gem.ID }); i >= 0 {
		// PoB names supports without the suffix (SkillsTab.lua).
		out.Name = strings.TrimSuffix(text.GemNames[i].Name, " Support")
	}
	return out
}

// grantedGem writes a skill an item or passive grants by its skill id alone, as PoB does.
func grantedGem(gem build.Gem) outGem {
	id := gem.Skill
	for pobID, game := range pobSkillIDs() {
		if game == id {
			id = pobID
		}
	}
	return outGem{
		SkillID: id, Level: gem.Level, Quality: gem.Quality,
		Enabled: strconv.FormatBool(!gem.Disabled), EnableGlobal1: xmlTrue, EnableGlobal2: xmlTrue,
	}
}

// pobSlot is the inverse of slots.
func pobSlot(s build.Slot) string {
	for name, slot := range slots() {
		if slot == s {
			return name
		}
	}
	return ""
}

type outSkills struct {
	Active string        `xml:"activeSkillSet,attr"`
	Sets   []outSkillSet `xml:"SkillSet"`
}

type outSkillSet struct {
	ID     string     `xml:"id,attr"`
	Groups []outSkill `xml:"Skill"`
}

type outSkill struct {
	Enabled    string   `xml:"enabled,attr"`
	Slot       string   `xml:"slot,attr,omitempty"`
	Source     string   `xml:"source,attr,omitempty"`
	MainActive string   `xml:"mainActiveSkill,attr"`
	FullDPS    string   `xml:"includeInFullDPS,attr,omitempty"`
	GroupCount string   `xml:"groupCount,attr,omitempty"`
	Gems       []outGem `xml:"Gem"`
}

type outGem struct {
	Name          string `xml:"nameSpec,attr,omitempty"`
	GemID         string `xml:"gemId,attr,omitempty"`
	VariantID     string `xml:"variantId,attr,omitempty"`
	SkillID       string `xml:"skillId,attr,omitempty"`
	Level         int    `xml:"level,attr"`
	Quality       int    `xml:"quality,attr"`
	Enabled       string `xml:"enabled,attr"`
	EnableGlobal1 string `xml:"enableGlobal1,attr"`
	EnableGlobal2 string `xml:"enableGlobal2,attr"`
	MinionItemSet int    `xml:"skillMinionItemSet,attr,omitempty"`
	MinionSkill   string `xml:"skillMinionSkill,attr,omitempty"`
}
