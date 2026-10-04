package main

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"slices"
	"strings"

	"golang.org/x/tools/go/packages"
)

// fixtures are packages the check must report, or must pass, each sitting in
// a directory of the repository. want is the number of uses of the fields, the
// type and the functions it expects reported, and allowed the number it
// expects the allowlist to accept. inTest is the number of test-only helper
// uses it expects from a test, and misused the number from anywhere else.
var fixtures = []struct {
	name    string
	file    string
	want    int
	allowed int
	inTest  int
	misused int
	src     string
}{
	{name: "a planted read of Block.Source", file: "core/fixture/f.go", want: 1, src: `package f
import "github.com/neokapi/neokapi/core/model"
func source(b *model.Block) []model.Run { return b.Source }`},
	{name: "each field, in selectors and a composite literal's key", file: "core/fixture/f.go", want: 5, src: `package f
import "github.com/neokapi/neokapi/core/model"
func build() *model.Block {
	b := &model.Block{Source: nil}
	b.SourceStatus = model.SourceStatusWritten
	_ = len(b.Targets)
	b.Source = append(b.Source, model.Run{})
	return b
}`},
	{name: "a field reached through an embedded Block", file: "host/fixture/f.go", want: 2, src: `package f
import "github.com/neokapi/neokapi/core/model"
type wrapped struct{ model.Block }
func read(w *wrapped) ([]model.Run, model.SourceStatus) { return w.Source, w.Block.SourceStatus }`},
	{name: "same-named fields of other types, the status type and the accessors", file: "core/fixture/f.go", src: `package f
import "github.com/neokapi/neokapi/core/model"
type message struct {
	Source       []string
	Targets      map[string]string
	SourceStatus string
}
func read(m message, a *model.AltTranslation, b *model.Block) {
	_, _, _ = m.Source, m.Targets, m.SourceStatus
	_ = a.Source
	var s model.SourceStatus = model.SourceStatusWritten
	_ = s
	_ = b.SourceRuns()
	e, _ := b.Edition(model.EditionKey{})
	b.SetEdition(model.Variant("fr"), e)
	b.SetEditionStatus(model.Variant("fr"), model.Status("translated"))
	for k := range b.EachEdition {
		_ = k
	}
	_ = b.TargetText("fr")
}`},
	{name: "model.Target named, built and handed out", file: "bowrain/fixture/f.go", want: 5, src: `package f
import "github.com/neokapi/neokapi/core/model"
func target(b *model.Block) *model.Target {
	t := model.NewTarget(nil, model.TargetStatusDraft)
	_ = &model.Target{}
	b.SetTarget("fr", t)
	return b.Target("fr")
}`},
	{name: "a use in an allowed package", file: "core/plugin/protoconvert/f.go", allowed: 2, src: `package f
import "github.com/neokapi/neokapi/core/model"
func source(b *model.Block) ([]model.Run, int) { return b.Source, len(b.Targets) }`},
	{name: "a use in core/model itself", file: "core/model/f.go", src: `package f
import "github.com/neokapi/neokapi/core/model"
func source(b *model.Block) []model.Run {
	b.FileTargetAsSpelled(model.EditionKey{Locale: "nb_NO"}, model.Edition{})
	return b.Source
}`},
	{name: "a test-only helper called outside a test", file: "core/fixture/f.go", misused: 1, src: `package f
import "github.com/neokapi/neokapi/core/model"
func plant(b *model.Block) {
	b.FileTargetAsSpelled(model.EditionKey{Locale: "nb_NO"}, model.Edition{})
}`},
	{name: "a test-only helper as a method value, a method expression and through an embedded Block", file: "host/fixture/f.go", misused: 3, src: `package f
import "github.com/neokapi/neokapi/core/model"
type wrapped struct{ model.Block }
func plant(w *wrapped, b *model.Block) {
	f := b.FileTargetAsSpelled
	f(model.EditionKey{}, model.Edition{})
	(*model.Block).FileTargetAsSpelled(b, model.EditionKey{}, model.Edition{})
	w.FileTargetAsSpelled(model.EditionKey{}, model.Edition{})
}`},
	{name: "a test-only helper in an allowed package", file: "core/plugin/protoconvert/f.go", misused: 1, src: `package f
import "github.com/neokapi/neokapi/core/model"
func plant(b *model.Block) {
	b.FileTargetAsSpelled(model.EditionKey{Locale: "nb_NO"}, model.Edition{})
}`},
	{name: "a test-only helper called from a test", file: "core/fixture/f_test.go", inTest: 1, src: `package f
import "github.com/neokapi/neokapi/core/model"
func plant(b *model.Block) {
	b.FileTargetAsSpelled(model.EditionKey{Locale: "nb_NO"}, model.Edition{})
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
		found := inv.remaining()
		accepted := len(inv.uses) - found
		misused := len(inv.misused())
		inTest := len(inv.helpers) - misused
		if found != fx.want || accepted != fx.allowed || inTest != fx.inTest || misused != fx.misused {
			var got []string
			for _, u := range sortUses(slices.Concat(inv.uses, inv.helpers)) {
				got = append(got, fmt.Sprintf("%d:%d %s", u.Line, u.Col, u.Kind))
			}
			failures = append(failures, fmt.Sprintf("%s: want %d reported, %d allowed, %d test-only helper uses in a test and %d outside one, got %d, %d, %d and %d: %s",
				fx.name, fx.want, fx.allowed, fx.inTest, fx.misused, found, accepted, inTest, misused, strings.Join(got, "; ")))
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
