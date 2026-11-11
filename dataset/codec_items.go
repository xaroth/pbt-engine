package dataset

import (
	"fmt"
	"maps"
	"slices"

	flatbuffers "github.com/google/flatbuffers/go"

	"github.com/xaroth/pbt-engine/dataset/internal/fb"
)

// itemsGroup returns the files of d's items group, none when d has no group.
func (d *Dataset) itemsGroup() []group {
	if d.Items == nil {
		return nil
	}
	return []group{{itemsFile, encodeItems(d.Items)}, {modsFile, encodeMods(d.Stats, d.Mods)}}
}

// encodeItems writes it; Write validates it first, so every class and tag a base names exists.
func encodeItems(it *Items) []byte {
	b := flatbuffers.NewBuilder(0)
	classIndex := indexOf(it.Classes, func(c ItemClass) string { return c.ID })
	tagIndex := indexOf(it.Tags, func(t string) string { return t })
	baseOffs := make([]flatbuffers.UOffsetT, len(it.Bases))
	for i, base := range it.Bases {
		baseOffs[i] = encodeBase(b, classIndex, tagIndex, base)
	}
	bases := tableVector(b, fb.ItemsStartBasesVector, baseOffs)
	classOffs := make([]flatbuffers.UOffsetT, len(it.Classes))
	for i, c := range it.Classes {
		id := b.CreateString(c.ID)
		fb.ItemClassStart(b)
		fb.ItemClassAddId(b, id)
		classOffs[i] = fb.ItemClassEnd(b)
	}
	classes := tableVector(b, fb.ItemsStartClassesVector, classOffs)
	tags := stringVector(b, fb.ItemsStartTagsVector, it.Tags)
	fb.ItemsStart(b)
	fb.ItemsAddClasses(b, classes)
	fb.ItemsAddTags(b, tags)
	fb.ItemsAddBases(b, bases)
	fb.FinishItemsBuffer(b, fb.ItemsEnd(b))
	return b.FinishedBytes()
}

func encodeBase(b *flatbuffers.Builder, classIndex, tagIndex map[string]uint32, base ItemBase) flatbuffers.UOffsetT {
	id := b.CreateString(base.ID)
	implicitOffs := make([]flatbuffers.UOffsetT, len(base.Implicits))
	for i, m := range base.Implicits {
		implicitOffs[i] = b.CreateSharedString(m)
	}
	implicits := tableVector(b, fb.ItemBaseStartImplicitsVector, implicitOffs)
	tags := uint32Vector(b, fb.ItemBaseStartTagsVector, indexes(tagIndex, base.Tags))
	armour, weapon, flask := encodeArmour(b, base.Armour), encodeWeapon(b, base.Weapon), encodeFlask(b, base.Flask)
	fb.ItemBaseStart(b)
	fb.ItemBaseAddId(b, id)
	fb.ItemBaseAddClass(b, classIndex[base.Class])
	fb.ItemBaseAddImplicits(b, implicits)
	fb.ItemBaseAddTags(b, tags)
	if armour != 0 {
		fb.ItemBaseAddArmour(b, armour)
	}
	fb.ItemBaseAddBlock(b, base.Block)
	if weapon != 0 {
		fb.ItemBaseAddWeapon(b, weapon)
	}
	if flask != 0 {
		fb.ItemBaseAddFlask(b, flask)
	}
	return fb.ItemBaseEnd(b)
}

// encodeArmour returns 0 for nil a, as do encodeWeapon and encodeFlask.
func encodeArmour(b *flatbuffers.Builder, a *ArmourValues) flatbuffers.UOffsetT {
	if a == nil {
		return 0
	}
	fb.ArmourValuesStart(b)
	fb.ArmourValuesAddArmour(b, fb.CreateValueRange(b, a.Armour.Min, a.Armour.Max))
	fb.ArmourValuesAddEvasion(b, fb.CreateValueRange(b, a.Evasion.Min, a.Evasion.Max))
	fb.ArmourValuesAddEnergyShield(b, fb.CreateValueRange(b, a.EnergyShield.Min, a.EnergyShield.Max))
	fb.ArmourValuesAddWard(b, fb.CreateValueRange(b, a.Ward.Min, a.Ward.Max))
	fb.ArmourValuesAddMovementSpeed(b, a.MovementSpeed)
	return fb.ArmourValuesEnd(b)
}

