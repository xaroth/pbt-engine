package resolve

import (
	"cmp"
	"errors"
	"fmt"
	"slices"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/dataset"
)

// ErrNoActiveSkill is a skill group without an enabled active skill to resolve.
var ErrNoActiveSkill = errors.New("resolve: skill group has no active skill")

// Warning codes of skill resolution.
const (
	// SupportNotApplied is a support gem in the group that does not support the skill.
	SupportNotApplied build.Code = "support-not-applied"
	// GemLevelBeyondData is a gem whose level with bonuses has no row in the dataset; its
	// highest row is used.
	GemLevelBeyondData build.Code = "gem-level-beyond-data"
)

// GemSource is a gem in a skill group, by index in build.Skills.Groups and in the group's gems.
type GemSource struct {
	Group, Gem int
	// ID is the gem's dataset Gem.ID.
	ID string
}

func (s GemSource) String() string {
	return fmt.Sprintf("skill group %d gem %d (%s)", s.Group+1, s.Gem+1, s.ID)
}

func (GemSource) source() {}

// Skill is an active skill resolved to its stats: its gem's level and quality with what items
// and supports add, its stat sets at that level, and the supports in its group that apply.
type Skill struct {
	Source GemSource
	// GrantedEffect and ActiveSkill are the skill's GrantedEffect and ActiveSkill IDs.
	GrantedEffect, ActiveSkill string
	Level, Quality             int
	// Types are the skill's ActiveSkillType ids, with those the applied supports add, in the
	// dataset's order.
	Types []string
	// CastTime is the skill's in milliseconds; AttackTime and AttackSpeedMultiplier are its
	// level row's.
	CastTime, AttackTime, AttackSpeedMultiplier int32
	// Costs are the skill's level row costs, before supports' multipliers.
	Costs []dataset.Cost
	// Reservation is what the skill reserves before supports' multipliers: its level row's, or
	// that of the last applied support whose level row reserves, such as one that makes a
	// warcry reserve.
	Reservation Reservation
	// StatSets are the skill's stat sets at its level, the primary set first.
	StatSets []SkillStatSet
	// Supports are the group's supports that apply to the skill, in group order.
	Supports []Support
}

// Reservation is the life and mana a skill reserves: flat amounts, and percentages of the
// maximum in hundredths of a percent.
type Reservation struct {
	ManaFlat, ManaPercent, LifeFlat, LifePercent int32
}

func rowReservation(row dataset.EffectLevel) Reservation {
	return Reservation{
		ManaFlat: row.ManaReservationFlat, ManaPercent: row.ManaReservationPercent,
		LifeFlat: row.LifeReservationFlat, LifePercent: row.LifeReservationPercent,
	}
}

// IsZero reports whether r reserves nothing.
func (r Reservation) IsZero() bool { return r == Reservation{} }

// SkillStatSet is one stat set of a skill at the skill's level.
type SkillStatSet struct {
	ID string
	// SpellCritChance and AttackCritChance are in hundredths of a percent.
	SpellCritChance, AttackCritChance int32
	// DamageEffectiveness and BaseMultiplier are in ten-thousandths, added to 1.
	DamageEffectiveness, BaseMultiplier int32
	// Stats are the set's implicit and constant stats, its level row's stats and the stats of
	// the gem's quality. Quality stats are in every set, so a skill uses one set, not their sum.
	Stats []StatValue
}

// Support is a support gem applied to a skill, with the stats it gives the skill.
type Support struct {
	Source        GemSource
	GrantedEffect string
	// Level and Quality include what items add.
	Level, Quality int
	// CostMultiplier is the support's cost and reservation multiplier in percent.
	CostMultiplier int32
	Stats          []StatValue
	// Reservation is what the support's level row reserves.
	Reservation Reservation
}

// MainSkill resolves the main skill of c's build, as GroupSkill does.
func MainSkill(c *build.Checked, bonuses []GemBonus) (*Skill, []build.Warning, error) {
	return GroupSkill(c, c.Build().Skills.MainGroup, bonuses)
}

