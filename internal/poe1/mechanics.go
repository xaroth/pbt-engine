package poe1

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/xaroth/pbt-engine/internal/core"
)

// percentScale turns a percentage into a fraction.
const percentScale = 100

// mechanic is one mechanic of the character sheet: its engine stats, how they are defined and
// the table rules it implements.
type mechanic struct {
	name string
	// after names the mechanics whose stats this one reads or whose definitions it extends.
	after []string
	// stats are its engine stats. A stat follows the stats it includes or shares.
	stats []string
	// def completes the definition of any stat, its own or one of a mechanic it comes after;
	// ids holds the stats of this mechanic and those it comes after. Nil leaves defs as they
	// are.
	def func(def core.StatDef, ids map[string]core.StatID) core.StatDef
	// rules are the table rules it implements, for mechanics a modifier cannot express.
	rules []tableRule
	// complete runs once every mechanic's stats are registered, for stats and formulas that
	// cannot be registered one stat at a time.
	complete func(b *core.ModelBuilder, r registered)
}

// tableRule is a table rule and its parameters.
type tableRule struct {
	name   string
	params []string
}

// mechanics are the mechanics of the character sheet. Their order here does not matter: the
// model registers them after the mechanics they come after.
func mechanics() []mechanic {
	return []mechanic{
		attributeMechanic(),
		defenceMechanic(),
		characterMechanic(),
		resistanceMechanic(),
		regenerationMechanic(),
		conditionMechanic(),
		arcaneSurgeMechanic(),
		resonanceMechanic(),
		heldMechanic(),
		equipmentMechanic(),
		chargeMechanic(),
		blockMechanic(),
		movementMechanic(),
		skillStageMechanic(),
		transferMechanic(),
		reservationMechanic(),
		buffMechanic(),
		curseMechanic(),
		costMechanic(),
		damageMechanic(),
		enemyMechanic(),
		exposureMechanic(),
		witherMechanic(),
		conversionMechanic(),
		accuracyMechanic(),
		hitEffectsMechanic(),
		hitMechanic(),
		handsMechanic(),
		dotMechanic(),
		ailmentMechanic(),
		igniteMechanic(),
		inflictMechanic(),
		bleedMechanic(),
		poisonMechanic(),
		totalDotMechanic(),
	}
}

// registered is what the mechanics registered on a model builder: each stat's id and the
// definition it was registered with.
type registered struct {
	ids  map[string]core.StatID
	defs map[string]core.StatDef
}

// errMechanics is a list of mechanics a model cannot be built from.
var errMechanics = errors.New("poe1: mechanics")

// unregisteredStat is mustID's panic value for a stat a mechanic may not read.
type unregisteredStat string

// mustID returns the id of name among ids. It panics when name is not there: id 0 is a real
// stat, so a missing name would silently refer to it. addMechanics turns the panic into an
// error naming the mechanic that read it.
func mustID(ids map[string]core.StatID, name string) core.StatID {
	id, ok := ids[name]
	if !ok {
		panic(unregisteredStat(name))
	}
	return id
}

// addMechanics registers the stats of ms on an empty builder b, each mechanic after those it
// comes after, then completes them. It fails when a mechanic comes after one that is not in ms
// or that comes after it, when a stat belongs to two mechanics, when a definition reads a stat
// of a mechanic its mechanic does not come after, or when a stat includes or shares one listed
// after it.
func addMechanics(b *core.ModelBuilder, ms []mechanic) (registered, error) {
	ordered, err := orderMechanics(ms)
	if err != nil {
		return registered{}, err
	}
	// Every stat's id is known before any definition, so a formula may read a stat listed
	// after it.
	ids, err := statIDs(ordered)
	if err != nil {
		return registered{}, err
	}
	visible := visibleStats(ordered, ids)
	r := registered{ids: ids, defs: make(map[string]core.StatDef, len(ids))}
	for _, m := range ordered {
		for _, s := range m.stats {
			def, err := defineStat(s, ordered, visible)
			if err == nil {
				err = checkEarlier(def, ids, m.name)
			}
			if err != nil {
				return registered{}, err
			}
			if _, err := b.AddStat(def); err != nil {
				return registered{}, fmt.Errorf("%w: mechanic %s: %w", errMechanics, m.name, err)
			}
			r.defs[s] = def
		}
	}
	for _, m := range ordered {
		if m.complete != nil {
			m.complete(b, r)
		}
	}
	return r, nil
}