func encodeWeapon(b *flatbuffers.Builder, w *WeaponValues) flatbuffers.UOffsetT {
	if w == nil {
		return 0
	}
	fb.WeaponValuesStart(b)
	fb.WeaponValuesAddPhysicalDamage(b, fb.CreateValueRange(b, w.PhysicalDamage.Min, w.PhysicalDamage.Max))
	fb.WeaponValuesAddAttackTime(b, w.AttackTime)
	fb.WeaponValuesAddCriticalChance(b, w.CriticalChance)
	fb.WeaponValuesAddRange(b, w.Range)
	return fb.WeaponValuesEnd(b)
}

func encodeFlask(b *flatbuffers.Builder, f *FlaskValues) flatbuffers.UOffsetT {
	if f == nil {
		return 0
	}
	fb.FlaskValuesStart(b)
	fb.FlaskValuesAddKind(b, byte(f.Kind))
	fb.FlaskValuesAddLife(b, f.Life)
	fb.FlaskValuesAddMana(b, f.Mana)
	fb.FlaskValuesAddDuration(b, f.Duration)
	fb.FlaskValuesAddMaxCharges(b, f.MaxCharges)
	fb.FlaskValuesAddChargesPerUse(b, f.ChargesPerUse)
	return fb.FlaskValuesEnd(b)
}

func decodeItems(buf []byte) (Items, error) {
	if !fb.ItemsBufferHasIdentifier(buf) {
		return Items{}, errIdentifier
	}
	root := fb.GetRootAsItems(buf, 0)
	it := Items{Classes: make([]ItemClass, root.ClassesLength()), Tags: make([]string, root.TagsLength())}
	var c fb.ItemClass
	for i := range it.Classes {
		root.Classes(&c, i)
		it.Classes[i] = ItemClass{ID: string(c.Id())}
	}
	for i := range it.Tags {
		it.Tags[i] = string(root.Tags(i))
	}
	intern := interner{}
	it.Bases = make([]ItemBase, root.BasesLength())
	var base fb.ItemBase
	for i := range it.Bases {
		root.Bases(&base, i)
		var err error
		if it.Bases[i], err = decodeBase(&base, it, intern); err != nil {
			return Items{}, fmt.Errorf("base %s: %w", base.Id(), err)
		}
	}
	return it, nil
}

func decodeBase(base *fb.ItemBase, it Items, intern interner) (ItemBase, error) {
	class := int(base.Class())
	if class >= len(it.Classes) {
		return ItemBase{}, fmt.Errorf("class %d out of range (%d classes): %w", class, len(it.Classes), ErrInvalid)
	}
	tags, err := tagsAt(it.Tags, base.TagsLength(), base.Tags)
	if err != nil {
		return ItemBase{}, err
	}
	out := ItemBase{ID: string(base.Id()), Class: it.Classes[class].ID, Tags: tags, Block: base.Block()}
	for i := range base.ImplicitsLength() {
		out.Implicits = append(out.Implicits, intern.string(base.Implicits(i)))
	}
	var r fb.ValueRange
	rng := func(get func(*fb.ValueRange) *fb.ValueRange) ValueRange {
		if get(&r) == nil {
			return ValueRange{}
		}
		return ValueRange{Min: r.Min(), Max: r.Max()}
	}
	if a := base.Armour(nil); a != nil {
		out.Armour = &ArmourValues{
			Armour: rng(a.Armour), Evasion: rng(a.Evasion), EnergyShield: rng(a.EnergyShield), Ward: rng(a.Ward),
			MovementSpeed: a.MovementSpeed(),
		}
	}
	if w := base.Weapon(nil); w != nil {
		out.Weapon = &WeaponValues{
			PhysicalDamage: rng(w.PhysicalDamage), AttackTime: w.AttackTime(), CriticalChance: w.CriticalChance(), Range: w.Range(),
		}
	}
	if f := base.Flask(nil); f != nil {
		out.Flask = &FlaskValues{
			Kind: FlaskKind(f.Kind()), Life: f.Life(), Mana: f.Mana(), Duration: f.Duration(),
			MaxCharges: f.MaxCharges(), ChargesPerUse: f.ChargesPerUse(),
		}
	}
	return out, nil
}

