package main

import (
	"fmt"
	"go/ast"
	"go/token"
	"slices"
	"strings"

	"github.com/neokapi/neokapi/scripts/internal/buildconfigs"
	"github.com/neokapi/neokapi/scripts/internal/guardpackages"
)

// fixtures are packages the check must report, or must pass, each with the
// number of findings it expects.
var fixtures = []struct {
	name string
	want int
	src  string
}{
	// The matcher.
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
	{"a method named Open", 0, `package f
type store struct{}
func (store) Open(p string) error { return nil }
func use(s store) error { return s.Open("x") }`},
	{"a file opened for reading inside a root", 0, `package f
import "github.com/neokapi/neokapi/core/rootpath"
func read(root, p string) error { f, err := rootpath.OpenRegular(root, p); if err == nil { f.Close() }; return err }`},
	{"the sinks added beside os", 8, `package f
import (
	"archive/zip"
	"database/sql"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"

	"github.com/bmatcuk/doublestar/v4"
)
func all(p string) {
	_, _ = filepath.Glob(p)
	_, _ = filepath.EvalSymlinks(p)
	_ = os.Chmod(p, 0o600)
	_, _ = ioutil.ReadFile(p)
	_, _ = doublestar.FilepathGlob(p)
	_, _ = sql.Open("sqlite3", p)
	_, _ = zip.OpenReader(p)
	_ = http.FileServer(http.Dir(p))
}`},
	{"a value of type http.Dir that is no conversion", 0, `package f
import "net/http"
var root http.Dir
func serve() http.Handler { return http.FileServer(root) }`},

	// The comment rules.
	{"a call with a reason above it", 0, `package f
import "os"
func read(p string) ([]byte, error) {
	//pathguard: a system location the fixture reads
	return os.ReadFile(p)
}`},
	{"a call with a reason on its line", 0, `package f
import "os"
func read(p string) ([]byte, error) {
	return os.ReadFile(p) //pathguard: a system location the fixture reads
}`},
	{"a reason two lines above the call", 2, `package f
import "os"
func read(p string) ([]byte, error) {
	//pathguard: a system location the fixture reads

	return os.ReadFile(p)
}`},
	{"a reason after code on the line above", 2, `package f
import "os"
func read(p string) ([]byte, error) {
	_ = p //pathguard: a system location the fixture reads
	return os.ReadFile(p)
}`},
	{"a comment with no reason", 2, `package f
import "os"
func read(p string) ([]byte, error) {
	//pathguard:
	return os.ReadFile(p)
}`},
	{"a comment on a line with no call", 1, `package f
func read(p string) string {
	//pathguard: a system location the fixture reads
	return p
}`},
	{"a marker spelled with a space", 2, `package f
import "os"
// read reads p.
// pathguard: a system location the fixture reads
func read(p string) ([]byte, error) { return os.ReadFile(p) }`},
	{"a comment allows every call on its line", 0, `package f
import "os"
func ensure(p string) error {
	//pathguard: a system location the fixture reads
	if _, err := os.Stat(p); err != nil { return os.MkdirAll(p, 0o755) }
	return nil
}`},

	// The walk.
	{"a function literal in a package-level variable", 1, `package f
import "os"
var read = func(p string) ([]byte, error) { return os.ReadFile(p) }`},
	{"a function literal inside a function", 1, `package f
import "os"
func run(p string) func() error {
	return func() error { return os.Remove(p) }
}`},
	{"a function value", 1, `package f
import "os"
func pick() func(string) (*os.File, error) {
	open := os.Open
	return open
}`},
	{"a function passed as an argument", 1, `package f
import "os"
func apply(f func(string) ([]byte, error), p string) { _, _ = f(p) }
func run(p string) { apply(os.ReadFile, p) }`},
	{"a function value in a package-level variable", 1, `package f
import "os"
var remove = os.Remove`},

	// The wrapper rule, one level.
	{"a caller of a wrapper", 1, `package f
import "os"
func load(p string) ([]byte, error) {
	//pathguard: the fixture's wrapper
	return os.ReadFile(p)
}
func use() { _, _ = load("x") }`},
	{"a caller of a method wrapper", 1, `package f
import "os"
type cache struct{}
func (cache) load(p string) ([]byte, error) {
	return os.ReadFile(p) //pathguard: the fixture's wrapper
}
func use(c cache) { _, _ = c.load("x") }`},
	{"a caller of a wrapper with a reason", 0, `package f
import "os"
func load(p string) ([]byte, error) {
	//pathguard: the fixture's wrapper
	return os.ReadFile(p)
}
func use() {
	//pathguard: a literal name in the fixture
	_, _ = load("x")
}`},
	{"a wrapper taken as a value", 1, `package f
import "os"
func load(p string) ([]byte, error) {
	//pathguard: the fixture's wrapper
	return os.ReadFile(string(p))
}
var loader = load`},
	{"a function whose allowed call names no parameter", 0, `package f
import "os"
func config(name string) ([]byte, error) {
	_ = name
	//pathguard: a fixed system location
	return os.ReadFile("/etc/fixture")
}
func use() { _, _ = config("x") }`},
	{"a caller of a caller of a wrapper", 0, `package f
import "os"
func load(p string) ([]byte, error) {
	//pathguard: the fixture's wrapper
	return os.ReadFile(p)
}
func through(p string) ([]byte, error) {
	//pathguard: a name the fixture's caller gives
	return load(p)
}
func use() { _, _ = through("x") }`},
	{"a caller of a helper that contains its paths", 0, `package f
import "os"
// load reads p beneath root.
//
//pathguard:contains it reads p only after checking it lies beneath root
func load(root, p string) ([]byte, error) {
	//pathguard: the fixture's checked path
	return os.ReadFile(p)
}
func use() { _, _ = load("/r", "/r/x") }`},
	{"a helper's own call still needs its comment", 1, `package f
import "os"
//pathguard:contains it reads p only after checking it lies beneath root
func load(root, p string) ([]byte, error) { return os.ReadFile(p) }
func use() { _, _ = load("/r", "/r/x") }`},
	{"a contains marker with no reason", 1, `package f
import "os"
//pathguard:contains
func load(p string) ([]byte, error) {
	return os.ReadFile(p) //pathguard: the fixture's checked path
}
func use() { _, _ = load("x") }`},
	{"a contains marker on a function with no path parameter", 1, `package f
import "os"
//pathguard:contains it reads a fixed file
func load() ([]byte, error) {
	return os.ReadFile("/etc/fixture") //pathguard: a fixed system location
}`},
	{"a contains marker outside a doc comment", 1, `package f
func use() {
	//pathguard:contains a stray marker
}`},
	{"a function whose call is reported, not allowed", 1, `package f
import "os"
func load(p string) ([]byte, error) { return os.ReadFile(p) }
func use() { _, _ = load("x") }`},
}

