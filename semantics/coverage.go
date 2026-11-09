package semantics

import (
	"cmp"
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"

	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/resolve"
)

// StatUse is a stat in a scope as a dataset's content gives it.
type StatUse struct {
	Stat  string
	Scope resolve.Scope
	// Uses counts the bundles (nodes, mastery effects, node buffs, characters, levels, quest
	// rewards) that give the stat.
	Uses int
	// Feeds is the caller's verdict on whether the stat feeds the values it asks about.
	Feeds bool
	// Entry is the table entry for the dataset's version, or nil.
	Entry *Entry
}

// Gap reports whether the stat feeds the values asked about and has no entry.
func (u *StatUse) Gap() bool { return u.Feeds && u.Entry == nil }

// Coverage is how far a table covers the stats a dataset version's content gives. Stats are
// sorted by uses, most first, then by stat and scope.
type Coverage struct {
	Game, Version, Variant string
	Stats                  []StatUse
}

// Summary counts the stats of a Coverage by entry status, and those without an entry by
// whether they feed.
type Summary struct {
	Authored, Unknown, DisplayOnly int
	// Gaps feed and have no entry; Outside neither feed nor have one.
	Gaps, Outside int
}

// Cover computes the coverage of the stats the dataset in s gives as resolve emits them: the
// bundles of every node of the tree variant, with mastery effects, every character, the level
// and the quest rewards of a completed campaign. feeds says which stats the caller needs
// meanings for.
func Cover(s *dataset.Store, variant string, t *Table, feeds func(stat string, scope resolve.Scope) bool) (*Coverage, error) {
	m := s.Manifest()
	version, err := dataset.ParseGameVersion(m.Version)
	if err != nil {
		return nil, fmt.Errorf("semantics: dataset %w", err)
	}
	uses, err := statUses(s, variant)
	if err != nil {
		return nil, err
	}
	c := &Coverage{Game: m.Game, Version: m.Version, Variant: variant, Stats: make([]StatUse, 0, len(uses))}
	for k, n := range uses {
		u := StatUse{Stat: k.stat, Scope: k.scope, Uses: n, Feeds: feeds(k.stat, k.scope)}
		if e, ok := t.Lookup(k.stat, k.scope, version); ok {
			u.Entry = e
		}
		c.Stats = append(c.Stats, u)
	}
	slices.SortFunc(c.Stats, func(a, b StatUse) int {
		return cmp.Or(cmp.Compare(b.Uses, a.Uses), cmp.Compare(a.Stat, b.Stat), cmp.Compare(a.Scope, b.Scope))
	})
	return c, nil
}

type useKey struct {
	stat  string
	scope resolve.Scope
}

// statUses counts how often each stat appears in the bundles Cover walks.
func statUses(s *dataset.Store, variant string) (map[useKey]int, error) {
	chars, err := s.Characters()
	if err != nil {
		return nil, fmt.Errorf("semantics: load characters: %w", err)
	}
	tree, err := s.Tree(variant)
	if err != nil {
		return nil, fmt.Errorf("semantics: load tree: %w", err)
	}
	bundles, err := treeBundles(s, variant, tree)
	if err != nil {
		return nil, err
	}
	uses := map[useKey]int{}
	add := func(scope resolve.Scope, svs []resolve.StatValue) {
		for _, sv := range svs {
			uses[useKey{sv.Stat, scope}]++
		}
	}
	for i := range chars {
		cs, err := resolve.CharacterStats(s, chars[i].ID)
		if err != nil {
			return nil, fmt.Errorf("semantics: character stats: %w", err)
		}
		add(resolve.Global, cs)
		add(resolve.Global, []resolve.StatValue{{Stat: resolve.LevelStat}})
	}
	for _, b := range bundles {
		add(b.Scope, b.Stats)
	}
	return uses, nil
}

