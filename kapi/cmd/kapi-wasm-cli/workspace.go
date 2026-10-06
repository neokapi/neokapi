//go:build js && wasm

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall/js"
	"time"

	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/storage"
	"github.com/neokapi/neokapi/host"
	"github.com/neokapi/neokapi/kpz"
	"github.com/neokapi/neokapi/terms"
)

// The browser workspace is a cache: the browser may evict what a page keeps,
// and a page in another browser never sees it. kapiExportWorkspace packs what
// the engine holds into a workspace package (kpz.KindWorkspace), the files of
// the engine's file system and the context of every project among them, and
// kapiImportWorkspace reads one back. A project's context travels as its
// operation log (a context package, as `kapi context export` writes one), so
// reading it back rebuilds the project's stores by merging the log. A terms
// store outside every project (`kapi terms import` run outside a project, or
// one named with --file) has no log, so it travels as a terms bundle and is
// written back into a store at the same path.

// exportSkip lists the directories an export leaves out: the data root (the
// workspace's own databases live in the driver's namespace, and its files are
// derived), scratch space, and the lab directory: the project the engine
// seeds for its annotators, and the traces a lab reads back.
var exportSkip = []string{browserDataDir, "/tmp", filepath.Dir(labProjectDir)}

// workspaceExport is what kapiExportWorkspace reports beside the package.
type workspaceExport struct {
	Files      int                 `json:"files"`
	Projects   []exportedProject   `json:"projects"`
	TermStores []exportedTermStore `json:"termStores"`
	Skipped    []skippedProject    `json:"skipped,omitempty"`
	pkg        *kpz.Package
}

// exportedTermStore is a terms store outside every project an export carries,
// and how many concepts it holds.
type exportedTermStore struct {
	Store    string `json:"store"`
	Concepts int    `json:"concepts"`
}

type exportedProject struct {
	Root       string `json:"root"`
	Operations int    `json:"operations"`
}

type skippedProject struct {
	Root   string `json:"root"`
	Reason string `json:"reason"`
}

// workspaceImport is what kapiImportWorkspace resolves to.
type workspaceImport struct {
	Files      int                 `json:"files"`
	Projects   []importedProject   `json:"projects"`
	TermStores []exportedTermStore `json:"termStores"`
}

type importedProject struct {
	Root   string `json:"root"`
	Merged int    `json:"merged"`
}

// kapiExportWorkspace packs the engine's file system and the context of each
// project in it, and each terms store outside every project. It returns a
// Promise of {data, files, projects, termStores, skipped}: data is the
// package's bytes (a Uint8Array), and skipped names each project
// whose context could not be read, with the reason. A project whose log holds
// nothing yet travels as its files alone.
func kapiExportWorkspace(_ js.Value, _ []js.Value) any {
	return goPromise(func() (any, error) {
		engineMu.Lock()
		defer engineMu.Unlock()
		if err := ensureApp(); err != nil {
			return nil, err
		}
		res, err := exportWorkspace(context.Background(), "/")
		if err != nil {
			return nil, err
		}
		data, err := res.pkg.Marshal()
		if err != nil {
			return nil, err
		}
		report, err := json.Marshal(res)
		if err != nil {
			return nil, err
		}
		var out map[string]any
		if err := json.Unmarshal(report, &out); err != nil {
			return nil, err
		}
		arr := js.Global().Get("Uint8Array").New(len(data))
		js.CopyBytesToJS(arr, data)
		out["data"] = arr
		return out, nil
	})
}

// kapiImportWorkspace reads a workspace package (a Uint8Array) into the
// engine: it writes the package's files, replacing a file at the same path,
// merges each project's context into the project's log, and adds each terms
// store's concepts to a store at its path. It returns a Promise of the report
// as a JSON string, {files, projects: [{root, merged}], termStores: [{store,
// concepts}]}.
func kapiImportWorkspace(_ js.Value, args []js.Value) any {
	var data []byte
	if len(args) >= 1 && args[0].InstanceOf(js.Global().Get("Uint8Array")) {
		data = make([]byte, args[0].Length())
		js.CopyBytesToGo(data, args[0])
	}
	return goPromise(func() (any, error) {
		if data == nil {
			return nil, errors.New("kapiImportWorkspace takes the package as a Uint8Array")
		}
		engineMu.Lock()
		defer engineMu.Unlock()
		if err := ensureApp(); err != nil {
			return nil, err
		}
		res, err := importWorkspace(context.Background(), "/", data)
		if err != nil {
			return nil, err
		}
		out, err := json.Marshal(res)
		return string(out), err
	})
}

