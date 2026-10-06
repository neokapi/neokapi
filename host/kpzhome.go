package host

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/projector"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/workhome"
	"github.com/neokapi/neokapi/core/workspace"
)

// A KPZ opened for editing keeps each source document it carries in the
// workspace home, whole (workhome.Documents): the change service reads and
// writes work.kpz!guide.md like any document, with if_match, and each write is
// a content.edit operation in a log kept in the KPZ's working cache
// (<cache>/log). The first edit to a document opens it: the cache's copy of
// the source is recorded as the document's first head. A source the KPZ
// carries as a skeleton alone keeps no text and cannot be opened.
//
// The log is the truth for a document opened this way. Before a transform,
// a merge, kapi info or kapi pack reads the cache, the cache's copy of each
// edited source is brought to the document's head and its skeleton captured
// again (syncKpzEdits), so the edit reaches every output and the next pack.
// An edited source travels in the pack with its bytes, because a skeleton
// keeps no source text.

// kpzLogKey is the project key a KPZ's log keeps its documents under.
const kpzLogKey workspace.ProjectKey = "kpz"

// kpzLogDir is the directory of a cache's log.
func kpzLogDir(cacheDir string) string { return filepath.Join(cacheDir, "log") }

// kpzLog is the log of one KPZ's working cache, and the projector over it.
type kpzLog struct {
	ws *workspace.Workspace
	p  *projector.Projector
	st projector.Stores
}

// kpzLogs are the KPZ logs this process holds open, by cache directory, so
// every change service and every read of one KPZ shares one projector.
var kpzLogs = struct {
	sync.Mutex
	m map[string]*kpzLog
}{m: map[string]*kpzLog{}}

// openKpzLog opens the log of the cache in dir, creating it on first use.
func openKpzLog(ctx context.Context, dir string) (*kpzLog, error) {
	kpzLogs.Lock()
	defer kpzLogs.Unlock()
	if l, ok := kpzLogs.m[dir]; ok {
		return l, nil
	}
	ws, err := workspace.OpenLocal(ctx, kpzLogDir(dir))
	if err != nil {
		return nil, fmt.Errorf("kpz: open the log: %w", err)
	}
	raw, err := ws.Context(ctx, kpzLogKey)
	if err != nil {
		_ = ws.Close()
		return nil, fmt.Errorf("kpz: open the log: %w", err)
	}
	st, err := projector.ContextStores(raw)
	if err != nil {
		_ = ws.Close()
		return nil, err
	}
	p, err := projector.New(ws, kpzLogKey, st)
	if err != nil {
		_ = ws.Close()
		return nil, err
	}
	l := &kpzLog{ws: ws, p: p, st: st}
	kpzLogs.m[dir] = l
	return l, nil
}

// closeKpzLog closes the log of the cache in dir, before the cache is
// rebuilt or removed.
func closeKpzLog(dir string) {
	kpzLogs.Lock()
	defer kpzLogs.Unlock()
	if l, ok := kpzLogs.m[dir]; ok {
		_ = l.ws.Close()
		delete(kpzLogs.m, dir)
	}
}

// kpzSource finds the source of a cache a document key names: its name, or
// its path in the archive.
func (c *kpzCache) kpzSource(key string) (int, bool) {
	for i, s := range c.meta.Sources {
		if s.Name == key || s.Path == key {
			return i, true
		}
	}
	return -1, false
}

// kpzDocuments is the workspace home over the source documents of the KPZ at
// kpzPath, which references name ref!<document>.
func (a *App) kpzDocuments(ctx context.Context, ref, kpzPath string) (*workhome.Documents, error) {
	c, err := a.ensureKpzCache(ctx, kpzPath)
	if err != nil {
		return nil, err
	}
	l, err := openKpzLog(ctx, c.dir)
	if err != nil {
		return nil, err
	}
	src := model.LocaleID(a.SourceLocale())
	var target model.LocaleID
	if c.meta.Recipe != nil {
		if sl := recipeSourceLang(c.meta.Recipe); sl != "" {
			src = model.LocaleID(sl)
		}
		if ts := recipeTargetLangs(c.meta.Recipe); len(ts) > 0 {
			target = model.LocaleID(ts[0])
		}
	}
	return &workhome.Documents{
		Store: l.st.Heads, Log: l.p, Formats: a.FormatReg, SourceLocale: src, TargetLocale: target,
		Prefix: ref + "!", WorkDir: filepath.Join(c.dir, "work"),
		Seed: func(ctx context.Context, key string) (workhome.Seeded, bool, error) { return a.kpzSeed(ctx, c, key) },
	}, nil
}

