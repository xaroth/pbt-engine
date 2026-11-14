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

// compileItems sets r's items from the checked build's items with their abyss jewels and the
// jewels in its allocated sockets, each compiled once per content. On a dataset without the
// items group they are left out with a warning.
func (s *Sim) compileItems(r *run, cb *build.Build) error {
	jewels := socketed(cb)
	if len(cb.Items) == 0 && len(jewels) == 0 {
		return nil
	}
	bases, err := s.bases()
	if _, missing := errors.AsType[*dataset.MissingGroupError](err); missing {
		r.warnings = append(r.warnings, Warning{
			Code: ItemsNotSimulated, Subject: "items",
			Message: fmt.Sprintf("dataset has no items group; %d items left out", len(cb.Items)+len(jewels)),
		})
		return nil
	}
	if err != nil {
		return fmt.Errorf("simulator: %w", err)
	}
	for _, slot := range slices.Sorted(maps.Keys(cb.Items)) {
		if err := s.compileItem(r, slot, cb.Items[slot], bases); err != nil {
			return err
		}
	}
	for _, node := range jewels {
		it := cb.Jewels[node]
		c, key, err := s.compile(&it, bases)
		if err != nil {
			return fmt.Errorf("simulator: jewel %d: %w", node, err)
		}
		r.items = append(r.items, poe1.Item{Node: node, Name: it.Name, Key: key, Stats: c.Stats})
	}
	return nil
}

// compileItem adds to r's items the item it in slot, then its abyss jewels. Each jewel counts
// as part of its item: it shares the item's slot and weapon, so the hands read the same
// whichever of the two they meet last, and its index keeps two equal jewels apart.
func (s *Sim) compileItem(r *run, slot build.Slot, it build.Item, bases map[string]*dataset.ItemBase) error {
	abyss := it.AbyssJewels
	it.AbyssJewels = nil
	c, key, err := s.compile(&it, bases)
	if err != nil {
		return fmt.Errorf("simulator: item %s: %w", slot, err)
	}
	r.items = append(r.items, poe1.Item{Slot: slot, Name: it.Name, Key: key, Stats: c.Stats, Weapon: c.Weapon})
	if len(c.Assumed) > 0 {
		r.warnings = append(r.warnings, Warning{
			Code: ItemDefenceAssumed, Subject: "item " + string(slot), Slot: slot,
			Message: "text gives no " + strings.Join(c.Assumed, ", ") + "; the base's highest roll taken",
		})
	}
	for _, i := range slices.Sorted(maps.Keys(abyss)) {
		j := abyss[i]
		jc, jkey, err := s.compile(&j, bases)
		if err != nil {
			return fmt.Errorf("simulator: abyss jewel %s %d: %w", slot, i, err)
		}
		r.items = append(r.items, poe1.Item{
			Slot: slot, Name: j.Name, Key: fmt.Sprintf("abyss %d %s", i, jkey), Stats: jc.Stats, Weapon: c.Weapon,
		})
	}
	return nil
}

// socketed returns the sockets of b's jewels that are allocated, sorted. A jewel in a socket
// not allocated gives nothing.
func socketed(b *build.Build) []uint32 {
	var out []uint32
	for _, node := range slices.Sorted(maps.Keys(b.Jewels)) {
		if slices.Contains(b.Nodes, node) {
			out = append(out, node)
		}
	}
	return out
}

// compile returns it compiled and its content key. The check left only items whose base the
// dataset has.
func (s *Sim) compile(it *build.Item, bases map[string]*dataset.ItemBase) (poe1.CompiledItem, string, error) {
	data, err := json.Marshal(it)
	if err != nil {
		return poe1.CompiledItem{}, "", err //nolint:wrapcheck // wrapped by the caller
	}
	key := string(data)
	c, ok := s.compiledItem(key)
	if !ok {
		c = poe1.CompileItem(it, bases[it.Base])
		s.keepCompiled(key, c)
	}
	return c, key, nil
}

func (s *Sim) compiledItem(key string) (poe1.CompiledItem, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.compiled[key]
	return c, ok
}

func (s *Sim) keepCompiled(key string, c poe1.CompiledItem) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.compiled == nil || len(s.compiled) >= maxCompiled {
		s.compiled = map[string]poe1.CompiledItem{}
	}
	s.compiled[key] = c
}