// exportWorkspace packs the files under root and the context of every project
// among them. The caller holds engineMu.
func exportWorkspace(ctx context.Context, root string) (*workspaceExport, error) {
	res := &workspaceExport{pkg: &kpz.Package{Kind: kpz.KindWorkspace, Created: time.Now().UTC().Format(time.RFC3339)}}
	var projects []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if slices.Contains(exportSkip, p) {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		res.pkg.Files = append(res.pkg.Files, kpz.FileDoc{Path: kpz.FilePath(rel), Content: kpz.FileContent(p)})
		if d.Name() == project.RecipeFileName {
			projects = append(projects, filepath.Dir(p))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	res.Files = len(res.pkg.Files)
	res.Projects = []exportedProject{}
	for _, dir := range projects {
		rel, err := filepath.Rel(root, dir)
		if err != nil {
			return nil, err
		}
		rel = filepath.ToSlash(rel)
		pkg, exported, err := app.ProjectContextPackage(ctx, dir)
		switch {
		case errors.Is(err, host.ErrNoContext):
			continue
		case err != nil:
			res.Skipped = append(res.Skipped, skippedProject{Root: dir, Reason: err.Error()})
			continue
		}
		data, err := pkg.Marshal()
		if err != nil {
			return nil, fmt.Errorf("pack the context of %s: %w", dir, err)
		}
		res.pkg.Contexts = append(res.pkg.Contexts, kpz.ContextDoc{
			Path:    fmt.Sprintf("%s%d.kpz", kpz.ContextsDir, len(res.pkg.Contexts)+1),
			Project: rel,
			Data:    data,
		})
		res.Projects = append(res.Projects, exportedProject{Root: dir, Operations: exported.Operations})
	}
	stores, err := termStoresOutsideProjects(ctx, root, projects)
	if err != nil {
		return nil, err
	}
	res.TermStores = []exportedTermStore{}
	for _, path := range stores {
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil, err
		}
		data, concepts, err := termStoreBundle(ctx, path)
		if err != nil {
			return nil, fmt.Errorf("pack the terms store %s: %w", path, err)
		}
		res.pkg.TermStores = append(res.pkg.TermStores, kpz.TermStoreDoc{
			Path:  fmt.Sprintf("%s%d.terms.json", kpz.TermStoresDir, len(res.pkg.TermStores)+1),
			Store: filepath.ToSlash(rel),
			Data:  data,
		})
		res.TermStores = append(res.TermStores, exportedTermStore{Store: path, Concepts: concepts})
	}
	return res, nil
}

// termStoresOutsideProjects lists the terms stores at or below root that
// belong to no project: every database the driver holds there that carries
// the terms schema, apart from the data root (the workspace), the directories
// an export leaves out, and each project's own state directory, whose stores
// are projections of the project's log.
func termStoresOutsideProjects(ctx context.Context, root string, projects []string) ([]string, error) {
	paths, err := storage.List(root)
	if err != nil {
		return nil, err
	}
	skip := []string{filepath.Clean(host.DataDir())}
	for _, dir := range exportSkip {
		skip = append(skip, filepath.Clean(dir))
	}
	for _, dir := range projects {
		layout, err := project.LayoutFor(dir)
		if err != nil {
			return nil, err
		}
		skip = append(skip, filepath.Clean(layout.StateDir))
	}
	var out []string
	for _, p := range paths {
		if slices.ContainsFunc(skip, func(dir string) bool { return within(p, dir) }) {
			continue
		}
		ok, err := isTermStore(ctx, p)
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, p)
		}
	}
	return out, nil
}

// within reports whether path is dir or lies below it.
func within(path, dir string) bool {
	return path == dir || strings.HasPrefix(path, strings.TrimSuffix(dir, string(filepath.Separator))+string(filepath.Separator))
}

// isTermStore reports whether the database at path carries the terms
// schema, read without changing it.
func isTermStore(ctx context.Context, path string) (bool, error) {
	db, err := storage.OpenReadOnly(path)
	if err != nil {
		// A file that is not a database is not a terms store.
		return false, nil
	}
	defer func() { _ = db.Close() }()
	ok, err := terms.IsStore(ctx, db)
	if err != nil {
		return false, fmt.Errorf("%s: %w", path, err)
	}
	return ok, nil
}

