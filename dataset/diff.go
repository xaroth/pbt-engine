package dataset

import (
	"cmp"
	"maps"
	"slices"
)

// Diff lists what changed between two datasets, keyed by game ids. Stat handles differ between
// datasets, so stats are compared by stat id.
type Diff struct {
	StatsAdded, StatsRemoved               []string
	CharactersChanged                      []string
	ConstantsAdded, ConstantsRemoved       []string
	ConstantsChanged                       []string
	Trees                                  []TreeDiff
	TreeVariantsAdded, TreeVariantsRemoved []string
	// Items is empty when neither dataset has the items group; a dataset without it compares
	// as one with an empty group.
	Items ItemsDiff
	// Skills is empty when neither dataset has the skills group; a dataset without it compares
	// as one with an empty group.
	Skills SkillsDiff
	// Buffs is empty when neither dataset has the buffs group; a dataset without it compares
	// as one with an empty group.
	Buffs BuffsDiff
	// Monsters is empty when neither dataset has the monsters group; a dataset without it
	// compares as one with an empty group.
	Monsters MonstersDiff
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

// NodeDiff is a node present in both trees whose content changed.
type NodeDiff struct {
	ID uint32
	// Fields names what changed: kind, ascendancy, stats, buffs, skill_points,
	// mastery_effects, neighbours.
	Fields []string
	// StatsBefore and StatsAfter are the node stats by stat id when stats changed.
	StatsBefore, StatsAfter map[string]int32
}

// Compare returns the changes from a to b in stats, characters, constants, trees, items,
// skills, buffs and monsters. It loads every tree and the items, skills, buffs and monsters
// groups of both; CompareTree compares one tree.
func Compare(a, b *Store) (Diff, error) {
	x, err := tablesOf(a, a.Manifest().Trees())
	if err != nil {
		return Diff{}, err
	}
	y, err := tablesOf(b, b.Manifest().Trees())
	if err != nil {
		return Diff{}, err
	}
	var d Diff
	d.StatsAdded, d.StatsRemoved = setDiff(statIDs(x), statIDs(y))
	compareCharacters(&d, x, y)
	compareConstants(&d, x, y)
	compareTreeGroups(&d, x, y)
	if d.Items, err = compareItemGroups(a, b, x, y); err != nil {
		return Diff{}, err
	}
	if d.Skills, err = compareSkillGroups(a, b, x, y); err != nil {
		return Diff{}, err
	}
	if d.Buffs, err = compareBuffGroups(a, b, x, y); err != nil {
		return Diff{}, err
	}
	if d.Monsters, err = compareMonsterGroups(a, b, x, y); err != nil {
		return Diff{}, err
	}
	return d, nil
}

// CompareTree returns the node changes from a's tree of variant to b's. It fails when either
// dataset has no such tree.
func CompareTree(a, b *Store, variant string) (TreeDiff, error) {
	x, err := tablesOf(a, []string{variant})
	if err != nil {
		return TreeDiff{}, err
	}
	y, err := tablesOf(b, []string{variant})
	if err != nil {
		return TreeDiff{}, err
	}
	return compareTrees(x, y, x.trees[0], y.trees[0]), nil
}

// tables are the groups Compare reads from a store.
type tables struct {
	stats      []Stat
	characters []Character
	constants  []Constant
	trees      []*Tree
}

func tablesOf(s *Store, variants []string) (*tables, error) {
	var t tables
	var err error
	if t.stats, err = s.Stats(); err != nil {
		return nil, err
	}
	if t.characters, err = s.Characters(); err != nil {
		return nil, err
	}
	if t.constants, err = s.Constants(); err != nil {
		return nil, err
	}
	for _, v := range variants {
		tr, err := s.Tree(v)
		if err != nil {
			return nil, err
		}
		t.trees = append(t.trees, tr)
	}
	return &t, nil
}

func compareCharacters(d *Diff, a, b *tables) {
	ac, bc := byKey(a.characters, func(c Character) string { return c.ID }), byKey(b.characters, func(c Character) string { return c.ID })
	for _, id := range sortedUnion(ac, bc) {
		x, inA := ac[id]
		y, inB := bc[id]
		if !inA || !inB || !sameCharacter(a, b, x, y) {
			d.CharactersChanged = append(d.CharactersChanged, id)
		}
	}
}

func compareConstants(d *Diff, a, b *tables) {
	ak, bk := byKey(a.constants, func(c Constant) string { return c.ID }), byKey(b.constants, func(c Constant) string { return c.ID })
	d.ConstantsAdded, d.ConstantsRemoved = setDiff(keys(ak), keys(bk))
	for _, id := range sortedUnion(ak, bk) {
		x, inA := ak[id]
		y, inB := bk[id]
		if inA && inB && x != y {
			d.ConstantsChanged = append(d.ConstantsChanged, id)
		}
	}
}

func compareTreeGroups(d *Diff, a, b *tables) {
	at, bt := byKey(a.trees, func(t *Tree) string { return t.Variant }), byKey(b.trees, func(t *Tree) string { return t.Variant })
	d.TreeVariantsAdded, d.TreeVariantsRemoved = setDiff(keys(at), keys(bt))
	for _, v := range sortedUnion(at, bt) {
		x, inA := at[v]
		y, inB := bt[v]
		if inA && inB {
			d.Trees = append(d.Trees, compareTrees(a, b, x, y))
		}
	}
}

func compareTrees(a, b *tables, x, y *Tree) TreeDiff {
	td := TreeDiff{Variant: x.Variant}
	xn := byKey(x.Nodes, func(n Node) uint32 { return n.ID })
	yn := byKey(y.Nodes, func(n Node) uint32 { return n.ID })
	td.Added, td.Removed = setDiff(keys(xn), keys(yn))
	td.EffectsAdded, td.EffectsRemoved = setDiff(keys(effectIDs(x)), keys(effectIDs(y)))
	for _, id := range sortedUnion(xn, yn) {
		p, inX := xn[id]
		q, inY := yn[id]
		if !inX || !inY {
			continue
		}
		td.NodesCompared++
		if nd := compareNode(a, b, p, q); len(nd.Fields) > 0 {
			td.Changed = append(td.Changed, nd)
		}
	}
	return td
}

func effectIDs(t *Tree) map[uint32]bool {
	ids := map[uint32]bool{}
	for _, n := range t.Nodes {
		for _, e := range n.MasteryEffects {
			ids[e.ID] = true
		}
	}
	return ids
}

// compareNode returns the fields that differ between p and q.
func compareNode(a, b *tables, p, q Node) NodeDiff {
	nd := NodeDiff{ID: p.ID}
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
	if !maps.Equal(before, after) {
		nd.StatsBefore, nd.StatsAfter = before, after
	}
	return nd
}

func statMap(t *tables, svs []StatValue) map[string]int32 {
	out := make(map[string]int32, len(svs))
	for _, sv := range svs {
		out[t.stats[sv.Stat].ID] = sv.Value
	}
	return out
}

func sameCharacter(a, b *tables, x, y Character) bool {
	return x.BaseStrength == y.BaseStrength && x.BaseDexterity == y.BaseDexterity &&
		x.BaseIntelligence == y.BaseIntelligence && x.BaseMaxLife == y.BaseMaxLife && x.BaseMaxMana == y.BaseMaxMana &&
		maps.Equal(statMap(a, x.Stats), statMap(b, y.Stats))
}

func statIDs(t *tables) map[string]bool {
	out := make(map[string]bool, len(t.stats))
	for _, s := range t.stats {
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