// kpzSeed is a source document of the cache as it opens for editing: the
// cache's copy of its bytes.
func (a *App) kpzSeed(ctx context.Context, c *kpzCache, key string) (workhome.Seeded, bool, error) {
	i, ok := c.kpzSource(key)
	if !ok {
		return workhome.Seeded{}, false, nil
	}
	src := c.meta.Sources[i]
	formatID := src.FormatID
	if formatID == "" {
		if det := a.kpzDetectFormat(src.Path); det != nil {
			formatID = string(det(src.Path))
		}
	}
	if formatID == "" {
		return workhome.Seeded{}, false, &change.Error{Code: change.CodeUnsupported, Capability: "format", Message: "no format reads " + src.Name}
	}
	if c.hasSource(src) {
		data, err := os.ReadFile(c.sourcePath(src.Name))
		if err != nil {
			return workhome.Seeded{}, false, err
		}
		return workhome.Seeded{Data: data, Format: formatID}, true, nil
	}
	// A skeleton names each block and keeps none of its source text, so a
	// document the KPZ carries as a skeleton alone has nothing to edit.
	return workhome.Seeded{}, false, &change.Error{Code: change.CodeUnsupported, Capability: "source",
		Message: fmt.Sprintf("%s carries %s as a skeleton without its text; extract it again with --with-source to edit it", filepath.Base(c.meta.KpzPath), src.Name)}
}

// syncKpzEdits brings the cache's copy of every source document edited
// through the workspace home to the document's head, with its identity and
// its round-trip skeleton taken from those bytes, so a transform, a merge and
// a pack read the edit. The pack carries an edited document's bytes. A cache whose documents were never opened for
// editing has no log, and nothing changes.
func (a *App) syncKpzEdits(ctx context.Context, c *kpzCache) error {
	if _, err := os.Stat(kpzLogDir(c.dir)); err != nil {
		return nil
	}
	l, err := openKpzLog(ctx, c.dir)
	if err != nil {
		return err
	}
	if err := l.p.CatchUp(ctx); err != nil {
		return err
	}
	heads, err := l.st.Heads.Documents(ctx)
	if err != nil {
		return err
	}
	changed := false
	for _, h := range heads {
		i, ok := c.kpzSource(h.Key)
		if !ok {
			continue
		}
		src := &c.meta.Sources[i]
		data, err := l.ws.Blob(ctx, h.Blob)
		if err != nil {
			return fmt.Errorf("kpz: read %s: %w", h.Key, err)
		}
		if held, rerr := os.ReadFile(c.sourcePath(src.Name)); rerr == nil && src.RawPresent && bytes.Equal(held, data) {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(c.sourcePath(src.Name)), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(c.sourcePath(src.Name), data, 0o644); err != nil {
			return fmt.Errorf("kpz: write %s: %w", src.Name, err)
		}
		src.RawPresent, src.HasRawSource = true, true
		src.ContentHash = project.HashBytes(data)
		if src.FormatID == "" {
			src.FormatID = h.Format
		}
		skel, err := captureSkeletonBytes(ctx, a.FormatReg, registry.FormatID(h.Format), src.Name, data, model.LocaleID(a.SourceLocale()))
		if err != nil {
			return fmt.Errorf("capture round-trip skeleton for %s: %w", src.Name, err)
		}
		if len(skel) > 0 {
			name := src.Name + ".skel"
			if err := os.WriteFile(c.skeletonPath(name), skel, 0o644); err != nil {
				return err
			}
			src.Skeleton = name
		}
		changed = true
	}
	if !changed {
		return nil
	}
	return c.save()
}

// kpzHomes picks the home of each document a change set names: the
// workspace home of a KPZ for a reference of the form work.kpz!<document>,
// and the file home for every other.
type kpzHomes struct {
	app  *App
	ctx  context.Context
	file change.Home
	// root is the directory a relative KPZ reference resolves under.
	root string
	mu   sync.Mutex
	kpz  map[string]*workhome.Documents
}

// For returns the home of doc.
func (h *kpzHomes) For(doc string) (change.Home, error) {
	ref, path, ok := h.kpzRef(doc)
	if !ok {
		return h.file, nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if d, ok := h.kpz[path]; ok {
		return d, nil
	}
	d, err := h.app.kpzDocuments(h.ctx, ref, path)
	if err != nil {
		return nil, err
	}
	if h.kpz == nil {
		h.kpz = map[string]*workhome.Documents{}
	}
	h.kpz[path] = d
	return d, nil
}

// kpzRef reports whether doc names a document of a KPZ that exists, and
// returns the KPZ as the reference names it and as a file.
func (h *kpzHomes) kpzRef(doc string) (ref, path string, ok bool) {
	i := strings.Index(strings.ToLower(doc), workspaceExt+"!")
	if i < 0 {
		return "", "", false
	}
	ref = doc[:i+len(workspaceExt)]
	if !IsKpzPath(ref) {
		return "", "", false
	}
	path = ref
	if !filepath.IsAbs(path) {
		path = filepath.Join(h.root, filepath.FromSlash(ref))
	}
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return "", "", false
	}
	return ref, path, true
}
