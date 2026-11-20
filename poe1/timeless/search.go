package timeless

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/internal/poe1/radius"
	rules "github.com/xaroth/pbt-engine/internal/poe1/timeless"
)

// conquerRadius is the radius every timeless jewel that conquers passives reaches.
const conquerRadius = "Large"

// Errors of a Reader's calls, besides dataset.ErrNotInDataset and ErrNotSocket.
var (
	// ErrSeed is a seed a jewel of the version cannot show.
	ErrSeed = errors.New("timeless: no such seed")
	// ErrWant is a wanted result the version never gives, or one that names neither a passive
	// nor a stat.
	ErrWant = errors.New("timeless: unknown wanted result")
)

// ErrNotSocket is a node that is not a jewel socket on the tree.
var ErrNotSocket = radius.ErrNotSocket

// Reader conquers the passives around a socket with a timeless jewel, and searches the seeds
// of a version for wanted results, on one dataset. What it derives from the dataset is made on
// first use and kept with the store. A Reader is safe for concurrent use.
type Reader struct {
	store *dataset.Store
}

// New returns a Reader of s.
func New(s *dataset.Store) *Reader {
	return &Reader{store: s}
}

// SeedRange is the seeds a jewel shows: First to Last, Step apart.
type SeedRange = rules.SeedRange

// JewelVersion is an alternate tree version a timeless jewel has.
type JewelVersion struct {
	// Index is the version's number, as a jewel's version stat gives it.
	Index int `json:"index"`
	// ID is the version's id, such as "Eternal" for Elegant Hubris.
	ID    string    `json:"id"`
	Seeds SeedRange `json:"seeds"`
}

// Versions returns the alternate tree versions of the tree variant, such as
// build.DefaultVariant, that a jewel in a socket can have, with their seed ranges as the jewel
// mods give them.
func (r *Reader) Versions(variant string) ([]JewelVersion, error) {
	versions, seeds, err := r.versions(variant)
	if err != nil {
		return nil, err
	}
	var out []JewelVersion
	for i, s := range seeds {
		if s.Step > 0 {
			out = append(out, JewelVersion{Index: i, ID: versions.Name(i), Seeds: s})
		}
	}
	return out, nil
}

// Socketed is a timeless jewel in a socket.
type Socketed struct {
	Socket uint32 `json:"socket"`
	// Version is JewelVersion.Index.
	Version int `json:"version"`
	// Seed is the seed the jewel shows, such as 67340 on an Elegant Hubris.
	Seed      uint32    `json:"seed"`
	Conqueror Conqueror `json:"conqueror"`
}

// Passive is what a jewel makes of a passive in its radius.
type Passive struct {
	Node uint32 `json:"node"`
	// Replaced makes Stats take the place of the passive's stats; otherwise they add to them.
	Replaced bool `json:"replaced"`
	// Name is the replacing passive's name; empty when not replaced or the language has none.
	Name string `json:"name"`
	// Stats are the stats the jewel gives the passive, in the order it rolls them; empty when
	// it leaves the passive as it is.
	Stats []Stat `json:"stats"`
}

// Stat is a stat by its id with a value.
type Stat struct {
	ID    string `json:"id"`
	Value int32  `json:"value"`
}

// Conquer returns what jewel j makes of each passive in its radius, by node id, on the tree
// variant, naming replacements in language, such as "en". An unknown version returns an error
// matching dataset.ErrNotInDataset, a seed the version's jewel cannot show ErrSeed, and a socket
// that is not one ErrNotSocket.
func (r *Reader) Conquer(variant string, j Socketed, language string) ([]Passive, error) {
	s, err := r.socket(variant, j.Socket, j.Version, language)
	if err != nil {
		return nil, err
	}
	if !s.seeds.Contains(j.Seed) {
		return nil, fmt.Errorf("%w: %d for version %s, which shows %d to %d", ErrSeed, j.Seed, s.versions.Name(j.Version), s.seeds.First, s.seeds.Last)
	}
	return s.conquer(j.Version, j.Seed, j.Conqueror), nil
}

// Query is what a search looks for: seeds of a version whose jewel, in a socket, gives the
// wanted results.
type Query struct {
	Socket uint32 `json:"socket"`
	// Version is JewelVersion.Index.
	Version int    `json:"version"`
	Wants   []Want `json:"wants"`
	// Allocated, when not nil, limits the search to these passives of the radius; nil searches
	// every passive in it.
	Allocated []uint32 `json:"allocated"`
	// Language names the passives Wants and the matches name, such as "en".
	Language string `json:"language"`
	// Limit bounds the matches returned; 0 returns every match.
	Limit int `json:"limit"`
}

