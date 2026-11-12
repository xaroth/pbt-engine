package dataset

import (
	"fmt"
	"slices"
)

// validateItemsGroup checks d's items group, its references to stats and the item names in
// d's texts.
func (d *Dataset) validateItemsGroup() error {
	if d.Items == nil {
		if len(d.Mods) > 0 {
			return fmt.Errorf("mods without items: %w", ErrInvalid)
		}
		return validateItemNames(d.Texts, nil, nil)
	}
	if err := validateItems(*d.Items); err != nil {
		return err
	}
	if err := validateMods(len(d.Stats), d.Items.Tags, d.Mods); err != nil {
		return err
	}
	if err := validateModRefs(*d.Items, d.Mods); err != nil {
		return err
	}
	return validateItemNames(d.Texts, d.Items, d.Mods)
}

// validateItems checks classes, tags, bases, bench crafts and essence mods: ids present and
// unique, every class and tag they name known, ranges ordered, and the crafting fields set as
// Crafting says.
func validateItems(it Items) error {
	classes, err := idSet("item class", it.Classes, func(c ItemClass) string { return c.ID })
	if err != nil {
		return err
	}
	tags, err := idSet("tag", it.Tags, func(t string) string { return t })
	if err != nil {
		return err
	}
	if _, err := idSet("base", it.Bases, func(b ItemBase) string { return b.ID }); err != nil {
		return err
	}
	for _, b := range it.Bases {
		if err := validateBase(classes, tags, b); err != nil {
			return fmt.Errorf("base %s: %w", b.ID, err)
		}
	}
	if err := validateModClasses("bench craft", classes, it.BenchCrafts); err != nil {
		return err
	}
	if err := validateModClasses("essence mod", classes, it.EssenceMods); err != nil {
		return err
	}
	return validateCrafting(it)
}

// validateCrafting checks that a group with Crafting has a mod domain on every base, bench
// crafts and essence mods, and one without has none of them.
func validateCrafting(it Items) error {
	for _, b := range it.Bases {
		// A mod domain is a row of the game's ModDomains table, numbered from 1.
		if it.Crafting != (b.ModDomain > 0) || b.ModDomain < 0 {
			return fmt.Errorf("base %s: mod domain %d with crafting %t: %w", b.ID, b.ModDomain, it.Crafting, ErrInvalid)
		}
	}
	if it.Crafting != (len(it.BenchCrafts) > 0) {
		return fmt.Errorf("%d bench crafts with crafting %t: %w", len(it.BenchCrafts), it.Crafting, ErrInvalid)
	}
	if it.Crafting != (len(it.EssenceMods) > 0) {
		return fmt.Errorf("%d essence mods with crafting %t: %w", len(it.EssenceMods), it.Crafting, ErrInvalid)
	}
	return nil
}

func validateBase(classes, tags map[string]bool, b ItemBase) error {
	if !classes[b.Class] {
		return fmt.Errorf("unknown item class %q: %w", b.Class, ErrInvalid)
	}
	if err := knownTags(tags, b.Tags); err != nil {
		return err
	}
	return validateValues(b)
}

// validateValues checks the armour, weapon and flask values of b.
func validateValues(b ItemBase) error {
	if a := b.Armour; a != nil {
		if err := ordered(a.Armour, a.Evasion, a.EnergyShield, a.Ward); err != nil {
			return err
		}
	}
	if w := b.Weapon; w != nil {
		if err := ordered(w.PhysicalDamage); err != nil {
			return err
		}
	}
	if f := b.Flask; f != nil && (f.Kind < LifeFlask || f.Kind > UtilityFlask) {
		return fmt.Errorf("unknown flask kind %d: %w", f.Kind, ErrInvalid)
	}
	return nil
}

func ordered(rs ...ValueRange) error {
	for _, r := range rs {
		if r.Min > r.Max {
			return fmt.Errorf("range %d to %d: %w", r.Min, r.Max, ErrInvalid)
		}
	}
	return nil
}

// validateMods checks mods against a stats group of n stats and the items group's tags.
func validateMods(n int, tags []string, mods []Mod) error {
	if _, err := idSet("mod", mods, func(m Mod) string { return m.ID }); err != nil {
		return err
	}
	known := make(map[string]bool, len(tags))
	for _, t := range tags {
		known[t] = true
	}
	for _, m := range mods {
		if err := validateMod(n, known, m); err != nil {
			return fmt.Errorf("mod %s: %w", m.ID, err)
		}
	}
	return nil
}