// GroupSkill resolves the active skill of skill group i of c's build: the group's active gem
// and which of its skills is chosen. bonuses are the stats that add levels or quality to gems;
// a support that does not apply warns. It fails with ErrNoActiveSkill when the group has no
// enabled active gem, and when a dataset group fails to load.
func GroupSkill(c *build.Checked, i int, bonuses []GemBonus) (*Skill, []build.Warning, error) {
	r, err := newSkillResolver(c, bonuses)
	if err != nil {
		return nil, nil, err
	}
	b := c.Build()
	if i < 0 || i >= len(b.Skills.Groups) {
		return nil, nil, fmt.Errorf("%w: group %d of %d", ErrNoActiveSkill, i+1, len(b.Skills.Groups))
	}
	g := b.Skills.Groups[i]
	if g.Active >= len(g.Gems) || g.Gems[g.Active].Disabled {
		return nil, nil, fmt.Errorf("%w: group %d", ErrNoActiveSkill, i+1)
	}
	sk, err := r.gemSkill(i, g.Active, g.ActiveSkill)
	if err != nil {
		return nil, nil, err
	}
	return sk, r.warnings, nil
}

// SkillWarnings is a resolved skill and the warnings resolving it gave.
type SkillWarnings struct {
	Skill    *Skill
	Warnings []build.Warning
}

// Skills resolves every skill c's build uses: each skill of each enabled active gem in the
// groups outside the weapon swap, in group and gem order, each with its group's supports as
// GroupSkill applies them. Gems the dataset lacks and skills without levels are left out. It
// fails when a dataset group fails to load.
func Skills(c *build.Checked, bonuses []GemBonus) ([]SkillWarnings, error) {
	r, err := newSkillResolver(c, bonuses)
	if err != nil {
		return nil, err
	}
	var out []SkillWarnings
	for i, g := range c.Build().Skills.Groups {
		if g.Slot.Swap() {
			continue
		}
		for j, gem := range g.Gems {
			if gem.Support || gem.Disabled {
				continue
			}
			for which := range 2 {
				r.warnings = nil
				sk, err := r.gemSkill(i, j, which)
				if err != nil {
					continue
				}
				out = append(out, SkillWarnings{Skill: sk, Warnings: r.warnings})
			}
		}
	}
	return out, nil
}

// gemSkill resolves skill which (0 the first, 1 the second) of gem j of group i, supported by
// the group's other gems. It fails with ErrNoActiveSkill when that is not an active skill with
// levels, and with dataset.ErrNotInDataset when the dataset lacks the gem.
func (r *skillResolver) gemSkill(i, j, which int) (*Skill, error) {
	g := r.groups[i]
	gem := g.Gems[j]
	ge, err := r.skillEffect(gem, which)
	if err != nil {
		return nil, fmt.Errorf("group %d: %w", i+1, err)
	}
	src := GemSource{Group: i, Gem: j, ID: gem.ID}
	sk := &Skill{Source: src, GrantedEffect: ge.ID, ActiveSkill: ge.ActiveSkill, CastTime: ge.CastTime}
	types := r.supportedTypes(src, g, ge, sk)

	tags := r.tags(gem)
	level, quality := r.itemBonus(g.Slot, gem, tags)
	for _, sp := range sk.Supports {
		l, q := supportedBonus(r.bonuses, sp.Stats, gem.Support, tags)
		level, quality = level+l, quality+q
	}
	sk.Quality = gem.Quality + quality
	sk.Level = r.clampLevel(src, ge, gem.Level+level)
	row := ge.Levels[rowAt(ge.Levels, sk.Level, effectLevel)]
	sk.AttackTime, sk.AttackSpeedMultiplier = row.AttackTime, row.AttackSpeedMultiplier
	sk.Costs = slices.Clone(row.Costs)
	sk.Reservation = rowReservation(row)
	for _, sp := range sk.Supports {
		if !sp.Reservation.IsZero() {
			sk.Reservation = sp.Reservation
		}
	}
	for _, id := range ge.StatSets {
		sk.StatSets = append(sk.StatSets, r.statSet(r.data.StatSet(id), sk.Level, sk.Quality, ge.QualityStats))
	}
	sk.Types = r.typeNames(types)
	return sk, nil
}

// skillResolver holds what resolving one skill reads, and the warnings it gives.
type skillResolver struct {
	data     *dataset.SkillIndex
	stats    []dataset.Stat
	items    build.Items
	groups   []build.SkillGroup
	bonuses  []GemBonus
	warnings []build.Warning
}

func newSkillResolver(c *build.Checked, bonuses []GemBonus) (*skillResolver, error) {
	data, err := c.Store().SkillIndex()
	if err != nil {
		return nil, fmt.Errorf("resolve: load skills: %w", err)
	}
	stats, err := c.Store().Stats()
	if err != nil {
		return nil, fmt.Errorf("resolve: load stats: %w", err)
	}
	return &skillResolver{data: data, stats: stats, items: c.Build().Items, groups: c.Build().Skills.Groups, bonuses: bonuses}, nil
}

