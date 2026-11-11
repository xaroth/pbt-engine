package dataset

import (
	"fmt"
	"slices"

	flatbuffers "github.com/google/flatbuffers/go"

	"github.com/xaroth/pbt-engine/dataset/internal/fb"
	"github.com/xaroth/pbt-engine/statdesc"
)

type startFunc = func(*flatbuffers.Builder, int) flatbuffers.UOffsetT

func statValueVector(b *flatbuffers.Builder, start startFunc, svs []StatValue) flatbuffers.UOffsetT {
	start(b, len(svs))
	for _, sv := range slices.Backward(svs) {
		fb.CreateStatValue(b, sv.Stat, sv.Value)
	}
	return b.EndVector(len(svs))
}

func uint32Vector(b *flatbuffers.Builder, start startFunc, vs []uint32) flatbuffers.UOffsetT {
	start(b, len(vs))
	for _, v := range slices.Backward(vs) {
		b.PrependUint32(v)
	}
	return b.EndVector(len(vs))
}

func encodeScopes(b *flatbuffers.Builder, scopes []statdesc.Scope) flatbuffers.UOffsetT {
	scopeOffs := make([]flatbuffers.UOffsetT, len(scopes))
	for si, sc := range scopes {
		descOffs := make([]flatbuffers.UOffsetT, len(sc.Descriptions))
		for di, d := range sc.Descriptions {
			descOffs[di] = encodeDescription(b, d)
		}
		name := b.CreateString(sc.Name)
		parent := b.CreateString(sc.Parent)
		descs := tableVector(b, fb.ScopeStartDescriptionsVector, descOffs)
		fb.ScopeStart(b)
		fb.ScopeAddName(b, name)
		fb.ScopeAddParent(b, parent)
		fb.ScopeAddDescriptions(b, descs)
		scopeOffs[si] = fb.ScopeEnd(b)
	}
	return tableVector(b, fb.TextStartScopesVector, scopeOffs)
}

func encodeDescription(b *flatbuffers.Builder, d statdesc.Description) flatbuffers.UOffsetT {
	statOffs := make([]flatbuffers.UOffsetT, len(d.Stats))
	for i, st := range d.Stats {
		statOffs[i] = b.CreateSharedString(st)
	}
	varOffs := make([]flatbuffers.UOffsetT, len(d.Variants))
	for vi, v := range d.Variants {
		varOffs[vi] = encodeVariant(b, v)
	}
	stats := tableVector(b, fb.DescriptionStartStatsVector, statOffs)
	variants := tableVector(b, fb.DescriptionStartVariantsVector, varOffs)
	fb.DescriptionStart(b)
	fb.DescriptionAddStats(b, stats)
	fb.DescriptionAddVariants(b, variants)
	return fb.DescriptionEnd(b)
}

func encodeVariant(b *flatbuffers.Builder, v statdesc.Variant) flatbuffers.UOffsetT {
	text := b.CreateString(v.Text)
	hOffs := make([]flatbuffers.UOffsetT, len(v.Handlers))
	for hi, h := range v.Handlers {
		name := b.CreateSharedString(h.Name)
		fb.HandlerStart(b)
		fb.HandlerAddName(b, name)
		fb.HandlerAddStat(b, byte(h.Stat)) //nolint:gosec // validate bounds h.Stat by a description's at most 256 stats
		hOffs[hi] = fb.HandlerEnd(b)
	}
	handlers := tableVector(b, fb.VariantStartHandlersVector, hOffs)
	fb.VariantStartLimitsVector(b, len(v.Limits))
	for _, l := range slices.Backward(v.Limits) {
		fb.CreateLimit(b, l.Min, l.Max, l.Not)
	}
	limits := b.EndVector(len(v.Limits))
	fb.VariantStart(b)
	fb.VariantAddLimits(b, limits)
	fb.VariantAddText(b, text)
	fb.VariantAddHandlers(b, handlers)
	return fb.VariantEnd(b)
}