// termStoreBundle packs the terms store at path as a terms bundle, and
// reports how many concepts it holds.
func termStoreBundle(ctx context.Context, path string) ([]byte, int, error) {
	tb, err := terms.NewSQLiteStore(path)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = tb.Close() }()
	n, err := tb.Count(ctx)
	if err != nil {
		return nil, 0, err
	}
	var buf bytes.Buffer
	if err := host.ExportKTB(ctx, tb, &buf); err != nil {
		return nil, 0, err
	}
	return buf.Bytes(), n, nil
}

// importTermStore adds a terms bundle's concepts and relations to the store
// at path, creating it when there is none. A concept already there under the
// same id is replaced.
func importTermStore(ctx context.Context, path string, data []byte) (int, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return 0, err
	}
	tb, err := terms.NewSQLiteStore(path)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tb.Close() }()
	return host.ImportKTBFile(ctx, tb, bytes.NewReader(data))
}

// importWorkspace writes a workspace package's files under root and merges
// each project's context. The caller holds engineMu.
func importWorkspace(ctx context.Context, root string, data []byte) (*workspaceImport, error) {
	pkg, err := kpz.Unmarshal(data)
	if err != nil {
		return nil, err
	}
	if pkg.Kind != kpz.KindWorkspace {
		return nil, fmt.Errorf("this is a %s package; a workspace package is the kind an export from the browser engine writes", pkg.Kind)
	}
	res := &workspaceImport{Projects: []importedProject{}}
	for _, f := range pkg.Files {
		dst := filepath.Join(root, filepath.FromSlash(kpz.FileRel(f.Path)))
		if !strings.HasPrefix(dst, filepath.Clean(root)) {
			return nil, fmt.Errorf("%s is not a path inside the workspace", f.Path)
		}
		body, err := kpz.ReadAll(f.Content)
		if err != nil {
			return nil, err
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(dst, body, 0o644); err != nil {
			return nil, err
		}
		res.Files++
	}
	for _, c := range pkg.Contexts {
		dir := filepath.Join(root, filepath.FromSlash(c.Project))
		if _, err := os.Stat(filepath.Join(dir, project.RecipeFileName)); err != nil {
			return nil, fmt.Errorf("the package carries the context of %s, and the package holds no %s there", dir, project.RecipeFileName)
		}
		inner, err := kpz.Unmarshal(c.Data)
		if err != nil {
			return nil, fmt.Errorf("read the context of %s: %w", dir, err)
		}
		pulled, err := app.ImportContextPackage(ctx, dir, inner, c.Path)
		if err != nil {
			return nil, fmt.Errorf("merge the context of %s: %w", dir, err)
		}
		res.Projects = append(res.Projects, importedProject{Root: dir, Merged: pulled.Merged})
	}
	res.TermStores = []exportedTermStore{}
	for _, s := range pkg.TermStores {
		path := filepath.Join(root, filepath.FromSlash(s.Store))
		if !within(path, filepath.Clean(root)) {
			return nil, fmt.Errorf("%s is not a path inside the workspace", s.Store)
		}
		n, err := importTermStore(ctx, path, s.Data)
		if err != nil {
			return nil, fmt.Errorf("write the terms store %s: %w", path, err)
		}
		res.TermStores = append(res.TermStores, exportedTermStore{Store: path, Concepts: n})
	}
	return res, nil
}

// goPromise runs work in a goroutine and returns a Promise of its answer. The
// work cannot run inside the callback, since the engine's file system calls
// wait on the event loop (see kapiRun).
func goPromise(work func() (any, error)) js.Value {
	executor := js.FuncOf(func(_ js.Value, p []js.Value) any {
		resolve, reject := p[0], p[1]
		go func() {
			out, err := func() (out any, err error) {
				defer func() {
					if r := recover(); r != nil {
						err = fmt.Errorf("internal error: %v", r)
					}
				}()
				return work()
			}()
			if err != nil {
				reject.Invoke(js.Global().Get("Error").New(err.Error()))
				return
			}
			resolve.Invoke(out)
		}()
		return js.Undefined()
	})
	defer executor.Release()
	return js.Global().Get("Promise").New(executor)
}
