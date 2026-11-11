package simulator_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// pending is a walkthrough step the engine cannot do yet.
type pending struct {
	// needs is the capability the step waits for, one of capabilities.
	needs string
	// waitsOn are symbols the code uses that do not exist yet, as pkg.Name, pkg.Type.Member or
	// pkg.*.Member (a member of any type). Once all exist, the step is likely possible.
	waitsOn []string
	// code is the step as intended; it parses as Go statements.
	code string
}

// capabilities are what pending steps wait for.
func capabilities() []string {
	return []string{
		"an enemy actor",
		"Path of Building share codes",
		"skills on a build",
		"items on a build",
		"charges in config",
		"conditions derived from the build",
		"minions",
		"totems",
		"a party",
	}
}

// TestWalkthroughPending checks the walkthrough's pending steps and reports how much of it runs.
func TestWalkthroughPending(t *testing.T) {
	t.Parallel()
	symbols := sourceSymbols(t)
	// Without these the index is broken and every pending step would look blocked forever.
	for _, sym := range []string{"simulator.New", "simulator.Sim.Simulate", "build.Build.Nodes", "poe1.Life", "simulator.*.Explain"} {
		if !symbols.has(sym) {
			t.Fatalf("symbol index lacks %s", sym)
		}
	}
	steps := walkthrough()
	names := map[string]bool{}
	used := map[string]bool{}
	running := 0
	for _, s := range steps {
		if s.name == "" || names[s.name] {
			t.Errorf("step %q: name empty or repeated", s.name)
		}
		names[s.name] = true
		switch {
		case (s.run == nil) == (s.pending == nil):
			t.Errorf("step %q: neither running nor pending, or both", s.name)
		case s.run != nil:
			running++
		default:
			used[s.pending.needs] = true
			checkPending(t, s.name, s.pending, symbols)
		}
	}
	for _, c := range capabilities() {
		if !used[c] {
			t.Errorf("capability %q: no pending step needs it", c)
		}
	}
	t.Logf("walkthrough: %d steps run, %d pending", running, len(steps)-running)
}

func checkPending(t *testing.T, name string, p *pending, symbols symbolSet) {
	t.Helper()
	if !slices.Contains(capabilities(), p.needs) {
		t.Errorf("step %q: needs %q, want one of capabilities", name, p.needs)
	}
	if _, err := parser.ParseFile(token.NewFileSet(), "", "package p\nfunc _() {\n"+p.code+"\n}", 0); err != nil {
		t.Errorf("step %q: code does not parse: %v", name, err)
	}
	if len(p.waitsOn) == 0 {
		t.Errorf("step %q: waits on no symbol", name)
		return
	}
	missing := 0
	for _, sym := range p.waitsOn {
		// A package the index does not parse would keep its symbols missing forever.
		if pkg, _, _ := strings.Cut(sym, "."); !symbols.has(pkg) {
			t.Errorf("step %q: waits on %s, in a package the symbol index does not parse", name, sym)
		}
		if !strings.Contains(p.code, sym[strings.LastIndex(sym, ".")+1:]) {
			t.Errorf("step %q: waits on %s, which its code does not use", name, sym)
		}
		if !symbols.has(sym) {
			missing++
		}
	}
	if missing == 0 {
		t.Errorf("step %q: every symbol it waits on exists; make it a running step", name)
	}
}

// symbolSet holds the declared names of the packages pending steps use: pkg for each package
// parsed, pkg.Name for top-level declarations, pkg.Type.Member for methods and struct fields.
type symbolSet map[string]bool

func (s symbolSet) has(sym string) bool {
	pkg, member, anyType := strings.Cut(sym, ".*.")
	if !anyType {
		return s[sym]
	}
	for k := range s {
		if strings.HasPrefix(k, pkg+".") && strings.HasSuffix(k, "."+member) && strings.Count(k, ".") == 2 {
			return true
		}
	}
	return false
}

// sourceSymbols parses the packages pending steps use. poe1 holds the game's output and config
// keys.
func sourceSymbols(t *testing.T) symbolSet {
	t.Helper()
	dirs := map[string]string{
		"build": "../build", "config": "../config", "dataset": "../dataset", "output": "../output",
		"pob": "../pob", "simulator": ".", "poe1": "../internal/poe1",
	}
	out := symbolSet{}
	for pkg, dir := range dirs {
		out[pkg] = true
		files, err := filepath.Glob(filepath.Join(dir, "*.go"))
		if err != nil {
			t.Fatal(err)
		}
		fset := token.NewFileSet()
		for _, f := range files {
			if strings.HasSuffix(f, "_test.go") {
				continue
			}
			file, err := parser.ParseFile(fset, f, nil, parser.SkipObjectResolution)
			if err != nil {
				t.Fatal(err)
			}
			for _, d := range file.Decls {
				switch d := d.(type) {
				case *ast.FuncDecl:
					out.addFunc(pkg, d)
				case *ast.GenDecl:
					out.addGen(pkg, d)
				}
			}
		}
	}
	return out
}

func (s symbolSet) addFunc(pkg string, d *ast.FuncDecl) {
	if d.Recv == nil {
		s[pkg+"."+d.Name.Name] = true
		return
	}
	typ := d.Recv.List[0].Type
	if star, ok := typ.(*ast.StarExpr); ok {
		typ = star.X
	}
	if ix, ok := typ.(*ast.IndexExpr); ok {
		typ = ix.X
	}
	if id, ok := typ.(*ast.Ident); ok {
		s[pkg+"."+id.Name+"."+d.Name.Name] = true
	}
}

func (s symbolSet) addGen(pkg string, d *ast.GenDecl) {
	for _, spec := range d.Specs {
		switch spec := spec.(type) {
		case *ast.ValueSpec:
			for _, n := range spec.Names {
				s[pkg+"."+n.Name] = true
			}
		case *ast.TypeSpec:
			s[pkg+"."+spec.Name.Name] = true
			st, ok := spec.Type.(*ast.StructType)
			if !ok {
				continue
			}
			for _, f := range st.Fields.List {
				for _, n := range f.Names {
					s[pkg+"."+spec.Name.Name+"."+n.Name] = true
				}
			}
		}
	}
}
