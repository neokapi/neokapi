// Command projectionguard checks that nothing writes a project's context
// stores except the projector (core/projector).
//
// The terms store, the content memory, the voice profiles and the rules widened
// to the whole workspace are projections of the workspace's operation log. A
// write that reaches one of them any other way leaves a row the log does not
// explain, which a rebuild then drops without anyone noticing. So this
// type-checks every Apache-licensed package that could reach the stores and
// reports two things:
//
//   - a call to one of a store's write methods on the store's own type, and
//   - a store of that type handed to an interface through which it could be
//     written: an argument, a return value, an assignment or a field.
//
// The projector's stores (projector.Terms, projector.Memory, projector.Voice,
// projector.Rules) override every write method, so a caller holding one of
// those is never reported. A store a person names on the command line (a
// standalone `--memory` file, a `--termstore`) is not a projection, and the
// few functions that open one are listed in allowed with the reason.
//
// Run from the repository root:
//
//	go run ./scripts/projectionguard            # check the repository
//	go run ./scripts/projectionguard -self-test # prove the check on fixtures
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// targets are the packages checked: every Apache module that reaches a
// project store. Bowrain keeps its own stores and is outside the line.
var targets = []string{"./core/...", "./host/...", "./cli/...", "./kapi/...", "./apps/kapi-desktop/backend/..."}

// exempt packages write the stores by design: the projector itself, and the
// project store that opens them.
var exempt = map[string]bool{
	"github.com/neokapi/neokapi/core/projector": true,
	"github.com/neokapi/neokapi/core/projectdb": true,
	// The workspace's conformance suite drives the rule store it tests.
	"github.com/neokapi/neokapi/core/workspace/workspacetest": true,
}

// writes names each store type and the methods that write it.
var writes = map[string]map[string]bool{
	"github.com/neokapi/neokapi/memory.SQLiteStore": set("Add", "AddWithStream", "BulkAddWithStream", "Delete",
		"CreateImportSession", "UpdateImportSessionCount", "DeleteImportSession"),
	"github.com/neokapi/neokapi/terms.SQLiteStore": set("AddConcept", "AddConceptWithStream", "DeleteConcept",
		"AddRelation", "AddRelationWithStream", "DeleteRelation"),
	"github.com/neokapi/neokapi/voice.SQLiteStore":        set("CreateProfile", "UpdateProfile", "DeleteProfile"),
	"github.com/neokapi/neokapi/core/workspace.Workspace": set("WidenRule", "NarrowRule"),
}

// allowed lists the functions that write a store which is not a projection,
// keyed by the file and the function, with the reason.
var allowed = map[string]string{
	"host/memory.go:OpenMemorySQLite":                      "opens a standalone content memory named by --name, --file or --local",
	"host/termbase.go:OpenTermsSQLite":                     "opens a standalone terms store named by --name, --file or --local",
	"host/voicestore.go:OpenVoiceStore":                    "opens a standalone voice store named by --name, --file or --local",
	"host/flow.go:OpenToolMemory":                          "reads a standalone content memory named by --memory",
	"host/flow.go:openTerms":                               "reads a standalone terms store named by --termstore",
	"host/contextsearch.go:ContextSearchSourcesFor":        "reads standalone stores named by --termstore and --memory",
	"host/voicestore.go:VoiceLookupStore":                  "reads a standalone voice store named by --name, --file or --local",
	"host/projectstore.go:Projector":                       "binds the projector to the log it records into",
	"host/facetest/fixture/fixture.go:SeedTerms":           "seeds the terms of a throwaway fixture store for a screenshot",
	"host/contextexport_workspace.go:openWorkspaceContext": "restores a whole workspace with no checkout to open a project store in; its write moves to the log with snapshot and restore",
	"apps/kapi-desktop/backend/memory.go:OpenMemory":       "opens a content-memory file a person picked",
	"apps/kapi-desktop/backend/termbase.go:OpenTerms":      "opens a terms file a person picked",
}

func set(names ...string) map[string]bool {
	out := make(map[string]bool, len(names))
	for _, n := range names {
		out[n] = true
	}
	return out
}

func main() {
	selfTest := flag.Bool("self-test", false, "prove the check on fixtures")
	flag.Parse()
	var (
		found []string
		err   error
	)
	if *selfTest {
		err = runSelfTest()
	} else {
		found, err = check(targets)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "projectionguard:", err)
		os.Exit(2)
	}
	if len(found) > 0 {
		fmt.Fprintln(os.Stderr, "projectionguard: these write a project's context stores without the projector (core/projector):")
		for _, f := range found {
			fmt.Fprintln(os.Stderr, "  "+f)
		}
		fmt.Fprintln(os.Stderr, "Write through App.Projector (host) or projector.New, whose stores record every write in the log.")
		os.Exit(1)
	}
}