// runSelfTest checks every fixture, then plants a violation in every package
// the check covers, and fails when a fixture reports other than it expects or
// a planted violation goes unreported.
func runSelfTest() error {
	var failures []string
	for _, e := range excluded {
		if e.reason == "" {
			failures = append(failures, e.path+": excluded with no reason")
		}
	}
	for _, p := range mustCover {
		if !covered(p) {
			failures = append(failures, p+": the coverage rule leaves it out")
		}
	}
	pkgs, err := guardpackages.List("github.com/neokapi/neokapi/core/rootpath", "os", "io/fs", "io/ioutil", "path/filepath", "embed",
		"archive/zip", "database/sql", "net/http", "github.com/bmatcuk/doublestar/v4")
	if err != nil {
		return err
	}
	fset := token.NewFileSet()
	imp := pkgs.Importer(fset)
	for i, fx := range fixtures {
		a := newAnalysis(fset, true, func(s string) string { return s })
		f, err := a.parse(fmt.Sprintf("fixture%d.go", i), []byte(fx.src))
		if err != nil {
			return fmt.Errorf("%s: %w", fx.name, err)
		}
		// A fixture sits at a covered path, so its own functions can be wrappers.
		if err := a.addPackage(imp, modulePrefix+"/fixture", []*ast.File{f}); err != nil {
			return fmt.Errorf("%s: %w", fx.name, err)
		}
		found := a.findings()
		if len(found) != fx.want {
			texts := make([]string, len(found))
			for j, x := range found {
				texts[j] = x.text
			}
			failures = append(failures, fmt.Sprintf("%s: want %d findings, got %d: %s", fx.name, fx.want, len(found), strings.Join(texts, "; ")))
		}
	}

	found, planted, err := check(buildconfigs.All[0], true)
	if err != nil {
		return err
	}
	for pkg, file := range planted {
		if !slices.ContainsFunc(found, func(f finding) bool { return f.pkg == pkg && strings.HasPrefix(f.text, file+":") }) {
			failures = append(failures, pkg+": a violation planted in "+file+" went unreported")
		}
	}
	for _, p := range mustCover {
		if _, ok := planted[p]; !ok {
			failures = append(failures, p+": not checked")
		}
	}
	if len(failures) > 0 {
		return fmt.Errorf("self-test failed:\n  %s", strings.Join(failures, "\n  "))
	}
	fmt.Printf("pathguard: self-test passed (%d fixtures, %d packages planted)\n", len(fixtures), len(planted))
	return nil
}
