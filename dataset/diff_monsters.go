package dataset

import (
	"errors"
	"reflect"
)

// MonstersDiff lists the monsters group's changes by id. Levels, display minions and totems
// are keyed by their numbers; spectre overrides by monster and edition totems by granted
// effect.
type MonstersDiff struct {
	TypesAdded, TypesRemoved, TypesChanged                   []string
	ResistancesAdded, ResistancesRemoved, ResistancesChanged []string
	VarietiesAdded, VarietiesRemoved, VarietiesChanged       []string
	LevelsAdded, LevelsRemoved, LevelsChanged                []int32
	// BaseStatsChanged are the stat ids of Monster.ot base stats added, removed or changed.
	BaseStatsChanged                                                        []string
	ModsAdded, ModsRemoved, ModsChanged                                     []string
	DisplayMinionsAdded, DisplayMinionsRemoved, DisplayMinionsChanged       []int32
	TotemsAdded, TotemsRemoved, TotemsChanged                               []int32
	SpectreOverridesAdded, SpectreOverridesRemoved, SpectreOverridesChanged []string
	MinionTypesAdded, MinionTypesRemoved, MinionTypesChanged                []string
	EditionTotemsAdded, EditionTotemsRemoved, EditionTotemsChanged          []string
}

// monsterGroupOf returns the monsters group of s, or an empty one when s has none.
func monsterGroupOf(s *Store) (*Monsters, error) {
	m, err := s.Monsters()
	if _, ok := errors.AsType[*MissingGroupError](err); ok {
		return &Monsters{}, nil
	}
	return m, err
}

func compareMonsterGroups(a, b *Store, x, y *tables) (MonstersDiff, error) {
	p, err := monsterGroupOf(a)
	if err != nil {
		return MonstersDiff{}, err
	}
	q, err := monsterGroupOf(b)
	if err != nil {
		return MonstersDiff{}, err
	}
	m := newRemap(x.stats, y.stats, nil, nil)
	var d MonstersDiff
	d.TypesAdded, d.TypesRemoved, d.TypesChanged = changes(p.Types, q.Types, monsterTypeID, same[MonsterType])
	d.ResistancesAdded, d.ResistancesRemoved, d.ResistancesChanged = changes(p.Resistances, q.Resistances, monsterResistanceID, same[MonsterResistance])
	d.VarietiesAdded, d.VarietiesRemoved, d.VarietiesChanged = changes(p.Varieties, q.Varieties, varietyID, same[MonsterVariety])
	d.LevelsAdded, d.LevelsRemoved, d.LevelsChanged = changes(p.Levels, q.Levels, func(l MonsterLevel) int32 { return l.Level }, same[MonsterLevel])
	before, after := statMap(x, p.BaseStats), statMap(y, q.BaseStats)
	for _, id := range sortedUnion(before, after) {
		v, inA := before[id]
		w, inB := after[id]
		if inA != inB || v != w {
			d.BaseStatsChanged = append(d.BaseStatsChanged, id)
		}
	}
	d.ModsAdded, d.ModsRemoved, d.ModsChanged = changes(p.Mods, q.Mods, modID, func(m, n Mod) bool { return sameMod(x, y, m, n) })
	d.DisplayMinionsAdded, d.DisplayMinionsRemoved, d.DisplayMinionsChanged = changes(p.DisplayMinions, q.DisplayMinions,
		displayMinionID, same[DisplayMinion])
	d.TotemsAdded, d.TotemsRemoved, d.TotemsChanged = changes(p.Totems, q.Totems, totemKey, same[Totem])
	d.SpectreOverridesAdded, d.SpectreOverridesRemoved, d.SpectreOverridesChanged = changes(p.SpectreOverrides, q.SpectreOverrides,
		func(s SpectreOverride) string { return s.Monster }, same[SpectreOverride])
	d.MinionTypesAdded, d.MinionTypesRemoved, d.MinionTypesChanged = changes(p.MinionTypes, q.MinionTypes, minionTypeID,
		func(s, t MinionType) bool { return reflect.DeepEqual(s, m.minionType(t)) })
	d.EditionTotemsAdded, d.EditionTotemsRemoved, d.EditionTotemsChanged = changes(p.EditionTotems, q.EditionTotems,
		func(e EditionTotem) string { return e.GrantedEffect }, same[EditionTotem])
	return d, nil
}

// same compares rows that hold no stat handles. Whole rows are compared so a field added later
// is compared without a change here; validation rejects NaN, which DeepEqual would report as
// changed.
func same[T any](a, b T) bool { return reflect.DeepEqual(a, b) }

func (m remap) minionType(t MinionType) MinionType {
	t.LimitStat, t.ActiveCountStat = m.optionalStat(t.LimitStat), m.optionalStat(t.ActiveCountStat)
	return t
}
