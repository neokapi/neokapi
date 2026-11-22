// Command editguard checks where a document's bytes are written to a file.
//
// Content changes go through one function, change.ApplyBlock, and a format
// writer turns the changed blocks back into bytes. Those bytes reach a content
// file through the file home (core/change/filehome): a change set's commit and
// a flow's produced document are both staged beside the file and renamed onto
// it under the file's lock, only while the file still holds what was read. A
// document written any other way bypasses that lock and that check. So this
// type-checks every package that could write a document and reports:
//
//   - each call to a method SetOutput(path string) error, which points a writer
//     at a file and writes a whole document there;
//   - each call to the replacing functions of core/atomicfile (Replace,
//     ReplaceBytes, Stage, StageWithParents, StageWithin, StageWithinMode)
//     outside the file home, the one place that stages and renames a content
//     file;
//
// except in the format packages, where one writer wraps another, and at the
// call sites listed in allowed, keyed by file and function, each with what it
// writes: an export, a file that is not a content file, or a write that has not
// moved to the file home yet.
//
// A new place that writes a document to a file is reported until it is listed
// here, so the list stays the complete set of places a document reaches disk
// outside the file home.
//
// Run from the repository root:
//
//	go run ./scripts/editguard            # check the repository
//	go run ./scripts/editguard -self-test # prove the check on fixtures
package main

import (
	"errors"
	"flag"
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/neokapi/neokapi/scripts/internal/buildconfigs"
	"github.com/neokapi/neokapi/scripts/internal/guardpackages"
)

// targets are the packages checked: every module that writes documents.
var targets = []string{
	"./core/...", "./host/...", "./cli/...", "./kapi/...", "./apps/kapi-desktop/backend/...",
	"./bowrain/...", "./examples/...", "./cmd/...",
}

// exemptPrefixes are the packages that write documents by design: the format
// packages, where one writer wraps another, and the file home, which stages
// and renames every content file a change set or a flow writes.
var exemptPrefixes = []string{
	"github.com/neokapi/neokapi/core/format",
	"github.com/neokapi/neokapi/core/formats/",
	"github.com/neokapi/neokapi/core/change/filehome",
	"github.com/neokapi/neokapi/core/atomicfile",
	"github.com/neokapi/neokapi/core/rootpath",
}

// replacing names, by package, the functions that write a whole file: the
// replacing functions of core/atomicfile, which stage and rename a file.
var replacing = map[string]map[string]bool{
	"github.com/neokapi/neokapi/core/atomicfile": {"Replace": true, "ReplaceBytes": true, "Stage": true, "StageWithParents": true, "StageWithin": true, "StageWithinMode": true},
}

// allowed lists the functions that point a writer at a file, keyed by the file
// and the function, with what each writes.
var allowed = map[string]string{
	"apps/kapi-desktop/backend/filters.go:writeFiltersFile":             "records the checkout's personal filters and selected filter",
	"apps/kapi-desktop/backend/filters.go:ensureLocalFiltersGitignored": "adds the personal filter filename to the checkout's ignore rules",
	"host/toolbox_conv.go:convertDocument":                              "exports a document converted to another format",
	"host/apply_comment.go:write":                                       "writes the code comments kapi apply edits, which the file home does not write yet",
	"bowrain/connector/file.go:publishFile":                             "publishes a document to a connector's file destination",
	"examples/go-quickstart/main.go:run":                                "writes the example's bilingual output",
}

func main() {
	selfTest := flag.Bool("self-test", false, "prove the check on fixtures")
	configName := flag.String("config", "host", "build configuration to check under (scripts/internal/buildconfigs)")
	flag.Parse()
	var (
		found []string
		err   error
	)
	if *selfTest {
		err = runSelfTest()
	} else {
		var cfgs []buildconfigs.Config
		if cfgs, err = buildconfigs.Named(*configName); err == nil {
			found, err = check(cfgs[0], targets)
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "editguard:", err)
		os.Exit(2)
	}
	if len(found) > 0 {
		fmt.Fprintln(os.Stderr, "editguard: these write a document to a file outside the file home and the listed places:")
		for _, f := range found {
			fmt.Fprintln(os.Stderr, "  "+f)
		}
		fmt.Fprintln(os.Stderr, "Commit a document through core/change/filehome (a change set through the change service, a whole document a run produced through Home.Produce), or list the new place in scripts/editguard with what it writes.")
		os.Exit(1)
	}
}

