package timeless

import (
	"fmt"

	"github.com/xaroth/pbt-engine/dataset"
)

// The stats of a timeless jewel's mod.
const (
	VersionStat  = "local_unique_jewel_alternate_tree_version"
	SeedStat     = "local_unique_jewel_alternate_tree_seed"
	KeystoneStat = "local_unique_jewel_alternate_tree_keystone"
	RevisionStat = "local_unique_jewel_alternate_tree_internal_revision"
	eternalID    = "Eternal"
)

// ModJewel is a timeless jewel as its mod's stats give it.
type ModJewel struct {
	// Version is the alternate tree version's row, its index in Tree.TimelessVersions.
	Version int
	// Seed is the seed stat: the seed drawn from, which Elegant Hubris shows 20 times larger.
	Seed      uint32
	Conqueror Conqueror
}

// JewelOf reads a timeless jewel from its mod's stats by id, or reports false when they do not
// name one. A jewel without the revision stat is of a conqueror's first revision.
func JewelOf(stats map[string]int32) (ModJewel, bool) {
	version, okVersion := stats[VersionStat]
	seed, okSeed := stats[SeedStat]
	keystone, okKeystone := stats[KeystoneStat]
	revision := stats[RevisionStat]
	if !okVersion || !okSeed || !okKeystone || version < 0 || seed < 0 || keystone < 0 || revision < 0 {
		return ModJewel{}, false
	}
	return ModJewel{
		Version: int(version), Seed: uint32(seed),
		Conqueror: Conqueror{Index: uint32(keystone), Version: uint32(revision)},
	}, true
}

// Change is what a jewel makes of a passive, in the dataset's stats.
type Change struct {
	// Replace makes Stats take the place of the passive's stats; otherwise they add to them.
	Replace bool
	Stats   []dataset.StatValue
}

// Versions are the alternate tree versions of a tree variant, ready to conquer its passives.
type Versions struct {
	rows  []dataset.TimelessVersion
	trees []*Tree
}

// VersionsOf returns the alternate tree versions of the tree variant of s, made once per store.
func VersionsOf(s *dataset.Store, variant string) (*Versions, error) {
	//nolint:wrapcheck // the build function wraps its errors
	return dataset.Derived(s, versionsKey{variant}, func(s *dataset.Store) (*Versions, error) {
		tree, err := s.Tree(variant)
		if err != nil {
			return nil, fmt.Errorf("timeless: %w", err)
		}
		v := &Versions{rows: tree.TimelessVersions, trees: make([]*Tree, len(tree.TimelessVersions))}
		for i := range v.rows {
			if v.trees[i], err = NewTree(TablesOf(&v.rows[i])); err != nil {
				return nil, fmt.Errorf("timeless: version %s: %w", v.rows[i].ID, err)
			}
		}
		return v, nil
	})
}

// versionsKey keys the versions of a tree variant among a store's derived values.
type versionsKey struct{ variant string }

// Name returns the id of version, such as "Vaal", or "" when there is no such version.
func (v *Versions) Name(version int) string {
	if version < 0 || version >= len(v.rows) {
		return ""
	}
	return v.rows[version].ID
}

// ChangesPassives reports whether the jewels of version change passives other than keystones
// here. Only Elegant Hubris's do so far; the others' replacements and additions need rules of
// their own.
func (v *Versions) ChangesPassives(version int) bool { return v.Name(version) == eternalID }

// Conquer gives what j makes of passive n, or reports false when it leaves n as it is: an
// unknown version, a passive it does not change, or a kind ChangesPassives leaves out.
func (v *Versions) Conquer(j ModJewel, n Node) (Change, bool) {
	if v.Name(j.Version) == "" || (n.Kind != Keystone && !v.ChangesPassives(j.Version)) {
		return Change{}, false
	}
	rows, seed := &v.rows[j.Version], j.Seed
	if rows.ID == eternalID {
		seed *= eternalSeedStep
	}
	res := v.trees[j.Version].Conquer(Jewel{Seed: seed, Conqueror: j.Conqueror}, n)
	var out Change
	if res.Replaced {
		out.Replace = true
		for s, r := range rows.Replacements[res.Replacement].Stats {
			out.Stats = append(out.Stats, dataset.StatValue{Stat: r.Stat, Value: res.Rolls[s]})
		}
	}
	for _, a := range res.Additions() {
		for s, r := range rows.Additions[a.Addition].Stats {
			out.Stats = append(out.Stats, dataset.StatValue{Stat: r.Stat, Value: a.Rolls[s]})
		}
	}
	return out, out.Replace || len(out.Stats) > 0
}

// TablesOf gives a dataset's alternate tree version as tables.
func TablesOf(v *dataset.TimelessVersion) Tables {
	t := Tables{Version: Version{
		ID:                       v.ID,
		SmallAttributeReplaced:   v.SmallAttributeReplaced,
		SmallNormalReplaced:      v.SmallNormalReplaced,
		SmallAttributeAdditions:  Range(v.SmallAttributeAdditions),
		SmallNormalAdditions:     Range(v.SmallNormalAdditions),
		NotableAdditions:         Range(v.NotableAdditions),
		NotableReplacementChance: v.NotableReplacementChance,
	}}
	for _, r := range v.Replacements {
		t.Replacements = append(t.Replacements, Replacement{
			Kinds: kinds(r.Kinds), Stats: ranges(r.Stats), Weight: r.Weight,
			Additions: Range(r.Additions), Conqueror: Conqueror(r.Conqueror),
		})
	}
	for _, a := range v.Additions {
		t.Additions = append(t.Additions, Addition{Kinds: kinds(a.Kinds), Stats: ranges(a.Stats), Weight: a.Weight})
	}
	return t
}

func kinds(ks []dataset.TimelessKind) []Kind {
	out := make([]Kind, 0, len(ks))
	for _, k := range ks {
		out = append(out, Kind(k))
	}
	return out
}

func ranges(rs []dataset.StatRange) []Range {
	out := make([]Range, 0, len(rs))
	for _, r := range rs {
		out = append(out, Range{Min: r.Min, Max: r.Max})
	}
	return out
}
