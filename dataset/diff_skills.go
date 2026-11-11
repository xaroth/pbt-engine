package dataset

import (
	"errors"
	"math"
	"reflect"
)

// SkillsDiff lists the skills group's changes by id.
type SkillsDiff struct {
	TypesAdded, TypesRemoved                                          []string
	GemsAdded, GemsRemoved, GemsChanged                               []string
	GrantedEffectsAdded, GrantedEffectsRemoved, GrantedEffectsChanged []string
	StatSetsAdded, StatSetsRemoved, StatSetsChanged                   []string
	ActiveSkillsAdded, ActiveSkillsRemoved, ActiveSkillsChanged       []string
}

// skillGroupOf returns the skills group of s, or an empty one when s has none.
func skillGroupOf(s *Store) (*Skills, error) {
	sk, err := s.Skills()
	if _, ok := errors.AsType[*MissingGroupError](err); ok {
		return &Skills{}, nil
	}
	return sk, err
}

func compareSkillGroups(a, b *Store, x, y *tables) (SkillsDiff, error) {
	p, err := skillGroupOf(a)
	if err != nil {
		return SkillsDiff{}, err
	}
	q, err := skillGroupOf(b)
	if err != nil {
		return SkillsDiff{}, err
	}
	m := newRemap(x.stats, y.stats, p.Types, q.Types)
	var d SkillsDiff
	typeID := func(t string) string { return t }
	d.TypesAdded, d.TypesRemoved = setDiff(keys(byKey(p.Types, typeID)), keys(byKey(q.Types, typeID)))
	d.GemsAdded, d.GemsRemoved, d.GemsChanged = changes(p.Gems, q.Gems,
		gemID, func(g, h Gem) bool { return reflect.DeepEqual(g, h) })
	d.GrantedEffectsAdded, d.GrantedEffectsRemoved, d.GrantedEffectsChanged = changes(p.GrantedEffects, q.GrantedEffects,
		grantedEffectID, func(g, h GrantedEffect) bool { return reflect.DeepEqual(g, m.grantedEffect(h)) })
	d.StatSetsAdded, d.StatSetsRemoved, d.StatSetsChanged = changes(p.StatSets, q.StatSets,
		statSetID, func(s, t StatSet) bool { return reflect.DeepEqual(s, m.statSet(t)) })
	d.ActiveSkillsAdded, d.ActiveSkillsRemoved, d.ActiveSkillsChanged = changes(p.ActiveSkills, q.ActiveSkills,
		activeSkillID, func(s, t ActiveSkill) bool { return reflect.DeepEqual(s, m.activeSkill(t)) })
	return d, nil
}

// remap renumbers b's stat handles and skill type indexes as a's, so rows of the two datasets
// compare field by field. Whole rows are compared with reflect.DeepEqual so a field added to a
// row is compared without a change here. Stats and types a does not have get a number no
// handle or index of a has. DeepEqual never sees a NaN, which it would report as changed:
// validation rejects NaN and infinite floats on Write and when a group is read.
type remap struct {
	stats []uint32
	types []uint16
}

func newRemap(aStats, bStats []Stat, aTypes, bTypes []string) remap {
	stats := make(map[string]uint32, len(aStats))
	for i, s := range aStats {
		stats[s.ID] = uint32(i)
	}
	types := make(map[string]uint16, len(aTypes))
	for i, t := range aTypes {
		types[t] = uint16(i)
	}
	m := remap{stats: make([]uint32, len(bStats)), types: make([]uint16, len(bTypes))}
	for i, s := range bStats {
		h, ok := stats[s.ID]
		m.stats[i] = math.MaxUint32
		if ok {
			m.stats[i] = h
		}
	}
	for i, t := range bTypes {
		x, ok := types[t]
		m.types[i] = math.MaxUint16
		if ok {
			m.types[i] = x
		}
	}
	return m
}

// mapped returns f applied to each element of s, keeping nil as nil.
func mapped[T any](s []T, f func(T) T) []T {
	if s == nil {
		return nil
	}
	out := make([]T, len(s))
	for i, v := range s {
		out[i] = f(v)
	}
	return out
}

func (m remap) stat(h uint32) uint32 { return m.stats[h] }

func (m remap) skillType(t uint16) uint16 { return m.types[t] }

func (m remap) statValue(sv StatValue) StatValue {
	return StatValue{Stat: m.stat(sv.Stat), Value: sv.Value}
}

func (m remap) grantedEffect(g GrantedEffect) GrantedEffect {
	g.QualityStats = mapped(g.QualityStats, m.statValue)
	g.AllowedTypes = mapped(g.AllowedTypes, m.skillType)
	g.ExcludedTypes = mapped(g.ExcludedTypes, m.skillType)
	g.AddedTypes = mapped(g.AddedTypes, m.skillType)
	g.AddedMinionTypes = mapped(g.AddedMinionTypes, m.skillType)
	return g
}

func (m remap) statSet(s StatSet) StatSet {
	s.ImplicitStats = mapped(s.ImplicitStats, m.stat)
	s.ConstantStats = mapped(s.ConstantStats, m.statValue)
	s.Levels = mapped(s.Levels, func(l StatSetLevel) StatSetLevel {
		l.FloatStats = mapped(l.FloatStats, func(f FloatStat) FloatStat { f.Stat = m.stat(f.Stat); return f })
		l.Stats = mapped(l.Stats, func(st LevelStat) LevelStat { st.Stat = m.stat(st.Stat); return st })
		l.Flags = mapped(l.Flags, m.stat)
		return l
	})
	return s
}

func (m remap) activeSkill(a ActiveSkill) ActiveSkill {
	a.Types = mapped(a.Types, m.skillType)
	a.MinionTypes = mapped(a.MinionTypes, m.skillType)
	return a
}