func encodeMods(all []Stat, mods []Mod) []byte {
	b := flatbuffers.NewBuilder(0)
	lists := make([][]StatValue, len(mods))
	usedTags := map[string]bool{}
	for i, m := range mods {
		for _, s := range m.Stats {
			lists[i] = append(lists[i], StatValue{Stat: s.Stat})
		}
		for _, t := range m.Tags {
			usedTags[t] = true
		}
		for _, t := range m.AddedTags {
			usedTags[t] = true
		}
		for _, w := range m.SpawnWeights {
			usedTags[w.Tag] = true
		}
		for _, w := range m.GenerationWeights {
			usedTags[w.Tag] = true
		}
	}
	st := newStatTable(all, lists...)
	tagIDs := slices.Sorted(maps.Keys(usedTags))
	tagIndex := indexOf(tagIDs, func(t string) string { return t })
	offs := make([]flatbuffers.UOffsetT, len(mods))
	for i, m := range mods {
		offs[i] = encodeMod(b, st, tagIndex, m)
	}
	vec := tableVector(b, fb.ModsStartModsVector, offs)
	statIDs := stringVector(b, fb.ModsStartStatIdsVector, st.ids)
	tags := stringVector(b, fb.ModsStartTagIdsVector, tagIDs)
	fb.ModsStart(b)
	fb.ModsAddMods(b, vec)
	fb.ModsAddStatIds(b, statIDs)
	fb.ModsAddTagIds(b, tags)
	fb.FinishModsBuffer(b, fb.ModsEnd(b))
	return b.FinishedBytes()
}

func encodeMod(b *flatbuffers.Builder, st statTable, tagIndex map[string]uint32, m Mod) flatbuffers.UOffsetT {
	id := b.CreateString(m.ID)
	typ := b.CreateSharedString(m.Type)
	familyOffs := make([]flatbuffers.UOffsetT, len(m.Families))
	for i, f := range m.Families {
		familyOffs[i] = b.CreateSharedString(f)
	}
	families := tableVector(b, fb.ModStartFamiliesVector, familyOffs)
	fb.ModStartStatsVector(b, len(m.Stats))
	for _, s := range slices.Backward(m.Stats) {
		fb.CreateModStat(b, st.local[s.Stat], s.Min, s.Max, s.RuthlessMin, s.RuthlessMax)
	}
	stats := b.EndVector(len(m.Stats))
	tags := uint32Vector(b, fb.ModStartTagsVector, indexes(tagIndex, m.Tags))
	added := uint32Vector(b, fb.ModStartAddedTagsVector, indexes(tagIndex, m.AddedTags))
	spawn := tagWeightVector(b, fb.ModStartSpawnWeightsVector, tagIndex, m.SpawnWeights)
	generationWeights := tagWeightVector(b, fb.ModStartGenerationWeightsVector, tagIndex, m.GenerationWeights)
	fb.ModStart(b)
	fb.ModAddId(b, id)
	fb.ModAddDomain(b, m.Domain)
	fb.ModAddGeneration(b, m.Generation)
	fb.ModAddLevel(b, m.Level)
	fb.ModAddType(b, typ)
	fb.ModAddFamilies(b, families)
	fb.ModAddStats(b, stats)
	fb.ModAddTags(b, tags)
	fb.ModAddAddedTags(b, added)
	fb.ModAddSpawnWeights(b, spawn)
	fb.ModAddGenerationWeights(b, generationWeights)
	fb.ModAddGameMode(b, m.GameMode)
	return fb.ModEnd(b)
}

func tagWeightVector(b *flatbuffers.Builder, start startFunc, tagIndex map[string]uint32, ws []TagWeight) flatbuffers.UOffsetT {
	start(b, len(ws))
	for _, w := range slices.Backward(ws) {
		fb.CreateTagWeight(b, tagIndex[w.Tag], w.Weight)
	}
	return b.EndVector(len(ws))
}

// decodeMods reads a mods file whose stat ids resolve through index and whose tag ids must be
// among tags, the items group's tags.
func decodeMods(buf []byte, index map[string]uint32, tags []string) ([]Mod, error) {
	if !fb.ModsBufferHasIdentifier(buf) {
		return nil, errIdentifier
	}
	root := fb.GetRootAsMods(buf, 0)
	h, err := handles(index, root.StatIdsLength(), root.StatIds)
	if err != nil {
		return nil, err
	}
	tagIDs, err := localTags(tags, root.TagIdsLength(), root.TagIds)
	if err != nil {
		return nil, err
	}
	intern := interner{}
	out := make([]Mod, root.ModsLength())
	var m fb.Mod
	for i := range out {
		root.Mods(&m, i)
		if out[i], err = decodeMod(&m, h, tagIDs, intern); err != nil {
			return nil, fmt.Errorf("mod %s: %w", m.Id(), err)
		}
	}
	return out, nil
}

