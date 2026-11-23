// Command pathguard checks where the repository's Go code reaches the file
// system by path.
//
// A project's files are read and written beneath its root. A call that opens,
// lists, stats or changes a file by a path string follows whatever symbolic
// link sits on that path, so a link in a checkout can lead it outside the
// project. Package core/rootpath resolves a path inside a root and refuses one
// that leaves it, and the file home (core/change/filehome) writes content files
// under their lock.
//
// So this type-checks every package of every module in go.work, except the
// few listed in excluded. It reports each reference to a function in sinks
// and each conversion to net/http.Dir. A reference counts whether it is a call
// or a function value, in a function, a function literal or a package-level
// variable of a non-test file.
//
// A reference is allowed by a comment that gives the reason its path is safe
// to use as given, on its line or alone on the line above it:
//
//	//pathguard: the global config file in the user config dir
//	data, err := os.ReadFile(path)
//
// A comment with no reason is reported, and so is one on a line that reaches
// no file by path.
//
// A function of a checked package whose allowed call takes one of its own
// parameters as the path is a wrapper: each reference to it is reported in
// turn unless a comment allows it. Callers of a wrapper's callers are not
// followed.
//
// Run from the repository root:
//
//	go run ./scripts/pathguard                # check under the host configuration
//	go run ./scripts/pathguard -config model  # check under another configuration
//	go run ./scripts/pathguard -count         # print the number of findings
//	go run ./scripts/pathguard -self-test     # prove the check
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"go/token"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/neokapi/neokapi/scripts/internal/buildconfigs"
	"github.com/neokapi/neokapi/scripts/internal/guardpackages"
)

// modulePrefix is the import path every module of the repository starts with.
const modulePrefix = "github.com/neokapi/neokapi"

// excluded are the packages the check leaves out, each with its reason. A
// package is covered unless it, or a package above it, is listed here. Test
// files are never checked.
var excluded = []struct{ path, reason string }{
	{modulePrefix + "/core/rootpath", "the sanctioned primitive: it resolves a path inside a root and refuses one that leaves it"},
	{modulePrefix + "/core/change/filehome", "the sanctioned primitive: it writes content files beneath its root under their lock"},
	{modulePrefix + "/scripts", "repository tooling, which reads the checkout it runs in and never receives project content"},
	{modulePrefix + "/core/proto", "generated protobuf code"},
	{modulePrefix + "/core/plugin/proto", "generated protobuf code"},
	{modulePrefix + "/bowrain/proto", "generated protobuf code"},
	{modulePrefix + "/core/i18n/gen", "build-time catalog generator run by make, linked into no product binary"},
	{modulePrefix + "/core/place/internal/gen", "build-time code generator (package main), linked into no product binary"},
	{modulePrefix + "/core/internal/testutil", "test support, imported only by test files"},
	{modulePrefix + "/core/format/spectest", "the format spec test runner, imported only by test files"},
	{modulePrefix + "/core/plugin/conformance", "the plugin conformance suite, imported only by test files"},
}

// mustCover are packages the coverage rule has to reach. The self-test fails
// when an exclusion swallows one of them.
var mustCover = []string{
	modulePrefix + "/core/storage",
	modulePrefix + "/core/container",
	modulePrefix + "/core/redaction",
	modulePrefix + "/core/workspace",
	modulePrefix + "/core/tools",
}

// sink is a function that reaches a file by path: what a call does, and which
// arguments hold the path.
type sink struct {
	what     string
	pathArgs []int
}

// sinks names, by package, the functions that reach a file by path.
var sinks = map[string]map[string]sink{
	"os": {
		"Open":       {"opens a file by path", []int{0}},
		"OpenFile":   {"opens a file by path", []int{0}},
		"ReadFile":   {"reads a file by path", []int{0}},
		"ReadDir":    {"lists a directory by path", []int{0}},
		"Stat":       {"stats a file by path", []int{0}},
		"Lstat":      {"stats a file by path", []int{0}},
		"Create":     {"creates a file by path", []int{0}},
		"WriteFile":  {"writes a file by path", []int{0}},
		"MkdirAll":   {"creates directories by path", []int{0}},
		"Mkdir":      {"creates a directory by path", []int{0}},
		"MkdirTemp":  {"creates a directory in a directory named by path", []int{0}},
		"CreateTemp": {"creates a file in a directory named by path", []int{0}},
		"Remove":     {"removes a file by path", []int{0}},
		"RemoveAll":  {"removes a tree by path", []int{0}},
		"Rename":     {"renames a file by path", []int{0, 1}},
		"Symlink":    {"creates a symbolic link by path", []int{0, 1}},
		"Link":       {"creates a hard link by path", []int{0, 1}},
		"Readlink":   {"reads a symbolic link by path", []int{0}},
		"Chmod":      {"changes a file's mode by path", []int{0}},
		"Chown":      {"changes a file's owner by path", []int{0}},
		"Lchown":     {"changes a file's owner by path", []int{0}},
		"Chtimes":    {"changes a file's times by path", []int{0}},
		"Truncate":   {"truncates a file by path", []int{0}},
		"DirFS":      {"opens a file system rooted at a path", []int{0}},
	},
	"path/filepath": {
		"Walk":         {"walks a tree by path", []int{0}},
		"WalkDir":      {"walks a tree by path", []int{0}},
		"Glob":         {"lists files matching a path pattern", []int{0}},
		"EvalSymlinks": {"follows the links on a path", []int{0}},
	},
	"io/ioutil": {
		"ReadFile":  {"reads a file by path", []int{0}},
		"WriteFile": {"writes a file by path", []int{0}},
		"ReadDir":   {"lists a directory by path", []int{0}},
		"TempFile":  {"creates a file in a directory named by path", []int{0}},
		"TempDir":   {"creates a directory in a directory named by path", []int{0}},
	},
	"github.com/bmatcuk/doublestar/v4": {
		"Glob":         {"lists files matching a path pattern", []int{1}},
		"GlobWalk":     {"walks files matching a path pattern", []int{1}},
		"FilepathGlob": {"lists files matching a path pattern", []int{0}},
	},
	"database/sql": {
		"Open": {"opens a database by a name that may be a path", []int{1}},
	},
	"archive/zip": {
		"OpenReader": {"opens an archive by path", []int{0}},
	},
}

