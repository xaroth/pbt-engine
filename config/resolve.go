package config

import (
	"encoding/json"
	"fmt"
)

// Source says where a resolved value came from.
type Source uint8

// Sources, from lowest to highest priority.
const (
	Default Source = iota
	FromBuild
	FromSim
	FromCall
)

func (s Source) String() string {
	switch s {
	case Default:
		return "default"
	case FromBuild:
		return "build"
	case FromSim:
		return "sim"
	case FromCall:
		return "call"
	}
	return "source?"
}

// Spec describes one key to Resolve: its name, default, values and decoder.
type Spec struct {
	Name    string
	Default any
	// Values are the values the key takes, in display order; nil when it is not a closed set.
	Values []any
	decode func(json.RawMessage) (any, error)
}

// SpecOf returns the Spec of k with default def and, for a closed set, its values.
func SpecOf[T any](k Key[T], def T, values ...T) Spec {
	var vs []any
	if len(values) > 0 {
		vs = make([]any, len(values))
		for i, v := range values {
			vs[i] = v
		}
	}
	return Spec{Name: k.Name(), Default: def, Values: vs, decode: func(raw json.RawMessage) (any, error) { return k.Decode(raw) }}
}

// Layer is one set of values and where it comes from.
type Layer struct {
	Source Source
	Values Values
}

// Entry is a resolved value and its source.
type Entry struct {
	Value  any
	Source Source
}

// Resolved holds one entry per spec.
type Resolved map[string]Entry

// Problem is a value Resolve ignored: an unknown key or a value that does not decode. The key
// keeps the value of a lower layer or its default.
type Problem struct {
	Key    string
	Source Source
	Err    error
}

func (p Problem) String() string { return fmt.Sprintf("%s config %s: %v", p.Source, p.Key, p.Err) }

// Resolve gives each spec's key the value of the highest-priority layer that has a valid one,
// else its default. Values for keys no spec names, and values that do not decode, are
// returned as problems, in layer and name order.
func Resolve(specs []Spec, layers ...Layer) (Resolved, []Problem) {
	out := make(Resolved, len(specs))
	known := make(map[string]Spec, len(specs))
	for _, s := range specs {
		known[s.Name] = s
		out[s.Name] = Entry{Value: s.Default, Source: Default}
	}
	var problems []Problem
	for _, l := range layers {
		for _, name := range l.Values.Names() {
			s, ok := known[name]
			if !ok {
				problems = append(problems, Problem{Key: name, Source: l.Source, Err: ErrUnknownKey})
				continue
			}
			v, err := s.decode(l.Values[name])
			if err != nil {
				problems = append(problems, Problem{Key: name, Source: l.Source, Err: err})
				continue
			}
			if l.Source >= out[name].Source {
				out[name] = Entry{Value: v, Source: l.Source}
			}
		}
	}
	return out, problems
}

// Lookup returns k's resolved value and source; a key r does not hold gives T's zero value
// and Default.
func Lookup[T any](r Resolved, k Key[T]) (T, Source) {
	e, ok := r[k.Name()]
	if !ok {
		var zero T
		return zero, Default
	}
	v, _ := e.Value.(T)
	return v, e.Source
}
