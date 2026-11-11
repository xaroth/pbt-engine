package poe1_test

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	rules "github.com/xaroth/pbt-engine/internal/poe1"
)

// withheld returns the keys and values the rules define that hosts may not use, with the
// reason. A name the rules add that is neither exported nor listed here fails
// TestNamesMatchRules.
func withheld() map[string]string { return map[string]string{} }

// TestNamesMatchRules checks that this package exports, under the same name, every output key,
// config key, config value type and config value of the rules not withheld, and nothing else.
func TestNamesMatchRules(t *testing.T) {
	t.Parallel()
	internal := ruleNames(t)
	public := publicNames(t)
	withheld := withheld()
	for name := range internal {
		_, isPublic := public[name]
		_, isWithheld := withheld[name]
		switch {
		case isPublic && isWithheld:
			t.Errorf("%s is both exported and withheld", name)
		case !isPublic && !isWithheld:
			t.Errorf("rules define %s (%s); export it or list it as withheld", name, internal[name])
		}
	}
	for name, target := range public {
		if internal[name] == "" {
			t.Errorf("exports %s, which is not an output key, config key or config value of the rules", name)
		}
		if target != name {
			t.Errorf("exports %s as rules.%s, want the same name", name, target)
		}
	}
	for name := range withheld {
		if internal[name] == "" {
			t.Errorf("withholds %s, which the rules do not define", name)
		}
	}
}

// TestRuleNamesComplete checks the source scan against the rules' own lists, so a key declared
// in a form the scan does not read cannot slip past TestNamesMatchRules.
func TestRuleNamesComplete(t *testing.T) {
	t.Parallel()
	count := map[string]int{}
	for _, kind := range ruleNames(t) {
		count[kind]++
	}
	if got, want := count["output key"], len(rules.Outputs()); got != want {
		t.Errorf("scan found %d output keys, the catalogue has %d", got, want)
	}
	specs := rules.ConfigSpecs()
	if got, want := count["config key"], len(specs); got != want {
		t.Errorf("scan found %d config keys, the rules have %d", got, want)
	}
	for _, sp := range specs {
		if len(sp.Values) == 0 {
			continue
		}
		typ := fmt.Sprintf("%T", sp.Values[0])
		typ = typ[strings.LastIndex(typ, ".")+1:]
		if got := count["config value of "+typ]; got != len(sp.Values) {
			t.Errorf("config key %s: scan found %d values of %s, the key has %d", sp.Name, got, typ, len(sp.Values))
		}
	}
}

// ruleNames returns the output keys, config keys, config value types and config values the
// rules declare, by name, with their kind.
func ruleNames(t *testing.T) map[string]string {
	t.Helper()
	consts, types := ruleDecls(t)
	out := map[string]string{}
	valueTypes := map[string]bool{}
	for name, typ := range consts {
		switch {
		case typ == "output.Key":
			out[name] = "output key"
		case strings.HasPrefix(typ, "config.Key["):
			out[name] = "config key"
			if arg := strings.TrimSuffix(strings.TrimPrefix(typ, "config.Key["), "]"); types[arg] {
				valueTypes[arg] = true
				out[arg] = "config value type"
			}
		}
	}
	for name, typ := range consts {
		if valueTypes[typ] {
			out[name] = "config value of " + typ
		}
	}
	return out
}

// ruleDecls returns the rules' exported constants declared with a type, with that type, and
// the names of their types.
func ruleDecls(t *testing.T) (consts map[string]string, types map[string]bool) {
	t.Helper()
	consts, types = map[string]string{}, map[string]bool{}
	for _, d := range genDecls(t, filepath.Join("..", "internal", "poe1")) {
		for _, spec := range d.Specs {
			switch spec := spec.(type) {
			case *ast.TypeSpec:
				types[spec.Name.Name] = true
			case *ast.ValueSpec:
				if d.Tok != token.CONST || spec.Type == nil {
					continue
				}
				for _, n := range spec.Names {
					if n.IsExported() {
						consts[n.Name] = exprString(spec.Type)
					}
				}
			}
		}
	}
	return consts, types
}

// publicNames returns the names this package declares, each with the rules name it aliases.
func publicNames(t *testing.T) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, d := range genDecls(t, ".") {
		for _, spec := range d.Specs {
			switch spec := spec.(type) {
			case *ast.TypeSpec:
				out[spec.Name.Name] = ruleTarget(spec.Type)
				if !spec.Assign.IsValid() {
					t.Errorf("type %s is not an alias", spec.Name.Name)
				}
			case *ast.ValueSpec:
				for i, n := range spec.Names {
					target := ""
					if i < len(spec.Values) && spec.Type == nil {
						target = ruleTarget(spec.Values[i])
					}
					out[n.Name] = target
				}
			}
		}
	}
	return out
}

// ruleTarget returns X for the expression rules.X, else "".
func ruleTarget(e ast.Expr) string {
	sel, ok := e.(*ast.SelectorExpr)
	if !ok {
		return ""
	}
	if pkg, ok := sel.X.(*ast.Ident); !ok || pkg.Name != "rules" {
		return ""
	}
	return sel.Sel.Name
}

func genDecls(t *testing.T, dir string) []*ast.GenDecl {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	var out []*ast.GenDecl
	fset := token.NewFileSet()
	for _, f := range slices.DeleteFunc(files, func(f string) bool { return strings.HasSuffix(f, "_test.go") }) {
		file, err := parser.ParseFile(fset, f, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range file.Decls {
			if g, ok := d.(*ast.GenDecl); ok {
				out = append(out, g)
			}
		}
	}
	return out
}

func exprString(e ast.Expr) string {
	switch e := e.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.SelectorExpr:
		return exprString(e.X) + "." + e.Sel.Name
	case *ast.IndexExpr:
		return exprString(e.X) + "[" + exprString(e.Index) + "]"
	}
	return ""
}
