package dataset

import "fmt"

// Classes returns the classes of one tree variant, such as "standard", shared and read-only. It
// reads that variant's tree file and decodes none of its nodes, so it reads no stats or
// characters and its classes are not checked against them; Write and Tree check them. With
// Names, a host lists classes without loading the stats or text groups.
func (s *Store) Classes(variant string) ([]Class, error) {
	load, ok := s.classes[variant]
	if !ok {
		return nil, fmt.Errorf("dataset %s has no %s tree: %w", s.manifest.Version, variant, ErrNotInDataset)
	}
	return load()
}

func (s *Store) loadClasses(variant string) ([]Class, error) {
	type treeClasses struct {
		variant string
		classes []Class
	}
	name := TreeFile(variant)
	tc, err := readGroup(s, name, func(b []byte) (treeClasses, error) {
		v, cs, err := decodeTreeClasses(b)
		return treeClasses{v, cs}, err
	})
	if err != nil {
		return nil, err
	}
	if tc.variant != variant {
		return nil, fmt.Errorf("%s: holds tree variant %q: %w", name, tc.variant, ErrInvalid)
	}
	return tc.classes, nil
}
