// Package semantics says what game stat ids mean in engine terms. A table, authored per game
// edition from game data, maps (stat id, scope) to engine modifiers, each entry with a status
// and the sources it was authored from. Engine stats and rules belong to the rules code; the
// caller supplies their names when loading.
package semantics

import (
	"bytes"
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/internal/core"
	"github.com/xaroth/pbt-engine/internal/resolve"
)

// Details of ErrInvalid; each is part of the error text.
var (
	errUnknown        = errors.New("unknown")
	errTemplateText   = errors.New("object-template text")
	errFactor         = errors.New("factor")
	errNotSorted      = errors.New("not sorted by stat and scope")
	errOverlap        = errors.New("versions overlap")
	errNoStat         = errors.New("no stat")
	errNotBeforeUntil = errors.New("not before until")
	errReasonNoText   = errors.New("reason without text")
	errNoSources      = errors.New("no sources")
	errAuthoredNoMods = errors.New("authored without mods or rule")
	errParams         = errors.New("params")
	errAuthoredReason = errors.New("authored with a reason")
	errUnknownReason  = errors.New("unknown needs a reason other than")
	errUnknownRule    = errors.New("unknown names a rule exactly when its reason is rule")
	errDisplayReason  = errors.New("display-only needs reason")
	errDisplayRule    = errors.New("display-only with a rule")
	errWithMods       = errors.New("with mods")
	errOnlyAuthored   = errors.New("with unvalidated, omitted or unchecked; only authored entries have them")
	errStepPositive   = errors.New("want > 0")
	errCount          = errors.New("want a counter and atLeast > 0")
	errStatIDRefText  = errors.New("stat-id with ref or text")
	errWithoutRef     = errors.New("without ref")
	errWithoutText    = errors.New("without text")
	errWithHandlers   = errors.New("with handlers")
	errWithText       = errors.New("with text")
	errEmptyTerm      = errors.New("empty term")
)

// Format is the table file format Load reads.
const Format = 1

// ErrInvalid is wrapped by every error Load returns for a malformed table.
var ErrInvalid = errors.New("invalid semantics table")

// Vocabulary names what a table may refer to: the engine stats, rules and context values the
// rules code defines. RuleParams returns the parameters a rule takes, each naming an engine
// stat, and false for an unknown rule. IsContext reports whether name is a context value a
// modifier may require, as "dimension.value".
type Vocabulary interface {
	IsEngineStat(name string) bool
	RuleParams(name string) (params []string, ok bool)
	IsContext(name string) bool
}

// Status says how far the meaning of a stat is known.
type Status uint8

// Statuses.
const (
	// Authored: the entry's modifiers, and its rule if it names one, are the stat's effect on
	// engine stats, except what Omitted lists, and apply whether or not what Unchecked names
	// holds.
	Authored Status = iota + 1
	// Unknown: the stat has an effect the table does not express yet; Reason says why.
	Unknown
	// DisplayOnly: the stat only shows text; its effect comes from other stats.
	DisplayOnly
)

func statusNames() []string {
	return []string{Authored: "authored", Unknown: "unknown", DisplayOnly: "display-only"}
}

func (s Status) String() string { return name(statusNames(), int(s), "status?") }

// ReasonKind classifies why an entry is Unknown or DisplayOnly.
type ReasonKind uint8

// Reason kinds. Display is the only kind of a DisplayOnly entry.
const (
	// Condition: the effect depends on scenario state, such as being hit recently.
	Condition ReasonKind = iota + 1
	// Items: the effect depends on equipped items.
	Items
	// Charges: the effect scales with charges.
	Charges
	// Reservation: the effect depends on reserved life or mana.
	Reservation
	// Scale: the effect scales with a value that is not an engine stat, such as a count of
	// allocated masteries.
	Scale
	// Context: the effect applies in a context, such as against damage over time, that a
	// sheet value does not have.
	Context
	// Event: the effect recovers on an event or period instead of as a rate.
	Event
	// Rule: the effect needs rules code; Entry.Rule names it.
	Rule
	// Unestablished: game data does not settle what the stat does.
	Unestablished
	// Display: the stat only shows text.
	Display
)