func encodeText(t Text) []byte {
	b := flatbuffers.NewBuilder(0)
	scopes := encodeScopes(b, t.Descriptions)
	nodeOffs := make([]flatbuffers.UOffsetT, len(t.NodeNames))
	for i, n := range t.NodeNames {
		name := b.CreateString(n.Name)
		fb.NodeNameStart(b)
		fb.NodeNameAddId(b, n.ID)
		fb.NodeNameAddName(b, name)
		nodeOffs[i] = fb.NodeNameEnd(b)
	}
	nodes := tableVector(b, fb.TextStartNodeNamesVector, nodeOffs)
	chars := nameVector(b, fb.TextStartCharacterNamesVector, t.CharacterNames)
	ascOffs := make([]flatbuffers.UOffsetT, len(t.AscendancyNames))
	for i, n := range t.AscendancyNames {
		variant := b.CreateSharedString(n.Variant)
		id := b.CreateString(n.ID)
		name := b.CreateString(n.Name)
		fb.AscendancyNameStart(b)
		fb.AscendancyNameAddVariant(b, variant)
		fb.AscendancyNameAddId(b, id)
		fb.AscendancyNameAddName(b, name)
		ascOffs[i] = fb.AscendancyNameEnd(b)
	}
	ascs := tableVector(b, fb.TextStartAscendancyNamesVector, ascOffs)
	// Item names are left out when empty, so a text without them keeps the bytes it had before
	// the items group existed.
	var classes, bases, mods flatbuffers.UOffsetT
	if len(t.ItemClassNames)+len(t.BaseNames)+len(t.ModNames) > 0 {
		classes = nameVector(b, fb.TextStartItemClassNamesVector, t.ItemClassNames)
		bases = nameVector(b, fb.TextStartBaseNamesVector, t.BaseNames)
		mods = nameVector(b, fb.TextStartModNamesVector, t.ModNames)
	}
	lang := b.CreateString(t.Language)
	fb.TextStart(b)
	fb.TextAddLanguage(b, lang)
	fb.TextAddScopes(b, scopes)
	fb.TextAddNodeNames(b, nodes)
	fb.TextAddCharacterNames(b, chars)
	fb.TextAddAscendancyNames(b, ascs)
	if classes != 0 {
		fb.TextAddItemClassNames(b, classes)
		fb.TextAddBaseNames(b, bases)
		fb.TextAddModNames(b, mods)
	}
	fb.FinishTextBuffer(b, fb.TextEnd(b))
	return b.FinishedBytes()
}

func decodeText(buf []byte) (Text, error) {
	if !fb.TextBufferHasIdentifier(buf) {
		return Text{}, errIdentifier
	}
	root := fb.GetRootAsText(buf, 0)
	t := Text{Language: string(root.Language()), Descriptions: decodeScopes(root)}
	var nn fb.NodeName
	for i := range root.NodeNamesLength() {
		root.NodeNames(&nn, i)
		t.NodeNames = append(t.NodeNames, NodeName{ID: nn.Id(), Name: string(nn.Name())})
	}
	t.CharacterNames = decodeNames(root.CharacterNamesLength(), root.CharacterNames)
	t.ItemClassNames = decodeNames(root.ItemClassNamesLength(), root.ItemClassNames)
	t.BaseNames = decodeNames(root.BaseNamesLength(), root.BaseNames)
	t.ModNames = decodeNames(root.ModNamesLength(), root.ModNames)
	var an fb.AscendancyName
	for i := range root.AscendancyNamesLength() {
		root.AscendancyNames(&an, i)
		t.AscendancyNames = append(t.AscendancyNames, AscendancyName{Variant: string(an.Variant()), ID: string(an.Id()), Name: string(an.Name())})
	}
	return t, nil
}

func nameVector(b *flatbuffers.Builder, start startFunc, names []Name) flatbuffers.UOffsetT {
	offs := make([]flatbuffers.UOffsetT, len(names))
	for i, n := range names {
		id := b.CreateString(n.ID)
		name := b.CreateString(n.Name)
		fb.NameStart(b)
		fb.NameAddId(b, id)
		fb.NameAddName(b, name)
		offs[i] = fb.NameEnd(b)
	}
	return tableVector(b, start, offs)
}

func decodeNames(n int, get func(*fb.Name, int) bool) []Name {
	if n == 0 {
		return nil
	}
	out := make([]Name, n)
	var name fb.Name
	for i := range out {
		get(&name, i)
		out[i] = Name{ID: string(name.Id()), Name: string(name.Name())}
	}
	return out
}

