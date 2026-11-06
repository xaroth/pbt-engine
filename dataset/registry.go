package dataset

import (
	"cmp"
	"errors"
	"fmt"
	"io/fs"
	"slices"
	"strconv"
	"strings"
	"sync"
)

// ErrNoVersion is returned by Resolve when no dataset matches.
var ErrNoVersion = errors.New("no matching dataset")

// Registry holds opened datasets of several games and versions side by side. It is safe for
// concurrent use.
type Registry struct {
	mu       sync.RWMutex
	versions []*Version // sorted by game, then version
}

// OpenRoot opens every dataset in a direct subdirectory of root that holds a manifest.
// Other subdirectories are skipped.
func OpenRoot(root fs.FS) (*Registry, error) {
	ents, err := fs.ReadDir(root, ".")
	if err != nil {
		return nil, err
	}
	r := &Registry{}
	for _, e := range ents {
		if !e.IsDir() {
			continue
		}
		sub, err := fs.Sub(root, e.Name())
		if err != nil {
			return nil, err
		}
		if _, err := fs.Stat(sub, ManifestName); errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if _, err := r.Add(sub); err != nil {
			return nil, fmt.Errorf("%s: %w", e.Name(), err)
		}
	}
	return r, nil
}

// Add opens the dataset in fsys and adds it. A second dataset of the same game and version is
// an error.
func (r *Registry) Add(fsys fs.FS) (*Version, error) {
	v, err := Open(fsys)
	if err != nil {
		return nil, err
	}
	if v.Game() == "" || v.Name() == "" {
		return nil, fmt.Errorf("%s: game and version are required", ManifestName)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	i, found := slices.BinarySearchFunc(r.versions, v, compareVersions)
	if found {
		return nil, fmt.Errorf("dataset %s %s added twice", v.Game(), v.Name())
	}
	r.versions = slices.Insert(r.versions, i, v)
	return v, nil
}

// Versions returns the datasets of a game, oldest first.
func (r *Registry) Versions(game string) []*Version {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []*Version
	for _, v := range r.versions {
		if v.Game() == game {
			out = append(out, v)
		}
	}
	return out
}

// Resolve finds the dataset of a game for a requested version:
//
//   - "" or "latest": the newest version;
//   - an exact version, such as "3.29.3.3";
//   - a prefix of whole components, such as "3.29": the newest version under it;
//   - otherwise the newest older version of the same league (first two components), so a
//     patch without its own dataset uses the last one before it. Leagues never fall back to
//     each other, since mechanics change between them.
func (r *Registry) Resolve(game, version string) (*Version, error) {
	vs := r.Versions(game)
	if len(vs) == 0 {
		return nil, fmt.Errorf("%w: no %s datasets", ErrNoVersion, game)
	}
	if version == "" || version == "latest" {
		return vs[len(vs)-1], nil
	}
	want := splitVersion(version)
	for i := len(vs) - 1; i >= 0; i-- {
		got := splitVersion(vs[i].Name())
		if len(got) >= len(want) && slices.Equal(got[:len(want)], want) {
			return vs[i], nil
		}
	}
	if len(want) > 2 {
		for i := len(vs) - 1; i >= 0; i-- {
			got := splitVersion(vs[i].Name())
			if len(got) > 2 && slices.Equal(got[:2], want[:2]) && compareParts(got, want) < 0 {
				return vs[i], nil
			}
		}
	}
	return nil, fmt.Errorf("%w: %s %s", ErrNoVersion, game, version)
}

func splitVersion(s string) []string { return strings.Split(s, ".") }

func compareVersions(a, b *Version) int {
	return cmp.Or(cmp.Compare(a.Game(), b.Game()), compareParts(splitVersion(a.Name()), splitVersion(b.Name())))
}

// compareParts orders versions component by component, numerically where both are numbers.
func compareParts(a, b []string) int {
	for i := range min(len(a), len(b)) {
		x, errX := strconv.Atoi(a[i])
		y, errY := strconv.Atoi(b[i])
		c := cmp.Compare(a[i], b[i])
		if errX == nil && errY == nil {
			c = cmp.Compare(x, y)
		}
		if c != 0 {
			return c
		}
	}
	return cmp.Compare(len(a), len(b))
}