func (r *skillResolver) warnf(code build.Code, subject GemSource, format string, args ...any) {
	r.warnings = append(r.warnings, build.Warning{Code: code, Subject: subject.String(), Message: fmt.Sprintf(format, args...)})
}

// gemEffect returns the variant of gem the build names, its first when it names none.
func (r *skillResolver) gemEffect(gem build.Gem) *dataset.GemEffect {
	dg := r.data.Gem(gem.ID)
	if dg == nil || len(dg.Effects) == 0 {
		return nil
	}
	if e := dg.Effect(gem.Effect); e != nil {
		return e
	}
	return &dg.Effects[0]
}

func (r *skillResolver) tags(gem build.Gem) []string {
	if e := r.gemEffect(gem); e != nil {
		return e.Tags
	}
	return nil
}

// skillEffect returns skill which (0 the first, 1 the second) of gem, which must be an active
// skill with levels.
func (r *skillResolver) skillEffect(gem build.Gem, which int) (*dataset.GrantedEffect, error) {
	e := r.gemEffect(gem)
	if e == nil {
		return nil, fmt.Errorf("resolve: gem %s: %w", gem.ID, dataset.ErrNotInDataset)
	}
	id := e.GrantedEffect
	if which == 1 {
		id = e.SecondaryGrantedEffect
	}
	ge := r.data.GrantedEffect(id)
	if ge == nil || ge.Support || r.data.ActiveSkill(ge.ActiveSkill) == nil || len(ge.Levels) == 0 {
		return nil, fmt.Errorf("%w: skill %d of gem %s", ErrNoActiveSkill, which+1, gem.ID)
	}
	return ge, nil
}

// clampLevel returns level within the levels of ge, warning when it is beyond them.
func (r *skillResolver) clampLevel(src GemSource, ge *dataset.GrantedEffect, level int) int {
	top := int(ge.Levels[len(ge.Levels)-1].Level)
	if level > top {
		r.warnf(GemLevelBeyondData, src, "level %d with bonuses; %s has levels to %d; %d used", level, ge.ID, top, top)
		return top
	}
	return max(level, 1)
}

// rowAt returns the index of the highest row of levels at or below level, the first row when
// there is none. levels are sorted by level.
func rowAt[T any](levels []T, level int, levelOf func(T) int32) int {
	i, found := slices.BinarySearchFunc(levels, level, func(t T, l int) int { return cmp.Compare(int(levelOf(t)), l) })
	if found || i == 0 {
		return i
	}
	return i - 1
}

func effectLevel(l dataset.EffectLevel) int32   { return l.Level }
func statSetLevel(l dataset.StatSetLevel) int32 { return l.Level }

// statSet returns set at level, with the stats quality gives per qualityStats.
func (r *skillResolver) statSet(set *dataset.StatSet, level, quality int, qualityStats []dataset.StatValue) SkillStatSet {
	out := SkillStatSet{ID: set.ID}
	for _, h := range set.ImplicitStats {
		out.Stats = append(out.Stats, StatValue{Stat: r.stats[h].ID, Value: 1})
	}
	out.Stats = append(out.Stats, statValues(r.stats, set.ConstantStats)...)
	if len(set.Levels) > 0 {
		l := &set.Levels[rowAt(set.Levels, level, statSetLevel)]
		out.SpellCritChance, out.AttackCritChance = l.SpellCritChance, l.AttackCritChance
		out.DamageEffectiveness, out.BaseMultiplier = l.DamageEffectiveness, l.BaseMultiplier
		for _, f := range l.FloatStats {
			out.Stats = append(out.Stats, StatValue{Stat: r.stats[f.Stat].ID, Value: float64(f.Resolved)})
		}
		for _, s := range l.Stats {
			out.Stats = append(out.Stats, StatValue{Stat: r.stats[s.Stat].ID, Value: float64(s.Value)})
		}
		for _, h := range l.Flags {
			out.Stats = append(out.Stats, StatValue{Stat: r.stats[h].ID, Value: 1})
		}
	}
	return out.withQuality(r.stats, quality, qualityStats)
}

// qualityScale is the scale of quality stats: thousandths per 1% quality.
const qualityScale = 1000

// withQuality adds the stats of quality, given per 1% in thousandths and truncated.
func (s SkillStatSet) withQuality(stats []dataset.Stat, quality int, qualityStats []dataset.StatValue) SkillStatSet {
	if quality <= 0 {
		return s
	}
	for _, q := range qualityStats {
		if v := int(q.Value) * quality / qualityScale; v != 0 {
			s.Stats = append(s.Stats, StatValue{Stat: stats[q.Stat].ID, Value: float64(v)})
		}
	}
	return s
}
