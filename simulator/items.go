package simulator

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/internal/poe1"
)

func indexBases(s *dataset.Store) (map[string]*dataset.ItemBase, error) {
	it, err := s.Items()
	if err != nil {
		return nil, err //nolint:wrapcheck // wrapped by the caller
	}
	out := make(map[string]*dataset.ItemBase, len(it.Bases))
	for i := range it.Bases {
		out[it.Bases[i].ID] = &it.Bases[i]
	}
	return out, nil
}

// compileItems sets r's items from the checked build's, compiled once per item content. On a
// dataset without the items group the items are left out with a warning.
func (s *Sim) compileItems(r *run, equipped build.Items) error {
	if len(equipped) == 0 {
		return nil
	}
	bases, err := s.bases()
	if _, missing := errors.AsType[*dataset.MissingGroupError](err); missing {
		r.warnings = append(r.warnings, Warning{
			Code: ItemsNotSimulated, Subject: "items",
			Message: fmt.Sprintf("dataset has no items group; %d items left out", len(equipped)),
		})
		return nil
	}
	if err != nil {
		return fmt.Errorf("simulator: %w", err)
	}
	for _, slot := range slices.Sorted(maps.Keys(equipped)) {
		it := equipped[slot]
		data, err := json.Marshal(&it)
		if err != nil {
			return fmt.Errorf("simulator: item %s: %w", slot, err)
		}
		key := string(data)
		c, ok := s.compiledItem(key)
		if !ok {
			// The check left only items whose base the dataset has.
			stats, assumed := poe1.CompileItem(&it, bases[it.Base])
			c = compiledItem{stats: stats, assumed: assumed}
			s.keepCompiled(key, c)
		}
		r.items = append(r.items, poe1.Item{Slot: slot, Name: it.Name, Key: key, Stats: c.stats})
		if len(c.assumed) > 0 {
			r.warnings = append(r.warnings, Warning{
				Code: ItemDefenceAssumed, Subject: "item " + string(slot),
				Message: "text gives no " + strings.Join(c.assumed, ", ") + "; the base's highest roll taken",
			})
		}
	}
	return nil
}

func (s *Sim) compiledItem(key string) (compiledItem, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.compiled[key]
	return c, ok
}

func (s *Sim) keepCompiled(key string, c compiledItem) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.compiled == nil || len(s.compiled) >= maxCompiled {
		s.compiled = map[string]compiledItem{}
	}
	s.compiled[key] = c
}
