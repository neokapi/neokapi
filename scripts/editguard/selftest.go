package main

import (
	"fmt"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"io"
	"os"
	"strings"
)

// fixtures are packages the check must report, or must pass, each with the
// number of findings it expects.
var fixtures = []struct {
	name string
	want int
	src  string
}{
	{"a format writer pointed at a file", 1, `package f
import "github.com/neokapi/neokapi/core/format"
func write(w format.DataFormatWriter) error { return w.SetOutput("out.html") }`},
	{"any type with SetOutput(string) error", 1, `package f
type sink struct{}
func (sink) SetOutput(path string) error { return nil }
func write() error { return sink{}.SetOutput("x") }`},
	{"a flag set's output and other SetOutputs", 0, `package f
import (
	"io"
	"log"
	"github.com/spf13/pflag"
)
func quiet(fs *pflag.FlagSet, l *log.Logger) { fs.SetOutput(io.Discard); l.SetOutput(io.Discard) }`},
	{"a file replaced outside the file home", 2, `package f
import (
	"io"
	"github.com/neokapi/neokapi/core/atomicfile"
)
func write(w func(io.Writer) error) error {
	if _, err := atomicfile.Replace("out.html", w); err != nil {
		return err
	}
	_, err := atomicfile.Stage("out.html", w)
	return err
}`},
	{"reading a file's target with atomicfile", 0, `package f
import "github.com/neokapi/neokapi/core/atomicfile"
func where(p string) (string, error) { t, _, _, err := atomicfile.Resolve(p); return t, err }`},
}

// runSelfTest checks every fixture and fails when one reports other than it
// expects.
func runSelfTest() error {
	pkgs, err := list([]string{"github.com/neokapi/neokapi/core/format", "github.com/neokapi/neokapi/core/atomicfile", "github.com/spf13/pflag", "log"})
	if err != nil {
		return err
	}
	exports := map[string]string{}
	for _, p := range pkgs {
		if p.Export != "" {
			exports[p.ImportPath] = p.Export
		}
	}
	fset := token.NewFileSet()
	imp := importer.ForCompiler(fset, "gc", func(path string) (io.ReadCloser, error) {
		file, ok := exports[path]
		if !ok {
			return nil, fmt.Errorf("no export data for %s", path)
		}
		return os.Open(file)
	})
	var failures []string
	for _, fx := range fixtures {
		f, err := parser.ParseFile(fset, "fixture.go", fx.src, 0)
		if err != nil {
			return fmt.Errorf("%s: %w", fx.name, err)
		}
		found, err := checkPackage(fset, imp, "fixture", []*ast.File{f}, func(token.Pos) string { return "fixture.go" }, map[string]bool{})
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
	fmt.Println("editguard: self-test passed")
	return nil
}