func decodeScopes(root *fb.Text) []statdesc.Scope {
	out := make([]statdesc.Scope, root.ScopesLength())
	var sc fb.Scope
	var d fb.Description
	var v fb.Variant
	var h fb.Handler
	var l fb.Limit
	for si := range out {
		root.Scopes(&sc, si)
		s := statdesc.Scope{Name: string(sc.Name()), Parent: string(sc.Parent())}
		s.Descriptions = make([]statdesc.Description, sc.DescriptionsLength())
		for di := range s.Descriptions {
			sc.Descriptions(&d, di)
			desc := statdesc.Description{Stats: make([]string, d.StatsLength())}
			for i := range desc.Stats {
				desc.Stats[i] = string(d.Stats(i))
			}
			if n := d.VariantsLength(); n > 0 {
				desc.Variants = make([]statdesc.Variant, n)
			}
			for vi := range desc.Variants {
				d.Variants(&v, vi)
				variant := statdesc.Variant{Text: string(v.Text()), Limits: make([]statdesc.Limit, v.LimitsLength())}
				for li := range variant.Limits {
					v.Limits(&l, li)
					variant.Limits[li] = statdesc.Limit{Min: l.Min(), Max: l.Max(), Not: l.Not()}
				}
				if n := v.HandlersLength(); n > 0 {
					variant.Handlers = make([]statdesc.Handler, n)
				}
				for hi := range variant.Handlers {
					v.Handlers(&h, hi)
					variant.Handlers[hi] = statdesc.Handler{Name: string(h.Name()), Stat: int(h.Stat())}
				}
				desc.Variants[vi] = variant
			}
			s.Descriptions[di] = desc
		}
		out[si] = s
	}
	return out
}

func encodeConstants(cs []Constant) []byte {
	b := flatbuffers.NewBuilder(0)
	offs := make([]flatbuffers.UOffsetT, len(cs))
	for i, c := range cs {
		id := b.CreateString(c.ID)
		fb.ConstantStart(b)
		fb.ConstantAddId(b, id)
		fb.ConstantAddValue(b, c.Value)
		fb.ConstantAddPer(b, c.Per)
		offs[i] = fb.ConstantEnd(b)
	}
	vec := tableVector(b, fb.ConstantsStartConstantsVector, offs)
	fb.ConstantsStart(b)
	fb.ConstantsAddConstants(b, vec)
	fb.FinishConstantsBuffer(b, fb.ConstantsEnd(b))
	return b.FinishedBytes()
}

func decodeConstants(buf []byte) ([]Constant, error) {
	if !fb.ConstantsBufferHasIdentifier(buf) {
		return nil, errIdentifier
	}
	root := fb.GetRootAsConstants(buf, 0)
	out := make([]Constant, root.ConstantsLength())
	var c fb.Constant
	for i := range out {
		root.Constants(&c, i)
		out[i] = Constant{ID: string(c.Id()), Value: c.Value(), Per: c.Per()}
	}
	return out, nil
}

func encodeQuests(all []Stat, qs []Quest) []byte {
	b := flatbuffers.NewBuilder(0)
	lists := make([][]StatValue, len(qs))
	for i, q := range qs {
		lists[i] = q.Stats
	}
	st := newStatTable(all, lists...)
	offs := make([]flatbuffers.UOffsetT, len(qs))
	for i, q := range qs {
		quest, flag := b.CreateString(q.Quest), b.CreateString(q.Flag)
		stats := statValueVector(b, fb.QuestStartStatsVector, st.toLocal(q.Stats, true))
		fb.QuestStart(b)
		fb.QuestAddQuest(b, quest)
		fb.QuestAddFlag(b, flag)
		fb.QuestAddAct(b, q.Act)
		fb.QuestAddPassivePoints(b, q.PassivePoints)
		fb.QuestAddAscendancyPoints(b, q.AscendancyPoints)
		fb.QuestAddRefundPoints(b, q.RefundPoints)
		fb.QuestAddStats(b, stats)
		offs[i] = fb.QuestEnd(b)
	}
	vec := tableVector(b, fb.QuestsStartQuestsVector, offs)
	ids := stringVector(b, fb.QuestsStartStatIdsVector, st.ids)
	fb.QuestsStart(b)
	fb.QuestsAddQuests(b, vec)
	fb.QuestsAddStatIds(b, ids)
	fb.FinishQuestsBuffer(b, fb.QuestsEnd(b))
	return b.FinishedBytes()
}

func decodeQuests(buf []byte, index map[string]uint32) ([]Quest, error) {
	if !fb.QuestsBufferHasIdentifier(buf) {
		return nil, errIdentifier
	}
	root := fb.GetRootAsQuests(buf, 0)
	h, err := handles(index, root.StatIdsLength(), root.StatIds)
	if err != nil {
		return nil, err
	}
	out := make([]Quest, root.QuestsLength())
	var q fb.Quest
	for i := range out {
		root.Quests(&q, i)
		stats, err := fromLocal(&q, h, true)
		if err != nil {
			return nil, fmt.Errorf("quest flag %s: %w", q.Flag(), err)
		}
		out[i] = Quest{
			Quest: string(q.Quest()), Flag: string(q.Flag()), Act: q.Act(),
			PassivePoints: q.PassivePoints(), AscendancyPoints: q.AscendancyPoints(), RefundPoints: q.RefundPoints(),
			Stats: stats,
		}
	}
	return out, nil
}

