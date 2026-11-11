package dataset

import (
	"fmt"

	"github.com/xaroth/pbt-engine/dataset/internal/fb"
)

// skillDecoder resolves a skills file's indexes. It keeps the first bad index it meets, so
// decoding goes on without checks at every call and fails once at the end.
type skillDecoder struct {
	h                           []uint32
	classes, tags, costs, bases []string
	err                         error
}

func (d *skillDecoder) fail(err error) {
	if d.err == nil {
		d.err = err
	}
}

func (d *skillDecoder) str(what string, table []string, i int) string {
	if i >= len(table) {
		d.fail(fmt.Errorf("%s %d out of range (%d): %w", what, i, len(table), ErrInvalid))
		return ""
	}
	return table[i]
}

func (d *skillDecoder) strs(what string, table []string, n int, get func(int) uint16) []string {
	if n == 0 {
		return nil
	}
	out := make([]string, n)
	for i := range out {
		out[i] = d.str(what, table, int(get(i)))
	}
	return out
}

func (d *skillDecoder) stat(i uint32) uint32 {
	if int(i) >= len(d.h) {
		d.fail(fmt.Errorf("stat %d out of range (%d ids): %w", i, len(d.h), ErrInvalid))
		return 0
	}
	return d.h[i]
}

func (d *skillDecoder) stats(n int, get func(*fb.StatValue, int) bool) []StatValue {
	if n == 0 {
		return nil
	}
	out := make([]StatValue, n)
	var sv fb.StatValue
	for i := range out {
		get(&sv, i)
		out[i] = StatValue{Stat: d.stat(sv.Stat()), Value: sv.Value()}
	}
	return out
}

func (d *skillDecoder) handles(n int, get func(int) uint32) []uint32 {
	if n == 0 {
		return nil
	}
	out := make([]uint32, n)
	for i := range out {
		out[i] = d.stat(get(i))
	}
	return out
}

func readStrings(n int, get func(int) []byte) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = string(get(i))
	}
	return out
}

func readUint16s(n int, get func(int) uint16) []uint16 {
	if n == 0 {
		return nil
	}
	out := make([]uint16, n)
	for i := range out {
		out[i] = get(i)
	}
	return out
}

func decodeSkills(buf []byte, index map[string]uint32) (Skills, error) {
	if !fb.SkillsBufferHasIdentifier(buf) {
		return Skills{}, errIdentifier
	}
	root := fb.GetRootAsSkills(buf, 0)
	h, err := handles(index, root.StatIdsLength(), root.StatIds)
	if err != nil {
		return Skills{}, err
	}
	d := &skillDecoder{
		h:       h,
		classes: readStrings(root.ItemClassesLength(), root.ItemClasses),
		tags:    readStrings(root.GemTagsLength(), root.GemTags),
		costs:   readStrings(root.CostTypesLength(), root.CostTypes),
		bases:   readStrings(root.InterpolationBasesLength(), root.InterpolationBases),
	}
	s := Skills{
		Types:          readStrings(root.SkillTypesLength(), root.SkillTypes),
		Gems:           make([]Gem, root.GemsLength()),
		GrantedEffects: make([]GrantedEffect, root.GrantedEffectsLength()),
		StatSets:       make([]StatSet, root.StatSetsLength()),
		ActiveSkills:   make([]ActiveSkill, root.ActiveSkillsLength()),
	}
	var g fb.Gem
	for i := range s.Gems {
		root.Gems(&g, i)
		s.Gems[i] = d.gem(&g)
	}
	var ge fb.GrantedEffect
	for i := range s.GrantedEffects {
		root.GrantedEffects(&ge, i)
		s.GrantedEffects[i] = d.grantedEffect(&ge)
	}
	var ss fb.StatSet
	for i := range s.StatSets {
		root.StatSets(&ss, i)
		s.StatSets[i] = d.statSet(&ss)
	}
	var a fb.ActiveSkill
	for i := range s.ActiveSkills {
		root.ActiveSkills(&a, i)
		s.ActiveSkills[i] = d.activeSkill(&a)
	}
	if d.err != nil {
		return Skills{}, d.err
	}
	return s, nil
}

func (d *skillDecoder) gem(g *fb.Gem) Gem {
	out := Gem{
		ID:                  string(g.Id()),
		Colour:              GemColour(g.Colour()),
		StrengthPercent:     g.StrengthPercent(),
		DexterityPercent:    g.DexterityPercent(),
		IntelligencePercent: g.IntelligencePercent(),
		Support:             g.Support(),
		Vaal:                g.Vaal(),
		VaalGem:             string(g.VaalGem()),
		RegularGem:          string(g.RegularGem()),
		AwakenedGem:         string(g.AwakenedGem()),
		Effects:             make([]GemEffect, g.EffectsLength()),
	}
	var e fb.GemEffect
	for i := range out.Effects {
		g.Effects(&e, i)
		out.Effects[i] = GemEffect{
			ID:                             string(e.Id()),
			GrantedEffect:                  string(e.GrantedEffect()),
			SecondaryGrantedEffect:         string(e.SecondaryGrantedEffect()),
			HardmodeGrantedEffect:          string(e.HardmodeGrantedEffect()),
			HardmodeSecondaryGrantedEffect: string(e.HardmodeSecondaryGrantedEffect()),
			Tags:                           d.strs("gem tag", d.tags, e.TagsLength(), e.Tags),
			Style:                          GemStyle(e.Style()),
		}
	}
	return out
}

