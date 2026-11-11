package dataset

import "fmt"

// Monsters returns the monsters group, shared and read-only. It reads the skills group too, to
// check the granted effects varieties and edition totems name. A dataset without the group
// returns a *MissingGroupError.
func (s *Store) Monsters() (*Monsters, error) { return s.monsters() }

func (s *Store) checkMonstersFiles() error {
	_, hasMonsters := s.files[monstersFile]
	if _, hasSkills := s.files[skillsFile]; hasMonsters && !hasSkills {
		return fmt.Errorf("%s: %s needs %s: %w", ManifestName, monstersFile, skillsFile, ErrInvalid)
	}
	return nil
}

func (s *Store) loadMonsters() (*Monsters, error) {
	if _, ok := s.files[monstersFile]; !ok {
		return nil, &MissingGroupError{Version: s.manifest.Version, Group: "monsters"}
	}
	st, err := s.stats()
	if err != nil {
		return nil, err
	}
	sk, err := s.skills()
	if err != nil {
		return nil, err
	}
	m, err := readGroup(s, monstersFile, func(b []byte) (Monsters, error) { return decodeMonsters(b, st.index) })
	if err != nil {
		return nil, err
	}
	effects, _ := idSet("granted effect", sk.GrantedEffects, grantedEffectID)
	if err := validateMonsters(len(st.stats), effects, &m); err != nil {
		return nil, fmt.Errorf("%s: %w", monstersFile, err)
	}
	return &m, nil
}
