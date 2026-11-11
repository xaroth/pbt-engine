package dataset

import (
	"errors"
	"fmt"
)

// Buffs returns the buffs group, shared and read-only. When the dataset has the items group,
// it reads the items too, to check the flasks and tinctures. A dataset without the group
// returns a *MissingGroupError.
func (s *Store) Buffs() (*Buffs, error) { return s.buffs() }

func (s *Store) loadBuffs() (*Buffs, error) {
	if _, ok := s.files[buffsFile]; !ok {
		return nil, &MissingGroupError{Version: s.manifest.Version, Group: "buffs"}
	}
	st, err := s.stats()
	if err != nil {
		return nil, err
	}
	items, err := s.items()
	if _, ok := errors.AsType[*MissingGroupError](err); ok {
		items, err = nil, nil
	}
	if err != nil {
		return nil, err
	}
	b, err := readGroup(s, buffsFile, func(buf []byte) (Buffs, error) { return decodeBuffs(buf, st.index) })
	if err != nil {
		return nil, err
	}
	if err := validateBuffs(len(st.stats), items, &b); err != nil {
		return nil, fmt.Errorf("%s: %w", buffsFile, err)
	}
	return &b, nil
}