// listed is what `go list -json` reports about one package.
type listed struct {
	ImportPath      string
	Dir             string
	Export          string
	CompiledGoFiles []string
	DepOnly         bool
	Standard        bool
	Error           *struct{ Err string }
}

// list runs go list over the patterns and returns every package, with the
// export data of each dependency compiled.
func list(patterns []string) ([]listed, error) {
	args := append([]string{"list", "-e", "-compiled", "-tags", "fts5", "-export", "-deps",
		"-json=ImportPath,Dir,Export,CompiledGoFiles,DepOnly,Standard,Error"}, patterns...)
	cmd := exec.Command("go", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("go list: %w\n%s", err, stderr.String())
	}
	dec := json.NewDecoder(bytes.NewReader(out))
	var pkgs []listed
	for {
		var p listed
		if err := dec.Decode(&p); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return nil, fmt.Errorf("go list: %w", err)
		}
		pkgs = append(pkgs, p)
	}
	return pkgs, nil
}

// check type-checks every target package and reports each write found.
func check(patterns []string) ([]string, error) {
	pkgs, err := list(patterns)
	if err != nil {
		return nil, err
	}
	exports := map[string]string{}
	for _, p := range pkgs {
		if p.Export != "" {
			exports[p.ImportPath] = p.Export
		}
	}
	root, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	fset := token.NewFileSet()
	imp := importer.ForCompiler(fset, "gc", func(path string) (io.ReadCloser, error) {
		file, ok := exports[path]
		if !ok {
			return nil, fmt.Errorf("no export data for %s", path)
		}
		return os.Open(file)
	})
	var found []string
	checked := 0
	for _, p := range pkgs {
		if p.DepOnly || p.Standard || exempt[p.ImportPath] || !strings.HasPrefix(p.ImportPath, "github.com/neokapi/neokapi/") {
			continue
		}
		if p.Error != nil {
			return nil, fmt.Errorf("%s: %s", p.ImportPath, p.Error.Err)
		}
		if len(p.CompiledGoFiles) == 0 {
			continue // a directory of tests alone
		}
		var files []*ast.File
		for _, name := range p.CompiledGoFiles {
			path := name
			if !filepath.IsAbs(path) {
				path = filepath.Join(p.Dir, name)
			}
			f, perr := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
			if perr != nil {
				return nil, perr
			}
			files = append(files, f)
		}
		hits, cerr := checkPackage(fset, imp, p.ImportPath, files, func(pos token.Pos) string {
			rel, rerr := filepath.Rel(root, fset.Position(pos).Filename)
			if rerr != nil {
				return fset.Position(pos).Filename
			}
			return filepath.ToSlash(rel)
		})
		if cerr != nil {
			return nil, fmt.Errorf("%s: %w", p.ImportPath, cerr)
		}
		found = append(found, hits...)
		checked++

	}
	if checked == 0 {
		return nil, errors.New("no package to check: run from the repository root")
	}
	fmt.Printf("projectionguard: checked %d packages\n", checked)
	sort.Strings(found)
	return found, nil
}

// checkPackage type-checks one package and reports its writes. relFile names
// a position's file as the allowlist spells it.
func checkPackage(fset *token.FileSet, imp types.Importer, path string, files []*ast.File, relFile func(token.Pos) string) ([]string, error) {
	info := &types.Info{
		Types:      map[ast.Expr]types.TypeAndValue{},
		Selections: map[*ast.SelectorExpr]*types.Selection{},
		Uses:       map[*ast.Ident]types.Object{},
		Defs:       map[*ast.Ident]types.Object{},
	}
	var typeErrs []error
	conf := types.Config{Importer: imp, Error: func(err error) { typeErrs = append(typeErrs, err) }}
	if _, err := conf.Check(path, fset, files, info); err != nil && len(typeErrs) > 0 {
		return nil, typeErrs[0]
	}
	var found []string
	for _, f := range files {
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			where := relFile(fn.Pos()) + ":" + fn.Name.Name
			if _, ok := allowed[where]; ok {
				continue
			}
			report := func(pos token.Pos, what string) {
				p := fset.Position(pos)
				found = append(found, fmt.Sprintf("%s:%d: %s (in %s)", relFile(pos), p.Line, what, fn.Name.Name))
			}
			inspect(fn, info, report)
		}
	}
	return found, nil
}

