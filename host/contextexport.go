package host

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/projector"
	"github.com/neokapi/neokapi/core/workspace"
	"github.com/neokapi/neokapi/kpz"
)

// A transfer file is a project's shared context in one archive: the layout a
// context backend keeps (the segments of the operation log, the blobs they
// name, and a checkpoint), packed as a context .kpz (kpz.KindContext).
//
// `kapi context export` pushes the project into an empty layout held in memory
// and writes it out; `kapi context import <file>.kpz` reads one back and merges
// it the way a pull merges a backend, history included. Nothing a push leaves
// on the machine travels in it: withheld originals are never operations, and
// the kinds projector.LocalKinds names stay behind.

// ContextExport reports what an export wrote.
type ContextExport struct {
	// Path is the file written.
	Path string `json:"path"`
	// Bytes is its size.
	Bytes int64 `json:"bytes"`
	// Operations counts the operations it carries.
	Operations int `json:"operations"`
	// Segments and Blobs count the files of the layout it carries.
	Segments int `json:"segments"`
	Blobs    int `json:"blobs"`
	// Checkpoint is the operation its checkpoint stands at.
	Checkpoint string `json:"checkpoint,omitempty"`
}

// FormatText renders the export for a reader.
func (r ContextExport) FormatText(w io.Writer) error {
	_, err := fmt.Fprintf(w, "Wrote %s to %s (%s).\n",
		pluralUnit(r.Operations, "context operation", "context operations"), DisplayName(r.Path), humanBytes(r.Bytes))
	return err
}

// humanBytes renders a size the way a person reads one.
func humanBytes(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MiB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1f KiB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%d bytes", n)
}

// ExportProjectContext writes the project's context, with its whole history,
// to one transfer file.
func (a *App) ExportProjectContext(ctx context.Context, projectPath, out string) (ContextExport, error) {
	var res ContextExport
	if out == "" {
		return res, errors.New("name the file to write: kapi context export -o context.kpz")
	}
	abs, err := filepath.Abs(out)
	if err != nil {
		return res, err
	}
	res.Path = abs
	layout, err := project.LayoutFor(projectPath)
	if err != nil {
		return res, err
	}
	w, ws, err := a.projectLog(ctx, layout)
	if err != nil {
		return res, err
	}
	mem, err := workspace.NewMemoryRemote(abs)
	if err != nil {
		return res, err
	}
	s := ws.NewSync(mem, w.Key(), w.Syncer(), workspace.SyncOptions{LocalKinds: projector.LocalKinds, CheckpointEvery: 1})
	defer func() { _ = s.Forget(context.WithoutCancel(ctx)) }()
	pushed, err := s.Push(ctx)
	if err != nil {
		return res, err
	}
	if pushed.Pushed == 0 {
		return res, errors.New("this project's context holds nothing to export yet")
	}
	res.Operations, res.Segments, res.Blobs, res.Checkpoint = pushed.Pushed, pushed.Segments, pushed.Blobs, pushed.Checkpoint

	pkg := &kpz.Package{Kind: kpz.KindContext, Created: time.Now().UTC().Format(time.RFC3339)}
	for _, obj := range mem.Objects() {
		pkg.Layout = append(pkg.Layout, kpz.LayoutDoc{Path: obj.Name, Data: obj.Data})
	}
	res.Bytes, err = writePackage(pkg, abs)
	return res, err
}

// writePackage writes a package to a file through a temporary sibling, so a
// failed write never leaves half a file where a whole one was.
func writePackage(pkg *kpz.Package, out string) (int64, error) {
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return 0, err
	}
	tmp, err := os.CreateTemp(filepath.Dir(out), ".kapi-export-*")
	if err != nil {
		return 0, err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	n, err := pkg.WriteTo(tmp)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return 0, fmt.Errorf("write %s: %w", out, err)
	}
	return n, os.Rename(tmp.Name(), out)
}

