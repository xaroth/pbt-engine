package dataset

import (
	"fmt"

	"github.com/xaroth/pbt-engine/dataset/internal/fb"
)

func decodeMonsters(buf []byte, index map[string]uint32) (Monsters, error) {
	if !fb.MonstersBufferHasIdentifier(buf) {
		return Monsters{}, errIdentifier
	}
	root := fb.GetRootAsMonsters(buf, 0)
	h, err := handles(index, root.StatIdsLength(), root.StatIds)
	if err != nil {
		return Monsters{}, err
	}
	tagIDs := readStrings(root.TagIdsLength(), root.TagIds)
	for i := 1; i < len(tagIDs); i++ {
		if tagIDs[i-1] >= tagIDs[i] {
			return Monsters{}, fmt.Errorf("tag ids not sorted and unique at %q: %w", tagIDs[i], ErrInvalid)
		}
	}
	d := &skillDecoder{h: h}
	m := Monsters{
		Types:       decodeEach(root.TypesLength(), root.Types, decodeMonsterType),
		Resistances: decodeEach(root.ResistancesLength(), root.Resistances, decodeMonsterResistance),
		Varieties: decodeEach(root.VarietiesLength(), root.Varieties, func(v *fb.MonsterVariety) MonsterVariety {
			return d.variety(v, tagIDs)
		}),
		Levels:         decodeEach(root.LevelsLength(), root.Levels, decodeMonsterLevel),
		DisplayMinions: decodeEach(root.DisplayMinionsLength(), root.DisplayMinions, decodeDisplayMinion),
		Totems:         decodeEach(root.TotemsLength(), root.Totems, decodeTotem),
		SpectreOverrides: decodeEach(root.SpectreOverridesLength(), root.SpectreOverrides, func(s *fb.SpectreOverride) SpectreOverride {
			return SpectreOverride{Monster: string(s.Monster()), Spectre: string(s.Spectre())}
		}),
		MinionTypes: decodeEach(root.MinionTypesLength(), root.MinionTypes, func(t *fb.MinionType) MinionType {
			return MinionType{ID: string(t.Id()), LimitStat: d.optStat(t.LimitStat()), ActiveCountStat: d.optStat(t.ActiveCountStat())}
		}),
		EditionTotems: decodeEach(root.EditionTotemsLength(), root.EditionTotems, func(e *fb.EditionTotem) EditionTotem {
			return EditionTotem{GrantedEffect: string(e.GrantedEffect()), Totem: e.Totem(), Source: string(e.Source())}
		}),
	}
	if m.BaseStats, err = fromLocalStats(root.BaseStatsLength(), root.BaseStats, h); err != nil {
		return Monsters{}, fmt.Errorf("monster base stats: %w", err)
	}
	if m.Mods, err = decodeMonsterMods(root, h, tagIDs); err != nil {
		return Monsters{}, err
	}
	if d.err != nil {
		return Monsters{}, d.err
	}
	return m, nil
}

// decodeEach decodes the n tables get returns, or nil for none.
func decodeEach[T, F any](n int, get func(*F, int) bool, decode func(*F) T) []T {
	if n == 0 {
		return nil
	}
	out := make([]T, n)
	var f F
	for i := range out {
		get(&f, i)
		out[i] = decode(&f)
	}
	return out
}

// fromLocalStats returns a sorted stat list with dataset handles, sorted again by handle.
func fromLocalStats(n int, get func(*fb.StatValue, int) bool, h []uint32) ([]StatValue, error) {
	return fromLocal(statVector{n, get}, h, true)
}

// statVector adapts a stat value vector of any table to statValueSource.
type statVector struct {
	n   int
	get func(*fb.StatValue, int) bool
}

func (v statVector) Stats(obj *fb.StatValue, j int) bool { return v.get(obj, j) }
func (v statVector) StatsLength() int                    { return v.n }

func decodeMonsterMods(root *fb.Monsters, h []uint32, tagIDs []string) ([]Mod, error) {
	if root.ModsLength() == 0 {
		return nil, nil
	}
	intern := interner{}
	out := make([]Mod, root.ModsLength())
	var m fb.Mod
	for i := range out {
		root.Mods(&m, i)
		var err error
		if out[i], err = decodeMod(&m, h, tagIDs, intern); err != nil {
			return nil, fmt.Errorf("mod %s: %w", m.Id(), err)
		}
	}
	return out, nil
}

