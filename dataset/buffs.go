package dataset

import "fmt"

// Buffs is the buffs group: the game's buff definitions, the buffs of utility flasks and the
// tinctures. A buff is a set of stats an actor carries while it is active; a source such as a
// skill, a flask or a passive gives the values.
//
// The game files do not link skills to the buffs they grant, nor life, mana and hybrid flasks
// to their recovery buffs; those links are rules of the engine.
type Buffs struct {
	Definitions []BuffDefinition
	// Flasks are the flasks with a buff, a row each.
	Flasks    []FlaskBuff
	Tinctures []Tincture
}

// BuffDefinition is a row of BuffDefinitions. Stats are stat handles.
type BuffDefinition struct {
	// ID is the BuffDefinitions.Id, such as "player_aura_armour".
	ID string
	// Category is the BuffCategory as the game numbers it; its names are not part of the
	// dataset.
	Category int32
	// ID2 is the game's second id, naming behaviour buffs share, such as "CurseDebuff" on
	// curses and marks; empty on most.
	ID2 string
	// Stats and ExtraStats are the stats the buff carries, StatsKeys and Stats2 in game order;
	// a stat can appear twice. A source with values for the buff, such as a flask, has one
	// value per stat of Stats followed by ExtraStats. A skill's buff takes the skill's stats
	// whose ids are in Stats.
	Stats, ExtraStats []uint32
	// Flags are the GrantedFlags, stats set to 1 while the buff is active. ConditionStats are
	// stats the game counts the buff with, such as curse_count.
	Flags, ConditionStats []uint32
	// MaximumStat, CurrentStat and MinimumStat are the stat handles holding a charge-like
	// buff's maximum, current and minimum count; nil when unset.
	MaximumStat, CurrentStat, MinimumStat *uint32
	// Limit is the BuffLimit, the most stacks of the buff an actor can have; 0 when unset.
	Limit int32
	// SkillBuff is the game's IsSkillBuff.
	SkillBuff     bool
	Invisible     bool
	Removable     bool
	CancelOnDeath bool
	// Recovery is the game's IsRecovery.
	Recovery bool
}

// FlaskBuff is the buff of a Flasks row.
type FlaskBuff struct {
	// Item is the ItemBase.ID of the flask.
	Item string
	// Buff is the BuffDefinition.ID.
	Buff string
	// Values are the BuffStatValues, one per stat of the buff's Stats followed by ExtraStats.
	Values []int32
}

// Tincture is a row of Tinctures.
type Tincture struct {
	// Item is the ItemBase.ID of the tincture.
	Item string
	// DebuffInterval and Cooldown are in milliseconds.
	DebuffInterval, Cooldown int32
}

func buffID(b BuffDefinition) string   { return b.ID }
func flaskBuffItem(f FlaskBuff) string { return f.Item }
func tinctureItem(t Tincture) string   { return t.Item }

// validateBuffs checks b against a stats group of n stats and, for flasks and tinctures, the
// items group, which is nil when the dataset has none.
func validateBuffs(n int, items *Items, b *Buffs) error {
	if _, err := idSet("buff", b.Definitions, buffID); err != nil {
		return err
	}
	if err := checkEach("buff", b.Definitions, buffID, func(d BuffDefinition) error { return validateBuffDefinition(n, d) }); err != nil {
		return err
	}
	if items == nil {
		if len(b.Flasks) > 0 || len(b.Tinctures) > 0 {
			return fmt.Errorf("flask buffs or tinctures without the items group: %w", ErrInvalid)
		}
		return nil
	}
	bases := byKey(items.Bases, func(base ItemBase) string { return base.ID })
	if _, err := idSet("flask", b.Flasks, flaskBuffItem); err != nil {
		return err
	}
	defs := byKey(b.Definitions, buffID)
	if err := checkEach("flask", b.Flasks, flaskBuffItem, func(f FlaskBuff) error { return validateFlaskBuff(bases, defs, f) }); err != nil {
		return err
	}
	if _, err := idSet("tincture", b.Tinctures, tinctureItem); err != nil {
		return err
	}
	return checkEach("tincture", b.Tinctures, tinctureItem, func(t Tincture) error {
		if _, ok := bases[t.Item]; !ok {
			return fmt.Errorf("not a base in items: %w", ErrInvalid)
		}
		return nil
	})
}

func validateBuffDefinition(n int, d BuffDefinition) error {
	for _, l := range []struct {
		what  string
		stats []uint32
	}{{"stats", d.Stats}, {"extra stats", d.ExtraStats}, {"flags", d.Flags}, {"condition stats", d.ConditionStats}} {
		if err := handlesInRange(l.what, l.stats, n); err != nil {
			return err
		}
	}
	for _, s := range []struct {
		what string
		stat *uint32
	}{{"maximum", d.MaximumStat}, {"current", d.CurrentStat}, {"minimum", d.MinimumStat}} {
		if s.stat != nil {
			if err := handlesInRange(s.what, []uint32{*s.stat}, n); err != nil {
				return err
			}
		}
	}
	if d.Limit < 0 {
		return fmt.Errorf("limit %d negative: %w", d.Limit, ErrInvalid)
	}
	return nil
}

func validateFlaskBuff(bases map[string]ItemBase, defs map[string]BuffDefinition, f FlaskBuff) error {
	if base, ok := bases[f.Item]; !ok || base.Flask == nil {
		return fmt.Errorf("not a flask base in items: %w", ErrInvalid)
	}
	d, ok := defs[f.Buff]
	if !ok {
		return fmt.Errorf("buff %q not in buffs: %w", f.Buff, ErrInvalid)
	}
	if want := len(d.Stats) + len(d.ExtraStats); len(f.Values) != want {
		return fmt.Errorf("%d values for the %d stats of buff %s: %w", len(f.Values), want, d.ID, ErrInvalid)
	}
	return nil
}

// validateBuffNames checks the buff names of texts against b, which is nil when the dataset
// has no buffs group.
func validateBuffNames(texts []Text, b *Buffs) error {
	var defs map[string]bool
	if b != nil {
		defs, _ = idSet("buff", b.Definitions, buffID)
	}
	for _, t := range texts {
		for _, nm := range t.BuffNames {
			if !defs[nm.ID] {
				return fmt.Errorf("text %s: name for unknown buff %s: %w", t.Language, nm.ID, ErrInvalid)
			}
		}
	}
	return nil
}

// validateBuffsGroup checks d's buffs group and the buff names in d's texts.
func (d *Dataset) validateBuffsGroup() error {
	if d.Buffs != nil {
		if err := validateBuffs(len(d.Stats), d.Items, d.Buffs); err != nil {
			return err
		}
	}
	return validateBuffNames(d.Texts, d.Buffs)
}