func exempt(path string) bool {
	for _, p := range exemptPrefixes {
		if path == strings.TrimSuffix(p, "/") || strings.HasPrefix(path, p) {
			return true
		}
	}
	return false
}

// check type-checks every target package and reports each write found.
// In a configuration after the first, only the packages holding a file no
// earlier configuration compiles are checked.
func check(cfg buildconfigs.Config, patterns []string) ([]string, error) {
	first := buildconfigs.Index(cfg.Name) == 0
	pkgs, err := guardpackages.ListConfig(cfg, patterns...)
	if err != nil {
		return nil, err
	}
	root, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	fset := token.NewFileSet()
	imp := pkgs.Importer(fset)
	var found []string
	checked := 0
	seen := map[string]bool{}
	for _, p := range pkgs {
		if p.DepOnly || p.Standard || exempt(p.ImportPath) || !strings.HasPrefix(p.ImportPath, "github.com/neokapi/neokapi/") {
			continue
		}
		if len(p.CompiledGoFiles) == 0 {
			continue
		}
		files, err := p.ParseFiles(fset)
		if err != nil {
			return nil, err
		}
		hits, cerr := checkPackage(fset, imp, p.ImportPath, files, func(pos token.Pos) string {
			rel, rerr := filepath.Rel(root, fset.Position(pos).Filename)
			if rerr != nil {
				return fset.Position(pos).Filename
			}
			return filepath.ToSlash(rel)
		}, seen)
		if cerr != nil {
			return nil, fmt.Errorf("%s: %w", p.ImportPath, cerr)
		}
		found = append(found, hits...)
		checked++
	}
	if checked == 0 && first {
		return nil, errors.New("no package to check: run from the repository root")
	}
	// A later configuration sees part of the tree, so only the first can
	// tell that an entry writes nothing any more.
	for where := range allowed {
		if first && !seen[where] {
			found = append(found, where+": listed in allowed but writes no document here any more; remove the entry")
		}
	}
	fmt.Printf("editguard: checked %d packages (%s)\n", checked, cfg.Name)
	sort.Strings(found)
	return found, nil
}

// checkPackage type-checks one package and reports every call that points a
// writer at a file outside the allowed places. relFile names a position's file
// as the allowlist spells it; seen collects the allowed places that still
// write.
func checkPackage(fset *token.FileSet, imp types.Importer, path string, files []*ast.File, relFile func(token.Pos) string, seen map[string]bool) ([]string, error) {
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
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				what := ""
				if recv, ok := writesFile(info, call); ok {
					what = recv + ".SetOutput writes a document to a file"
				} else if name, ok := replacesFile(info, call); ok {
					what = name + " writes a file outside the file home"
				} else {
					return true
				}
				if _, ok := allowed[where]; ok {
					seen[where] = true
					return true
				}
				p := fset.Position(call.Pos())
				found = append(found, fmt.Sprintf("%s:%d: %s (in %s)", relFile(call.Pos()), p.Line, what, fn.Name.Name))
				return true
			})
		}
	}
	return found, nil
}

// writesFile reports whether a call points a writer at a file: a method named
// SetOutput taking one string and returning an error. It returns the
// receiver's type for the message.
func writesFile(info *types.Info, call *ast.CallExpr) (string, bool) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "SetOutput" {
		return "", false
	}
	s, ok := info.Selections[sel]
	if !ok || s.Kind() != types.MethodVal {
		return "", false
	}
	sig, ok := s.Type().(*types.Signature)
	if !ok || sig.Params().Len() != 1 || sig.Results().Len() != 1 {
		return "", false
	}
	if b, ok := sig.Params().At(0).Type().Underlying().(*types.Basic); !ok || b.Kind() != types.String {
		return "", false
	}
	if !types.Identical(sig.Results().At(0).Type(), types.Universe.Lookup("error").Type()) {
		return "", false
	}
	return types.TypeString(s.Recv(), func(p *types.Package) string { return p.Name() }), true
}

// replacesFile reports whether a call is one of the functions in replacing,
// and returns it as package.Name.
func replacesFile(info *types.Info, call *ast.CallExpr) (string, bool) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return "", false
	}
	fn, ok := info.Uses[sel.Sel].(*types.Func)
	if !ok || fn.Pkg() == nil || !replacing[fn.Pkg().Path()][fn.Name()] {
		return "", false
	}
	if sig, ok := fn.Type().(*types.Signature); !ok || sig.Recv() != nil {
		return "", false
	}
	return fn.Pkg().Name() + "." + fn.Name(), true
}