func (d *skillDecoder) grantedEffect(g *fb.GrantedEffect) GrantedEffect {
	out := GrantedEffect{
		ID:                        string(g.Id()),
		Support:                   g.Support(),
		ActiveSkill:               string(g.ActiveSkill()),
		CastTime:                  g.CastTime(),
		StatSets:                  readStrings(g.StatSetsLength(), g.StatSets),
		QualityStats:              d.stats(g.QualityStatsLength(), g.QualityStats),
		AllowedTypes:              readUint16s(g.AllowedTypesLength(), g.AllowedTypes),
		ExcludedTypes:             readUint16s(g.ExcludedTypesLength(), g.ExcludedTypes),
		AddedTypes:                readUint16s(g.AddedTypesLength(), g.AddedTypes),
		AddedMinionTypes:          readUint16s(g.AddedMinionTypesLength(), g.AddedMinionTypes),
		SupportWeaponRestrictions: d.strs("item class", d.classes, g.SupportWeaponRestrictionsLength(), g.SupportWeaponRestrictions),
		SupportsGemsOnly:          g.SupportsGemsOnly(),
		CannotBeSupported:         g.CannotBeSupported(),
		Levels:                    make([]EffectLevel, g.LevelsLength()),
	}
	var l fb.EffectLevel
	var c fb.Cost
	for i := range out.Levels {
		g.Levels(&l, i)
		lv := EffectLevel{
			Level: l.Level(), PlayerLevel: l.PlayerLevel(), CostMultiplier: l.CostMultiplier(),
			Cooldown: l.Cooldown(), CooldownGroup: l.CooldownGroup(), StoredUses: l.StoredUses(),
			VaalSouls: l.VaalSouls(), VaalStoredUses: l.VaalStoredUses(), SoulGainPrevention: l.SoulGainPrevention(),
			AttackSpeedMultiplier: l.AttackSpeedMultiplier(), AttackTime: l.AttackTime(),
			ManaReservationFlat: l.ManaReservationFlat(), ManaReservationPercent: l.ManaReservationPercent(),
			LifeReservationFlat: l.LifeReservationFlat(), LifeReservationPercent: l.LifeReservationPercent(),
		}
		if n := l.CostsLength(); n > 0 {
			lv.Costs = make([]Cost, n)
			for j := range lv.Costs {
				l.Costs(&c, j)
				lv.Costs[j] = Cost{Type: d.str("cost type", d.costs, int(c.Type())), Amount: c.Amount()}
			}
		}
		out.Levels[i] = lv
	}
	return out
}

func (d *skillDecoder) statSet(s *fb.StatSet) StatSet {
	out := StatSet{
		ID:                       string(s.Id()),
		ImplicitStats:            d.handles(s.ImplicitStatsLength(), s.ImplicitStats),
		ConstantStats:            d.stats(s.ConstantStatsLength(), s.ConstantStats),
		BaseEffectiveness:        s.BaseEffectiveness(),
		IncrementalEffectiveness: s.IncrementalEffectiveness(),
		Levels:                   make([]StatSetLevel, s.LevelsLength()),
	}
	var l fb.StatSetLevel
	for i := range out.Levels {
		s.Levels(&l, i)
		out.Levels[i] = d.statSetLevel(&l)
	}
	return out
}

func (d *skillDecoder) statSetLevel(l *fb.StatSetLevel) StatSetLevel {
	out := StatSetLevel{
		Level: l.Level(), PlayerLevel: l.PlayerLevel(),
		SpellCritChance: l.SpellCritChance(), AttackCritChance: l.AttackCritChance(),
		DamageEffectiveness: l.DamageEffectiveness(), BaseMultiplier: l.BaseMultiplier(),
		Flags: d.handles(l.FlagsLength(), l.Flags),
	}
	if n := l.FloatStatsLength(); n > 0 {
		out.FloatStats = make([]FloatStat, n)
		var f fb.FloatStat
		for i := range out.FloatStats {
			l.FloatStats(&f, i)
			out.FloatStats[i] = FloatStat{
				Stat: d.stat(f.Stat()), Value: f.Value(), Resolved: f.Resolved(),
				Interpolation: Interpolation(f.Interpolation()), Base: d.str("interpolation base", d.bases, int(f.Base())),
			}
		}
	}
	if n := l.StatsLength(); n > 0 {
		out.Stats = make([]LevelStat, n)
		var s fb.LevelStat
		for i := range out.Stats {
			l.Stats(&s, i)
			out.Stats[i] = LevelStat{Stat: d.stat(s.Stat()), Value: s.Value(), Interpolation: Interpolation(s.Interpolation())}
		}
	}
	return out
}

func (d *skillDecoder) activeSkill(a *fb.ActiveSkill) ActiveSkill {
	return ActiveSkill{
		ID:                 string(a.Id()),
		Types:              readUint16s(a.TypesLength(), a.Types),
		MinionTypes:        readUint16s(a.MinionTypesLength(), a.MinionTypes),
		WeaponRestrictions: d.strs("item class", d.classes, a.WeaponRestrictionsLength(), a.WeaponRestrictions),
		Totem:              a.Totem(),
		Mine:               a.Mine(),
		TransfigureBase:    string(a.TransfigureBase()),
	}
}

// optStat returns the handle of an optional file stat index, or nil for noStat.
func (d *skillDecoder) optStat(i uint32) *uint32 {
	if i == noStat {
		return nil
	}
	return new(d.stat(i))
}
