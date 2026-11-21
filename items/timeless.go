package items

import (
	"errors"
	"fmt"
	"slices"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/statdesc"
)

// The stat description handlers of a timeless jewel's mod that show its faction, by version,
// and its conqueror, by keystone, version and revision.
const (
	versionHandler   = "alternate_tree_version_name"
	conquerorHandler = "alternate_tree_version_keystone"
)

// timelessNames returns the options that name the factions and conquerors of the timeless
// versions of s's standard tree with the names of text; none when text or s has none of them.
func timelessNames(s *dataset.Store, text *dataset.Text) ([]statdesc.Option, error) {
	if len(text.TimelessVersionNames) == 0 && len(text.ConquerorNames) == 0 {
		return nil, nil
	}
	t, err := s.Tree(build.DefaultVariant)
	if errors.Is(err, dataset.ErrNotInDataset) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("items: %w", err)
	}
	factions := byID(text.TimelessVersionNames)
	versions := map[int32]string{}
	for i, v := range t.TimelessVersions {
		if name, ok := factions[v.ID]; ok {
			versions[int32(i)] = name
		}
	}
	return []statdesc.Option{
		statdesc.WithNames(versionHandler, versions),
		statdesc.WithNamer(conquerorHandler, newConquerors(t.TimelessVersions, byID(text.ConquerorNames))),
	}, nil
}

func byID(names []dataset.Name) map[string]string {
	out := make(map[string]string, len(names))
	for _, n := range names {
		out[n.ID] = n.Name
	}
	return out
}

// conquerorKey is a conqueror's keystone, version and revision.
type conquerorKey [3]int32

// conquerors names a conqueror by its conquerorKey, and reads a name back to the lowest key
// of that name.
type conquerors struct {
	byKey  map[conquerorKey]string
	byName map[string]conquerorKey
}

// newConquerors names the conqueror of each keystone of versions that names, by id, names.
func newConquerors(versions []dataset.TimelessVersion, names map[string]string) conquerors {
	c := conquerors{byKey: map[conquerorKey]string{}, byName: map[string]conquerorKey{}}
	for i, v := range versions {
		for _, r := range v.Replacements {
			name, ok := names[r.ID]
			if !ok || r.Conqueror == (dataset.TimelessConqueror{}) {
				continue
			}
			k := conquerorKey{int32(r.Conqueror.Index), int32(i), int32(r.Conqueror.Version)} //nolint:gosec // small game indices
			c.byKey[k] = name
			if old, seen := c.byName[name]; !seen || slices.Compare(k[:], old[:]) < 0 {
				c.byName[name] = k
			}
		}
	}
	return c
}

// Name returns the conqueror of the revision, or else of the latest revision before it, as the
// game's descriptions name a conqueror of a later revision that has none of its own.
func (c conquerors) Name(values []int32) (string, bool) {
	if len(values) != len(conquerorKey{}) {
		return "", false
	}
	name, best := "", int32(-1)
	for k, n := range c.byKey {
		if k[0] == values[0] && k[1] == values[1] && k[2] <= values[2] && k[2] > best {
			name, best = n, k[2]
		}
	}
	return name, best >= 0
}

func (c conquerors) Values(name string) ([]int32, bool) {
	k, ok := c.byName[name]
	return k[:], ok
}