func validateMod(n int, tags map[string]bool, m Mod) error {
	if m.Type == "" {
		return fmt.Errorf("empty type: %w", ErrInvalid)
	}
	// A game mode is a row of the game's GameModes table, so it is never negative.
	if m.GameMode < 0 {
		return fmt.Errorf("game mode %d: %w", m.GameMode, ErrInvalid)
	}
	for _, s := range m.Stats {
		if int(s.Stat) >= n {
			return fmt.Errorf("stat %d out of range (%d stats): %w", s.Stat, n, ErrInvalid)
		}
		if err := ordered(ValueRange{s.Min, s.Max}, ValueRange{s.RuthlessMin, s.RuthlessMax}); err != nil {
			return err
		}
	}
	return validateModTags(tags, m)
}

func validateModTags(tags map[string]bool, m Mod) error {
	if err := knownTags(tags, slices.Concat(m.Tags, m.AddedTags)); err != nil {
		return err
	}
	for _, w := range slices.Concat(m.SpawnWeights, m.GenerationWeights) {
		if !tags[w.Tag] {
			return errUnknownTag(w.Tag)
		}
	}
	return nil
}

// validateModRefs checks that every implicit of a base, bench craft mod and essence mod is one
// of mods.
func validateModRefs(it Items, mods []Mod) error {
	ids := make(map[string]bool, len(mods))
	for _, m := range mods {
		ids[m.ID] = true
	}
	for _, b := range it.Bases {
		for _, m := range b.Implicits {
			if !ids[m] {
				return fmt.Errorf("base %s: implicit %s not in mods: %w", b.ID, m, ErrInvalid)
			}
		}
	}
	for _, c := range it.BenchCrafts {
		if !ids[c.Mod] {
			return fmt.Errorf("bench craft %s: mod not in mods: %w", c.Mod, ErrInvalid)
		}
	}
	for _, e := range it.EssenceMods {
		if !ids[e.Mod] {
			return fmt.Errorf("essence mod %s: mod not in mods: %w", e.Mod, ErrInvalid)
		}
	}
	return nil
}

// validateModClasses checks that no two entries of mods, what in errors, name the same mod
// and that each names a nonempty set of known classes.
func validateModClasses(what string, classes map[string]bool, mods []ModClasses) error {
	if _, err := idSet(what, mods, func(m ModClasses) string { return m.Mod }); err != nil {
		return err
	}
	for _, m := range mods {
		if len(m.Classes) == 0 {
			return fmt.Errorf("%s %s: no item classes: %w", what, m.Mod, ErrInvalid)
		}
		seen := make(map[string]bool, len(m.Classes))
		for _, k := range m.Classes {
			if !classes[k] {
				return fmt.Errorf("%s %s: unknown item class %q: %w", what, m.Mod, k, ErrInvalid)
			}
			if seen[k] {
				return fmt.Errorf("%s %s: item class %s repeated: %w", what, m.Mod, k, ErrInvalid)
			}
			seen[k] = true
		}
	}
	return nil
}

// validateItemNames checks the item names of texts against the items group, which is nil for
// a dataset without one.
func validateItemNames(texts []Text, it *Items, mods []Mod) error {
	var classes, bases, modIDs map[string]bool
	if it != nil {
		classes, _ = idSet("item class", it.Classes, func(c ItemClass) string { return c.ID })
		bases, _ = idSet("base", it.Bases, func(b ItemBase) string { return b.ID })
		modIDs, _ = idSet("mod", mods, func(m Mod) string { return m.ID })
	}
	for _, t := range texts {
		for _, names := range []struct {
			what  string
			names []Name
			ids   map[string]bool
		}{{"item class", t.ItemClassNames, classes}, {"base", t.BaseNames, bases}, {"mod", t.ModNames, modIDs}} {
			for _, n := range names.names {
				if !names.ids[n.ID] {
					return fmt.Errorf("text %s: name for unknown %s %s: %w", t.Language, names.what, n.ID, ErrInvalid)
				}
			}
		}
	}
	return nil
}

func knownTags(tags map[string]bool, ts []string) error {
	for _, t := range ts {
		if !tags[t] {
			return errUnknownTag(t)
		}
	}
	return nil
}

func errUnknownTag(t string) error { return fmt.Errorf("unknown tag %q: %w", t, ErrInvalid) }

// idSet returns the ids of s, which must be present and unique.
func idSet[T any](what string, s []T, id func(T) string) (map[string]bool, error) {
	out := make(map[string]bool, len(s))
	for i, v := range s {
		k := id(v)
		if k == "" {
			return nil, fmt.Errorf("%s %d: empty id: %w", what, i, ErrInvalid)
		}
		if out[k] {
			return nil, fmt.Errorf("%s %s repeated: %w", what, k, ErrInvalid)
		}
		out[k] = true
	}
	return out, nil
}