func reasonKindNames() []string {
	return []string{
		Condition: "condition", Items: "items", Charges: "charges", Reservation: "reservation", Scale: "scale",
		Context: "context", Event: "event", Rule: "rule", Unestablished: "unestablished", Display: "display",
	}
}

func (k ReasonKind) String() string { return name(reasonKindNames(), int(k), "reason?") }

// Reason says why an entry is Unknown or DisplayOnly.
type Reason struct {
	Kind ReasonKind
	Text string
}

func (r Reason) String() string { return r.Kind.String() + ": " + r.Text }

// SourceKind is the kind of game source an entry was authored from.
type SourceKind uint8

// Source kinds.
const (
	// Description: a stat description template. Ref is the description file as the dataset
	// names it, Text the template text and Handlers its value handlers, in order.
	Description SourceKind = iota + 1
	// ObjectTemplate: an object template (.ot) file every character extends. Ref is its path,
	// Text a stat line "stat = value".
	ObjectTemplate
	// GameData: a Characters column, Ref "Characters.Column", that resolve carries as the
	// entry's stat.
	GameData
	// StatID: the stat id itself, for stats without description text. Ref and Text are empty.
	StatID
)

func sourceKindNames() []string {
	return []string{Description: "description", ObjectTemplate: "object-template", GameData: "game-data", StatID: "stat-id"}
}

func (k SourceKind) String() string { return name(sourceKindNames(), int(k), "source?") }

func name(names []string, i int, unknown string) string {
	if i > 0 && i < len(names) {
		return names[i]
	}
	return unknown
}

func parseName[T ~uint8](names []string, s, what string) (T, error) {
	if i := slices.Index(names, s); i > 0 {
		return T(i), nil
	}
	return 0, fmt.Errorf("%w %s %q", errUnknown, what, s)
}

// Source is a game source of an entry.
type Source struct {
	Kind     SourceKind
	Ref      string
	Text     string
	Handlers []string
}

// templateLine parses an ObjectTemplate source's Text.
func templateLine(text string) (stat string, value int32, err error) {
	stat, v, ok := strings.Cut(text, " = ")
	n, perr := strconv.ParseInt(v, 10, 32)
	if !ok || stat == "" || perr != nil {
		return "", 0, fmt.Errorf("%w %q: want \"stat = value\"", errTemplateText, text)
	}
	return stat, int32(n), nil
}

// Ratio is the factor from a game stat value to a modifier value, Num/Den.
type Ratio struct {
	Num, Den int32
}

// Apply returns the modifier value for game stat value v.
func (r Ratio) Apply(v float64) float64 { return v * float64(r.Num) / float64(r.Den) }

func (r Ratio) String() string { return fmt.Sprintf("%d/%d", r.Num, r.Den) }

func parseRatio(s string) (Ratio, error) {
	if s == "" {
		return Ratio{1, 1}, nil
	}
	num, den, hasDen := strings.Cut(s, "/")
	if !hasDen {
		den = "1"
	}
	n, err1 := strconv.ParseInt(num, 10, 32)
	d, err2 := strconv.ParseInt(den, 10, 32)
	if err1 != nil || err2 != nil || d <= 0 {
		return Ratio{}, fmt.Errorf("%w %q: want n or n/d with d > 0", errFactor, s)
	}
	return Ratio{int32(n), int32(d)}, nil
}

// Per scales a modifier by the value of engine stat Stat in steps of Step, as in "+1 Armour
// per 2 Strength". The interpret stage counts whole steps (core.Scale); an entry whose text
// does not establish that says so in Unvalidated.
type Per struct {
	Stat string
	Step int32
}

