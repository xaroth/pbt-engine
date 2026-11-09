package simulator

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/xaroth/pbt-engine/config"
	"github.com/xaroth/pbt-engine/output"
)

// ErrOption is an option that does not apply where it was given, such as Outputs to Configure,
// or a Set whose value fails its key's check.
var ErrOption = errors.New("simulator: bad option")

type optionKind uint8

const (
	optSet optionKind = iota + 1
	optClear
	optOutputs
)

// Option configures a sim (Configure) or one call (Simulate, Compare).
type Option struct {
	kind    optionKind
	key     string
	values  config.Values
	outputs []output.Key
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

// callOptions are the options of one call: config values set and keys cleared, and outputs.
type callOptions struct {
	set     config.Values
	cleared map[string]bool
	outputs []output.Key
}

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
		}
	}
	return co, nil
}
