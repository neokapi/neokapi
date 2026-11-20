// Package guardpackages loads compiled Go packages for the repository's architecture checks.
package guardpackages

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

// Packages contains the target packages and their dependencies.
type Packages []Package

// Package records the source and export data of a Go package.
type Package struct {
	ImportPath      string
	Dir             string
	Export          string
	CompiledGoFiles []string
	DepOnly         bool
	Standard        bool
}

// List compiles the packages that match patterns and their dependencies.
func List(patterns ...string) (Packages, error) {
	args := append([]string{"list", "-compiled", "-tags", "fts5", "-export", "-deps",
		"-json=ImportPath,Dir,Export,CompiledGoFiles,DepOnly,Standard"}, patterns...)
	cmd := exec.CommandContext(context.Background(), "go", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("go list: %w\n%s", err, stderr.String())
	}
	dec := json.NewDecoder(bytes.NewReader(out))
	var pkgs Packages
	for {
		var p Package
		if err := dec.Decode(&p); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return nil, fmt.Errorf("go list: %w", err)
		}
		pkgs = append(pkgs, p)
	}
	return pkgs, nil
}

// Importer resolves types from the compiler output for these packages.
func (pkgs Packages) Importer(fset *token.FileSet) types.Importer {
	exports := make(map[string]string, len(pkgs))
	for _, p := range pkgs {
		if p.Export != "" {
			exports[p.ImportPath] = p.Export
		}
	}
	return importer.ForCompiler(fset, "gc", func(path string) (io.ReadCloser, error) {
		file, ok := exports[path]
		if !ok {
			return nil, fmt.Errorf("no export data for %s", path)
		}
		return os.Open(file)
	})
}

// ParseFiles parses the files the compiler selected for this package.
func (p Package) ParseFiles(fset *token.FileSet) ([]*ast.File, error) {
	files := make([]*ast.File, 0, len(p.CompiledGoFiles))
	for _, name := range p.CompiledGoFiles {
		path := name
		if !filepath.IsAbs(path) {
			path = filepath.Join(p.Dir, name)
		}
		file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			return nil, err
		}
		files = append(files, file)
	}
	return files, nil
}
