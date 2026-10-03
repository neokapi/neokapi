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
//     ReplaceBytes, Stage, StageWithParents) outside the file home, the one
//     place that stages and renames a content file;
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
	"bytes"
	"context"
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
}

// atomicfilePath is the package whose replacing functions stage and rename a
// file, and replacing names them.
const atomicfilePath = "github.com/neokapi/neokapi/core/atomicfile"

var replacing = map[string]bool{"Replace": true, "ReplaceBytes": true, "Stage": true, "StageWithParents": true}

// allowed lists the functions that point a writer at a file, keyed by the file
// and the function, with what each writes.
var allowed = map[string]string{
	"host/toolbox_conv.go:convertDocument":  "exports a document converted to another format",
	"host/apply_comment.go:write":           "writes the code comments kapi apply edits, which the file home does not write yet",
	"bowrain/connector/file.go:publishFile": "publishes a document to a connector's file destination",
	"examples/go-quickstart/main.go:run":    "writes the example's bilingual output",
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
	cmd := exec.CommandContext(context.Background(), "go", args...)
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

func exempt(path string) bool {
	for _, p := range exemptPrefixes {
		if path == strings.TrimSuffix(p, "/") || strings.HasPrefix(path, p) {
			return true
		}
	}
	return false
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
	seen := map[string]bool{}
	for _, p := range pkgs {
		if p.DepOnly || p.Standard || exempt(p.ImportPath) || !strings.HasPrefix(p.ImportPath, "github.com/neokapi/neokapi/") {
			continue
		}
		if p.Error != nil {
			return nil, fmt.Errorf("%s: %s", p.ImportPath, p.Error.Err)
		}
		if len(p.CompiledGoFiles) == 0 {
			continue
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
		}, seen)
		if cerr != nil {
			return nil, fmt.Errorf("%s: %w", p.ImportPath, cerr)
		}
		found = append(found, hits...)
		checked++
	}
	if checked == 0 {
		return nil, errors.New("no package to check: run from the repository root")
	}
	for where := range allowed {
		if !seen[where] {
			found = append(found, where+": listed in allowed but writes no document here any more; remove the entry")
		}
	}
	fmt.Printf("editguard: checked %d packages\n", checked)
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
					what = "atomicfile." + name + " replaces a file outside the file home"
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

// replacesFile reports whether a call is one of core/atomicfile's replacing
// functions, and returns its name.
func replacesFile(info *types.Info, call *ast.CallExpr) (string, bool) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || !replacing[sel.Sel.Name] {
		return "", false
	}
	fn, ok := info.Uses[sel.Sel].(*types.Func)
	if !ok || fn.Pkg() == nil || fn.Pkg().Path() != atomicfilePath {
		return "", false
	}
	if sig, ok := fn.Type().(*types.Signature); !ok || sig.Recv() != nil {
		return "", false
	}
	return fn.Name(), true
}
