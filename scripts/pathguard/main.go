// Command pathguard checks where the packages that handle project content
// reach the file system by path.
//
// A project's files are read and written beneath its root. A call that opens,
// lists, stats or changes a file by a path string follows whatever symbolic
// link sits on that path, so a link in a checkout can lead it outside the
// project. Package core/rootpath resolves a path inside a root and refuses one
// that leaves it, and the file home (core/change/filehome) writes content files
// under their lock.
//
// So this type-checks the packages that handle project content. It reports
// each call to a by-path function of package os, to filepath.Walk and
// filepath.WalkDir, and to fs.WalkDir over os.DirFS. Calls are resolved to the
// function they reach, so a local function that shares a name is not
// reported. Calls to os.Stat and os.Lstat are reported too, because what they
// return decides what a run does.
//
// A call is allowed at the places listed in allowed, keyed by file and
// function. Each entry gives the reason the path is safe to use as given, such
// as a path a person named on the command line, or a system location like the
// temp dir or the user config dir.
//
// Run from the repository root:
//
//	go run ./scripts/pathguard            # check the repository
//	go run ./scripts/pathguard -count     # print the number of findings
//	go run ./scripts/pathguard -self-test # prove the check on fixtures
package main

import (
	"errors"
	"flag"
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"maps"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/neokapi/neokapi/scripts/internal/guardpackages"
)

// targets are the go list patterns that cover the checked packages.
var targets = []string{
	"./host/...", "./core/project/...", "./core/flow/...", "./core/change/...",
	"./core/formats/...", "./kapi/mcptools/...", "./bowrain/connector/...", "./bowrain/jobs/...",
}

// checkedPrefixes are the packages checked: each path and every package
// beneath it.
var checkedPrefixes = []string{
	"github.com/neokapi/neokapi/host",
	"github.com/neokapi/neokapi/core/project",
	"github.com/neokapi/neokapi/core/flow",
	"github.com/neokapi/neokapi/core/change",
	"github.com/neokapi/neokapi/core/formats",
	"github.com/neokapi/neokapi/kapi/mcptools",
	"github.com/neokapi/neokapi/bowrain/connector",
	"github.com/neokapi/neokapi/bowrain/jobs",
}

// byPath names, by package, the functions that reach a file by its path, each
// with what the call does.
var byPath = map[string]map[string]string{
	"os": {
		"Open":      "opens a file by path",
		"OpenFile":  "opens a file by path",
		"ReadFile":  "reads a file by path",
		"ReadDir":   "lists a directory by path",
		"Stat":      "stats a file by path",
		"Lstat":     "stats a file by path",
		"Create":    "creates a file by path",
		"WriteFile": "writes a file by path",
		"MkdirAll":  "creates directories by path",
		"Mkdir":     "creates a directory by path",
		"Remove":    "removes a file by path",
		"RemoveAll": "removes a tree by path",
		"Rename":    "renames a file by path",
		"Symlink":   "creates a symbolic link by path",
		"Link":      "creates a hard link by path",
	},
	"path/filepath": {
		"Walk":    "walks a tree by path",
		"WalkDir": "walks a tree by path",
	},
}

// allowed lists the functions that may reach a file by path, keyed by the
// file and the function, with the reason the path is safe to use as given.
// The entries live in the allow_*.go files, one per area of the tree.
var allowed = mergeAllowed(allowCore, allowProject, allowHostA, allowHostB, allowBowrain, allowCoord)

// mergeAllowed joins the per-area lists. A function listed twice is a
// mistake, and the later reason is kept.
func mergeAllowed(lists ...map[string]string) map[string]string {
	all := map[string]string{}
	for _, l := range lists {
		maps.Copy(all, l)
	}
	return all
}

func main() {
	selfTest := flag.Bool("self-test", false, "prove the check on fixtures")
	count := flag.Bool("count", false, "print the number of findings in total and per package, and exit 0")
	flag.Parse()
	if *selfTest {
		if err := runSelfTest(); err != nil {
			fmt.Fprintln(os.Stderr, "pathguard:", err)
			os.Exit(2)
		}
		return
	}
	found, err := check(targets, allowed)
	if err != nil {
		fmt.Fprintln(os.Stderr, "pathguard:", err)
		os.Exit(2)
	}
	if *count {
		printCount(found)
		return
	}
	if len(found) > 0 {
		fmt.Fprintln(os.Stderr, "pathguard: these reach the file system by path in a package that handles project content:")
		for _, f := range found {
			fmt.Fprintln(os.Stderr, "  "+f.text)
		}
		fmt.Fprintln(os.Stderr, "Reach a project file through core/rootpath (Contain, OpenRegular, Remove) or the file home, or list the place in scripts/pathguard with the reason its path is safe.")
		os.Exit(1)
	}
}

// finding is one reported call and the package it is in.
type finding struct {
	pkg  string
	text string
}

