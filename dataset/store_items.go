package dataset

// Items returns the items group's classes, tags, bases, bench crafts and essence mods, shared
// and read-only. It does not read the mods; Mods and Write check the implicits, bench crafts
// and essence mods against them. A dataset without the group returns a *MissingGroupError.
func (s *Store) Items() (*Items, error) { return s.items() }

// Mods returns the items group's mods, shared and read-only. It reads the items too, to check
// the mods' tags and the mods the items name. A dataset without the group returns a
// *MissingGroupError.
func (s *Store) Mods() ([]Mod, error) { return s.mods() }

func (s *Store) missingItems() error {
	if _, ok := s.files[itemsFile]; ok {
		return nil
	}
	return &MissingGroupError{Version: s.manifest.Version, Group: "items"}
}

func (s *Store) loadItems() (*Items, error) {
	if err := s.missingItems(); err != nil {
		return nil, err
	}
	it, err := readGroup(s, itemsFile, decodeItems)
	if err != nil {
		return nil, err
	}
	if err := validateItems(it); err != nil {
		return nil, err
	}
	return &it, nil
}

func (s *Store) loadMods() ([]Mod, error) {
	if err := s.missingItems(); err != nil {
		return nil, err
	}
	st, err := s.stats()
	if err != nil {
		return nil, err
	}
	it, err := s.items()
	if err != nil {
		return nil, err
	}
	mods, err := readGroup(s, modsFile, func(b []byte) ([]Mod, error) { return decodeMods(b, st.index, it.Tags) })
	if err != nil {
		return nil, err
	}
	if err := validateMods(len(st.stats), it.Tags, mods); err != nil {
		return nil, err
	}
	if err := validateModRefs(*it, mods); err != nil {
		return nil, err
	}
	return mods, nil
}
