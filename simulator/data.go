package simulator

import (
	"errors"
	"fmt"
	"strings"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/items"
)

// ErrAmbiguousName is a name of more than one gem that dataset.Store.GemByName cannot choose
// between; the error lists their ids.
var ErrAmbiguousName = errors.New("simulator: name of more than one gem")

// Data looks up the ids a build holds by the names players know, in the sim's dataset.
type Data struct {
	store *dataset.Store
	items func() (*items.Reader, error)
}

// Data returns the sim's name lookups.
func (s *Sim) Data() *Data { return &Data{store: s.store, items: s.items} }

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

// Item reads item text, as Path of Building writes and copies it or as the game copies it, into
// an item for build.Equip: its base by name and each mod line as the stats it reads as (see
// items.Reader.Parse). A line no stat description reads is a warning and stays on the item
// without stats. A dataset without the items group returns a *dataset.MissingGroupError, and one
// whose items group lacks the mod domains, bench crafts and essence mods items.ErrNoCrafting;
// text that is not an item items.ErrFormat, an unknown base items.ErrUnknownBase, an item no slot
// takes items.ErrNotEquipable and a flask in the game's text items.ErrNotSupported.
func (d *Data) Item(text string) (build.Item, []Warning, error) {
	r, err := d.items()
	if err != nil {
		return build.Item{}, nil, fmt.Errorf("simulator: %w", err)
	}
	it, warnings, err := r.Parse(text)
	if err != nil {
		return build.Item{}, nil, fmt.Errorf("simulator: %w", err)
	}
	return it, warnings, nil
}
