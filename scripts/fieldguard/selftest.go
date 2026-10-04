package main

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"strings"

	"golang.org/x/tools/go/packages"
)

// fixtures are packages the check must report, or must pass, each sitting in
// a directory of the repository. want is the number of uses outside a test it
// expects to fail the check, inTest the number of uses in a test it expects to
// count, and allowed the number it expects the allowlist to accept.
var fixtures = []struct {
	name    string
	file    string
	want    int
	inTest  int
	allowed int
	src     string
}{
	{name: "a planted read of Block.Editions", file: "core/fixture/f.go", want: 1, src: `package f
import "github.com/neokapi/neokapi/core/model"
func source(b *model.Block) *model.Edition { return b.Editions[model.EditionKey{}] }`},
	{name: "each field, in selectors and a composite literal's key", file: "core/fixture/f.go", want: 5, src: `package f
import "github.com/neokapi/neokapi/core/model"
func build() *model.Block {
	b := &model.Block{Editions: nil}
	b.Native = append(b.Native, model.EditionKey{})
	_ = len(b.Editions)
	delete(b.Editions, model.Variant("fr"))
	return b
}`},
	{name: "a field reached through an embedded Block", file: "host/fixture/f.go", want: 2, src: `package f
import "github.com/neokapi/neokapi/core/model"
type wrapped struct{ model.Block }
func read(w *wrapped) (int, int) { return len(w.Editions), len(w.Block.Native) }`},
	{name: "a direct write in a test", file: "core/fixture/f_test.go", inTest: 1, src: `package f
import "github.com/neokapi/neokapi/core/model"
func plant(b *model.Block) {
	b.Editions[model.EditionKey{Locale: "nb_NO"}] = &model.Edition{}
}`},
	{name: "same-named fields of other types and the accessors", file: "core/fixture/f.go", src: `package f
import "github.com/neokapi/neokapi/core/model"
type description struct {
	Editions []string
	Native   []string
}
func read(d description, b *model.Block) {
	_, _ = d.Editions, d.Native
	_ = b.SourceRuns()
	e, _ := b.Edition(model.EditionKey{})
	b.SetEdition(model.Variant("fr"), e)
	b.SetEditionStatus(model.Variant("fr"), model.Status("translated"))
	for k := range b.EachEdition {
		_ = k
	}
	_ = b.EditionKeys()
	_ = b.NativeEditions()
	b.MarkNative(model.Variant("fr"))
	_ = b.TargetText("fr")
}`},
	{name: "a use in an allowed package", file: "core/plugin/protoconvert/f.go", allowed: 2, src: `package f
import "github.com/neokapi/neokapi/core/model"
func source(b *model.Block) (int, int) { return len(b.Editions), len(b.Native) }`},
	{name: "a use in core/model itself", file: "core/model/f.go", src: `package f
import "github.com/neokapi/neokapi/core/model"
func source(b *model.Block) *model.Edition {
	return b.Editions[model.EditionKey{}]
}`},
}

// runSelfTest type-checks every fixture against core/model and fails when one
// reports other than it expects.
func runSelfTest(root string) error {
	pc := &packages.Config{
		Mode:    packages.NeedName | packages.NeedTypes,
		Context: context.Background(),
		Dir:     root,
	}
	pkgs, err := packages.Load(pc, modelPath)
	if err != nil {
		return err
	}
	if len(pkgs) != 1 || pkgs[0].Types == nil || len(pkgs[0].Errors) > 0 {
		return fmt.Errorf("could not load %s: %v", modelPath, pkgs)
	}
	model := pkgs[0].Types
	imp := importerFunc(func(p string) (*types.Package, error) {
		if p == modelPath {
			return model, nil
		}
		return nil, fmt.Errorf("a fixture may import only %s, not %s", modelPath, p)
	})
	var failures []string
	for _, fx := range fixtures {
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, fx.file, fx.src, parser.SkipObjectResolution)
		if err != nil {
			return fmt.Errorf("%s: %w", fx.name, err)
		}
		info := &types.Info{Uses: map[*ast.Ident]types.Object{}}
		conf := types.Config{Importer: imp}
		if _, err := conf.Check("fixture", fset, []*ast.File{f}, info); err != nil {
			return fmt.Errorf("%s: %w", fx.name, err)
		}
		inv := newInventory(root, []module{{Path: "fixture", Dir: ".", Work: true}})
		scan(fset, info, inv.add)
		found := len(inv.misused())
		accepted := len(inv.uses) - inv.remaining()
		inTest := inv.remaining() - found
		if found != fx.want || accepted != fx.allowed || inTest != fx.inTest {
			var got []string
			for _, u := range inv.sortedUses() {
				got = append(got, fmt.Sprintf("%d:%d %s", u.Line, u.Col, u.Kind))
			}
			failures = append(failures, fmt.Sprintf("%s: want %d failing, %d in a test and %d allowed, got %d, %d and %d: %s",
				fx.name, fx.want, fx.inTest, fx.allowed, found, inTest, accepted, strings.Join(got, "; ")))
		}
	}
	if len(failures) > 0 {
		return fmt.Errorf("self-test failed:\n  %s", strings.Join(failures, "\n  "))
	}
	fmt.Println("fieldguard: self-test passed")
	return nil
}

// importerFunc adapts a function to types.Importer.
type importerFunc func(path string) (*types.Package, error)

func (f importerFunc) Import(path string) (*types.Package, error) { return f(path) }