// httpDir is the conversion that serves files by path.
var httpDir = sink{"converts a path to http.Dir, which serves the files beneath it", []int{0}}

func main() {
	selfTest := flag.Bool("self-test", false, "prove the check on fixtures and on every covered package")
	count := flag.Bool("count", false, "print the number of findings in total and per package, and exit 0")
	configName := flag.String("config", "host", "build configuration to check under (scripts/internal/buildconfigs)")
	flag.Parse()
	if *selfTest {
		if err := runSelfTest(); err != nil {
			fmt.Fprintln(os.Stderr, "pathguard:", err)
			os.Exit(2)
		}
		return
	}
	cfgs, err := buildconfigs.Named(*configName)
	if err != nil {
		fmt.Fprintln(os.Stderr, "pathguard:", err)
		os.Exit(2)
	}
	found, _, err := check(cfgs[0], false)
	if err != nil {
		fmt.Fprintln(os.Stderr, "pathguard:", err)
		os.Exit(2)
	}
	if *count {
		printCount(found)
		return
	}
	if len(found) > 0 {
		fmt.Fprintln(os.Stderr, "pathguard: these reach the file system by path with no //pathguard: reason, or carry a comment that is empty or no longer true:")
		for _, f := range found {
			fmt.Fprintln(os.Stderr, "  "+f.text)
		}
		fmt.Fprintln(os.Stderr, "Reach a project file through core/rootpath (Contain, OpenRegular, Remove) or the file home, or put `//pathguard: <why the path is safe>` on the line of the call or alone on the line above it.")
		os.Exit(1)
	}
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

// exclusion returns the reason a package is left out, or "" when it is covered.
func exclusion(path string) string {
	for _, e := range excluded {
		if path == e.path || strings.HasPrefix(path, e.path+"/") {
			return e.reason
		}
	}
	return ""
}

// covered reports whether a package is one the check walks.
func covered(path string) bool {
	return (path == modulePrefix || strings.HasPrefix(path, modulePrefix+"/")) && exclusion(path) == ""
}

// modulePatterns returns a go list pattern for each module in go.work whose
// packages are not all excluded.
func modulePatterns() ([]string, error) {
	cmd := exec.CommandContext(context.Background(), "go", "list", "-m", "-json")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("go list -m: %w\n%s", err, stderr.String())
	}
	root, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	var patterns []string
	dec := json.NewDecoder(bytes.NewReader(out))
	for {
		var m struct{ Path, Dir string }
		if err := dec.Decode(&m); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return nil, fmt.Errorf("go list -m: %w", err)
		}
		if exclusion(m.Path) != "" {
			continue
		}
		rel, err := filepath.Rel(root, m.Dir)
		if err != nil || strings.HasPrefix(rel, "..") {
			return nil, fmt.Errorf("module %s lies outside %s: run from the repository root", m.Path, root)
		}
		patterns = append(patterns, "./"+filepath.ToSlash(filepath.Join(rel, "...")))
	}
	if len(patterns) == 0 {
		return nil, errors.New("no module to check: run from the repository root")
	}
	return patterns, nil
}

// check type-checks every covered package under one build configuration and
// returns what it reports. In a configuration after the first, only the
// packages holding a file no earlier configuration compiles are checked. With
// plant, each covered package gets one more file that opens a file by path,
// and the planted files are returned by package.
func check(cfg buildconfigs.Config, plant bool) ([]finding, map[string]string, error) {
	first := buildconfigs.Index(cfg.Name) == 0
	patterns, err := modulePatterns()
	if err != nil {
		return nil, nil, err
	}
	pkgs, err := guardpackages.ListConfig(cfg, patterns...)
	if err != nil {
		return nil, nil, err
	}
	root, err := os.Getwd()
	if err != nil {
		return nil, nil, err
	}
	fset := token.NewFileSet()
	imp := pkgs.Importer(fset)
	a := newAnalysis(fset, first, func(name string) string {
		rel, rerr := filepath.Rel(root, name)
		if rerr != nil {
			return name
		}
		return filepath.ToSlash(rel)
	})
	planted := map[string]string{}
	for _, p := range pkgs {
		if p.DepOnly || p.Standard || !covered(p.ImportPath) || len(p.CompiledGoFiles) == 0 {
			continue
		}
		files, err := a.parseFiles(p)
		if err != nil {
			return nil, nil, err
		}
		if plant {
			f, name, err := a.plantFile(p.Dir, files[0].Name.Name)
			if err != nil {
				return nil, nil, err
			}
			files = append(files, f)
			planted[p.ImportPath] = a.rel(name)
		}
		if err := a.addPackage(imp, p.ImportPath, files); err != nil {
			return nil, nil, fmt.Errorf("%s: %w", p.ImportPath, err)
		}
	}
	if len(a.checked) == 0 && first {
		return nil, nil, errors.New("no package to check: run from the repository root")
	}
	fmt.Printf("pathguard: checked %d packages (%s)\n", len(a.checked), cfg.Name)
	return a.findings(), planted, nil
}
