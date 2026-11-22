package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strings"

	"github.com/neokapi/neokapi/scripts/internal/guardpackages"
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
	{"a file replaced outside the file home", 3, `package f
import (
	"io"
	"github.com/neokapi/neokapi/core/atomicfile"
)
func write(w func(io.Writer) error) error {
	if _, err := atomicfile.Replace("out.html", w); err != nil {
		return err
	}
	if _, err := atomicfile.Stage("out.html", w); err != nil {
		return err
	}
	_, err := atomicfile.StageWithin(".", "out.html", w)
	return err
}`},
	{"reading a file's target with atomicfile", 0, `package f
import "github.com/neokapi/neokapi/core/atomicfile"
func where(p string) (string, error) { t, _, _, err := atomicfile.Resolve(p); return t, err }`},
	{"a file created for writing inside a root", 1, `package f
import "github.com/neokapi/neokapi/core/rootpath"
func write(root, p string) error { f, err := rootpath.CreateRegular(root, p); if err == nil { f.Close() }; return err }`},
	{"a file opened for reading inside a root", 0, `package f
import "github.com/neokapi/neokapi/core/rootpath"
func read(root, p string) error { f, err := rootpath.OpenRegular(root, p); if err == nil { f.Close() }; return err }`},
}

// runSelfTest checks every fixture and fails when one reports other than it
// expects.
func runSelfTest() error {
	pkgs, err := guardpackages.List("github.com/neokapi/neokapi/core/format", "github.com/neokapi/neokapi/core/atomicfile", "github.com/neokapi/neokapi/core/rootpath", "github.com/spf13/pflag", "log")
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