func ascendancyVector(b *flatbuffers.Builder, start startFunc, as []Ascendancy) flatbuffers.UOffsetT {
	offs := make([]flatbuffers.UOffsetT, len(as))
	for i, a := range as {
		id := b.CreateString(a.ID)
		fb.AscendancyStart(b)
		fb.AscendancyAddId(b, id)
		offs[i] = fb.AscendancyEnd(b)
	}
	return tableVector(b, start, offs)
}

func encodeTree(all []Stat, t Tree) []byte {
	b := flatbuffers.NewBuilder(0)
	st := newStatTable(all, treeStatLists(t)...)
	nodeOffs := make([]flatbuffers.UOffsetT, len(t.Nodes))
	for i, n := range t.Nodes {
		nodeOffs[i] = encodeNode(b, st, n)
	}
	nodes := tableVector(b, fb.TreeStartNodesVector, nodeOffs)
	classOffs := make([]flatbuffers.UOffsetT, len(t.Classes))
	for i, c := range t.Classes {
		char := b.CreateString(c.Character)
		ascs := ascendancyVector(b, fb.ClassStartAscendanciesVector, c.Ascendancies)
		fb.ClassStart(b)
		fb.ClassAddCharacter(b, char)
		fb.ClassAddBaseStrength(b, c.BaseStrength)
		fb.ClassAddBaseDexterity(b, c.BaseDexterity)
		fb.ClassAddBaseIntelligence(b, c.BaseIntelligence)
		fb.ClassAddStartNode(b, c.StartNode)
		fb.ClassAddAscendancies(b, ascs)
		classOffs[i] = fb.ClassEnd(b)
	}
	classes := tableVector(b, fb.TreeStartClassesVector, classOffs)
	alt := ascendancyVector(b, fb.TreeStartAlternateAscendanciesVector, t.AlternateAscendancies)
	jewels := uint32Vector(b, fb.TreeStartJewelSlotsVector, t.JewelSlots)
	variant := b.CreateString(t.Variant)
	ids := stringVector(b, fb.TreeStartStatIdsVector, st.ids)
	fb.TreeStart(b)
	fb.TreeAddVariant(b, variant)
	fb.TreeAddTotalPoints(b, t.TotalPoints)
	fb.TreeAddAscendancyPoints(b, t.AscendancyPoints)
	fb.TreeAddClasses(b, classes)
	fb.TreeAddAlternateAscendancies(b, alt)
	fb.TreeAddNodes(b, nodes)
	fb.TreeAddJewelSlots(b, jewels)
	fb.TreeAddStatIds(b, ids)
	fb.FinishTreeBuffer(b, fb.TreeEnd(b))
	return b.FinishedBytes()
}

func treeStatLists(t Tree) [][]StatValue {
	var lists [][]StatValue
	for _, n := range t.Nodes {
		lists = append(lists, n.Stats)
		lists = append(lists, n.Buffs...)
		for _, e := range n.MasteryEffects {
			lists = append(lists, e.Stats)
		}
	}
	return lists
}

func encodeNode(b *flatbuffers.Builder, st statTable, n Node) flatbuffers.UOffsetT {
	asc := b.CreateSharedString(n.Ascendancy)
	stats := statValueVector(b, fb.NodeStartStatsVector, st.toLocal(n.Stats, true))
	buffOffs := make([]flatbuffers.UOffsetT, len(n.Buffs))
	for bi, bf := range n.Buffs {
		svs := statValueVector(b, fb.BuffStartStatsVector, st.toLocal(bf, false))
		fb.BuffStart(b)
		fb.BuffAddStats(b, svs)
		buffOffs[bi] = fb.BuffEnd(b)
	}
	buffs := tableVector(b, fb.NodeStartBuffsVector, buffOffs)
	effOffs := make([]flatbuffers.UOffsetT, len(n.MasteryEffects))
	for ei, e := range n.MasteryEffects {
		svs := statValueVector(b, fb.MasteryEffectStartStatsVector, st.toLocal(e.Stats, false))
		fb.MasteryEffectStart(b)
		fb.MasteryEffectAddId(b, e.ID)
		fb.MasteryEffectAddStats(b, svs)
		effOffs[ei] = fb.MasteryEffectEnd(b)
	}
	effects := tableVector(b, fb.NodeStartMasteryEffectsVector, effOffs)
	neighbours := uint32Vector(b, fb.NodeStartNeighboursVector, n.Neighbours)
	fb.NodeStart(b)
	fb.NodeAddId(b, n.ID)
	fb.NodeAddKind(b, fb.NodeKind(n.Kind))
	fb.NodeAddAscendancy(b, asc)
	fb.NodeAddStats(b, stats)
	fb.NodeAddBuffs(b, buffs)
	fb.NodeAddSkillPoints(b, n.SkillPoints)
	fb.NodeAddMasteryEffects(b, effects)
	fb.NodeAddNeighbours(b, neighbours)
	fb.NodeAddMultipleChoice(b, n.MultipleChoice)
	fb.NodeAddMultipleChoiceOption(b, n.MultipleChoiceOption)
	return fb.NodeEnd(b)
}

