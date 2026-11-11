package simulator

import (
	"fmt"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/dataset"
)

// Data looks up the ids a build holds by the names players know, in the sim's dataset.
type Data struct {
	store *dataset.Store
}

// Data returns the sim's name lookups.
func (s *Sim) Data() *Data { return &Data{store: s.store} }

// Gem returns the id of the gem named name in English, such as "Fireball", "Inspiration" or
// the transfigured "Fireball of Splitting". A dataset without the skills group returns a
// *dataset.MissingGroupError; an unknown name dataset.ErrNotInDataset.
func (d *Data) Gem(name string) (build.GemID, error) {
	g, e, err := d.store.GemByName(name)
	if err != nil {
		return build.GemID{}, fmt.Errorf("simulator: %w", err)
	}
	id := build.GemID{ID: g.ID}
	if e != &g.Effects[0] {
		id.Effect = e.ID
	}
	return id, nil
}
