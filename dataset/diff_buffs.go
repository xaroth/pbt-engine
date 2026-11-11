package dataset

import (
	"errors"
	"reflect"
)

// BuffsDiff lists the buffs group's changes, buffs by id and flasks and tinctures by item.
type BuffsDiff struct {
	BuffsAdded, BuffsRemoved, BuffsChanged             []string
	FlasksAdded, FlasksRemoved, FlasksChanged          []string
	TincturesAdded, TincturesRemoved, TincturesChanged []string
}

// buffGroupOf returns the buffs group of s, or an empty one when s has none.
func buffGroupOf(s *Store) (*Buffs, error) {
	b, err := s.Buffs()
	if _, ok := errors.AsType[*MissingGroupError](err); ok {
		return &Buffs{}, nil
	}
	return b, err
}

func compareBuffGroups(a, b *Store, x, y *tables) (BuffsDiff, error) {
	p, err := buffGroupOf(a)
	if err != nil {
		return BuffsDiff{}, err
	}
	q, err := buffGroupOf(b)
	if err != nil {
		return BuffsDiff{}, err
	}
	m := newRemap(x.stats, y.stats, nil, nil)
	var d BuffsDiff
	d.BuffsAdded, d.BuffsRemoved, d.BuffsChanged = changes(p.Definitions, q.Definitions,
		buffID, func(s, t BuffDefinition) bool { return reflect.DeepEqual(s, m.buffDefinition(t)) })
	d.FlasksAdded, d.FlasksRemoved, d.FlasksChanged = changes(p.Flasks, q.Flasks,
		flaskBuffItem, func(f, g FlaskBuff) bool { return reflect.DeepEqual(f, g) })
	d.TincturesAdded, d.TincturesRemoved, d.TincturesChanged = changes(p.Tinctures, q.Tinctures,
		tinctureItem, func(s, t Tincture) bool { return s == t })
	return d, nil
}

// optionalStat remaps h unless it is nil.
func (m remap) optionalStat(h *uint32) *uint32 {
	if h == nil {
		return nil
	}
	return new(m.stat(*h))
}

func (m remap) buffDefinition(d BuffDefinition) BuffDefinition {
	d.Stats = mapped(d.Stats, m.stat)
	d.ExtraStats = mapped(d.ExtraStats, m.stat)
	d.Flags = mapped(d.Flags, m.stat)
	d.ConditionStats = mapped(d.ConditionStats, m.stat)
	d.MaximumStat, d.CurrentStat, d.MinimumStat = m.optionalStat(d.MaximumStat), m.optionalStat(d.CurrentStat), m.optionalStat(d.MinimumStat)
	return d
}