// inspect walks one function for writes.
func inspect(fn *ast.FuncDecl, info *types.Info, report func(token.Pos, string)) {
	var results *types.Tuple
	if obj, ok := info.Defs[fn.Name].(*types.Func); ok {
		results = obj.Signature().Results()
	}
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.FuncLit:
			// A closure's returns are its own; its calls are still checked.
			return true
		case *ast.CallExpr:
			if sel, ok := n.Fun.(*ast.SelectorExpr); ok {
				if s, ok := info.Selections[sel]; ok && s.Kind() == types.MethodVal {
					if store, ok := storeOf(s.Recv()); ok && writes[store][sel.Sel.Name] {
						report(n.Pos(), fmt.Sprintf("%s.%s writes %s directly", short(store), sel.Sel.Name, short(store)))
					}
				}
			}
			if name, ok := standalone(info, n.Fun); ok {
				report(n.Pos(), fmt.Sprintf("projector.%s writes a store directly, which only a store a person named may be", name))
			}
			if sig, ok := typeOf(info, n.Fun).(*types.Signature); ok {
				params := sig.Params()
				for i, arg := range n.Args {
					var param types.Type
					switch {
					case sig.Variadic() && i >= params.Len()-1:
						if sl, ok := params.At(params.Len() - 1).Type().(*types.Slice); ok {
							param = sl.Elem()
						}
					case i < params.Len():
						param = params.At(i).Type()
					}
					flows(info, arg, param, report)
				}
			}
		case *ast.ReturnStmt:
			if results != nil && len(n.Results) == results.Len() {
				for i, r := range n.Results {
					flows(info, r, results.At(i).Type(), report)
				}
			}
		case *ast.AssignStmt:
			if len(n.Lhs) == len(n.Rhs) {
				for i := range n.Lhs {
					if tv, ok := info.Types[n.Lhs[i]]; ok {
						flows(info, n.Rhs[i], tv.Type, report)
					}
				}
			}
		case *ast.CompositeLit:
			st, ok := underlyingStruct(info.Types[n].Type)
			if !ok {
				return true
			}
			for _, el := range n.Elts {
				kv, ok := el.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				key, ok := kv.Key.(*ast.Ident)
				if !ok {
					continue
				}
				for i := range st.NumFields() {
					if st.Field(i).Name() == key.Name {
						flows(info, kv.Value, st.Field(i).Type(), report)
					}
				}
			}
		}
		return true
	})
}

// flows reports a store value handed to an interface that could write it.
func flows(info *types.Info, expr ast.Expr, to types.Type, report func(token.Pos, string)) {
	if to == nil {
		return
	}
	tv, ok := info.Types[expr]
	if !ok {
		return
	}
	store, ok := storeOf(tv.Type)
	if !ok {
		return
	}
	iface, ok := to.Underlying().(*types.Interface)
	if !ok {
		return
	}
	if named, ok := to.(*types.Named); ok && named.Obj().Pkg() != nil && named.Obj().Pkg().Path() == projectorPkg {
		// The projector is the writer: handing it a store is how it is built.
		return
	}
	for i := range iface.NumMethods() {
		if writes[store][iface.Method(i).Name()] {
			report(expr.Pos(), fmt.Sprintf("hands the %s to %s, which can write it", short(store), types.TypeString(to, nil)))
			return
		}
	}
}

// standalone reports a call to one of the projector's constructors for a
// store that is not a projection. Such a store is written directly, so wrapping
// a project's own store in one would write past the log.
func standalone(info *types.Info, fun ast.Expr) (string, bool) {
	sel, ok := fun.(*ast.SelectorExpr)
	if !ok {
		return "", false
	}
	obj, ok := info.ObjectOf(sel.Sel).(*types.Func)
	if !ok || obj.Pkg() == nil || obj.Pkg().Path() != projectorPkg {
		return "", false
	}
	return obj.Name(), strings.HasPrefix(obj.Name(), "Standalone")
}

// typeOf is an expression's type, including a package-qualified function,
// which the checker records against its name rather than the selector.
func typeOf(info *types.Info, e ast.Expr) types.Type {
	if t := info.TypeOf(e); t != nil {
		return t
	}
	if sel, ok := e.(*ast.SelectorExpr); ok {
		if obj := info.ObjectOf(sel.Sel); obj != nil {
			return obj.Type()
		}
	}
	return nil
}

// projectorPkg is the one writer of the stores.
const projectorPkg = "github.com/neokapi/neokapi/core/projector"

// storeOf reports whether t is a pointer to one of the store types.
func storeOf(t types.Type) (string, bool) {
	if p, ok := t.(*types.Pointer); ok {
		t = p.Elem()
	}
	named, ok := t.(*types.Named)
	if !ok || named.Obj().Pkg() == nil {
		return "", false
	}
	key := named.Obj().Pkg().Path() + "." + named.Obj().Name()
	_, ok = writes[key]
	return key, ok
}

func underlyingStruct(t types.Type) (*types.Struct, bool) {
	if t == nil {
		return nil, false
	}
	if p, ok := t.Underlying().(*types.Pointer); ok {
		t = p.Elem()
	}
	st, ok := t.Underlying().(*types.Struct)
	return st, ok
}

// short renders a store type the way a reader names it.
func short(store string) string {
	return strings.TrimPrefix(store, "github.com/neokapi/neokapi/")
}
