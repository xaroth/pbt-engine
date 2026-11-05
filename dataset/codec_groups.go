package dataset

import (
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

func encodeDescriptions(scopes []statdesc.Scope) []byte {
	b := flatbuffers.NewBuilder(0)
	scopeOffs := make([]flatbuffers.UOffsetT, len(scopes))
	for si, sc := range scopes {
		descOffs := make([]flatbuffers.UOffsetT, len(sc.Descriptions))
		for di, d := range sc.Descriptions {
			statOffs := make([]flatbuffers.UOffsetT, len(d.Stats))
			for i, st := range d.Stats {
				statOffs[i] = b.CreateSharedString(st)
			}
			varOffs := make([]flatbuffers.UOffsetT, len(d.Variants))
			for vi, v := range d.Variants {
				text := b.CreateString(v.Text)
				hOffs := make([]flatbuffers.UOffsetT, len(v.Handlers))
				for hi, h := range v.Handlers {
					name := b.CreateSharedString(h.Name)
					fb.HandlerStart(b)
					fb.HandlerAddName(b, name)
					fb.HandlerAddStat(b, byte(h.Stat))
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
				varOffs[vi] = fb.VariantEnd(b)
			}
			stats := tableVector(b, fb.DescriptionStartStatsVector, statOffs)
			variants := tableVector(b, fb.DescriptionStartVariantsVector, varOffs)
			fb.DescriptionStart(b)
			fb.DescriptionAddStats(b, stats)
			fb.DescriptionAddVariants(b, variants)
			descOffs[di] = fb.DescriptionEnd(b)
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
	vec := tableVector(b, fb.DescriptionsStartScopesVector, scopeOffs)
	fb.DescriptionsStart(b)
	fb.DescriptionsAddScopes(b, vec)
	fb.FinishDescriptionsBuffer(b, fb.DescriptionsEnd(b))
	return b.FinishedBytes()
}

func decodeDescriptions(buf []byte) ([]statdesc.Scope, error) {
	if !fb.DescriptionsBufferHasIdentifier(buf) {
		return nil, errIdentifier
	}
	root := fb.GetRootAsDescriptions(buf, 0)
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
	return out, nil
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

func ascendancyVector(b *flatbuffers.Builder, start startFunc, as []Ascendancy) flatbuffers.UOffsetT {
	offs := make([]flatbuffers.UOffsetT, len(as))
	for i, a := range as {
		id := b.CreateString(a.ID)
		name := b.CreateString(a.Name)
		fb.AscendancyStart(b)
		fb.AscendancyAddId(b, id)
		fb.AscendancyAddName(b, name)
		offs[i] = fb.AscendancyEnd(b)
	}
	return tableVector(b, start, offs)
}

func encodeTree(t Tree) []byte {
	b := flatbuffers.NewBuilder(0)
	nodeOffs := make([]flatbuffers.UOffsetT, len(t.Nodes))
	for i, n := range t.Nodes {
		name := b.CreateString(n.Name)
		asc := b.CreateSharedString(n.Ascendancy)
		stats := statValueVector(b, fb.NodeStartStatsVector, n.Stats)
		buffOffs := make([]flatbuffers.UOffsetT, len(n.Buffs))
		for bi, bf := range n.Buffs {
			svs := statValueVector(b, fb.BuffStartStatsVector, bf)
			fb.BuffStart(b)
			fb.BuffAddStats(b, svs)
			buffOffs[bi] = fb.BuffEnd(b)
		}
		buffs := tableVector(b, fb.NodeStartBuffsVector, buffOffs)
		effOffs := make([]flatbuffers.UOffsetT, len(n.MasteryEffects))
		for ei, e := range n.MasteryEffects {
			svs := statValueVector(b, fb.MasteryEffectStartStatsVector, e.Stats)
			fb.MasteryEffectStart(b)
			fb.MasteryEffectAddId(b, e.ID)
			fb.MasteryEffectAddStats(b, svs)
			effOffs[ei] = fb.MasteryEffectEnd(b)
		}
		effects := tableVector(b, fb.NodeStartMasteryEffectsVector, effOffs)
		neighbours := uint32Vector(b, fb.NodeStartNeighboursVector, n.Neighbours)
		fb.NodeStart(b)
		fb.NodeAddId(b, n.ID)
		fb.NodeAddName(b, name)
		fb.NodeAddKind(b, fb.NodeKind(n.Kind))
		fb.NodeAddAscendancy(b, asc)
		fb.NodeAddStats(b, stats)
		fb.NodeAddBuffs(b, buffs)
		fb.NodeAddSkillPoints(b, n.SkillPoints)
		fb.NodeAddMasteryEffects(b, effects)
		fb.NodeAddNeighbours(b, neighbours)
		fb.NodeAddMultipleChoice(b, n.MultipleChoice)
		fb.NodeAddMultipleChoiceOption(b, n.MultipleChoiceOption)
		nodeOffs[i] = fb.NodeEnd(b)
	}
	nodes := tableVector(b, fb.TreeStartNodesVector, nodeOffs)
	classOffs := make([]flatbuffers.UOffsetT, len(t.Classes))
	for i, c := range t.Classes {
		name := b.CreateString(c.Name)
		ascs := ascendancyVector(b, fb.ClassStartAscendanciesVector, c.Ascendancies)
		fb.ClassStart(b)
		fb.ClassAddName(b, name)
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
	fb.TreeStart(b)
	fb.TreeAddVariant(b, variant)
	fb.TreeAddTotalPoints(b, t.TotalPoints)
	fb.TreeAddAscendancyPoints(b, t.AscendancyPoints)
	fb.TreeAddClasses(b, classes)
	fb.TreeAddAlternateAscendancies(b, alt)
	fb.TreeAddNodes(b, nodes)
	fb.TreeAddJewelSlots(b, jewels)
	fb.FinishTreeBuffer(b, fb.TreeEnd(b))
	return b.FinishedBytes()
}

type statValueSource interface {
	Stats(obj *fb.StatValue, j int) bool
	StatsLength() int
}

func decodeStatValues(src statValueSource) []StatValue {
	n := src.StatsLength()
	if n == 0 {
		return nil
	}
	out := make([]StatValue, n)
	var sv fb.StatValue
	for i := range out {
		src.Stats(&sv, i)
		out[i] = StatValue{Stat: sv.Stat(), Value: sv.Value()}
	}
	return out
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
		out[i] = Ascendancy{ID: string(a.Id()), Name: string(a.Name())}
	}
	return out
}

func decodeTree(buf []byte) (Tree, error) {
	if !fb.TreeBufferHasIdentifier(buf) {
		return Tree{}, errIdentifier
	}
	root := fb.GetRootAsTree(buf, 0)
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
			Name:             string(c.Name()),
			BaseStrength:     c.BaseStrength(),
			BaseDexterity:    c.BaseDexterity(),
			BaseIntelligence: c.BaseIntelligence(),
			StartNode:        c.StartNode(),
			Ascendancies:     decodeAscendancies(c.Ascendancies, c.AscendanciesLength()),
		})
	}
	var n fb.Node
	var bf fb.Buff
	var e fb.MasteryEffect
	t.Nodes = make([]Node, root.NodesLength())
	for i := range t.Nodes {
		root.Nodes(&n, i)
		node := Node{
			ID:                   n.Id(),
			Name:                 string(n.Name()),
			Kind:                 NodeKind(n.Kind()),
			Ascendancy:           string(n.Ascendancy()),
			Stats:                decodeStatValues(&n),
			SkillPoints:          n.SkillPoints(),
			MultipleChoice:       n.MultipleChoice(),
			MultipleChoiceOption: n.MultipleChoiceOption(),
		}
		for j := range n.BuffsLength() {
			n.Buffs(&bf, j)
			node.Buffs = append(node.Buffs, decodeStatValues(&bf))
		}
		for j := range n.MasteryEffectsLength() {
			n.MasteryEffects(&e, j)
			node.MasteryEffects = append(node.MasteryEffects, MasteryEffect{ID: e.Id(), Stats: decodeStatValues(&e)})
		}
		for j := range n.NeighboursLength() {
			node.Neighbours = append(node.Neighbours, n.Neighbours(j))
		}
		t.Nodes[i] = node
	}
	for j := range root.JewelSlotsLength() {
		t.JewelSlots = append(t.JewelSlots, root.JewelSlots(j))
	}
	return t, nil
}
