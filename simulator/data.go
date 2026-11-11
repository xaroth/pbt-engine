package simulator

import (
	"errors"
	"fmt"
	"strings"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/dataset"
)

// ErrAmbiguousName is a name of more than one gem that dataset.Store.GemByName cannot choose
// between; the error lists their ids.
var ErrAmbiguousName = errors.New("simulator: name of more than one gem")

// Data looks up the ids a build holds by the names players know, in the sim's dataset.
type Data struct {
	store *dataset.Store
}

// Data returns the sim's name lookups.
func (s *Sim) Data() *Data { return &Data{store: s.store} }

// Gem returns the id of the gem named name in English, such as "Fireball", "Inspiration" or
// the transfigured "Fireball of Splitting", chosen among gems of one name as
// dataset.Store.GemByName does. A dataset without the skills group returns a
// *dataset.MissingGroupError, an unknown name dataset.ErrNotInDataset, and a name the choice
// cannot decide ErrAmbiguousName.
func (d *Data) Gem(name string) (build.GemID, error) {
	m, err := d.store.GemByName(name)
	if err != nil {
		return build.GemID{}, fmt.Errorf("simulator: %w", err)
	}
	if len(m.Others) > 0 {
		return build.GemID{}, fmt.Errorf("%w: %q is %s and %s", ErrAmbiguousName, name, m.Gem.ID, strings.Join(m.Others, ", "))
	}
	id := build.GemID{ID: m.Gem.ID}
	if m.Effect != &m.Gem.Effects[0] {
		id.Effect = m.Effect.ID
	}
	return id, nil
}
