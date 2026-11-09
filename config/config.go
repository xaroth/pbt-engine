// Package config holds configuration values by typed key, and resolves a key across layers
// (per call, sim, build, default), keeping where each value came from. It knows no game: each
// game's rules define their keys and defaults.
package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"slices"
)

// Errors of decoding and resolving; match them with errors.Is.
var (
	ErrUnknownKey = errors.New("config: unknown key")
	ErrBadValue   = errors.New("config: bad value")
)

// Key names a value of type T. Keys are constants, such as
// `const Campaign config.Key[CampaignStage] = "campaign"`.
type Key[T any] string

// Name returns the key's name.
func (k Key[T]) Name() string { return string(k) }

// Validator is implemented by value types with a closed set of values; Decode and Set call it.
type Validator interface {
	Valid() error
}

// Decode decodes raw as a value of k, rejecting unknown fields and values that fail Valid.
func (k Key[T]) Decode(raw json.RawMessage) (T, error) {
	var v T
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&v); err != nil {
		return v, fmt.Errorf("%w: %s: %w", ErrBadValue, k, err)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return v, fmt.Errorf("%w: %s: data after the value", ErrBadValue, k)
	}
	if err := valid(v); err != nil {
		return v, fmt.Errorf("%w: %s: %w", ErrBadValue, k, err)
	}
	return v, nil
}

func valid(v any) error {
	if c, ok := v.(Validator); ok {
		if err := c.Valid(); err != nil {
			return fmt.Errorf("invalid: %w", err)
		}
	}
	return nil
}

// Values holds raw values by key name, as a build stores them: {"campaign": "complete"}.
type Values map[string]json.RawMessage

// Clone returns a deep copy of v.
func (v Values) Clone() Values {
	if v == nil {
		return nil
	}
	out := make(Values, len(v))
	for name, raw := range v {
		out[name] = slices.Clone(raw)
	}
	return out
}

// Names returns the key names in v, sorted.
func (v Values) Names() []string { return slices.Sorted(maps.Keys(v)) }

// Set stores x under k in *v, creating the map; a value that fails Valid is an error.
func Set[T any](v *Values, k Key[T], x T) error {
	if err := valid(x); err != nil {
		return fmt.Errorf("%w: %s: %w", ErrBadValue, k, err)
	}
	raw, err := json.Marshal(x)
	if err != nil {
		return fmt.Errorf("%w: %s: %w", ErrBadValue, k, err)
	}
	if *v == nil {
		*v = Values{}
	}
	(*v)[k.Name()] = raw
	return nil
}

// Get returns the value under k, and false when v has none.
func Get[T any](v Values, k Key[T]) (T, bool, error) {
	raw, ok := v[k.Name()]
	if !ok {
		var zero T
		return zero, false, nil
	}
	x, err := k.Decode(raw)
	return x, true, err
}

// Clear removes the value of the named key.
func (v Values) Clear(name string) { delete(v, name) }