// Count holds a modifier until the dataset counter Stat reaches AtLeast, as in "10% more
// Maximum Life if you have at least 6 Life Masteries allocated". A counter is a stat the
// dataset counts rather than an engine stat: a mastery group's count stat, 1 per allocated
// mastery of the group.
type Count struct {
	Stat    string
	AtLeast int32
}

// ContextFilter restricts a modifier to the contexts it applies in, by context value names
// ("dimension.value"). A context matches when it has at least one value of every term of In
// and none of NotIn: "Elemental Damage with Attack Skills" is In [[type.Attack], [damage.fire,
// damage.cold, damage.lightning]]. The zero ContextFilter matches every context, the sheet's
// included; one with a term matches no sheet value, since the sheet's context has no values.
type ContextFilter struct {
	In    [][]string
	NotIn []string
}

// IsZero reports whether c matches every context.
func (c ContextFilter) IsZero() bool { return len(c.In) == 0 && len(c.NotIn) == 0 }

// Mod is one engine modifier a stat gives: engine stat, op and value as the game stat value
// times Factor, scaled by Per when set, held only while If holds when set, in the contexts
// Context matches. Only an override has a zero Factor, for effects such as "Removes all Mana".
type Mod struct {
	Stat    string
	Op      core.Op
	Factor  Ratio
	Per     *Per
	If      *Count
	Context ContextFilter
}

// Entry is the meaning of one stat in one scope over a range of game versions.
type Entry struct {
	Stat  string
	Scope resolve.Scope
	// From and Until bound the game versions the entry holds for, Until excluded. The zero
	// GameVersion leaves that end open.
	From, Until dataset.GameVersion
	Status      Status
	// Mods are set only when Status is Authored.
	Mods []Mod
	// Rule names the rules code that carries the effect, on an Authored entry or an Unknown
	// one whose Reason is Rule. The rule reads the stat's value.
	Rule string
	// Params are the rule's arguments, parameter name to engine stat, such as "from":
	// "maximum_life" and "to": "armour" for "Gain X% of Maximum Life as Extra Armour". An
	// Authored entry with a rule sets every parameter the rule takes; an Unknown one may leave
	// them out.
	Params map[string]string
	// RuleFactor turns the stat's value into the value the rule reads, as Mod.Factor does for
	// a modifier: Iron Reflexes is a stat of value 1 that converts 100%. 1/1 when the entry
	// sets none, zero when it has no rule.
	RuleFactor Ratio
	Sources    []Source
	// Reason is set when Status is Unknown or DisplayOnly.
	Reason Reason
	// Unvalidated, on an Authored entry, says what its sources do not establish and what
	// would validate it. Empty means the sources establish every modifier.
	Unvalidated string
	// Omitted, on an Authored entry, lists the effects of the stat's text that the modifiers
	// do not carry, such as a keystone's effects outside the engine stats. Empty means none.
	Omitted string
	// Unchecked, on an Authored entry, names a condition of the stat's text that the modifiers
	// apply without checking, and why that is right for now. Unlike Omitted, the entry can
	// over-apply once the condition can fail. Empty means none.
	Unchecked string
}

// Holds reports whether the entry holds for game version v.
func (e *Entry) Holds(v dataset.GameVersion) bool {
	return (e.From.IsZero() || v.Compare(e.From) >= 0) && (e.Until.IsZero() || v.Compare(e.Until) < 0)
}

func (e *Entry) String() string {
	s := e.Stat + " " + e.Scope.String()
	if !e.From.IsZero() || !e.Until.IsZero() {
		s += " [" + e.From.String() + ", " + e.Until.String() + ")"
	}
	return s
}

// Table is the semantics of one game edition. Entries are sorted by stat, scope (global before
// buff) and From; the ranges of one stat and scope do not overlap.
type Table struct {
	Game    string
	Entries []Entry
}

