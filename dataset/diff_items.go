package dataset

import (
	"cmp"
	"errors"
	"slices"
)

// ItemsDiff lists what changed in the items group, by id.
type ItemsDiff struct {
	ClassesAdded, ClassesRemoved        []string
	TagsAdded, TagsRemoved              []string
	BasesAdded, BasesRemoved            []string
	BasesChanged                        []string
	ModsAdded, ModsRemoved, ModsChanged []string
}

// itemGroup is the items group of one store, empty when the store has none.
type itemGroup struct {
	items Items
	mods  []Mod
}

func itemGroupOf(s *Store) (itemGroup, error) {
	it, err := s.Items()
	if _, ok := errors.AsType[*MissingGroupError](err); ok {
		return itemGroup{}, nil
	}
	if err != nil {
		return itemGroup{}, err
	}
	mods, err := s.Mods()
	if err != nil {
		return itemGroup{}, err
	}
	return itemGroup{*it, mods}, nil
}

func compareItemGroups(a, b *Store, x, y *tables) (ItemsDiff, error) {
	p, err := itemGroupOf(a)
	if err != nil {
		return ItemsDiff{}, err
	}
	q, err := itemGroupOf(b)
	if err != nil {
		return ItemsDiff{}, err
	}
	var d ItemsDiff
	classID := func(c ItemClass) string { return c.ID }
	d.ClassesAdded, d.ClassesRemoved = setDiff(keys(byKey(p.items.Classes, classID)), keys(byKey(q.items.Classes, classID)))
	tagID := func(t string) string { return t }
	d.TagsAdded, d.TagsRemoved = setDiff(keys(byKey(p.items.Tags, tagID)), keys(byKey(q.items.Tags, tagID)))
	d.BasesAdded, d.BasesRemoved, d.BasesChanged = changes(p.items.Bases, q.items.Bases,
		func(b ItemBase) string { return b.ID }, sameBase)
	d.ModsAdded, d.ModsRemoved, d.ModsChanged = changes(p.mods, q.mods,
		func(m Mod) string { return m.ID }, func(m, n Mod) bool { return sameMod(x, y, m, n) })
	return d, nil
}

// changes returns the ids only in b, only in a, and in both but not the same, each sorted.
func changes[T any, K cmp.Ordered](a, b []T, id func(T) K, same func(T, T) bool) (added, removed, changed []K) {
	ak, bk := byKey(a, id), byKey(b, id)
	added, removed = setDiff(keys(ak), keys(bk))
	for _, k := range sortedUnion(ak, bk) {
		x, inA := ak[k]
		y, inB := bk[k]
		if inA && inB && !same(x, y) {
			changed = append(changed, k)
		}
	}
	return added, removed, changed
}

func sameBase(a, b ItemBase) bool {
	return a.Class == b.Class && slices.Equal(a.Implicits, b.Implicits) && slices.Equal(a.Tags, b.Tags) &&
		samePtr(a.Armour, b.Armour) && a.Block == b.Block && samePtr(a.Weapon, b.Weapon) && samePtr(a.Flask, b.Flask)
}

func samePtr[T comparable](a, b *T) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// sameMod compares m of a with n of b, stats by stat id.
func sameMod(a, b *tables, m, n Mod) bool {
	return m.Domain == n.Domain && m.Generation == n.Generation && m.Level == n.Level && m.Type == n.Type &&
		slices.Equal(m.Families, n.Families) && sameModStats(a, b, m.Stats, n.Stats) && sameModTags(m, n)
}

func sameModStats(a, b *tables, x, y []ModStat) bool {
	return slices.EqualFunc(x, y, func(s, t ModStat) bool {
		return a.stats[s.Stat].ID == b.stats[t.Stat].ID &&
			s.Min == t.Min && s.Max == t.Max && s.RuthlessMin == t.RuthlessMin && s.RuthlessMax == t.RuthlessMax
	})
}

func sameModTags(m, n Mod) bool {
	return slices.Equal(m.Tags, n.Tags) && slices.Equal(m.AddedTags, n.AddedTags) &&
		slices.Equal(m.SpawnWeights, n.SpawnWeights) && slices.Equal(m.GenerationWeights, n.GenerationWeights)
}
