package build

import "github.com/xaroth/pbt-engine/dataset"

// New starts a level 1 build of class (a Character.ID) on the default tree variant of the
// dataset s, with nothing allocated, and checks it. It returns Check's *CheckError when the
// class is not in that tree and characters.
func New(s *dataset.Store, class string) (*Checked, error) {
	m := s.Manifest()
	c, _, err := Check(&Build{
		Format:  Format,
		Game:    m.Game,
		Version: m.Version,
		Variant: DefaultVariant,
		Class:   class,
		Level:   1,
	}, s)
	return c, err
}