// treeBundles returns every quest reward, then the bundles of every node of tree and its
// mastery effects.
func treeBundles(s *dataset.Store, variant string, tree *dataset.Tree) ([]resolve.Bundle, error) {
	bundles, err := resolve.QuestBundles(s)
	if err != nil {
		return nil, fmt.Errorf("semantics: quest bundles: %w", err)
	}
	for i := range tree.Nodes {
		n := &tree.Nodes[i]
		nb, err := resolve.NodeBundles(s, variant, n.ID)
		if err != nil {
			return nil, fmt.Errorf("semantics: node bundles: %w", err)
		}
		bundles = append(bundles, nb...)
		for _, me := range n.MasteryEffects {
			b, err := resolve.MasteryEffectBundle(s, variant, n.ID, me.ID)
			if err != nil {
				return nil, fmt.Errorf("semantics: mastery bundle: %w", err)
			}
			bundles = append(bundles, b)
		}
	}
	return bundles, nil
}

// Summary counts c's stats.
func (c *Coverage) Summary() Summary {
	var s Summary
	for i := range c.Stats {
		u := &c.Stats[i]
		switch {
		case u.Entry != nil && u.Entry.Status == Authored:
			s.Authored++
		case u.Entry != nil && u.Entry.Status == Unknown:
			s.Unknown++
		case u.Entry != nil:
			s.DisplayOnly++
		case u.Feeds:
			s.Gaps++
		default:
			s.Outside++
		}
	}
	return s
}

// WriteText writes c as text: a summary line, the unknown entries counted by reason kind,
// then, in c's order, one line per gap, unknown entry, and unvalidated or omitted note of an
// authored entry.
func (c *Coverage) WriteText(w io.Writer) error {
	var b strings.Builder
	s := c.Summary()
	fmt.Fprintf(&b, "coverage %s %s %s tree and characters: %d stats; %d authored, %d unknown, %d display-only, %d gaps, %d outside\n",
		c.Game, c.Version, c.Variant, len(c.Stats), s.Authored, s.Unknown, s.DisplayOnly, s.Gaps, s.Outside)
	writeReasonCounts(&b, c.Stats)
	for i := range c.Stats {
		writeStatUse(&b, &c.Stats[i])
	}
	if _, err := io.WriteString(w, b.String()); err != nil {
		return fmt.Errorf("semantics: write: %w", err)
	}
	return nil
}

func writeReasonCounts(b *strings.Builder, stats []StatUse) {
	kinds := map[ReasonKind]int{}
	for i := range stats {
		if e := stats[i].Entry; e != nil && e.Status == Unknown {
			kinds[e.Reason.Kind]++
		}
	}
	if len(kinds) == 0 {
		return
	}
	order := slices.SortedFunc(maps.Keys(kinds), func(a, b ReasonKind) int { return cmp.Or(cmp.Compare(kinds[b], kinds[a]), cmp.Compare(a, b)) })
	counts := make([]string, len(order))
	for i, k := range order {
		counts[i] = fmt.Sprintf("%s %d", k, kinds[k])
	}
	fmt.Fprintf(b, "unknown by reason: %s\n", strings.Join(counts, ", "))
}

func writeStatUse(b *strings.Builder, u *StatUse) {
	e := u.Entry
	switch {
	case u.Gap():
		fmt.Fprintf(b, "gap %s %s, uses %d\n", u.Stat, u.Scope, u.Uses)
	case e == nil:
	case e.Status == Unknown && e.Rule != "":
		fmt.Fprintf(b, "unknown %s %s, uses %d: %s %s: %s\n", u.Stat, u.Scope, u.Uses, e.Reason.Kind, e.Rule, e.Reason.Text)
	case e.Status == Unknown:
		fmt.Fprintf(b, "unknown %s %s, uses %d: %s\n", u.Stat, u.Scope, u.Uses, e.Reason)
	case e.Status == Authored:
		if e.Unvalidated != "" {
			fmt.Fprintf(b, "unvalidated %s %s, uses %d: %s\n", u.Stat, u.Scope, u.Uses, e.Unvalidated)
		}
		if e.Omitted != "" {
			fmt.Fprintf(b, "omitted %s %s, uses %d: %s\n", u.Stat, u.Scope, u.Uses, e.Omitted)
		}
	}
}