func decodeMod(m *fb.Mod, h []uint32, tagIDs []string, intern interner) (Mod, error) {
	out := Mod{
		ID:         string(m.Id()),
		Domain:     m.Domain(),
		Generation: m.Generation(),
		GameMode:   m.GameMode(),
		Level:      m.Level(),
		Type:       intern.string(m.Type()),
	}
	if n := m.FamiliesLength(); n > 0 {
		out.Families = make([]string, n)
		for i := range n {
			out.Families[i] = intern.string(m.Families(i))
		}
	}
	if n := m.StatsLength(); n > 0 {
		out.Stats = make([]ModStat, n)
		var s fb.ModStat
		for i := range n {
			m.Stats(&s, i)
			if int(s.Stat()) >= len(h) {
				return Mod{}, fmt.Errorf("stat %d out of range (%d ids): %w", s.Stat(), len(h), ErrInvalid)
			}
			out.Stats[i] = ModStat{Stat: h[s.Stat()], Min: s.Min(), Max: s.Max(), RuthlessMin: s.RuthlessMin(), RuthlessMax: s.RuthlessMax()}
		}
	}
	var err error
	if out.Tags, err = tagsAt(tagIDs, m.TagsLength(), m.Tags); err != nil {
		return Mod{}, err
	}
	if out.AddedTags, err = tagsAt(tagIDs, m.AddedTagsLength(), m.AddedTags); err != nil {
		return Mod{}, err
	}
	if out.SpawnWeights, err = tagWeights(tagIDs, m.SpawnWeightsLength(), m.SpawnWeights); err != nil {
		return Mod{}, err
	}
	if out.GenerationWeights, err = tagWeights(tagIDs, m.GenerationWeightsLength(), m.GenerationWeights); err != nil {
		return Mod{}, err
	}
	return out, nil
}

// localTags resolves a file's tag ids, which must be sorted, unique and among tags.
func localTags(tags []string, n int, id func(int) []byte) ([]string, error) {
	known := indexOf(tags, func(t string) string { return t })
	out := make([]string, n)
	for i := range out {
		s := string(id(i))
		if i > 0 && s <= out[i-1] {
			return nil, fmt.Errorf("tag ids not sorted and unique at %q: %w", s, ErrInvalid)
		}
		k, ok := known[s]
		if !ok {
			return nil, fmt.Errorf("unknown tag %q: %w", s, ErrInvalid)
		}
		out[i] = tags[k]
	}
	return out, nil
}

func tagsAt(tags []string, n int, at func(int) uint32) ([]string, error) {
	if n == 0 {
		return nil, nil
	}
	out := make([]string, n)
	for i := range out {
		k := int(at(i))
		if k >= len(tags) {
			return nil, errTag(k, len(tags))
		}
		out[i] = tags[k]
	}
	return out, nil
}

func tagWeights(tags []string, n int, at func(*fb.TagWeight, int) bool) ([]TagWeight, error) {
	if n == 0 {
		return nil, nil
	}
	out := make([]TagWeight, n)
	var w fb.TagWeight
	for i := range out {
		at(&w, i)
		k := int(w.Tag())
		if k >= len(tags) {
			return nil, errTag(k, len(tags))
		}
		out[i] = TagWeight{Tag: tags[k], Weight: w.Weight()}
	}
	return out, nil
}

func errTag(k, n int) error {
	return fmt.Errorf("tag %d out of range (%d tags): %w", k, n, ErrInvalid)
}

// interner shares one string per distinct value among decoded rows.
type interner map[string]string

func (in interner) string(b []byte) string {
	if s, ok := in[string(b)]; ok {
		return s
	}
	s := string(b)
	in[s] = s
	return s
}

func indexOf[T any](s []T, key func(T) string) map[string]uint32 {
	out := make(map[string]uint32, len(s))
	for i, v := range s {
		out[key(v)] = uint32(i)
	}
	return out
}

func indexes(index map[string]uint32, ids []string) []uint32 {
	out := make([]uint32, len(ids))
	for i, id := range ids {
		out[i] = index[id]
	}
	return out
}
