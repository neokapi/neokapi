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

	"github.com/neokapi/neokapi/scripts/internal/buildconfigs"
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
	return list(nil, "fts5", patterns)
}

// ListConfig compiles, under one build configuration, the packages a guard
// checks there. The first configuration in buildconfigs.All checks every
// package that matches patterns, as List does. A later one checks only the
// packages holding a non-test file that no earlier configuration compiles,
// such as a _js.go or a `//go:build notelemetry` file, and returns none when
// there is no such package. In both cases a package to check is one with
// DepOnly false.
func ListConfig(cfg buildconfigs.Config, patterns ...string) (Packages, error) {
	i := buildconfigs.Index(cfg.Name)
	if i < 0 {
		return nil, fmt.Errorf("unknown build configuration %q", cfg.Name)
	}
	if i == 0 {
		return list(cfg.Env, cfg.Tags, patterns)
	}
	seen := map[string]bool{}
	for _, earlier := range buildconfigs.All[:i] {
		files, err := sourceFiles(earlier, patterns)
		if err != nil {
			return nil, err
		}
		for _, fs := range files {
			for _, f := range fs {
				seen[f] = true
			}
		}
	}
	files, err := sourceFiles(cfg, patterns)
	if err != nil {
		return nil, err
	}
	var fresh []string
	for pkg, fs := range files {
		for _, f := range fs {
			if !seen[f] {
				fresh = append(fresh, pkg)
				break
			}
		}
	}
	if len(fresh) == 0 {
		return nil, nil
	}
	return list(cfg.Env, cfg.Tags, fresh)
}

// sourceFiles lists, by import path, the non-test Go files each package that
// matches patterns compiles under cfg. It compiles nothing.
func sourceFiles(cfg buildconfigs.Config, patterns []string) (map[string][]string, error) {
	args := append([]string{"list", "-e", "-tags", cfg.Tags, "-json=ImportPath,Dir,GoFiles,CgoFiles"}, patterns...)
	cmd := exec.CommandContext(context.Background(), "go", args...)
	cmd.Env = append(os.Environ(), cfg.Env...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("go list (%s): %w\n%s", cfg.Name, err, stderr.String())
	}
	files := map[string][]string{}
	dec := json.NewDecoder(bytes.NewReader(out))
	for {
		var p struct {
			ImportPath, Dir   string
			GoFiles, CgoFiles []string
		}
		if err := dec.Decode(&p); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return nil, fmt.Errorf("go list (%s): %w", cfg.Name, err)
		}
		for _, f := range append(p.GoFiles, p.CgoFiles...) {
			files[p.ImportPath] = append(files[p.ImportPath], filepath.Join(p.Dir, f))
		}
	}
	return files, nil
}

func list(env []string, tags string, patterns []string) (Packages, error) {
	args := append([]string{"list", "-compiled", "-tags", tags, "-export", "-deps",
		"-json=ImportPath,Dir,Export,CompiledGoFiles,DepOnly,Standard"}, patterns...)
	cmd := exec.CommandContext(context.Background(), "go", args...)
	cmd.Env = append(os.Environ(), env...)
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
	return p.ParseFilesMode(fset, parser.SkipObjectResolution)
}

// ParseFilesMode parses the files the compiler selected for this package with
// the given parser mode, such as parser.ParseComments for a guard that reads
// comment directives.
func (p Package) ParseFilesMode(fset *token.FileSet, mode parser.Mode) ([]*ast.File, error) {
	files := make([]*ast.File, 0, len(p.CompiledGoFiles))
	for _, name := range p.CompiledGoFiles {
		path := name
		if !filepath.IsAbs(path) {
			path = filepath.Join(p.Dir, name)
		}
		file, err := parser.ParseFile(fset, path, nil, mode)
		if err != nil {
			return nil, err
		}
		files = append(files, file)
	}
	return files, nil
}