// printCount prints the total and a tally per package, largest first.
func printCount(found []finding) {
	tally := map[string]int{}
	for _, f := range found {
		tally[f.pkg]++
	}
	pkgs := make([]string, 0, len(tally))
	for p := range tally {
		pkgs = append(pkgs, p)
	}
	sort.Slice(pkgs, func(i, j int) bool {
		if tally[pkgs[i]] != tally[pkgs[j]] {
			return tally[pkgs[i]] > tally[pkgs[j]]
		}
		return pkgs[i] < pkgs[j]
	})
	fmt.Printf("pathguard: %d findings\n", len(found))
	for _, p := range pkgs {
		fmt.Printf("  %5d  %s\n", tally[p], p)
	}
}

// inScope reports whether a package is one of the checked packages.
func inScope(path string, prefixes []string) bool {
	for _, p := range prefixes {
		if path == p || strings.HasPrefix(path, p+"/") {
			return true
		}
	}
	return false
}

// check type-checks every checked package and reports each by-path call
// outside the allowed places.
func check(patterns []string, allow map[string]string) ([]finding, error) {
	pkgs, err := guardpackages.List(patterns...)
	if err != nil {
		return nil, err
	}
	root, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	fset := token.NewFileSet()
	imp := pkgs.Importer(fset)
	var found []finding
	checked := 0
	seen := map[string]bool{}
	for _, p := range pkgs {
		if p.DepOnly || p.Standard || !inScope(p.ImportPath, checkedPrefixes) || len(p.CompiledGoFiles) == 0 {
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
		}, allow, seen)
		if cerr != nil {
			return nil, fmt.Errorf("%s: %w", p.ImportPath, cerr)
		}
		for _, h := range hits {
			found = append(found, finding{pkg: p.ImportPath, text: h})
		}
		checked++
	}
	if checked == 0 {
		return nil, errors.New("no package to check: run from the repository root")
	}
	for where := range allow {
		if !seen[where] {
			found = append(found, finding{pkg: "(allowlist)", text: where + ": listed in allowed but reaches no file by path here any more; remove the entry"})
		}
	}
	fmt.Printf("pathguard: checked %d packages\n", checked)
	sort.Slice(found, func(i, j int) bool { return found[i].text < found[j].text })
	return found, nil
}

// checkPackage type-checks one package and reports every by-path call outside
// the allowed places. relFile names a position's file as the allowlist spells
// it; seen collects the allowed places that still make such a call.
func checkPackage(fset *token.FileSet, imp types.Importer, path string, files []*ast.File, relFile func(token.Pos) string, allow map[string]string, seen map[string]bool) ([]string, error) {
	info := &types.Info{
		Types: map[ast.Expr]types.TypeAndValue{},
		Uses:  map[*ast.Ident]types.Object{},
		Defs:  map[*ast.Ident]types.Object{},
	}
	var typeErrs []error
	conf := types.Config{Importer: imp, Error: func(err error) { typeErrs = append(typeErrs, err) }}
	if _, err := conf.Check(path, fset, files, info); err != nil && len(typeErrs) > 0 {
		return nil, typeErrs[0]
	}
	var found []string
	for _, f := range files {
		if strings.HasSuffix(fset.Position(f.Pos()).Filename, "_test.go") {
			continue
		}
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
				name, what, ok := reachesPath(info, call)
				if !ok {
					return true
				}
				if _, ok := allow[where]; ok {
					seen[where] = true
					return true
				}
				p := fset.Position(call.Pos())
				found = append(found, fmt.Sprintf("%s:%d: %s %s (in %s)", relFile(call.Pos()), p.Line, name, what, fn.Name.Name))
				return true
			})
		}
	}
	return found, nil
}

// reachesPath reports whether a call reaches a file by path, and returns the
// function as package.Name with what the call does.
func reachesPath(info *types.Info, call *ast.CallExpr) (name, what string, ok bool) {
	fn := callee(info, call)
	if fn == nil {
		return "", "", false
	}
	pkg := fn.Pkg().Path()
	if what, ok := byPath[pkg][fn.Name()]; ok {
		return fn.Pkg().Name() + "." + fn.Name(), what, true
	}
	if pkg == "io/fs" && fn.Name() == "WalkDir" && len(call.Args) > 0 {
		if arg, ok := call.Args[0].(*ast.CallExpr); ok {
			if d := callee(info, arg); d != nil && d.Pkg().Path() == "os" && d.Name() == "DirFS" {
				return "fs.WalkDir", "walks a tree by path through os.DirFS", true
			}
		}
	}
	return "", "", false
}

// callee returns the package-level function a call reaches, or nil for a
// method, a function value or a conversion.
func callee(info *types.Info, call *ast.CallExpr) *types.Func {
	var id *ast.Ident
	switch fun := call.Fun.(type) {
	case *ast.SelectorExpr:
		id = fun.Sel
	case *ast.Ident:
		id = fun
	default:
		return nil
	}
	fn, ok := info.Uses[id].(*types.Func)
	if !ok || fn.Pkg() == nil {
		return nil
	}
	if sig, ok := fn.Type().(*types.Signature); !ok || sig.Recv() != nil {
		return nil
	}
	return fn
}