// Want is a result a search looks for, totalled over the passives searched: with Name, the
// number of passives replaced by the passive of that name; with Stat, the sum of that stat's
// values the jewel gives them.
type Want struct {
	Name string `json:"name"`
	// Stat is a stat id, such as "base_maximum_life".
	Stat string `json:"stat"`
	// Weight scales the total into a seed's score.
	Weight float64 `json:"weight"`
	// Min is the least total a seed must reach.
	Min int64 `json:"min"`
}

// Match is a seed whose jewel gives every wanted result.
type Match struct {
	Seed uint32 `json:"seed"`
	// Score is the sum of each want's total times its weight.
	Score float64 `json:"score"`
	// Totals holds each want's total, in the query's order.
	Totals []int64 `json:"totals"`
	// Passives is what the jewel makes of each passive searched, by node id. A keystone, which
	// only the jewel's conqueror changes, is left out.
	Passives []Passive `json:"passives"`
}

// Progress reports a search under way: the seeds scanned, the seeds in all and the matches
// found so far, before the query's limit is applied.
type Progress = rules.Progress

// Search scans every seed of the query's version in its socket and returns the seeds whose
// totals reach every want's Min, by score from highest, then by seed. Keystones are not
// searched: the conqueror, not the seed, picks theirs. Errors are those of Conquer, and ErrWant
// for a want the version never gives.
//
// A not nil progress is called every few hundred seeds and once after the last, from the
// goroutine that called Search. Once ctx is done, Search returns ctx.Err() and no matches
// within a seed's time.
func (r *Reader) Search(ctx context.Context, variant string, q Query, progress Progress) ([]Match, error) {
	s, err := r.socket(variant, q.Socket, q.Version, q.Language)
	if err != nil {
		return nil, err
	}
	wants, err := s.wants(q.Version, q.Wants)
	if err != nil {
		return nil, err
	}
	if q.Allocated != nil {
		s.nodes = slices.DeleteFunc(s.nodes, func(n rules.Node) bool { return !slices.Contains(q.Allocated, n.ID) })
	}
	s.nodes = slices.DeleteFunc(s.nodes, func(n rules.Node) bool { return n.Kind == Keystone })
	found, err := s.versions.Search(ctx, q.Version, s.seeds, s.nodes, wants, q.Limit, progress)
	if err != nil {
		return nil, err //nolint:wrapcheck // the caller's own error
	}
	out := make([]Match, len(found))
	for i, m := range found {
		out[i] = Match{Seed: m.Seed, Score: m.Score, Totals: m.Totals, Passives: s.conquer(q.Version, m.Seed, Conqueror{})}
	}
	return out, nil
}

// socket is what a call reads for one socket and version.
type socket struct {
	versions *rules.Versions
	seeds    SeedRange
	nodes    []rules.Node
	stats    []dataset.Stat
	tree     *dataset.Tree
	names    map[string]string
}

func (r *Reader) versions(variant string) (*rules.Versions, []SeedRange, error) {
	versions, err := rules.VersionsOf(r.store, variant)
	if err != nil {
		return nil, nil, err //nolint:wrapcheck // names the package already
	}
	seeds, err := rules.Seeds(r.store, variant)
	if err != nil {
		return nil, nil, err //nolint:wrapcheck // names the package already
	}
	return versions, seeds, nil
}

func (r *Reader) socket(variant string, id uint32, version int, language string) (*socket, error) {
	versions, seeds, err := r.versions(variant)
	if err != nil {
		return nil, err
	}
	if version < 0 || version >= len(seeds) || seeds[version].Step == 0 {
		return nil, fmt.Errorf("timeless: version %d of the %s tree: %w", version, variant, dataset.ErrNotInDataset)
	}
	s := &socket{versions: versions, seeds: seeds[version]}
	if s.tree, err = r.store.Tree(variant); err != nil {
		return nil, fmt.Errorf("timeless: %w", err)
	}
	if s.stats, err = r.store.Stats(); err != nil {
		return nil, fmt.Errorf("timeless: %w", err)
	}
	if s.names, err = r.names(language); err != nil {
		return nil, err
	}
	ids, err := r.radius(variant, s.tree, id)
	if err != nil {
		return nil, err
	}
	s.nodes = nodesOf(s.tree, s.stats, ids)
	return s, nil
}

// nodesOf gives the passives ids with their kinds.
func nodesOf(tree *dataset.Tree, stats []dataset.Stat, ids []uint32) []rules.Node {
	out := make([]rules.Node, 0, len(ids))
	for _, id := range ids {
		i, ok := slices.BinarySearchFunc(tree.Nodes, id, func(n dataset.Node, id uint32) int { return cmp.Compare(n.ID, id) })
		if ok {
			out = append(out, rules.Node{ID: id, Kind: rules.KindOf(&tree.Nodes[i], stats)})
		}
	}
	return out
}

