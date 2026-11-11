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
	for _, p := range drift(ruleNames(t), declaredNames(parseDir(t, ".")), withheld()) {
		t.Error(p)
	}
}

// TestDriftReports checks that drift reports each kind of mismatch: the package with one
// declaration added, and the rules with one name added.
func TestDriftReports(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name, source, rule, want string
	}{
		{name: "function", source: `func Extra() string { return "" }`, want: "exports Extra, which is not"},
		{name: "variable", source: `var Extra = "x"`, want: "exports Extra, which is not"},
		{name: "type that is not an alias", source: `type Extra string`, want: "type Extra is not an alias"},
		{name: "alias of another name", source: `const Life = rules.Mana`, want: "exports Life as rules.Mana"},
		{name: "rules name not exported", rule: "Vigour", want: "rules define Vigour"},
	}
	files := parseDir(t, ".")
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			internal := ruleNames(t)
			if testCase.rule != "" {
				internal[testCase.rule] = "output key"
			}
			all := files
			if testCase.source != "" {
				extra, err := parser.ParseFile(token.NewFileSet(), "extra.go", "package poe1\n"+testCase.source, parser.SkipObjectResolution)
				if err != nil {
					t.Fatal(err)
				}
				all = append(slices.Clone(files), extra)
			}
			problems := drift(internal, declaredNames(all), withheld())
			if !slices.ContainsFunc(problems, func(p string) bool { return strings.Contains(p, testCase.want) }) {
				t.Errorf("problems %q, want one containing %q", problems, testCase.want)
			}
		})
	}
}

// drift returns how the names declared in this package, each with the rules name it aliases
// ("" when it aliases none), differ from the rules' names not withheld.
func drift(internal, public, withheld map[string]string) []string {
	var out []string
	for name := range internal {
		_, isPublic := public[name]
		_, isWithheld := withheld[name]
		switch {
		case isPublic && isWithheld:
			out = append(out, name+" is both exported and withheld")
		case !isPublic && !isWithheld:
			out = append(out, fmt.Sprintf("rules define %s (%s); export it or list it as withheld", name, internal[name]))
		}
	}
	for name, target := range public {
		if p := publicProblem(internal, name, target); p != "" {
			out = append(out, p)
		}
	}
	for name := range withheld {
		if internal[name] == "" {
			out = append(out, "withholds "+name+", which the rules do not define")
		}
	}
	slices.Sort(out)
	return out
}

// publicProblem returns what is wrong with the package declaring name as an alias of rules
// name target, or "".
func publicProblem(internal map[string]string, name, target string) string {
	switch {
	case target == notAlias:
		return "type " + name + " is not an alias"
	case internal[name] == "":
		return "exports " + name + ", which is not an output key, config key or config value of the rules"
	case target != name:
		return "exports " + name + " as rules." + target + ", want the same name"
	}
	return ""
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
	for _, d := range genDecls(parseDir(t, filepath.Join("..", "internal", "poe1"))) {
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

// notAlias marks a declared type that is not an alias in declaredNames.
const notAlias = "!"

// declaredNames returns the names files declare at the top level, each with the rules name it
// aliases: "" for a function, a method or a value that aliases none, notAlias for a type that
// is not an alias.
func declaredNames(files []*ast.File) map[string]string {
	out := map[string]string{}
	for _, f := range files {
		for _, d := range f.Decls {
			switch d := d.(type) {
			case *ast.FuncDecl:
				out[d.Name.Name] = ""
			case *ast.GenDecl:
				addDeclared(out, d)
			}
		}
	}
	return out
}

func addDeclared(out map[string]string, d *ast.GenDecl) {
	for _, spec := range d.Specs {
		switch spec := spec.(type) {
		case *ast.TypeSpec:
			out[spec.Name.Name] = ruleTarget(spec.Type)
			if !spec.Assign.IsValid() {
				out[spec.Name.Name] = notAlias
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

// genDecls returns the declarations of files other than functions and methods.
func genDecls(files []*ast.File) []*ast.GenDecl {
	var out []*ast.GenDecl
	for _, f := range files {
		for _, d := range f.Decls {
			if g, ok := d.(*ast.GenDecl); ok {
				out = append(out, g)
			}
		}
	}
	return out
}

// parseDir parses the non-test Go files of dir.
func parseDir(t *testing.T, dir string) []*ast.File {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	out := make([]*ast.File, 0, len(files))
	fset := token.NewFileSet()
	for _, f := range slices.DeleteFunc(files, func(f string) bool { return strings.HasSuffix(f, "_test.go") }) {
		file, err := parser.ParseFile(fset, f, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, file)
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
