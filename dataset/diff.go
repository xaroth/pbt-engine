package dataset

import (
	"cmp"
	"maps"
	"reflect"
	"slices"

	"github.com/xaroth/pbt-engine/statdesc"
)

// Diff lists what changed between two datasets, keyed by game ids. Stat handles differ between
// datasets, so stats are compared by stat id.
type Diff struct {
	StatsAdded, StatsRemoved         []string
	CharactersChanged                []string
	ConstantsAdded, ConstantsRemoved []string
	ConstantsChanged                 []string
	Trees                            []TreeDiff
	// Texts compares the languages present in both datasets.
	Texts                                  []TextDiff
	LanguagesAdded, LanguagesRemoved       []string
	TreeVariantsAdded, TreeVariantsRemoved []string
}

// TreeDiff lists node changes in one tree variant present in both datasets.
type TreeDiff struct {
	Variant        string
	Added, Removed []uint32
	Changed        []NodeDiff
	EffectsAdded   []uint32
	EffectsRemoved []uint32
	NodesCompared  int
}

// TextDiff lists display text changes in one language.
type TextDiff struct {
	Language              string
	ScopesChanged         []string
	NodeNamesChanged      []uint32
	CharacterNamesChanged []string
	// AscendancyNamesChanged holds "<variant>/<id>".
	AscendancyNamesChanged []string
}

// NodeDiff is a node present in both trees whose content changed.
type NodeDiff struct {
	ID uint32
	// Fields names what changed: kind, ascendancy, stats, buffs, skill_points,
	// mastery_effects, neighbours.
	Fields []string
	// StatsBefore and StatsAfter are the node stats by stat id when stats changed.
	StatsBefore, StatsAfter map[string]int32
}

// Compare returns the changes from a to b.
func Compare(a, b *Dataset) Diff {
	var d Diff
	d.StatsAdded, d.StatsRemoved = setDiff(statIDs(a), statIDs(b))

	ac, bc := byKey(a.Characters, func(c Character) string { return c.ID }), byKey(b.Characters, func(c Character) string { return c.ID })
	for _, id := range sortedUnion(ac, bc) {
		x, inA := ac[id]
		y, inB := bc[id]
		if !inA || !inB || !sameCharacter(a, b, x, y) {
			d.CharactersChanged = append(d.CharactersChanged, id)
		}
	}

	ak, bk := byKey(a.Constants, func(c Constant) string { return c.ID }), byKey(b.Constants, func(c Constant) string { return c.ID })
	d.ConstantsAdded, d.ConstantsRemoved = setDiff(keys(ak), keys(bk))
	for _, id := range sortedUnion(ak, bk) {
		x, inA := ak[id]
		y, inB := bk[id]
		if inA && inB && x != y {
			d.ConstantsChanged = append(d.ConstantsChanged, id)
		}
	}

	ax, bx := byKey(a.Texts, func(t Text) string { return t.Language }), byKey(b.Texts, func(t Text) string { return t.Language })
	d.LanguagesAdded, d.LanguagesRemoved = setDiff(keys(ax), keys(bx))
	for _, lang := range sortedUnion(ax, bx) {
		x, inA := ax[lang]
		y, inB := bx[lang]
		if inA && inB {
			d.Texts = append(d.Texts, compareTexts(x, y))
		}
	}

	at, bt := byKey(a.Trees, func(t Tree) string { return t.Variant }), byKey(b.Trees, func(t Tree) string { return t.Variant })
	d.TreeVariantsAdded, d.TreeVariantsRemoved = setDiff(keys(at), keys(bt))
	for _, v := range sortedUnion(at, bt) {
		x, inA := at[v]
		y, inB := bt[v]
		if inA && inB {
			d.Trees = append(d.Trees, compareTrees(a, b, x, y))
		}
	}
	return d
}

func compareTrees(a, b *Dataset, x, y Tree) TreeDiff {
	td := TreeDiff{Variant: x.Variant}
	xn := byKey(x.Nodes, func(n Node) uint32 { return n.ID })
	yn := byKey(y.Nodes, func(n Node) uint32 { return n.ID })
	td.Added, td.Removed = setDiff(keys(xn), keys(yn))
	xe, ye := map[uint32]bool{}, map[uint32]bool{}
	for _, n := range x.Nodes {
		for _, e := range n.MasteryEffects {
			xe[e.ID] = true
		}
	}
	for _, n := range y.Nodes {
		for _, e := range n.MasteryEffects {
			ye[e.ID] = true
		}
	}
	td.EffectsAdded, td.EffectsRemoved = setDiff(keys(xe), keys(ye))
	for _, id := range sortedUnion(xn, yn) {
		p, inX := xn[id]
		q, inY := yn[id]
		if !inX || !inY {
			continue
		}
		td.NodesCompared++
		nd := NodeDiff{ID: id}
		add := func(changed bool, field string) {
			if changed {
				nd.Fields = append(nd.Fields, field)
			}
		}
		before, after := statMap(a, p.Stats), statMap(b, q.Stats)
		add(p.Kind != q.Kind, "kind")
		add(p.Ascendancy != q.Ascendancy, "ascendancy")
		add(!maps.Equal(before, after), "stats")
		add(!slices.EqualFunc(p.Buffs, q.Buffs, func(s, t []StatValue) bool { return maps.Equal(statMap(a, s), statMap(b, t)) }), "buffs")
		add(p.SkillPoints != q.SkillPoints, "skill_points")
		add(!slices.EqualFunc(p.MasteryEffects, q.MasteryEffects, func(s, t MasteryEffect) bool {
			return s.ID == t.ID && maps.Equal(statMap(a, s.Stats), statMap(b, t.Stats))
		}), "mastery_effects")
		add(!slices.Equal(p.Neighbours, q.Neighbours), "neighbours")
		if len(nd.Fields) == 0 {
			continue
		}
		if !maps.Equal(before, after) {
			nd.StatsBefore, nd.StatsAfter = before, after
		}
		td.Changed = append(td.Changed, nd)
	}
	return td
}