func (r *Reader) radius(variant string, tree *dataset.Tree, socket uint32) ([]uint32, error) {
	i := slices.IndexFunc(tree.JewelRadii, func(jr dataset.JewelRadius) bool { return jr.ID == conquerRadius })
	if i < 0 {
		return nil, fmt.Errorf("timeless: jewel radius %q in the %s tree: %w", conquerRadius, variant, dataset.ErrNotInDataset)
	}
	ix, err := radius.Of(r.store, variant)
	if err != nil {
		return nil, fmt.Errorf("timeless: %w", err)
	}
	ids, err := ix.Nodes(socket, radius.Disc(tree.JewelRadii[i]))
	if err != nil {
		return nil, fmt.Errorf("timeless: %w", err)
	}
	return ids, nil
}

// names returns the replacement names of language by row id.
func (r *Reader) names(language string) (map[string]string, error) {
	//nolint:wrapcheck // the build function wraps its errors
	return dataset.Derived(r.store, namesKey{language}, func(s *dataset.Store) (map[string]string, error) {
		names, err := s.Names(language)
		if err != nil {
			return nil, fmt.Errorf("timeless: %w", err)
		}
		out := make(map[string]string, len(names.TimelessNames))
		for _, n := range names.TimelessNames {
			out[n.ID] = n.Name
		}
		return out, nil
	})
}

// namesKey keys the replacement names of a language among a store's derived values.
type namesKey struct{ language string }

// conquer gives what the jewel of version with seed makes of each of the socket's passives.
func (s *socket) conquer(version int, seed uint32, c Conqueror) []Passive {
	// The mod's seed stat is the shown seed over the step: Elegant Hubris shows 20 times it.
	j := rules.ModJewel{Version: version, Seed: seed / s.seeds.Step, Conqueror: c}
	out := make([]Passive, 0, len(s.nodes))
	for _, n := range s.nodes {
		p := Passive{Node: n.ID, Stats: []Stat{}}
		if ch, ok := s.versions.Conquer(j, n); ok {
			p.Replaced, p.Name = ch.Replace, s.names[ch.ReplacementID]
			for _, sv := range ch.Stats {
				p.Stats = append(p.Stats, Stat{ID: s.stats[sv.Stat].ID, Value: sv.Value})
			}
		}
		out = append(out, p)
	}
	return out
}

// wants resolves the query's wants to the version's rows: names by display name, since some
// row ids do not match their stats, and stats by id.
func (s *socket) wants(version int, in []Want) ([]rules.Want, error) {
	rows := s.versions.Rows(version)
	out := make([]rules.Want, 0, len(in))
	for _, w := range in {
		rw := rules.Want{Weight: w.Weight, Min: w.Min}
		var err error
		switch {
		case w.Name != "" && w.Stat == "":
			rw.Replacements, err = s.named(rows, w.Name)
		case w.Stat != "" && w.Name == "":
			rw.Stat, err = s.stat(rows, w.Stat)
		default:
			err = fmt.Errorf("%w: a want names one passive or one stat", ErrWant)
		}
		if err != nil {
			return nil, err
		}
		out = append(out, rw)
	}
	return out, nil
}

// named returns the rows of v, keystones left out, whose name is name.
func (s *socket) named(v *dataset.TimelessVersion, name string) ([]int, error) {
	var out []int
	for i, row := range v.Replacements {
		if !keystone(&row) && s.names[row.ID] == name {
			out = append(out, i)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%w: version %s has no passive named %q", ErrWant, v.ID, name)
	}
	return out, nil
}

// stat returns the handle of stat id, which a row of v must give.
func (s *socket) stat(v *dataset.TimelessVersion, id string) (uint32, error) {
	for i, st := range s.stats {
		if st.ID == id && gives(v, uint32(i)) {
			return uint32(i), nil
		}
	}
	return 0, fmt.Errorf("%w: version %s never gives stat %q", ErrWant, v.ID, id)
}

// gives tells whether a row of v has stat.
func gives(v *dataset.TimelessVersion, stat uint32) bool {
	has := func(rs []dataset.StatRange) bool {
		return slices.ContainsFunc(rs, func(r dataset.StatRange) bool { return r.Stat == stat })
	}
	return slices.ContainsFunc(v.Replacements, func(r dataset.TimelessReplacement) bool {
		return !keystone(&r) && has(r.Stats)
	}) || slices.ContainsFunc(v.Additions, func(a dataset.TimelessAddition) bool { return has(a.Stats) })
}

// keystone tells whether r replaces keystones, which a search leaves out.
func keystone(r *dataset.TimelessReplacement) bool {
	return slices.Contains(r.Kinds, dataset.TimelessKeystone)
}
