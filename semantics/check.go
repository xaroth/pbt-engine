package semantics

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/resolve"
	"github.com/xaroth/pbt-engine/statdesc"
)

// Problems Check reports, each wrapped with the entry it is about.
var (
	ErrOtherGame      = errors.New("table is for another game")
	ErrUnknownStat    = errors.New("stat not in dataset")
	ErrSourceMismatch = errors.New("source does not match dataset")
	ErrTextNotChecked = errors.New("dataset has no English text to check description sources against")
)

// sourceLanguage is the language of description source texts.
const sourceLanguage = "en"

// Check checks the entries of t that hold for the dataset version in s against it: each stat
// must be in the dataset's stats; a description source must quote a template of that stat in
// the file its text renders from in the entry's scope, which is files[scope] or, when that
// file does not describe the stat, the nearest parent file that does; an object-template
// source's stat line must hold for every character; a game-data source must name the
// Characters column resolve carries as the entry's stat. Problems are joined.
func (t *Table) Check(s *dataset.Store, files map[resolve.Scope]string) error {
	m := s.Manifest()
	if m.Game != t.Game {
		return fmt.Errorf("semantics: %w: table %s, dataset %s", ErrOtherGame, t.Game, m.Game)
	}
	version, err := dataset.ParseGameVersion(m.Version)
	if err != nil {
		return fmt.Errorf("semantics: dataset %w", err)
	}
	stats, err := s.Stats()
	if err != nil {
		return err
	}
	chars, err := s.Characters()
	if err != nil {
		return err
	}
	known := make(map[string]bool, len(stats))
	for _, st := range stats {
		known[st.ID] = true
	}
	var text *dataset.Text
	if slices.Contains(m.Languages(), sourceLanguage) {
		if text, err = s.Text(sourceLanguage); err != nil {
			return err
		}
	}
	var errs []error
	for i := range t.Entries {
		e := &t.Entries[i]
		if !e.Holds(version) {
			continue
		}
		if !known[e.Stat] {
			errs = append(errs, fmt.Errorf("semantics: %s: %w %s", e, ErrUnknownStat, m.Version))
		}
		for _, src := range e.Sources {
			var ok bool
			switch src.Kind {
			case Description:
				if text == nil {
					errs = append(errs, fmt.Errorf("semantics: %s: %w", e, ErrTextNotChecked))
					continue
				}
				ok = src.Ref == renderedFrom(text, files[e.Scope], e.Stat) && describes(text, src, e.Stat)
			case ObjectTemplate:
				ok = everyCharacterHas(stats, chars, src.Text)
			case GameData:
				ok = carriesColumn(src.Ref, e.Stat)
			default:
				continue
			}
			if !ok {
				errs = append(errs, fmt.Errorf("semantics: %s: %w %s: %s %s %q", e, ErrSourceMismatch, m.Version, src.Kind, src.Ref, src.Text))
			}
		}
	}
	return errors.Join(errs...)
}

// renderedFrom returns the file a stat's text renders from when looked up in file: file or
// its nearest parent that describes the stat, or "" when none does.
func renderedFrom(text *dataset.Text, file, stat string) string {
	byName := make(map[string]*statdesc.Scope, len(text.Descriptions))
	for i := range text.Descriptions {
		byName[text.Descriptions[i].Name] = &text.Descriptions[i]
	}
	// Bounded by the number of files, so a parent cycle ends.
	for range text.Descriptions {
		sc := byName[file]
		if sc == nil {
			return ""
		}
		if slices.ContainsFunc(sc.Descriptions, func(d statdesc.Description) bool { return slices.Contains(d.Stats, stat) }) {
			return file
		}
		file = sc.Parent
	}
	return ""
}

// describes reports whether a description of stat in file src.Ref has a variant with text
// src.Text and handlers src.Handlers.
func describes(text *dataset.Text, src Source, stat string) bool {
	i := slices.IndexFunc(text.Descriptions, func(sc statdesc.Scope) bool { return sc.Name == src.Ref })
	if i < 0 {
		return false
	}
	for _, d := range text.Descriptions[i].Descriptions {
		if !slices.Contains(d.Stats, stat) {
			continue
		}
		for _, va := range d.Variants {
			names := make([]string, len(va.Handlers))
			for j, h := range va.Handlers {
				names[j] = h.Name
			}
			if va.Text == src.Text && slices.Equal(names, src.Handlers) {
				return true
			}
		}
	}
	return false
}

// everyCharacterHas reports whether every character's template stats hold the stat line.
func everyCharacterHas(stats []dataset.Stat, chars []dataset.Character, line string) bool {
	stat, value, err := templateLine(line)
	if err != nil || len(chars) == 0 {
		return false
	}
	for i := range chars {
		if !slices.ContainsFunc(chars[i].Stats, func(sv dataset.StatValue) bool { return stats[sv.Stat].ID == stat && sv.Value == value }) {
			return false
		}
	}
	return true
}

// carriesColumn reports whether ref is "Characters.Column" for the column resolve carries as
// stat.
func carriesColumn(ref, stat string) bool {
	column, ok := strings.CutPrefix(ref, "Characters.")
	return ok && slices.ContainsFunc(resolve.CharacterColumns(), func(c resolve.CharacterColumn) bool { return c.Column == column && c.Stat == stat })
}
