package dataset

import "fmt"

// Classes returns the classes of one tree variant, such as "standard", shared and read-only. It
// reads that variant's tree file and decodes none of its nodes, so it reads no stats or
// characters and its classes are not checked against them; Write and Tree check them. With
// Names, a host lists classes without loading the stats or text groups.
func (s *Store) Classes(variant string) ([]Class, error) {
	tc, err := s.treeClasses(variant)
	return tc.classes, err
}

// AlternateAscendancies returns the alternate ascendancies of one tree variant, the bloodlines,
// shared and read-only. It reads as Classes does, so it loads neither the stats nor the text
// groups.
func (s *Store) AlternateAscendancies(variant string) ([]Ascendancy, error) {
	tc, err := s.treeClasses(variant)
	return tc.alternates, err
}

// treeClasses is what of a tree file Classes and AlternateAscendancies read.
type treeClasses struct {
	variant    string
	classes    []Class
	alternates []Ascendancy
}

func (s *Store) treeClasses(variant string) (treeClasses, error) {
	l, ok := s.classes[variant]
	if !ok {
		return treeClasses{}, fmt.Errorf("dataset %s has no %s tree: %w", s.manifest.Version, variant, ErrNotInDataset)
	}
	return l.get(func() (treeClasses, error) { return s.loadClasses(variant) })
}

func (s *Store) loadClasses(variant string) (treeClasses, error) {
	name := TreeFile(variant)
	tc, err := readGroup(s, name, decodeTreeClasses)
	if err != nil {
		return treeClasses{}, err
	}
	if tc.variant != variant {
		return treeClasses{}, fmt.Errorf("%s: holds tree variant %q: %w", name, tc.variant, ErrInvalid)
	}
	return tc, nil
}
