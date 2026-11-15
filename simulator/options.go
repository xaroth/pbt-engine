package simulator

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/xaroth/pbt-engine/config"
	"github.com/xaroth/pbt-engine/output"
)

// Errors of options; match them with errors.Is.
var (
	// ErrOption is an option that does not apply where it was given, such as Outputs to
	// Configure, or a Set whose value fails its key's check.
	ErrOption = errors.New("simulator: bad option")
	// ErrNotSkillOutput is a RankBySkill key that is not an output of the skill group.
	ErrNotSkillOutput = errors.New("simulator: not a skill output")
	// ErrNoSkill is a RankBySkill ref at which the base build has no skill that resolves.
	ErrNoSkill = errors.New("simulator: no skill at ref")
)

type optionKind uint8

const (
	optSet optionKind = iota + 1
	optClear
	optOutputs
	optRankBy
	optWorkers
	optEverySkill
)

// Option configures a sim (Configure) or one call (Simulate, Compare).
type Option struct {
	kind    optionKind
	key     string
	values  config.Values
	outputs []output.Key
	skill   *SkillRef
	workers int
	err     error
}

// Set sets config key k to v: on the sim with Configure, or for one call. A value set on the
// sim wins over the build's, so different builds compare on equal terms.
func Set[T any](k config.Key[T], v T) Option {
	var vals config.Values
	err := config.Set(&vals, k, v)
	return Option{kind: optSet, key: k.Name(), values: vals, err: err}
}

// SetRaw sets the named key to a JSON value, for hosts that send keys and values as text. An
// unknown key or a value that does not decode is a warning of the run, as in a build.
func SetRaw(name string, value json.RawMessage) Option {
	return Option{kind: optSet, key: name, values: config.Values{name: slices.Clone(value)}}
}

// Clear removes the named key from the sim's config with Configure; for one call it ignores
// the sim's value of the key, so the build's or the default applies.
func Clear(name string) Option { return Option{kind: optClear, key: name} }

// Outputs limits a call to the listed outputs; without it every output is computed.
func Outputs(keys ...output.Key) Option {
	return Option{kind: optOutputs, outputs: append([]output.Key(nil), keys...)}
}

// RankBy sorts a sweep's results by the change of output k, largest first; ties keep variant
// order. The output is computed even when Outputs does not list it. A call fails with
// ErrUnknownOutput for k.
func RankBy(k output.Key) Option { return Option{kind: optRankBy, outputs: []output.Key{k}} }

// RankBySkill is RankBy for output k of the skill at ref, an output of the skill group. That
// skill's outputs are computed as with EverySkill; a variant without the skill sorts last. A
// call fails with ErrUnknownOutput or ErrNotSkillOutput for k, and a sweep with ErrNoSkill when
// the base build has no skill at ref.
func RankBySkill(ref SkillRef, k output.Key) Option {
	return Option{kind: optRankBy, outputs: []output.Key{k}, skill: &ref}
}

// EverySkill makes a call compute the outputs of the skill group of each skill of the build,
// the main skill included (Stats.Skills), besides those of the main skill it always computes.
// Each skill costs a reading of those outputs.
func EverySkill() Option { return Option{kind: optEverySkill} }

// Workers sets how many goroutines a sweep runs, each on its own sheet; 0 or less means
// runtime.GOMAXPROCS. Results do not depend on it.
func Workers(n int) Option { return Option{kind: optWorkers, workers: n} }

// callOptions are the options of one call: config values set and keys cleared, outputs, the
// skills whose outputs are computed besides the main skill's, and for sweeps the ranking and
// workers.
type callOptions struct {
	set     config.Values
	cleared map[string]bool
	outputs []output.Key
	skills  skillAsk
	rankBy  output.Key
	// rankSkill is the skill rankBy is read in; nil for the outputs themselves.
	rankSkill *SkillRef
	workers   int
}

// skillAsk is which skills a call computes the outputs of: every skill, or those listed.
type skillAsk struct {
	every bool
	refs  []SkillRef
}

// any reports whether the call asks for any skill's outputs.
func (a skillAsk) any() bool { return a.every || len(a.refs) > 0 }

func (a skillAsk) wants(ref SkillRef) bool { return a.every || slices.Contains(a.refs, ref) }

func readCallOptions(opts []Option) (callOptions, error) {
	co := callOptions{set: config.Values{}, cleared: map[string]bool{}}
	for _, o := range opts {
		if o.err != nil {
			return co, fmt.Errorf("%w: %w", ErrOption, o.err)
		}
		switch o.kind {
		case optSet:
			co.set[o.key] = o.values[o.key]
			delete(co.cleared, o.key)
		case optClear:
			co.cleared[o.key] = true
			delete(co.set, o.key)
		case optOutputs:
			co.outputs = append(co.outputs, o.outputs...)
		case optRankBy:
			if err := co.rank(o); err != nil {
				return co, err
			}
		case optEverySkill:
			co.skills.every = true
		case optWorkers:
			co.workers = o.workers
		}
	}
	return co, nil
}

// rank takes a RankBy or RankBySkill option, failing for an unknown key and for a RankBySkill
// key that is not an output of the skill group.
func (co *callOptions) rank(o Option) error {
	k := o.outputs[0]
	co.rankBy, co.rankSkill = k, o.skill
	out, ok := catalogue()[k]
	switch {
	case !ok:
		return fmt.Errorf("%w %q", ErrUnknownOutput, k)
	case o.skill == nil:
		return nil
	case !out.IsSkill():
		return fmt.Errorf("%w: %q", ErrNotSkillOutput, k)
	}
	co.skills.refs = append(co.skills.refs, *o.skill)
	return nil
}