// Lookup returns the entry of stat in scope for game version v.
func (t *Table) Lookup(stat string, scope resolve.Scope, v dataset.GameVersion) (*Entry, bool) {
	i, _ := slices.BinarySearchFunc(t.Entries, stat, func(e Entry, s string) int { return cmp.Compare(e.Stat, s) })
	for ; i < len(t.Entries) && t.Entries[i].Stat == stat; i++ {
		if e := &t.Entries[i]; e.Scope == scope && e.Holds(v) {
			return e, true
		}
	}
	return nil, false
}

type fileTable struct {
	Format  int         `json:"format"`
	Game    string      `json:"game"`
	Entries []fileEntry `json:"entries"`
}

type fileEntry struct {
	Stat        string            `json:"stat"`
	Scope       string            `json:"scope"`
	From        string            `json:"from"`
	Until       string            `json:"until"`
	Status      string            `json:"status"`
	Mods        []fileMod         `json:"mods"`
	Rule        string            `json:"rule"`
	Params      map[string]string `json:"params"`
	RuleFactor  string            `json:"ruleFactor"`
	Sources     []fileSource      `json:"sources"`
	Reason      *fileReason       `json:"reason"`
	Unvalidated string            `json:"unvalidated"`
	Omitted     string            `json:"omitted"`
	Unchecked   string            `json:"unchecked"`
}

type fileReason struct {
	Kind string `json:"kind"`
	Text string `json:"text"`
}

type fileMod struct {
	Stat   string     `json:"stat"`
	Op     string     `json:"op"`
	Factor string     `json:"factor"`
	Per    *filePer   `json:"per"`
	If     *fileCount `json:"if"`
	In     [][]string `json:"in"`
	NotIn  []string   `json:"notIn"`
}

type fileCount struct {
	Stat    string `json:"count"`
	AtLeast int32  `json:"atLeast"`
}

type filePer struct {
	Stat string `json:"stat"`
	Step int32  `json:"step"`
}

type fileSource struct {
	Kind     string   `json:"kind"`
	Ref      string   `json:"ref"`
	Text     string   `json:"text"`
	Handlers []string `json:"handlers"`
}

// Load parses a table in JSON. Every engine stat and rule it names must be in vocab. Errors
// wrap ErrInvalid.
func Load(data []byte, vocab Vocabulary) (*Table, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var f fileTable
	if err := dec.Decode(&f); err != nil {
		return nil, fmt.Errorf("semantics: %w: %w", ErrInvalid, err)
	}
	// More would accept a trailing } or ].
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("semantics: %w: data after the table", ErrInvalid)
	}
	if f.Format != Format {
		return nil, fmt.Errorf("semantics: %w: format %d, want %d", ErrInvalid, f.Format, Format)
	}
	if f.Game == "" {
		return nil, fmt.Errorf("semantics: %w: no game", ErrInvalid)
	}
	t := &Table{Game: f.Game, Entries: make([]Entry, 0, len(f.Entries))}
	for i := range f.Entries {
		e, err := parseEntry(&f.Entries[i], vocab)
		if err != nil {
			return nil, fmt.Errorf("semantics: %w: entry %d (%s): %w", ErrInvalid, i, f.Entries[i].Stat, err)
		}
		if i > 0 {
			if err := checkOrder(&t.Entries[i-1], &e); err != nil {
				return nil, fmt.Errorf("semantics: %w: entry %d (%s): %w", ErrInvalid, i, e.String(), err)
			}
		}
		t.Entries = append(t.Entries, e)
	}
	return t, nil
}

func checkOrder(prev, e *Entry) error {
	c := cmp.Or(cmp.Compare(prev.Stat, e.Stat), cmp.Compare(prev.Scope, e.Scope))
	switch {
	case c > 0:
		return errNotSorted
	case c < 0:
		return nil
	case e.From.IsZero() || prev.Until.IsZero() || prev.Until.Compare(e.From) > 0:
		return fmt.Errorf("%w %s", errOverlap, prev.String())
	}
	return nil
}

