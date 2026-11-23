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
	"github.com/xaroth/pbt-engine/internal/resolve"
	"github.com/xaroth/pbt-engine/poe1/passives"
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

func indexModTags(s *dataset.Store) (*poe1.ModTags, error) {
	mods, err := s.Mods()
	if err != nil {
		return nil, err //nolint:wrapcheck // wrapped by the caller
	}
	stats, err := s.Stats()
	if err != nil {
		return nil, err //nolint:wrapcheck // wrapped by the caller
	}
	return poe1.NewModTags(mods, stats), nil
}

// compileItems sets the items of r's sheet from the checked build's items with their abyss jewels and the
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
	own := map[build.Slot]int{}
	for _, slot := range slices.Sorted(maps.Keys(cb.Items)) {
		own[slot] = len(r.sheet.Items)
		if err := s.compileItem(r, slot, cb.Items[slot], bases); err != nil {
			return err
		}
	}
	reflectRings(r.sheet.Items, own)
	return s.compileJewels(r, cb, jewels, bases)
}

// compileMinionGear sets the gear of r's minions from the minion items of cb's skill groups,
// each compiled once per content. On a dataset without the items group, compileItems warns.
func (s *Sim) compileMinionGear(r *run, cb *build.Build) error {
	for i, g := range cb.Skills.Groups {
		if len(g.MinionItems) == 0 {
			continue
		}
		bases, err := s.bases()
		if _, missing := errors.AsType[*dataset.MissingGroupError](err); missing {
			return nil
		}
		if err != nil {
			return fmt.Errorf("simulator: %w", err)
		}
		if r.gear == nil {
			r.gear = poe1.MinionGear{}
		}
		for _, slot := range slices.Sorted(maps.Keys(g.MinionItems)) {
			it := g.MinionItems[slot]
			c, key, err := s.compile(&it, bases)
			if err != nil {
				return fmt.Errorf("simulator: group %d minion item %s: %w", i+1, slot, err)
			}
			r.gear[i] = append(r.gear[i], poe1.Item{
				Slot: slot, Name: it.Name, Key: key, Stats: c.Stats, Weapon: c.Weapon, Shield: c.Shield, Armour: c.Armour,
			})
		}
	}
	return nil
}

// compileJewels adds to the items of r's sheet the jewels of cb in the allocated sockets, each
// with the effect the others give it (poe1.SocketJewelEffect) and the effect it gains per
// passive between its socket and the class start (poe1.EffectPerPassiveToStart).
func (s *Sim) compileJewels(r *run, cb *build.Build, sockets []uint32, bases map[string]*dataset.ItemBase) error {
	jewels := make([]poe1.Item, len(sockets))
	stats := make([][]resolve.StatValue, len(sockets))
	for i, node := range sockets {
		it := cb.Jewels[node]
		c, key, err := s.compile(&it, bases)
		if err != nil {
			return fmt.Errorf("simulator: jewel %d: %w", node, err)
		}
		jewels[i] = poe1.Item{Node: node, Name: it.Name, Key: key, Stats: c.Stats}
		stats[i] = c.Stats
	}
	adorned := poe1.AdornedEffect(stats...)
	r.startDistances = startDistances(r.checked, sockets, stats)
	for i, node := range sockets {
		it := cb.Jewels[node]
		inc := poe1.SocketJewelEffect(&it, adorned)
		if per := poe1.EffectPerPassiveToStart(stats[i]); per != 0 {
			jewels[i].Passives = r.startDistances[node]
			inc += per * float64(jewels[i].Passives)
		}
		poe1.ScaleJewel(&jewels[i], inc)
	}
	r.sheet.Items = append(r.sheet.Items, jewels...)
	return nil
}

// startDistances returns, by socket, the passives between the class start and each of sockets
// whose jewel, with stats, gains effect per passive between them; nil when there is none.
func startDistances(c *build.Checked, sockets []uint32, stats [][]resolve.StatValue) map[uint32]int {
	var counted []uint32
	for i, node := range sockets {
		if poe1.EffectPerPassiveToStart(stats[i]) != 0 {
			counted = append(counted, node)
		}
	}
	if len(counted) == 0 {
		return nil
	}
	out := make(map[uint32]int, len(counted))
	for i, n := range passives.StartDistances(c.Nodes(), c.Class().StartNode, counted...) {
		out[counted[i]] = n
	}
	return out
}

// reflectRings applies poe1.ReflectRings to items, where at holds the index of each slot's own
// item.
func reflectRings(items []poe1.Item, at map[build.Slot]int) {
	rings := map[build.Slot]*poe1.Item{}
	for _, s := range []build.Slot{build.SlotRing1, build.SlotRing2, build.SlotRing3} {
		if i, ok := at[s]; ok {
			rings[s] = &items[i]
		}
	}
	poe1.ReflectRings(rings)
}

// compileItem adds to the items of r's sheet the item it in slot, then its abyss jewels with the
// effect the item gives them (poe1.AbyssJewelEffect). Each jewel counts
// as part of its item: it shares the item's slot, weapon and shield, so the hands read the same
// whichever of the two they meet last, and its index keeps two equal jewels apart. Only a
// body armour with stats is told whether it has Life Modifiers: only the body armour is asked,
// and a line without stats is none.
func (s *Sim) compileItem(r *run, slot build.Slot, it build.Item, bases map[string]*dataset.ItemBase) error {
	abyss := it.AbyssJewels
	it.AbyssJewels = nil
	c, key, err := s.compile(&it, bases)
	if err != nil {
		return fmt.Errorf("simulator: item %s: %w", slot, err)
	}
	life := false
	if slot == build.SlotBodyArmour && hasStats(it) {
		tags, err := s.modTags()
		if err != nil {
			return fmt.Errorf("simulator: item %s: %w", slot, err)
		}
		life = tags.LifeModifiers(&it)
	}
	r.sheet.Items = append(r.sheet.Items, poe1.Item{
		Slot: slot, Name: it.Name, Key: key, Stats: c.Stats, Weapon: c.Weapon, Shield: c.Shield,
		Armour: c.Armour, LifeModifiers: life, MovementPenalty: c.MovementPenalty,
	})
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
		jewel := poe1.Item{
			Slot: slot, Name: j.Name, Key: fmt.Sprintf("abyss %d %s", i, jkey), Stats: jc.Stats, Weapon: c.Weapon, Shield: c.Shield,
		}
		poe1.ScaleJewel(&jewel, poe1.AbyssJewelEffect(c.Stats))
		r.sheet.Items = append(r.sheet.Items, jewel)
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