// projectLog returns the project's writer and the workspace its log lives in.
func (a *App) projectLog(ctx context.Context, layout project.Layout) (*projector.Projector, *workspace.Workspace, error) {
	w, err := a.Projector(ctx, layout.Root)
	if err != nil {
		return nil, nil, err
	}
	if !w.Logged() {
		return nil, nil, errors.New("this project's store has no workspace log")
	}
	ws, err := a.projectWorkspace(ctx, layout.Root)
	return w, ws, err
}

// projectWorkspace returns the workspace the project rooted at root keeps its
// context in: the machine's workspace, or under test one of the project's own.
func (a *App) projectWorkspace(ctx context.Context, root string) (*workspace.Workspace, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	s := a.ensureProjectStores()
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.workspaceAt(ctx, s.workspaceRootFor(abs))
}

// ContextFileImport reports what reading a transfer file merged.
type ContextFileImport struct {
	workspace.PullReport
	// Path is the file read.
	Path string `json:"path"`
}

// FormatText renders the import for a reader.
func (r ContextFileImport) FormatText(w io.Writer) error {
	if r.Merged == 0 {
		_, err := fmt.Fprintf(w, "%s holds nothing this project does not already hold.\n", DisplayName(r.Path))
		return err
	}
	from := ""
	if r.Checkpoint != "" {
		from = ", starting from its checkpoint"
	}
	_, err := fmt.Fprintf(w, "Merged %s from %s%s.\n",
		pluralUnit(r.Merged, "context operation", "context operations"), DisplayName(r.Path), from)
	return err
}

// ImportContextFile merges a transfer file into the project's context, the
// way a pull merges a backend: every operation it carries that the log does
// not hold, with its history, applied to the stores.
func (a *App) ImportContextFile(ctx context.Context, projectPath, file string) (ContextFileImport, error) {
	res := ContextFileImport{Path: file}
	pkg, closer, err := kpz.OpenFile(file)
	if err != nil {
		return res, fmt.Errorf("read %s: %w", file, err)
	}
	_ = closer.Close()
	switch {
	case pkg.Kind != kpz.KindContext:
		return res, fmt.Errorf("%s is a %s package; `kapi context import` reads a context file, which `kapi context export` writes", file, pkg.Kind)
	case len(pkg.Layout) == 0:
		return res, fmt.Errorf("%s holds no context operations. A context file from an earlier kapi carried the stores rather than their history; export it again with this version", file)
	}
	layout, err := project.LayoutFor(projectPath)
	if err != nil {
		return res, err
	}
	w, ws, err := a.projectLog(ctx, layout)
	if err != nil {
		return res, err
	}
	objs := make([]workspace.Object, 0, len(pkg.Layout))
	for _, l := range pkg.Layout {
		objs = append(objs, workspace.Object{Name: l.Path, Data: l.Data})
	}
	if other := layoutProject(objs); other != "" && other != string(w.Key()) {
		return res, fmt.Errorf("%s holds the context of project %s, and this project is %s. Import it in a checkout of that project", file, other, w.Key())
	}
	abs, err := filepath.Abs(file)
	if err != nil {
		return res, err
	}
	mem, err := workspace.NewMemoryRemote(abs, objs...)
	if err != nil {
		return res, fmt.Errorf("%s: %w", file, err)
	}
	s := ws.NewSync(mem, w.Key(), w.Syncer(), workspace.SyncOptions{LocalKinds: projector.LocalKinds})
	defer func() { _ = s.Forget(context.WithoutCancel(ctx)) }()
	res.PullReport, err = s.Pull(ctx)
	return res, err
}

// layoutProject reads the project the first segment's first operation
// belongs to, empty when there is none.
func layoutProject(objs []workspace.Object) string {
	for _, obj := range objs {
		if len(obj.Name) < len(workspace.RemoteLogDir) || obj.Name[:len(workspace.RemoteLogDir)] != workspace.RemoteLogDir {
			continue
		}
		line, _, _ := bytes.Cut(obj.Data, []byte("\n"))
		var head struct {
			Project string `json:"project"`
		}
		if json.Unmarshal(line, &head) == nil {
			return head.Project
		}
	}
	return ""
}
