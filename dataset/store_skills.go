package dataset

import "fmt"

// Skills returns the skills group, shared and read-only. A dataset without the group returns a
// *MissingGroupError.
func (s *Store) Skills() (*Skills, error) {
	x, err := s.SkillIndex()
	if err != nil {
		return nil, err
	}
	return x.Skills, nil
}

// SkillIndex returns the skills group indexed by id, indexed once and shared. It fails as
// Skills does.
func (s *Store) SkillIndex() (*SkillIndex, error) { return s.skills.get(s.loadSkills) }

func (s *Store) loadSkills() (*SkillIndex, error) {
	if _, ok := s.files[SkillsFile]; !ok {
		return nil, &MissingGroupError{Version: s.manifest.Version, Group: "skills"}
	}
	st, err := s.statsGroup()
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
	return NewSkillIndex(&sk), nil
}
