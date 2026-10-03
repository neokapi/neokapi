package main

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
)

// modulePath is the framework module; a package's import path under it is its
// directory in the repository.
const modulePath = "github.com/neokapi/neokapi/"

// goDocs holds the doc comments and string constants of Go packages, read
// from their source, keyed by package name and declaration (change.Result,
// change.Result.Record).
type goDocs struct {
	types  map[string]string
	fields map[string]string
	consts map[string][]goConst
	// unspelled names the constants of a named type whose value the source
	// does not spell as a string literal (a conversion, another constant, an
	// implicit repetition), by type.
	unspelled map[string][]string
}

// goConst is one string constant of a named type, in source order.
type goConst struct {
	name  string
	value string
	doc   string
}

// parseGoDocs reads the doc comments and typed string constants of the
// packages in dirs, which are relative to the repository root.
func parseGoDocs(dirs ...string) (*goDocs, error) {
	root, err := repoRoot()
	if err != nil {
		return nil, err
	}
	d := newGoDocs()
	for _, dir := range dirs {
		files, err := filepath.Glob(filepath.Join(root, dir, "*.go"))
		if err != nil {
			return nil, err
		}
		if len(files) == 0 {
			return nil, fmt.Errorf("no Go files in %s", filepath.Join(root, dir))
		}
		slices.Sort(files)
		fset := token.NewFileSet()
		for _, file := range files {
			if strings.HasSuffix(file, "_test.go") {
				continue
			}
			src, err := os.ReadFile(file)
			if err != nil {
				return nil, err
			}
			f, err := parser.ParseFile(fset, file, src, parser.ParseComments)
			if err != nil {
				return nil, err
			}
			d.add(f.Name.Name, f)
		}
	}
	return d, nil
}

func newGoDocs() *goDocs {
	return &goDocs{types: map[string]string{}, fields: map[string]string{}, consts: map[string][]goConst{}, unspelled: map[string][]string{}}
}

// repoRoot is the nearest directory at or above the working directory that
// holds go.work: the repository root, whether the generator runs from there
// or its tests run from this module.
func repoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.work")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("no go.work at or above the working directory; run the generator inside the repository")
		}
		dir = parent
	}
}

func (d *goDocs) add(pkg string, f *ast.File) {
	for _, decl := range f.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok {
			continue
		}
		switch gen.Tok {
		case token.TYPE:
			for _, spec := range gen.Specs {
				ts := spec.(*ast.TypeSpec)
				doc := ts.Doc
				if doc == nil && len(gen.Specs) == 1 {
					doc = gen.Doc
				}
				key := pkg + "." + ts.Name.Name
				d.types[key] = commentText(doc)
				st, ok := ts.Type.(*ast.StructType)
				if !ok {
					continue
				}
				for _, field := range st.Fields.List {
					text := commentText(field.Doc)
					if text == "" {
						text = commentText(field.Comment)
					}
					for _, name := range field.Names {
						d.fields[key+"."+name.Name] = text
					}
				}
			}
		case token.CONST:
			// prev is the type of the last typed spec, which a spec with
			// neither a type nor a value repeats.
			var prev *ast.Ident
			for _, spec := range gen.Specs {
				vs := spec.(*ast.ValueSpec)
				if vs.Type == nil && len(vs.Values) == 0 && prev != nil {
					for _, name := range vs.Names {
						d.unspell(pkg+"."+prev.Name, name.Name)
					}
					continue
				}
				ident, ok := vs.Type.(*ast.Ident)
				if !ok {
					prev = nil
					continue
				}
				prev = ident
				key := pkg + "." + ident.Name
				for i, name := range vs.Names {
					var lit *ast.BasicLit
					if i < len(vs.Values) {
						lit, _ = vs.Values[i].(*ast.BasicLit)
					}
					if lit == nil || lit.Kind != token.STRING {
						d.unspell(key, name.Name)
						continue
					}
					value, err := strconv.Unquote(lit.Value)
					if err != nil {
						d.unspell(key, name.Name)
						continue
					}
					doc := commentText(vs.Doc)
					if doc == "" {
						doc = commentText(vs.Comment)
					}
					d.consts[key] = append(d.consts[key], goConst{name: name.Name, value: value, doc: doc})
				}
			}
		}
	}
}

func (d *goDocs) unspell(key, name string) {
	d.unspelled[key] = append(d.unspelled[key], name)
}

// stringConsts is the constants of the string type t, in source order. A
// constant of t whose value the source does not spell as a string literal
// fails, so a union never leaves one out.
func (d *goDocs) stringConsts(t reflect.Type) ([]goConst, error) {
	key := goKey(t)
	if names := d.unspelled[key]; len(names) > 0 {
		return nil, fmt.Errorf("the value of %s, of type %s, is not a string literal; spell it as one", strings.Join(names, ", "), goPath(t))
	}
	cs := d.consts[key]
	if len(cs) == 0 {
		return nil, fmt.Errorf("no constants of type %s found in the source", goPath(t))
	}
	return cs, nil
}

// commentText is a comment group's text without its trailing newline.
func commentText(g *ast.CommentGroup) string {
	if g == nil {
		return ""
	}
	return strings.TrimSpace(g.Text())
}

// goKey is the key a type's declaration is held under: change.Result.
func goKey(t reflect.Type) string {
	return path.Base(t.PkgPath()) + "." + t.Name()
}

// goPath names a type by its directory in the repository: core/change.Result.
func goPath(t reflect.Type) string {
	return strings.TrimPrefix(t.PkgPath(), modulePath) + "." + t.Name()
}

// typeDoc is a type's doc comment.
func (d *goDocs) typeDoc(t reflect.Type) string {
	return d.types[goKey(t)]
}

// fieldDoc is a struct field's doc comment.
func (d *goDocs) fieldDoc(t reflect.Type, field string) string {
	return d.fields[goKey(t)+"."+field]
}

// constDoc is a constant's doc as a union member's: without the constant's
// own name, which the TypeScript does not have. "SetApplied: every operation
// landed." reads "every operation landed."; "ModeApply applies the change
// set." reads "applies the change set.".
func constDoc(c goConst) string {
	doc := oneLine(c.doc)
	if rest, ok := strings.CutPrefix(doc, c.name+": "); ok {
		return rest
	}
	if rest, ok := strings.CutPrefix(doc, c.name+" "); ok {
		return rest
	}
	return doc
}
