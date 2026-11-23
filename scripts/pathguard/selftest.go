package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strings"

	"github.com/neokapi/neokapi/scripts/internal/guardpackages"
)

// fixtureAllowed is the allowlist the fixtures are checked against.
var fixtureAllowed = map[string]string{
	"fixture.go:listed": "a system location the fixture reads",
}

// fixtures are packages the check must report, or must pass, each with the
// number of findings it expects.
var fixtures = []struct {
	name string
	want int
	src  string
}{
	{"a file read by path", 1, `package f
import "os"
func read(p string) ([]byte, error) { return os.ReadFile(p) }`},
	{"a stat and a directory made by path", 2, `package f
import "os"
func ensure(p string) error {
	if _, err := os.Stat(p); err == nil {
		return nil
	}
	return os.MkdirAll(p, 0o755)
}`},
	{"a tree walked by path", 1, `package f
import (
	"io/fs"
	"path/filepath"
)
func walk(root string) error {
	return filepath.WalkDir(root, func(string, fs.DirEntry, error) error { return nil })
}`},
	{"a tree walked through os.DirFS", 1, `package f
import (
	"io/fs"
	"os"
)
func walk(root string) error {
	return fs.WalkDir(os.DirFS(root), ".", func(string, fs.DirEntry, error) error { return nil })
}`},
	{"a tree walked in a file system that is not a path", 0, `package f
import (
	"embed"
	"io/fs"
)
var data embed.FS
func walk() error {
	return fs.WalkDir(data, ".", func(string, fs.DirEntry, error) error { return nil })
}`},
	{"a local function named ReadFile", 0, `package f
func ReadFile(p string) ([]byte, error) { return nil, nil }
func read(p string) ([]byte, error) { return ReadFile(p) }`},
	{"a file opened for reading inside a root", 0, `package f
import "github.com/neokapi/neokapi/core/rootpath"
func read(root, p string) error { f, err := rootpath.OpenRegular(root, p); if err == nil { f.Close() }; return err }`},
	{"a function on the allowlist", 0, `package f
import "os"
func listed(p string) ([]byte, error) { return os.ReadFile(p) }`},
}

// runSelfTest checks every fixture and fails when one reports other than it
// expects.
func runSelfTest() error {
	pkgs, err := guardpackages.List("github.com/neokapi/neokapi/core/rootpath", "os", "io/fs", "path/filepath", "embed")
	if err != nil {
		return err
	}
	fset := token.NewFileSet()
	imp := pkgs.Importer(fset)
	var failures []string
	for _, fx := range fixtures {
		f, err := parser.ParseFile(fset, "fixture.go", fx.src, 0)
		if err != nil {
			return fmt.Errorf("%s: %w", fx.name, err)
		}
		found, err := checkPackage(fset, imp, "fixture", []*ast.File{f}, func(token.Pos) string { return "fixture.go" }, fixtureAllowed, map[string]bool{})
		if err != nil {
			return fmt.Errorf("%s: %w", fx.name, err)
		}
		if len(found) != fx.want {
			failures = append(failures, fmt.Sprintf("%s: want %d findings, got %d: %s", fx.name, fx.want, len(found), strings.Join(found, "; ")))
		}
	}
	if len(failures) > 0 {
		return fmt.Errorf("self-test failed:\n  %s", strings.Join(failures, "\n  "))
	}
	fmt.Println("pathguard: self-test passed")
	return nil
}