type statValueSource interface {
	Stats(obj *fb.StatValue, j int) bool
	StatsLength() int
}

type ascendancySource = func(obj *fb.Ascendancy, j int) bool

func decodeAscendancies(get ascendancySource, n int) []Ascendancy {
	if n == 0 {
		return nil
	}
	out := make([]Ascendancy, n)
	var a fb.Ascendancy
	for i := range out {
		get(&a, i)
		out[i] = Ascendancy{ID: string(a.Id())}
	}
	return out
}

func decodeTree(buf []byte, index map[string]uint32) (Tree, error) {
	if !fb.TreeBufferHasIdentifier(buf) {
		return Tree{}, errIdentifier
	}
	root := fb.GetRootAsTree(buf, 0)
	h, err := handles(index, root.StatIdsLength(), root.StatIds)
	if err != nil {
		return Tree{}, err
	}
	t := Tree{
		Variant:               string(root.Variant()),
		TotalPoints:           root.TotalPoints(),
		AscendancyPoints:      root.AscendancyPoints(),
		AlternateAscendancies: decodeAscendancies(root.AlternateAscendancies, root.AlternateAscendanciesLength()),
	}
	var c fb.Class
	for i := range root.ClassesLength() {
		root.Classes(&c, i)
		t.Classes = append(t.Classes, Class{
			Character:        string(c.Character()),
			BaseStrength:     c.BaseStrength(),
			BaseDexterity:    c.BaseDexterity(),
			BaseIntelligence: c.BaseIntelligence(),
			StartNode:        c.StartNode(),
			Ascendancies:     decodeAscendancies(c.Ascendancies, c.AscendanciesLength()),
		})
	}
	var n fb.Node
	t.Nodes = make([]Node, root.NodesLength())
	for i := range t.Nodes {
		root.Nodes(&n, i)
		if t.Nodes[i], err = decodeNode(&n, h); err != nil {
			return Tree{}, err
		}
	}
	for j := range root.JewelSlotsLength() {
		t.JewelSlots = append(t.JewelSlots, root.JewelSlots(j))
	}
	return t, nil
}

func decodeNode(n *fb.Node, h []uint32) (Node, error) {
	stats, err := fromLocal(n, h, true)
	if err != nil {
		return Node{}, fmt.Errorf("node %d: %w", n.Id(), err)
	}
	node := Node{
		ID:                   n.Id(),
		Kind:                 NodeKind(n.Kind()),
		Ascendancy:           string(n.Ascendancy()),
		Stats:                stats,
		SkillPoints:          n.SkillPoints(),
		MultipleChoice:       n.MultipleChoice(),
		MultipleChoiceOption: n.MultipleChoiceOption(),
	}
	var bf fb.Buff
	for j := range n.BuffsLength() {
		n.Buffs(&bf, j)
		buff, err := fromLocal(&bf, h, false)
		if err != nil {
			return Node{}, fmt.Errorf("node %d buff %d: %w", n.Id(), j, err)
		}
		node.Buffs = append(node.Buffs, buff)
	}
	var e fb.MasteryEffect
	for j := range n.MasteryEffectsLength() {
		n.MasteryEffects(&e, j)
		effect, err := fromLocal(&e, h, false)
		if err != nil {
			return Node{}, fmt.Errorf("node %d mastery effect %d: %w", n.Id(), e.Id(), err)
		}
		node.MasteryEffects = append(node.MasteryEffects, MasteryEffect{ID: e.Id(), Stats: effect})
	}
	for j := range n.NeighboursLength() {
		node.Neighbours = append(node.Neighbours, n.Neighbours(j))
	}
	return node, nil
}
