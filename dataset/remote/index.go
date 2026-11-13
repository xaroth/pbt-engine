package remote

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/xaroth/pbt-engine/dataset"
)

// Entry is a league's latest dataset in the index: its game version, its dataset id, the
// SHA-256 of its tree-art manifest and its dataset format.
type Entry struct {
	Version string `json:"version"`
	Dataset string `json:"dataset"`
	Art     string `json:"art"`
	Format  int    `json:"format"`
}

// UnmarshalJSON reads an entry without a format as format 1, which entries had before they named it.
func (e *Entry) UnmarshalJSON(b []byte) error {
	type plain Entry
	p := plain{Format: 1}
	if err := json.Unmarshal(b, &p); err != nil {
		return fmt.Errorf("entry: %w", err)
	}
	*e = Entry(p)
	return nil
}

// Index is index.json: per game, per league.
type Index map[string]map[string]Entry

// Latest reads the index from f and returns the entry of game's league. It fails with
// ErrUnknownLeague when the index has none, and with dataset.ErrFormat when the entry is in
// another format than dataset.FormatVersion.
func Latest(ctx context.Context, f Fetcher, game, league string) (Entry, error) {
	idx, err := readIndex(ctx, f)
	if err != nil {
		return Entry{}, err
	}
	return idx.latest(game, league)
}

func readIndex(ctx context.Context, f Fetcher) (Index, error) {
	b, err := f.Fetch(ctx, IndexName)
	if err != nil {
		return nil, fmt.Errorf("index: %w", err)
	}
	var idx Index
	if err := json.Unmarshal(b, &idx); err != nil {
		return nil, fmt.Errorf("%s: %w", IndexName, err)
	}
	return idx, nil
}

// latest is the entry of game's league when the engine reads its format. The league must be a
// valid name, since Pull makes a directory of it.
func (idx Index) latest(game, league string) (Entry, error) {
	e, ok := idx[game][league]
	if !ok || !validName(league) {
		return e, fmt.Errorf("%w: %s %s", ErrUnknownLeague, game, league)
	}
	if e.Format != dataset.FormatVersion {
		return e, fmt.Errorf("%s %s: %w %d, want %d", game, league, dataset.ErrFormat, e.Format, dataset.FormatVersion)
	}
	return e, nil
}