func parseScope(name string) (resolve.Scope, error) {
	for _, s := range []resolve.Scope{resolve.Global, resolve.Buff} {
		if s.String() == name {
			return s, nil
		}
	}
	return 0, fmt.Errorf("%w scope %q", errUnknown, name)
}

func parseOp(name string) (core.Op, error) {
	for _, o := range []core.Op{core.OpBase, core.OpInc, core.OpMore, core.OpOverride, core.OpFlag} {
		if o.String() == name {
			return o, nil
		}
	}
	return 0, fmt.Errorf("%w op %q", errUnknown, name)
}

func parseBound(s string) (dataset.GameVersion, error) {
	if s == "" {
		return dataset.GameVersion{}, nil
	}
	v, err := dataset.ParseGameVersion(s)
	if err != nil {
		return dataset.GameVersion{}, fmt.Errorf("game version: %w", err)
	}
	return v, nil
}

func parseEntry(f *fileEntry, vocab Vocabulary) (Entry, error) {
	e, err := parseEntryHeader(f)
	if err != nil {
		return Entry{}, err
	}
	if err := parseReason(&e, f); err != nil {
		return Entry{}, err
	}
	if err := checkStatus(&e, len(f.Mods)); err != nil {
		return Entry{}, err
	}
	if err := parseRule(&e, f, vocab); err != nil {
		return Entry{}, err
	}
	if err := parseMods(&e, f, vocab); err != nil {
		return Entry{}, err
	}
	if err := parseSources(&e, f); err != nil {
		return Entry{}, err
	}
	return e, nil
}

// parseRule checks the entry's rule, rule factor and params against vocab: params name engine
// stats, only with a rule; an authored entry sets every parameter the rule takes, an unknown one
// all or none. The rule factor is a non-zero ratio, only with a rule.
func parseRule(e *Entry, f *fileEntry, vocab Vocabulary) error {
	if e.Rule == "" {
		switch {
		case len(f.Params) > 0:
			return fmt.Errorf("%w without a rule", errParams)
		case f.RuleFactor != "":
			return fmt.Errorf("rule %w without a rule", errFactor)
		}
		return nil
	}
	var err error
	if e.RuleFactor, err = parseRuleFactor(f.RuleFactor); err != nil {
		return err
	}
	want, ok := vocab.RuleParams(e.Rule)
	if !ok {
		return fmt.Errorf("%w rule %q", errUnknown, e.Rule)
	}
	if len(f.Params) == 0 && e.Status != Authored {
		return nil
	}
	if err := checkParams(e.Rule, f.Params, want, vocab); err != nil {
		return err
	}
	e.Params = maps.Clone(f.Params)
	return nil
}

// checkParams checks that params sets exactly the parameters want, each to an engine stat.
func checkParams(rule string, params map[string]string, want []string, vocab Vocabulary) error {
	got, want := slices.Sorted(maps.Keys(params)), slices.Sorted(slices.Values(want))
	if !slices.Equal(got, want) {
		return fmt.Errorf("%w %v, rule %s takes %v", errParams, got, rule, want)
	}
	for _, k := range got {
		if !vocab.IsEngineStat(params[k]) {
			return fmt.Errorf("%w %s: %w engine stat %q", errParams, k, errUnknown, params[k])
		}
	}
	return nil
}

func parseRuleFactor(s string) (Ratio, error) {
	r, err := parseRatio(s)
	switch {
	case err != nil:
		return Ratio{}, fmt.Errorf("rule %w", err)
	case r.Num == 0:
		return Ratio{}, fmt.Errorf("rule %w 0", errFactor)
	}
	return r, nil
}

