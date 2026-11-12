package dataset

import "fmt"

// Skills returns the skills group, shared and read-only. A dataset without the group returns a
// *MissingGroupError.
func (s *Store) Skills() (*Skills, error) { return s.skills() }

func (s *Store) loadSkills() (*Skills, error) {
	if _, ok := s.files[SkillsFile]; !ok {
		return nil, &MissingGroupError{Version: s.manifest.Version, Group: "skills"}
	}
	st, err := s.stats()
	if err != nil {
		return nil, err
	}
	sk, err := readGroup(s, SkillsFile, func(b []byte) (Skills, error) { return decodeSkills(b, st.index) })
	if err != nil {
		return nil, err
	}
	if err := validateSkills(len(st.stats), &sk); err != nil {
		return nil, fmt.Errorf("%s: %w", SkillsFile, err)
	}
	return &sk, nil
}
