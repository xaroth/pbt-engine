package dataset

import "fmt"

// Layout returns the layout of one tree variant, such as "standard", shared and read-only. It
// reads that variant's tree too, to check the layout places exactly its nodes. A dataset
// without the layout returns an error matching ErrNotInDataset.
func (s *Store) Layout(variant string) (*Layout, error) {
	load, ok := s.layouts[variant]
	if !ok {
		return nil, fmt.Errorf("dataset %s has no %s layout: %w", s.manifest.Version, variant, ErrNotInDataset)
	}
	return load()
}

func (s *Store) loadLayout(variant string) (*Layout, error) {
	name := layoutPrefix + variant + fbSuffix
	t, err := s.Tree(variant)
	if err != nil {
		return nil, err
	}
	l, err := readGroup(s, name, decodeLayout)
	if err != nil {
		return nil, err
	}
	if l.Variant != variant {
		return nil, fmt.Errorf("%s: holds layout variant %q: %w", name, l.Variant, ErrInvalid)
	}
	if err := validateLayout(t, &l); err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	return &l, nil
}
