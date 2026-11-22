package poe1

import (
	"encoding/binary"
	"math"

	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/internal/resolve"
)

// appendSkill appends an encoding of every field of sk to b: two skills encode the same only
// when they are equal. Strings and slices carry their length, so no two field sequences run
// together.
func appendSkill(b []byte, sk *resolve.Skill) []byte {
	b = appendGemSource(b, sk.Source)
	b = binary.AppendVarint(b, int64(sk.Which))
	b = appendString(b, sk.GrantedEffect)
	b = appendString(b, sk.ActiveSkill)
	b = appendInts(b, int64(sk.Level), int64(sk.Quality))
	b = appendStrings(b, sk.Types)
	b = appendStrings(b, sk.WeaponRestrictions)
	b = appendInts(b, int64(sk.CastTime), int64(sk.AttackTime), int64(sk.AttackSpeedMultiplier),
		int64(sk.LevelRequirement), int64(sk.Totem), int64(sk.Cooldown))
	b = binary.AppendUvarint(b, uint64(len(sk.Costs)))
	for _, c := range sk.Costs {
		b = appendCost(b, c)
	}
	b = appendReservation(b, sk.Reservation)
	b = binary.AppendUvarint(b, uint64(len(sk.StatSets)))
	for _, set := range sk.StatSets {
		b = appendString(b, set.ID)
		b = appendInts(b, int64(set.SpellCritChance), int64(set.AttackCritChance),
			int64(set.DamageEffectiveness), int64(set.BaseMultiplier))
		b = appendStats(b, set.Stats)
	}
	b = appendInts(b, int64(sk.StatSet), int64(sk.Stages))
	b = binary.AppendUvarint(b, uint64(len(sk.Supports)))
	for _, sp := range sk.Supports {
		b = appendSource(b, sp.Source)
		b = appendString(b, sp.GrantedEffect)
		b = appendInts(b, int64(sp.Level), int64(sp.Quality), int64(sp.CostMultiplier))
		b = appendStats(b, sp.Stats)
		b = appendReservation(b, sp.Reservation)
	}
	b = binary.AppendUvarint(b, uint64(len(sk.Supported)))
	for _, r := range sk.Supported {
		b = appendGemSource(b, r.Source)
		b = binary.AppendVarint(b, int64(r.Which))
	}
	return b
}

func appendString(b []byte, s string) []byte {
	return append(binary.AppendUvarint(b, uint64(len(s))), s...)
}

func appendStrings(b []byte, ss []string) []byte {
	b = binary.AppendUvarint(b, uint64(len(ss)))
	for _, s := range ss {
		b = appendString(b, s)
	}
	return b
}

func appendInts(b []byte, vs ...int64) []byte {
	for _, v := range vs {
		b = binary.AppendVarint(b, v)
	}
	return b
}

func appendStats(b []byte, stats []resolve.StatValue) []byte {
	b = binary.AppendUvarint(b, uint64(len(stats)))
	for _, s := range stats {
		b = appendString(b, s.Stat)
		b = binary.LittleEndian.AppendUint64(b, math.Float64bits(s.Value))
	}
	return b
}

func appendCost(b []byte, c dataset.Cost) []byte {
	return binary.AppendVarint(appendString(b, c.Type), int64(c.Amount))
}

func appendReservation(b []byte, r resolve.Reservation) []byte {
	return appendInts(b, int64(r.ManaFlat), int64(r.ManaPercent), int64(r.LifeFlat), int64(r.LifePercent))
}

func appendGemSource(b []byte, s resolve.GemSource) []byte {
	return appendString(appendInts(b, int64(s.Group), int64(s.Gem)), s.ID)
}

// appendSource writes a gem source by its fields, the common case, and any other by its
// label, which names it in explanations and so tells it apart.
func appendSource(b []byte, s resolve.Source) []byte {
	if g, ok := s.(resolve.GemSource); ok {
		return appendGemSource(append(b, 'g'), g)
	}
	if s == nil {
		return append(b, 'n')
	}
	return appendString(append(b, 's'), s.String())
}