func decodeMonsterType(t *fb.MonsterType) MonsterType {
	return MonsterType{
		ID: string(t.Id()), Armour: t.Armour(), Evasion: t.Evasion(), EnergyShield: t.EnergyShield(), Accuracy: t.Accuracy(),
		DamageSpread: t.DamageSpread(), Resistances: string(t.Resistances()), PlayerMinion: t.PlayerMinion(),
		BaseDamageIgnoresAttackSpeed: t.BaseDamageIgnoresAttackSpeed(), AltLife1: t.AltLife1(), AltLife2: t.AltLife2(),
	}
}

func decodeMonsterResistance(r *fb.MonsterResistance) MonsterResistance {
	var x fb.Resistances
	get := func(f func(*fb.Resistances) *fb.Resistances) Resistances {
		if f(&x) == nil {
			return Resistances{}
		}
		return Resistances{Fire: x.Fire(), Cold: x.Cold(), Lightning: x.Lightning(), Chaos: x.Chaos()}
	}
	return MonsterResistance{ID: string(r.Id()), Normal: get(r.Normal), Cruel: get(r.Cruel), Merciless: get(r.Merciless)}
}

func (d *skillDecoder) variety(v *fb.MonsterVariety, tagIDs []string) MonsterVariety {
	strs := func(n int, get func(int) []byte) []string {
		if n == 0 {
			return nil
		}
		return readStrings(n, get)
	}
	var tags []string
	if n := v.TagsLength(); n > 0 {
		tags = make([]string, n)
		for i := range tags {
			tags[i] = d.str("tag", tagIDs, int(v.Tags(i)))
		}
	}
	return MonsterVariety{
		ID: string(v.Id()), Type: string(v.Type()),
		LifeMultiplier: v.LifeMultiplier(), DamageMultiplier: v.DamageMultiplier(), AilmentThreshold: v.AilmentThreshold(), AttackTime: v.AttackTime(),
		MainHand: string(v.MainHand()), OffHand: string(v.OffHand()),
		Tags:           tags,
		GrantedEffects: strs(v.GrantedEffectsLength(), v.GrantedEffects),
		Mods:           strs(v.ModsLength(), v.Mods),
		Mods2:          strs(v.Mods2Length(), v.Mods2),
		SpecialMods:    strs(v.SpecialModsLength(), v.SpecialMods),
		Part1Mods:      strs(v.Part1ModsLength(), v.Part1Mods),
		Part2Mods:      strs(v.Part2ModsLength(), v.Part2Mods),
		EndgameMods:    strs(v.EndgameModsLength(), v.EndgameMods),
	}
}

func decodeMonsterLevel(l *fb.MonsterLevel) MonsterLevel {
	return MonsterLevel{
		Level: l.Level(), Difficulty: l.Difficulty(),
		Life: l.Life(), AllyLife: l.AllyLife(), AltLife1: l.AltLife1(), AltLife2: l.AltLife2(),
		Damage: l.Damage(), AllyDamage: l.AllyDamage(), AltDamage1: l.AltDamage1(), AltDamage2: l.AltDamage2(),
		Armour: l.Armour(), Evasion: l.Evasion(), EvasiveEvasion: l.EvasiveEvasion(),
		Accuracy: l.Accuracy(), AilmentThreshold: l.AilmentThreshold(), PhysicalConversion: l.PhysicalConversion(),
	}
}

func decodeDisplayMinion(m *fb.DisplayMinion) DisplayMinion {
	return DisplayMinion{ID: m.Id(), Variety: string(m.Variety())}
}

func decodeTotem(t *fb.Totem) Totem {
	return Totem{Key: t.Key(), Skins: decodeEach(t.SkinsLength(), t.Skins, func(s *fb.TotemSkin) TotemSkin {
		return TotemSkin{Skin: s.Skin(), Variety: string(s.Variety())}
	})}
}
