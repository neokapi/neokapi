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
	{"a direct write to the terms store", 1, `package f
import (
	"context"
	"github.com/neokapi/neokapi/core/projectdb"
	"github.com/neokapi/neokapi/terms"
)
func write(ctx context.Context, db *projectdb.DB) error {
	return db.Terms().AddConcept(ctx, terms.Concept{ID: "c"})
}`},
	{"a store handed to an interface that writes", 2, `package f
import (
	"github.com/neokapi/neokapi/core/projectdb"
	"github.com/neokapi/neokapi/memory"
)
type holder struct{ tm memory.ContentMemory }
func hand(db *projectdb.DB) (memory.Store, holder) {
	return db.Memory(), holder{}
}
func keep(db *projectdb.DB) holder { return holder{tm: db.Memory()} }`},
	{"a store passed to a function that takes a writable interface", 1, `package f
import (
	"github.com/neokapi/neokapi/core/projectdb"
	"github.com/neokapi/neokapi/memory"
)
func use(tm memory.ContentMemory) {}
func pass(db *projectdb.DB) { use(db.Memory()) }`},
	{"a project store wrapped as a standalone one", 1, `package f
import (
	"github.com/neokapi/neokapi/core/projectdb"
	"github.com/neokapi/neokapi/core/projector"
)
func sneak(db *projectdb.DB) *projector.Memory { return projector.StandaloneMemory(db.Memory()) }`},
	{"reads and the projector's own stores", 0, `package f
import (
	"context"
	"github.com/neokapi/neokapi/core/projectdb"
	"github.com/neokapi/neokapi/core/projector"
	"github.com/neokapi/neokapi/memory"
	"github.com/neokapi/neokapi/terms"
)
func read(ctx context.Context, db *projectdb.DB) (int, error) { return db.Terms().Count(ctx) }
func write(ctx context.Context, p *projector.Projector) (memory.Store, error) {
	return p.Memory(), p.Terms().AddConcept(ctx, terms.Concept{ID: "c"})
}`},
}

// runSelfTest checks every fixture and fails when one reports other than it
// expects.
func runSelfTest() error {
	pkgs, err := list([]string{
		"github.com/neokapi/neokapi/core/projectdb",
		"github.com/neokapi/neokapi/core/projector",
	})
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
		found, err := checkPackage(fset, imp, "fixture", []*ast.File{f}, func(token.Pos) string { return "fixture.go" })
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
	fmt.Println("projectionguard: self-test passed")
	return nil
}
