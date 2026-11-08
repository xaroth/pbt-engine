package dataset

import (
	"cmp"
	"errors"
	"fmt"
	"io/fs"
	"slices"
	"sync"
)

// ErrNoVersion is returned by Resolve when no dataset matches.
var ErrNoVersion = errors.New("no matching dataset")

// Registry holds opened datasets of several games and versions side by side. The zero value
// is an empty registry. It is safe for concurrent use.
type Registry struct {
	mu      sync.RWMutex
	entries []entry // sorted by game, then version
}

type entry struct {
	game    string
	version GameVersion
	store   *Store
}

func compareEntries(a, b entry) int {
	return cmp.Or(cmp.Compare(a.game, b.game), a.version.Compare(b.version))
}

// OpenRoot opens every dataset in a direct subdirectory of root that holds a manifest. Other
// subdirectories are skipped. A dataset that fails to open or add, such as one in an older
// format, is skipped too, so one bad directory does not hide the others: OpenRoot then returns
// the registry of the rest together with the joined errors of the skipped ones, each prefixed
// with its directory. The registry is nil only when root cannot be read.
func OpenRoot(root fs.FS) (*Registry, error) {
	ents, err := fs.ReadDir(root, ".")
	if err != nil {
		return nil, fmt.Errorf("read root: %w", err)
	}
	r := &Registry{}
	var skipped []error
	for _, e := range ents {
		if !e.IsDir() {
			continue
		}
		sub, err := fs.Sub(root, e.Name())
		if err != nil {
			return nil, fmt.Errorf("open %s: %w", e.Name(), err)
		}
		if _, err := fs.Stat(sub, ManifestName); errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if _, err := r.Add(sub); err != nil {
			skipped = append(skipped, fmt.Errorf("%s: %w", e.Name(), err))
		}
	}
	return r, errors.Join(skipped...)
}

// Add opens the dataset in fsys and adds it. It fails when Open does, the manifest has no game
// or a version that is not numbers separated by dots, or the registry holds that game and
// version already.
func (r *Registry) Add(fsys fs.FS) (*Store, error) {
	s, err := Open(fsys)
	if err != nil {
		return nil, err
	}
	m := s.Manifest()
	if m.Game == "" {
		return nil, fmt.Errorf("%s: game is required: %w", ManifestName, ErrInvalid)
	}
	parsed, err := ParseGameVersion(m.Version)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", ManifestName, err)
	}
	e := entry{m.Game, parsed, s}
	r.mu.Lock()
	defer r.mu.Unlock()
	i, found := slices.BinarySearchFunc(r.entries, e, compareEntries)
	if found {
		return nil, fmt.Errorf("dataset %s %s %w", m.Game, m.Version, ErrDuplicate)
	}
	r.entries = slices.Insert(r.entries, i, e)
	return s, nil
}

// Stores returns the datasets of a game, oldest version first.
func (r *Registry) Stores(game string) []*Store {
	entries := r.game(game)
	out := make([]*Store, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.store)
	}
	return out
}

// Match says how Resolve chose a version. The zero value is no match, as in a Resolution
// returned with an error.
type Match uint8

// Matches.
const (
	// MatchExact is the version asked for.
	MatchExact Match = iota + 1
	// MatchNewest is the newest version of all ("" or "latest") or under a prefix ("3.29").
	MatchNewest
	// MatchEarlierPatch is a fallback: the version asked for has no dataset, and this is the
	// newest earlier patch of the same league.
	MatchEarlierPatch
	// MatchLaterPatch is a fallback: the version asked for has no dataset and no earlier patch
	// of its league does, and this is the newest patch of the same league.
	MatchLaterPatch
)

// Resolution is the result of Resolve.
type Resolution struct {
	Store *Store
	// Requested is the version string Resolve was given.
	Requested string
	Match     Match
}

// Resolve finds the dataset of a game for a requested version:
//
//   - "" or "latest": the newest version;
//   - an exact version, such as "3.29.3.3";
//   - a prefix of whole components, such as "3.29": the newest version under it;
//   - otherwise a patch of the same league (first two components): the newest earlier one, so
//     a patch without its own dataset uses the last one before it, else the newest of the
//     league. Leagues never fall back to each other, since mechanics change between them.
//
// It returns ErrNoVersion when nothing matches.
func (r *Registry) Resolve(game, version string) (Resolution, error) {
	res := Resolution{Requested: version}
	es := r.game(game)
	if len(es) == 0 {
		return res, fmt.Errorf("%w: no %s datasets", ErrNoVersion, game)
	}
	if version == "" || version == "latest" {
		res.Store, res.Match = es[len(es)-1].store, MatchNewest
		return res, nil
	}
	want, err := ParseGameVersion(version)
	if err != nil {
		return res, err
	}
	for _, e := range slices.Backward(es) {
		switch {
		case e.version.Compare(want) == 0:
			res.Store, res.Match = e.store, MatchExact
			return res, nil
		case e.version.HasPrefix(want):
			if res.Store == nil {
				res.Store, res.Match = e.store, MatchNewest
			}
		}
	}
	if res.Store != nil {
		return res, nil
	}
	if want.len() > leagueParts {
		var later *Store
		for _, e := range slices.Backward(es) {
			if e.version.len() <= leagueParts || e.version.League().Compare(want.League()) != 0 {
				continue
			}
			if e.version.Compare(want) < 0 {
				res.Store, res.Match = e.store, MatchEarlierPatch
				return res, nil
			}
			if later == nil {
				later = e.store
			}
		}
		if later != nil {
			res.Store, res.Match = later, MatchLaterPatch
			return res, nil
		}
	}
	return res, fmt.Errorf("%w: %s %s", ErrNoVersion, game, version)
}

func (r *Registry) game(game string) []entry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []entry
	for _, e := range r.entries {
		if e.game == game {
			out = append(out, e)
		}
	}
	return out
}