func parseEntryHeader(f *fileEntry) (Entry, error) {
	if f.Stat == "" {
		return Entry{}, errNoStat
	}
	e := Entry{Stat: f.Stat, Rule: f.Rule, Unvalidated: f.Unvalidated, Omitted: f.Omitted, Unchecked: f.Unchecked}
	var err error
	if e.Scope, err = parseScope(f.Scope); err != nil {
		return Entry{}, err
	}
	if e.Status, err = parseName[Status](statusNames(), f.Status, "status"); err != nil {
		return Entry{}, err
	}
	if e.From, err = parseBound(f.From); err != nil {
		return Entry{}, err
	}
	if e.Until, err = parseBound(f.Until); err != nil {
		return Entry{}, err
	}
	if !e.From.IsZero() && !e.Until.IsZero() && e.From.Compare(e.Until) >= 0 {
		return Entry{}, fmt.Errorf("from %s %w %s", e.From, errNotBeforeUntil, e.Until)
	}
	return e, nil
}

func parseReason(e *Entry, f *fileEntry) error {
	if f.Reason == nil {
		return nil
	}
	var err error
	if e.Reason.Kind, err = parseName[ReasonKind](reasonKindNames(), f.Reason.Kind, "reason kind"); err != nil {
		return err
	}
	if f.Reason.Text == "" {
		return errReasonNoText
	}
	e.Reason.Text = f.Reason.Text
	return nil
}

func parseMods(e *Entry, f *fileEntry, vocab Vocabulary) error {
	for i := range f.Mods {
		m, err := parseMod(&f.Mods[i], vocab)
		if err != nil {
			return fmt.Errorf("mod %d: %w", i, err)
		}
		e.Mods = append(e.Mods, m)
	}
	return nil
}

func parseSources(e *Entry, f *fileEntry) error {
	if len(f.Sources) == 0 {
		return errNoSources
	}
	for i := range f.Sources {
		s, err := parseSource(&f.Sources[i])
		if err != nil {
			return fmt.Errorf("source %d: %w", i, err)
		}
		e.Sources = append(e.Sources, s)
	}
	return nil
}

// checkStatus checks the fields that depend on e's status.
func checkStatus(e *Entry, mods int) error {
	var err error
	switch e.Status {
	case Authored:
		return checkAuthored(e, mods)
	case Unknown:
		err = checkUnknown(e)
	case DisplayOnly:
		err = checkDisplayOnly(e)
	}
	if err != nil {
		return err
	}
	switch {
	case mods > 0:
		return fmt.Errorf("%s %w", e.Status, errWithMods)
	case e.Unvalidated != "" || e.Omitted != "" || e.Unchecked != "":
		return fmt.Errorf("%s %w", e.Status, errOnlyAuthored)
	}
	return nil
}

func checkAuthored(e *Entry, mods int) error {
	switch {
	case mods == 0 && e.Rule == "":
		return errAuthoredNoMods
	case e.Reason.Kind != 0:
		return errAuthoredReason
	}
	return nil
}

func checkUnknown(e *Entry) error {
	switch {
	case e.Reason.Kind == 0 || e.Reason.Kind == Display:
		return fmt.Errorf("%w %s", errUnknownReason, Display)
	case (e.Reason.Kind == Rule) != (e.Rule != ""):
		return errUnknownRule
	}
	return nil
}

func checkDisplayOnly(e *Entry) error {
	switch {
	case e.Reason.Kind != Display:
		return fmt.Errorf("%w %s", errDisplayReason, Display)
	case e.Rule != "":
		return errDisplayRule
	}
	return nil
}