// statIDs numbers the stats of the ordered mechanics in order.
func statIDs(ordered []mechanic) (map[string]core.StatID, error) {
	ids := map[string]core.StatID{}
	owner := map[string]string{}
	for _, m := range ordered {
		for _, s := range m.stats {
			if o, dup := owner[s]; dup {
				return nil, fmt.Errorf("%w: stat %s belongs to mechanics %s and %s", errMechanics, s, o, m.name)
			}
			owner[s] = m.name
			ids[s] = core.StatID(len(ids)) //nolint:gosec // a few hundred stats
		}
	}
	return ids, nil
}

// defineStat returns the definition of stat name: each of the ordered mechanics completes it in
// turn, reading the stats visible to it.
func defineStat(name string, ordered []mechanic, visible []map[string]core.StatID) (core.StatDef, error) {
	def := core.StatDef{Name: name}
	for i, m := range ordered {
		var unread string
		if def, unread = m.define(def, visible[i]); unread != "" {
			return core.StatDef{}, fmt.Errorf("%w: mechanic %s, defining stat %s, reads stat %s, "+
				"which is not its own or of a mechanic it comes after", errMechanics, m.name, name, unread)
		}
	}
	return def, nil
}

// define applies m's definition to def, reading only ids. It returns the name of a stat the
// definition read that is not in ids, if any.
func (m mechanic) define(def core.StatDef, ids map[string]core.StatID) (out core.StatDef, unread string) {
	if m.def == nil {
		return def, ""
	}
	defer func() {
		if p := recover(); p != nil {
			name, ok := p.(unregisteredStat)
			if !ok {
				panic(p)
			}
			unread = string(name)
		}
	}()
	return m.def(def, ids), ""
}

// checkEarlier checks that def includes and shares only stats listed before it, naming them,
// where core would give only their ids.
func checkEarlier(def core.StatDef, ids map[string]core.StatID, mechanic string) error {
	id := ids[def.Name]
	refs := slices.Clone(def.Includes)
	for _, sh := range def.Shares {
		refs = append(refs, sh.Stat, sh.Rate.Stat)
	}
	for name, other := range ids {
		if other >= id && slices.Contains(refs, other) {
			return fmt.Errorf("%w: mechanic %s: stat %s includes or shares stat %s, which is listed after it", errMechanics, mechanic, def.Name, name)
		}
	}
	return nil
}

// visibleStats returns, for each of the ordered mechanics, the ids of its own stats and those
// of the mechanics it comes after, directly or not.
func visibleStats(ordered []mechanic, ids map[string]core.StatID) []map[string]core.StatID {
	index := make(map[string]int, len(ordered))
	for i, m := range ordered {
		index[m.name] = i
	}
	out := make([]map[string]core.StatID, len(ordered))
	for i, m := range ordered {
		v := map[string]core.StatID{}
		for _, s := range m.stats {
			v[s] = ids[s]
		}
		// The mechanics m comes after are earlier, so theirs are done.
		for _, a := range m.after {
			maps.Copy(v, out[index[a]])
		}
		out[i] = v
	}
	return out
}

// orderMechanics returns ms with each mechanic after the mechanics it comes after, otherwise
// in the order of ms.
func orderMechanics(ms []mechanic) ([]mechanic, error) {
	index, err := indexMechanics(ms)
	if err != nil {
		return nil, err
	}
	done := make([]bool, len(ms))
	ready := func(m mechanic) bool {
		return !done[index[m.name]] && !slices.ContainsFunc(m.after, func(a string) bool { return !done[index[a]] })
	}
	out := make([]mechanic, 0, len(ms))
	for len(out) < len(ms) {
		next := slices.IndexFunc(ms, ready)
		if next < 0 {
			var left []string
			for i, m := range ms {
				if !done[i] {
					left = append(left, m.name)
				}
			}
			return nil, fmt.Errorf("%w: %s come after each other", errMechanics, strings.Join(left, ", "))
		}
		done[next] = true
		out = append(out, ms[next])
	}
	return out, nil
}

// indexMechanics returns the index of each of ms by name. It fails on two mechanics of one
// name, or one that comes after a mechanic not in ms.
func indexMechanics(ms []mechanic) (map[string]int, error) {
	index := make(map[string]int, len(ms))
	for i, m := range ms {
		if _, dup := index[m.name]; dup {
			return nil, fmt.Errorf("%w: two named %s", errMechanics, m.name)
		}
		index[m.name] = i
	}
	for _, m := range ms {
		for _, a := range m.after {
			if _, ok := index[a]; !ok {
				return nil, fmt.Errorf("%w: mechanic %s comes after %s, which is not a mechanic", errMechanics, m.name, a)
			}
		}
	}
	return index, nil
}