func compareTexts(x, y Text) TextDiff {
	td := TextDiff{Language: x.Language}
	xs := byKey(x.Descriptions, func(s statdesc.Scope) string { return s.Name })
	ys := byKey(y.Descriptions, func(s statdesc.Scope) string { return s.Name })
	for _, name := range sortedUnion(xs, ys) {
		p, inX := xs[name]
		q, inY := ys[name]
		if !inX || !inY || !equalScope(p, q) {
			td.ScopesChanged = append(td.ScopesChanged, name)
		}
	}
	td.NodeNamesChanged = changedNames(x.NodeNames, y.NodeNames, func(n NodeName) (uint32, string) { return n.ID, n.Name })
	td.CharacterNamesChanged = changedNames(x.CharacterNames, y.CharacterNames, func(n Name) (string, string) { return n.ID, n.Name })
	td.AscendancyNamesChanged = changedNames(x.AscendancyNames, y.AscendancyNames, func(n AscendancyName) (string, string) {
		return n.Variant + "/" + n.ID, n.Name
	})
	return td
}

// changedNames returns the keys whose name differs, including keys present on one side only.
func changedNames[T any, K cmp.Ordered](x, y []T, kv func(T) (K, string)) []K {
	xm, ym := map[K]string{}, map[K]string{}
	for _, v := range x {
		k, n := kv(v)
		xm[k] = n
	}
	for _, v := range y {
		k, n := kv(v)
		ym[k] = n
	}
	var out []K
	for _, k := range sortedUnion(xm, ym) {
		p, inX := xm[k]
		q, inY := ym[k]
		if !inX || !inY || p != q {
			out = append(out, k)
		}
	}
	return out
}

// StatMap resolves stat handles in svs to stat ids.
func (d *Dataset) StatMap(svs []StatValue) map[string]int32 { return statMap(d, svs) }

func statMap(d *Dataset, svs []StatValue) map[string]int32 {
	out := make(map[string]int32, len(svs))
	for _, sv := range svs {
		out[d.Stats[sv.Stat].ID] = sv.Value
	}
	return out
}

func sameCharacter(a, b *Dataset, x, y Character) bool {
	return x.BaseStrength == y.BaseStrength && x.BaseDexterity == y.BaseDexterity &&
		x.BaseIntelligence == y.BaseIntelligence && x.BaseMaxLife == y.BaseMaxLife && x.BaseMaxMana == y.BaseMaxMana &&
		maps.Equal(statMap(a, x.Stats), statMap(b, y.Stats))
}

func equalScope(x, y statdesc.Scope) bool { return reflect.DeepEqual(x, y) }

func statIDs(d *Dataset) map[string]bool {
	out := make(map[string]bool, len(d.Stats))
	for _, s := range d.Stats {
		out[s.ID] = true
	}
	return out
}

func byKey[T any, K comparable](s []T, key func(T) K) map[K]T {
	out := make(map[K]T, len(s))
	for _, v := range s {
		out[key(v)] = v
	}
	return out
}

func keys[K cmp.Ordered, V any](m map[K]V) map[K]bool {
	out := make(map[K]bool, len(m))
	for k := range m {
		out[k] = true
	}
	return out
}

// setDiff returns the keys only in b (added) and only in a (removed), sorted.
func setDiff[K cmp.Ordered](a, b map[K]bool) (added, removed []K) {
	for k := range b {
		if !a[k] {
			added = append(added, k)
		}
	}
	for k := range a {
		if !b[k] {
			removed = append(removed, k)
		}
	}
	slices.Sort(added)
	slices.Sort(removed)
	return added, removed
}

func sortedUnion[K cmp.Ordered, V, W any](a map[K]V, b map[K]W) []K {
	seen := map[K]bool{}
	for k := range a {
		seen[k] = true
	}
	for k := range b {
		seen[k] = true
	}
	return slices.Sorted(maps.Keys(seen))
}
