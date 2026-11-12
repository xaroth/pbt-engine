package simulator_test

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/simulator"
)

// schemaDefs reads the schema's definitions as plain JSON.
func schemaDefs(tb testing.TB) map[string]map[string]any {
	tb.Helper()
	data, err := os.ReadFile(schemaFile)
	if err != nil {
		tb.Fatal(err)
	}
	var doc struct {
		Defs map[string]map[string]any `json:"$defs"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		tb.Fatal(err)
	}
	return doc.Defs
}

// The schema's definitions of Go types the wire carries as they are have the fields
// encoding/json writes for them: the same names, required where Go always writes them, of the
// same kind. A field added to one of these types fails here until the schema has it.
func TestWireSchemaMatchesGoTypes(t *testing.T) {
	t.Parallel()
	defs := schemaDefs(t)
	types := []reflect.Type{
		reflect.TypeFor[build.Edit](), reflect.TypeFor[build.SkillAt](), reflect.TypeFor[build.SkillGroup](),
		reflect.TypeFor[build.Gem](), reflect.TypeFor[build.Item](), reflect.TypeFor[build.ModLine](),
		reflect.TypeFor[build.StatValue](), reflect.TypeFor[build.Warning](), reflect.TypeFor[simulator.Output](),
	}
	for _, typ := range types {
		t.Run(typ.Name(), func(t *testing.T) {
			t.Parallel()
			checkDef(t, typ, defs)
		})
	}
}

// checkDef compares the definition of typ's name with the fields encoding/json writes for typ.
func checkDef(t *testing.T, typ reflect.Type, defs map[string]map[string]any) {
	t.Helper()
	def, ok := defs[typ.Name()]
	if !ok {
		t.Fatalf("schema has no definition %s", typ.Name())
	}
	want := map[string]string{}
	var required []string
	for _, f := range jsonFields(typ) {
		want[f.name] = goKind(f.typ, defs)
		if f.required {
			required = append(required, f.name)
		}
	}
	props, _ := def["properties"].(map[string]any)
	got := map[string]string{}
	for name, p := range props {
		got[name] = schemaKind(p.(map[string]any), defs)
		// A plain Go string may be any string enum of the schema, such as Unit.
		if want[name] == "string" && strings.HasPrefix(got[name], "enum:") {
			got[name] = "string"
		}
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("fields and kinds (-go +schema):\n%s", diff)
	}
	slices.Sort(required)
	if diff := cmp.Diff(required, sortedStrings(def["required"])); diff != "" {
		t.Errorf("required fields (-go +schema):\n%s", diff)
	}
	if def["additionalProperties"] != false {
		t.Error("additionalProperties is not false")
	}
}

// sortedStrings returns a JSON array of strings sorted, nil for none.
func sortedStrings(v any) []string {
	a, _ := v.([]any)
	if len(a) == 0 {
		return nil
	}
	out := make([]string, 0, len(a))
	for _, x := range a {
		out = append(out, x.(string))
	}
	slices.Sort(out)
	return out
}

// The schema's enums of build's string types list exactly the constants build declares of them.
func TestWireSchemaEnums(t *testing.T) {
	t.Parallel()
	defs := schemaDefs(t)
	consts := typedConsts(t, "../build")
	for _, name := range []string{"EditOp", "Slot", "Rarity", "ModKind"} {
		got := sortedStrings(defs[name]["enum"])
		want := slices.Sorted(slices.Values(consts[name]))
		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("%s (-build +schema):\n%s", name, diff)
		}
	}
}

// Every config key's default and values fit the schema's config value.
func TestWireConfigValues(t *testing.T) {
	t.Parallel()
	keys, err := newSim(t).ConfigKeys()
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range keys {
		for _, v := range append([]json.RawMessage{k.Default}, k.Values...) {
			validate(t, "ConfigValue", v)
		}
	}
}

type jsonField struct {
	name     string
	required bool
	typ      reflect.Type
}

// jsonFields lists the fields encoding/json writes for struct type typ, embedded structs
// flattened.
func jsonFields(typ reflect.Type) []jsonField {
	var out []jsonField
	for f := range typ.Fields() {
		tag, hasTag := f.Tag.Lookup("json")
		name, opts, _ := strings.Cut(tag, ",")
		switch {
		case name == "-":
			continue
		case f.Anonymous && !hasTag && f.Type.Kind() == reflect.Struct:
			out = append(out, jsonFields(f.Type)...)
			continue
		case !f.IsExported():
			continue
		case name == "":
			name = f.Name
		}
		optional := slices.Contains(strings.Split(opts, ","), "omitzero") || slices.Contains(strings.Split(opts, ","), "omitempty")
		out = append(out, jsonField{name: name, required: !optional, typ: f.Type})
	}
	return out
}

// goKind names the JSON kind of t in the terms schemaKind uses.
func goKind(t reflect.Type, defs map[string]map[string]any) string {
	if t == reflect.TypeFor[json.RawMessage]() {
		return "ref:ConfigValue"
	}
	switch t.Kind() {
	case reflect.Pointer:
		return goKind(t.Elem(), defs)
	case reflect.Struct:
		return "ref:" + t.Name()
	case reflect.Slice:
		return "array<" + goKind(t.Elem(), defs) + ">"
	case reflect.String:
		if _, ok := defs[t.Name()]["enum"]; ok {
			return "enum:" + t.Name()
		}
		return "string"
	default:
		return scalarKind(t.Kind())
	}
}

// schemaKind names the JSON kind a property schema allows: a referenced object definition, an
// enum definition, an array of a kind, or a type.
func schemaKind(p map[string]any, defs map[string]map[string]any) string {
	if ref, ok := p["$ref"].(string); ok {
		name := strings.TrimPrefix(ref, "#/$defs/")
		if _, ok := defs[name]["enum"]; ok {
			return "enum:" + name
		}
		return "ref:" + name
	}
	if p["type"] == "array" {
		return "array<" + schemaKind(p["items"].(map[string]any), defs) + ">"
	}
	if typ, ok := p["type"].(string); ok {
		return typ
	}
	return "untyped"
}

// typedConsts returns the string constants of each named type declared in the non-test
// files of the package in dir.
func typedConsts(tb testing.TB, dir string) map[string][]string {
	tb.Helper()
	fset := token.NewFileSet()
	entries, err := os.ReadDir(dir)
	if err != nil {
		tb.Fatal(err)
	}
	out := map[string][]string{}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, dir+"/"+e.Name(), nil, parser.SkipObjectResolution)
		if err != nil {
			tb.Fatal(err)
		}
		addConsts(tb, f, out)
	}
	return out
}

// addConsts adds the typed string constants f declares to out.
func addConsts(tb testing.TB, f *ast.File, out map[string][]string) {
	tb.Helper()
	for _, d := range f.Decls {
		gd, ok := d.(*ast.GenDecl)
		if !ok || gd.Tok != token.CONST {
			continue
		}
		for _, spec := range gd.Specs {
			vs := spec.(*ast.ValueSpec)
			typ, ok := vs.Type.(*ast.Ident)
			if !ok {
				continue
			}
			for _, v := range vs.Values {
				lit, ok := v.(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					continue
				}
				str, err := strconv.Unquote(lit.Value)
				if err != nil {
					tb.Fatal(err)
				}
				out[typ.Name] = append(out[typ.Name], str)
			}
		}
	}
}

func scalarKind(k reflect.Kind) string {
	switch k {
	case reflect.Bool:
		return "boolean"
	case reflect.Float32, reflect.Float64:
		return "number"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return "integer"
	default:
		return "unsupported " + k.String()
	}
}
