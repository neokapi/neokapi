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
// a directory of the repository, with the number of uses it expects reported
// and the number it expects the allowlist to accept.
var fixtures = []struct {
	name    string
	file    string
	want    int
	allowed int
	src     string
}{
	{"a planted read of Block.Source", "core/fixture/f.go", 1, 0, `package f
import "github.com/neokapi/neokapi/core/model"
func source(b *model.Block) []model.Run { return b.Source }`},
	{"each field, in selectors and a composite literal's key", "core/fixture/f.go", 5, 0, `package f
import "github.com/neokapi/neokapi/core/model"
func build() *model.Block {
	b := &model.Block{Source: nil}
	b.SourceStatus = model.SourceStatusWritten
	_ = len(b.Targets)
	b.Source = append(b.Source, model.Run{})
	return b
}`},
	{"a field reached through an embedded Block", "host/fixture/f.go", 2, 0, `package f
import "github.com/neokapi/neokapi/core/model"
type wrapped struct{ model.Block }
func read(w *wrapped) ([]model.Run, model.SourceStatus) { return w.Source, w.Block.SourceStatus }`},
	{"same-named fields of other types, the status type and the accessors", "core/fixture/f.go", 0, 0, `package f
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
	{"model.Target named, built and handed out", "bowrain/fixture/f.go", 5, 0, `package f
import "github.com/neokapi/neokapi/core/model"
func target(b *model.Block) *model.Target {
	t := model.NewTarget(nil, model.TargetStatusDraft)
	_ = &model.Target{}
	b.SetTarget("fr", t)
	return b.Target("fr")
}`},
	{"a use in an allowed package", "core/plugin/protoconvert/f.go", 0, 2, `package f
import "github.com/neokapi/neokapi/core/model"
func source(b *model.Block) ([]model.Run, int) { return b.Source, len(b.Targets) }`},
	{"a use in core/model itself", "core/model/f.go", 0, 0, `package f
import "github.com/neokapi/neokapi/core/model"
func source(b *model.Block) []model.Run { return b.Source }`},
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
		if found != fx.want || accepted != fx.allowed {
			var got []string
			for _, u := range inv.sortedUses() {
				got = append(got, fmt.Sprintf("%d:%d %s", u.Line, u.Col, u.Kind))
			}
			failures = append(failures, fmt.Sprintf("%s: want %d reported and %d allowed, got %d and %d: %s",
				fx.name, fx.want, fx.allowed, found, accepted, strings.Join(got, "; ")))
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