func parseMod(f *fileMod, vocab Vocabulary) (Mod, error) {
	if !vocab.IsEngineStat(f.Stat) {
		return Mod{}, fmt.Errorf("%w engine stat %q", errUnknown, f.Stat)
	}
	op, err := parseOp(f.Op)
	if err != nil {
		return Mod{}, err
	}
	factor, err := parseRatio(f.Factor)
	if err != nil {
		return Mod{}, err
	}
	if factor.Num == 0 && op != core.OpOverride {
		return Mod{}, fmt.Errorf("%w %s on %s; only override may set a value of 0", errFactor, factor, op)
	}
	m := Mod{Stat: f.Stat, Op: op, Factor: factor}
	if m.Per, err = parsePer(f.Per, vocab); err != nil {
		return Mod{}, err
	}
	if m.If, err = parseCount(f.If); err != nil {
		return Mod{}, err
	}
	if m.Context, err = parseContext(f.In, f.NotIn, vocab); err != nil {
		return Mod{}, err
	}
	return m, nil
}

// parseContext checks that every name is a context value of vocab and no term is empty.
func parseContext(in [][]string, notIn []string, vocab Vocabulary) (ContextFilter, error) {
	for i, term := range in {
		if len(term) == 0 {
			return ContextFilter{}, fmt.Errorf("in: %w %d", errEmptyTerm, i)
		}
		for _, n := range term {
			if !vocab.IsContext(n) {
				return ContextFilter{}, fmt.Errorf("in: %w context %q", errUnknown, n)
			}
		}
	}
	for _, n := range notIn {
		if !vocab.IsContext(n) {
			return ContextFilter{}, fmt.Errorf("notIn: %w context %q", errUnknown, n)
		}
	}
	c := ContextFilter{NotIn: slices.Clone(notIn)}
	for _, term := range in {
		c.In = append(c.In, slices.Clone(term))
	}
	return c, nil
}

func parsePer(f *filePer, vocab Vocabulary) (*Per, error) {
	switch {
	case f == nil:
		return nil, nil //nolint:nilnil // no per is valid
	case !vocab.IsEngineStat(f.Stat):
		return nil, fmt.Errorf("per: %w engine stat %q", errUnknown, f.Stat)
	case f.Step <= 0:
		return nil, fmt.Errorf("per %s: step %d, %w", f.Stat, f.Step, errStepPositive)
	}
	return &Per{Stat: f.Stat, Step: f.Step}, nil
}

func parseCount(f *fileCount) (*Count, error) {
	switch {
	case f == nil:
		return nil, nil //nolint:nilnil // no condition is valid
	case f.Stat == "" || f.AtLeast <= 0:
		return nil, fmt.Errorf("if: %w: count %q, atLeast %d", errCount, f.Stat, f.AtLeast)
	}
	return &Count{Stat: f.Stat, AtLeast: f.AtLeast}, nil
}

func parseSource(f *fileSource) (Source, error) {
	kind, err := parseName[SourceKind](sourceKindNames(), f.Kind, "kind")
	if err != nil {
		return Source{}, err
	}
	s := Source{Kind: kind, Ref: f.Ref, Text: f.Text, Handlers: f.Handlers}
	if err := checkSourceRef(&s); err != nil {
		return Source{}, err
	}
	if err := checkSourceText(&s); err != nil {
		return Source{}, err
	}
	if s.Kind == ObjectTemplate {
		if _, _, err := templateLine(s.Text); err != nil {
			return Source{}, err
		}
	}
	return s, nil
}

func checkSourceRef(s *Source) error {
	switch {
	case s.Kind == StatID && (s.Ref != "" || s.Text != ""):
		return errStatIDRefText
	case s.Kind != StatID && s.Ref == "":
		return fmt.Errorf("%s %w", s.Kind, errWithoutRef)
	}
	return nil
}

func checkSourceText(s *Source) error {
	switch {
	case s.Kind == Description && s.Text == "":
		return fmt.Errorf("%s %w", s.Kind, errWithoutText)
	case s.Kind != Description && len(s.Handlers) > 0:
		return fmt.Errorf("%s %w", s.Kind, errWithHandlers)
	case s.Kind == GameData && s.Text != "":
		return fmt.Errorf("%s %w", s.Kind, errWithText)
	}
	return nil
}
