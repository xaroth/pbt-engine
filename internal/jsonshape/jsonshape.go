// Package jsonshape checks JSON against the shape a JSON Schema of this module gives it, for
// tests. It reads only the keywords these schemas use for shape: object keys against
// properties, required and additionalProperties, JSON types, enums and consts, following $ref,
// items and oneOf. An object is closed unless additionalProperties gives a schema for the
// other keys.
package jsonshape

import (
	"encoding/json"
	"fmt"
	"maps"
	"math"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// Defs reads the definitions of the schema in file as plain JSON.
func Defs(tb testing.TB, file string) map[string]map[string]any {
	tb.Helper()
	data, err := os.ReadFile(file)
	if err != nil {
		tb.Fatal(err)
	}
	var doc struct {
		Defs map[string]map[string]any `json:"$defs"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		tb.Fatalf("%s: %v", file, err)
	}
	return doc.Defs
}

// Validate checks data against the definition def of the schema in file, and reports each
// mismatch as an error of tb.
func Validate(tb testing.TB, file, def string, data []byte) {
	tb.Helper()
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		tb.Fatal(err)
	}
	defs := Defs(tb, file)
	if _, ok := defs[def]; !ok {
		tb.Fatalf("%s has no definition %s", file, def)
	}
	for _, e := range shapeErrors(defs, map[string]any{"$ref": "#/$defs/" + def}, v, def) {
		tb.Errorf("%s: %s", file, e)
	}
}

func shapeErrors(defs map[string]map[string]any, sch map[string]any, v any, path string) []string {
	if ref, ok := sch["$ref"].(string); ok {
		name := strings.TrimPrefix(ref, "#/$defs/")
		def, ok := defs[name]
		if !ok {
			return []string{path + ": no definition " + name}
		}
		return shapeErrors(defs, def, v, path)
	}
	if branches, ok := sch["oneOf"].([]any); ok {
		return shapeErrors(defs, oneOfBranch(branches, v), v, path)
	}
	if e := valueError(sch, v); e != "" {
		return []string{path + ": " + e}
	}
	switch x := v.(type) {
	case map[string]any:
		return objectErrors(defs, sch, x, path)
	case []any:
		var errs []string
		if items, ok := sch["items"].(map[string]any); ok {
			for i, e := range x {
				errs = append(errs, shapeErrors(defs, items, e, path+"["+strconv.Itoa(i)+"]")...)
			}
		}
		return errs
	}
	return nil
}

// valueError checks v against the schema's const, enum and type.
func valueError(sch map[string]any, v any) string {
	if c, ok := sch["const"]; ok && c != v {
		return fmt.Sprint(v) + " is not " + fmt.Sprint(c)
	}
	if enum, ok := sch["enum"].([]any); ok && !slices.Contains(enum, v) {
		return fmt.Sprint(v) + " is not in the enum"
	}
	if types := typeList(sch["type"]); len(types) > 0 && !typeAllowed(types, jsonType(v)) {
		return jsonType(v) + ", want " + strings.Join(types, " or ")
	}
	return ""
}

// objectErrors checks an object's keys against properties, required and
// additionalProperties, and each value.
func objectErrors(defs map[string]map[string]any, sch, x map[string]any, path string) []string {
	var errs []string
	props, _ := sch["properties"].(map[string]any)
	for _, k := range stringList(sch["required"]) {
		if _, ok := x[k]; !ok {
			errs = append(errs, path+": missing "+k)
		}
	}
	other, _ := sch["additionalProperties"].(map[string]any)
	for _, k := range slices.Sorted(maps.Keys(x)) {
		p, ok := props[k].(map[string]any)
		if !ok {
			p = other
		}
		if p == nil {
			errs = append(errs, path+": unknown key "+k)
			continue
		}
		errs = append(errs, shapeErrors(defs, p, x[k], path+"."+k)...)
	}
	return errs
}

// oneOfBranch picks the branch for v: null's for null, else the one whose kind const matches
// v's kind, else the first.
func oneOfBranch(branches []any, v any) map[string]any {
	obj, _ := v.(map[string]any)
	for _, b := range branches {
		br, _ := b.(map[string]any)
		switch {
		case v == nil && br["type"] == "null":
			return br
		case obj != nil && br["$ref"] == nil:
			props, _ := br["properties"].(map[string]any)
			kind, _ := props["kind"].(map[string]any)
			if kind != nil && kind["const"] == obj["kind"] {
				return br
			}
		case v != nil && br["$ref"] != nil:
			return br
		}
	}
	first, _ := branches[0].(map[string]any)
	return first
}

// typeList returns a schema's type keyword as a list.
func typeList(t any) []string {
	if s, ok := t.(string); ok {
		return []string{s}
	}
	return stringList(t)
}

// stringList returns a JSON array of strings, sorted.
func stringList(v any) []string {
	a, _ := v.([]any)
	out := make([]string, 0, len(a))
	for _, x := range a {
		if s, ok := x.(string); ok {
			out = append(out, s)
		}
	}
	slices.Sort(out)
	return out
}

// typeAllowed reports whether JSON type t is one of types; an integer is a number.
func typeAllowed(types []string, t string) bool {
	return slices.Contains(types, t) || t == "integer" && slices.Contains(types, "number")
}

func jsonType(v any) string {
	switch x := v.(type) {
	case nil:
		return "null"
	case bool:
		return "boolean"
	case string:
		return "string"
	case float64:
		if x == math.Trunc(x) {
			return "integer"
		}
		return "number"
	case []any:
		return "array"
	case map[string]any:
		return "object"
	}
	return "unknown"
}
