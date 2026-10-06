package host

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/workhome"
	"github.com/neokapi/neokapi/kpz"
)

// A document a KPZ carries diverges as a whole when it is written from a
// version that is no longer its head. Two writers that each edited it from
// one head and whose logs met leave one write beside the other. So does the
// .kpz on disk when it is replaced, by a teammate's pack or a fresh extract,
// while the cache holds edits nobody has packed: each document the new file
// carries with other bytes than the cache opened it with is recorded as a
// write made from the version the cache opened (recordKpzRewrites). A read
// of the document reports such a write (change.Page.Divergent), Kapi Desktop
// lists it among the project's conflicts, and a person rebases it onto the
// head or discards it (RebaseKeptDocument, DiscardKeptDocument).

// kpzRewriter is the actor that records a document the .kpz on disk carries
// with other bytes than the cache opened it with.
var kpzRewriter = change.Actor{Kind: change.ActorTool, Name: "kpz"}

// recordKpzRewrites records, for each document of the cache's log, the bytes
// the .kpz at kpzPath carries for it when they differ from what the cache
// opened the document with, as a write made from that version. A document
// edited since it opened then lists the write as divergent; one nobody
// edited takes the new bytes as its head. A version the log holds already is
// not recorded again. It returns how many documents it recorded.
func (a *App) recordKpzRewrites(ctx context.Context, c *kpzCache, kpzPath string) (int, error) {
	if _, err := os.Stat(kpzLogDir(c.dir)); err != nil {
		return 0, nil
	}
	pkg, err := LoadWorkspace(kpzPath)
	if err != nil {
		return 0, err
	}
	raw := make(map[string]kpz.Content, len(pkg.Source))
	for _, s := range pkg.Source {
		raw[s.Path] = s.Content
	}
	l, err := openKpzLog(ctx, c.dir)
	if err != nil {
		return 0, err
	}
	if err := l.p.CatchUp(ctx); err != nil {
		return 0, err
	}
	heads, err := l.st.Heads.Documents(ctx)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, h := range heads {
		i, ok := c.kpzSource(h.Key)
		if !ok {
			continue
		}
		content, ok := raw[c.meta.Sources[i].Path]
		if !ok {
			continue
		}
		data, err := kpz.ReadAll(content)
		if err != nil {
			return n, fmt.Errorf("kpz: read %s: %w", h.Key, err)
		}
		writes, err := l.p.DocumentWrites(ctx, h.Key)
		if err != nil {
			return n, err
		}
		if len(writes) == 0 {
			continue
		}
		rev := workhome.DocumentRevision(data)
		if slices.ContainsFunc(writes, func(w workhome.DocWrite) bool { return w.After == rev }) {
			continue
		}
		opened := slices.MinFunc(writes, func(x, y workhome.DocWrite) int { return cmp.Compare(x.Op, y.Op) })
		seq, err := l.p.DocumentSubjectHead(ctx, h.Key)
		if err != nil {
			return n, err
		}
		if _, err := l.p.CommitDocument(ctx, workhome.DocCommit{Key: h.Key, Path: h.Path, Expect: seq, Base: opened.Op,
			Format: h.Format, Data: data, Before: opened.After, After: rev, Actor: kpzRewriter, Origin: "kpz",
			Note: filepath.Base(kpzPath) + " carries another version of " + h.Key}); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// ConflictDocument is a write to a whole document a KPZ in the project
// carries that did not land on the document's head.
const ConflictDocument = "document"

// kpzDivergence is one write that did not land on a document of a KPZ.
type kpzDivergence struct {
	// Doc is the reference that names the document, KpzPath the KPZ file.
	Doc     string
	KpzPath string
	docs    *workhome.Documents
	key     string
	div     workhome.DocDivergence
}

// kpzDivergences lists the writes that did not land on the documents of the
// KPZ files under root whose cache holds a log, each named by a reference
// relative to root; with root empty, of every KPZ on this machine whose cache
// holds one, each named by its absolute path.
func (a *App) kpzDivergences(ctx context.Context, root string) ([]kpzDivergence, error) {
	var out []kpzDivergence
	for _, kpzPath := range kpzCachesUnder(root) {
		ref := kpzPath
		if root != "" {
			rel, err := filepath.Rel(root, kpzPath)
			if err != nil {
				continue
			}
			ref = filepath.ToSlash(rel)
		}
		d, err := a.kpzDocuments(ctx, ref, kpzPath)
		if err != nil {
			return nil, err
		}
		heads, err := d.Divergences(ctx)
		if err != nil {
			return nil, err
		}
		for _, h := range heads {
			for _, dv := range h.Divergent {
				out = append(out, kpzDivergence{Doc: d.Prefix + h.Key, KpzPath: kpzPath, docs: d, key: h.Key, div: dv})
			}
		}
	}
	return out, nil
}

// kpzDocumentConflicts lists the writes that did not land on a document of a
// KPZ under the project at recipe whose cache holds a log: one conflict per
// write, with the blocks a rebase left contested.
func (a *App) kpzDocumentConflicts(ctx context.Context, recipe string) ([]KeptConflict, error) {
	divs, err := a.kpzDivergences(ctx, filepath.Dir(recipe))
	if err != nil {
		return nil, err
	}
	return a.documentConflicts(ctx, recipe, divs)
}

// WorkspaceDocumentConflicts lists the writes that did not land on a document
// of any KPZ on this machine whose working cache holds edits, inside a
// project or not. The caches are this user's, keyed by each KPZ's absolute
// path, so a surface can show them with no project open. Each document is
// named by the KPZ's absolute path.
func (a *App) WorkspaceDocumentConflicts(ctx context.Context) ([]KeptConflict, error) {
	divs, err := a.kpzDivergences(ctx, "")
	if err != nil {
		return nil, err
	}
	return a.documentConflicts(ctx, "", divs)
}

// documentConflicts is each divergent write as a conflict, with the blocks a
// rebase left contested read through the change service of recipe, or of the
// project each KPZ sits in when recipe is empty.
func (a *App) documentConflicts(ctx context.Context, recipe string, divs []kpzDivergence) ([]KeptConflict, error) {
	out := []KeptConflict{}
	for _, dv := range divs {
		kc := KeptConflict{Kind: ConflictDocument, Doc: dv.Doc, Edit: dv.div.Op, Rebased: dv.div.Rebased(), Blocks: []KeptConflictBlock{}}
		if dv.div.Rebased() {
			var err error
			if kc.Blocks, err = a.documentConflictBlocks(ctx, kpzServiceOptions(recipe, dv.KpzPath, "status"), dv.docs, dv.key, dv.div); err != nil {
				return nil, err
			}
		}
		out = append(out, kc)
	}
	return out, nil
}

// kpzServiceOptions is the change service a KPZ's document is edited
// through: the project at recipe; with recipe empty, the project the KPZ sits
// in, or the KPZ's directory when it sits in none.
func kpzServiceOptions(recipe, kpzPath, origin string) ChangeServiceOptions {
	if recipe == "" {
		if l, err := project.ResolveLayout(filepath.Dir(kpzPath)); err == nil && l.RecipePath != "" {
			recipe = l.RecipePath
		}
	}
	if recipe != "" {
		return ChangeServiceOptions{Project: recipe, Origin: origin}
	}
	return ChangeServiceOptions{Root: filepath.Dir(kpzPath), Origin: origin}
}

// documentConflictBlocks reads each block a rebase left contested as the
// document's head holds it, through the change service, beside the wording
// the divergent write left.
func (a *App) documentConflictBlocks(ctx context.Context, opts ChangeServiceOptions, d *workhome.Documents, key string, dv workhome.DocDivergence) ([]KeptConflictBlock, error) {
	other, err := d.DivergentWording(ctx, key, dv.Op, dv.Contested)
	if err != nil {
		return nil, err
	}
	svc, err := a.ChangeService(ctx, opts)
	if err != nil {
		return nil, err
	}
	var blocks []string
	var editions []model.EditionKey
	for _, b := range dv.Contested {
		if !slices.Contains(blocks, b.Block) {
			blocks = append(blocks, b.Block)
		}
		if b.Edition == "" {
			continue
		}
		if k, err := model.ParseEditionKey(b.Edition); err == nil && !slices.Contains(editions, k) {
			editions = append(editions, k)
		}
	}
	page, err := svc.Read(ctx, change.ReadRequest{Doc: d.Prefix + key, Blocks: blocks, Editions: editions, Limit: change.MaxReadLimit})
	if err != nil {
		return nil, err
	}
	read := map[string]change.BlockRead{}
	for _, b := range page.Blocks {
		read[b.Ref.Block] = b
	}
	out := make([]KeptConflictBlock, 0, len(dv.Contested))
	for _, db := range dv.Contested {
		kb := KeptConflictBlock{Block: db.Block, Edition: db.Edition}
		if w := other[db]; w.Absent {
			kb.Other = KeptWording{Rev: model.AbsentRevision, Absent: true}
		} else {
			kb.Other = KeptWording{Text: model.RunsEditText(w.Runs)}
		}
		b, ok := read[db.Block]
		switch {
		case !ok:
			kb.Held = KeptWording{Rev: model.AbsentRevision, Absent: true}
		case db.Edition == "":
			kb.Held = KeptWording{Text: b.Text, Rev: b.Rev}
		default:
			kb.Source = b.Text
			if ed, ok := b.Editions[db.Edition]; ok {
				kb.Held = KeptWording{Text: ed.Text, Rev: ed.Rev}
			} else {
				kb.Held = KeptWording{Rev: model.AbsentRevision, Absent: true}
			}
		}
		out = append(out, kb)
	}
	return out, nil
}

// kpzCachesUnder lists the KPZ files under root whose working cache holds a
// log of documents opened for editing; every such KPZ when root is empty.
func kpzCachesUnder(root string) []string {
	abs := ""
	if root != "" {
		var err error
		if abs, err = filepath.Abs(root); err != nil {
			return nil
		}
	}
	entries, err := os.ReadDir(kpzCacheRoot())
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(kpzCacheRoot(), e.Name())
		if _, err := os.Stat(kpzLogDir(dir)); err != nil {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, "meta.json"))
		if err != nil {
			continue
		}
		var meta struct {
			KpzPath string `json:"kpzPath"`
		}
		if json.Unmarshal(data, &meta) != nil || meta.KpzPath == "" {
			continue
		}
		if abs != "" {
			rel, err := filepath.Rel(abs, meta.KpzPath)
			if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				continue
			}
		}
		if info, err := os.Stat(meta.KpzPath); err != nil || info.IsDir() {
			continue
		}
		out = append(out, meta.KpzPath)
	}
	slices.Sort(out)
	return out
}

// DocumentRebase is what a rebase of a divergent write to a KPZ's document
// did, as a surface reports it.
type DocumentRebase struct {
	// Carried counts the changes applied to the document's head, and
	// Contested the blocks the head changed too, left for a person.
	Carried   int `json:"carried"`
	Contested int `json:"contested"`
	// Refused says why the change service refused to apply the changes;
	// nothing was settled then.
	Refused string `json:"refused,omitempty"`
}

// ErrNotKpzDocument is what RebaseKeptDocument and DiscardKeptDocument return
// for a reference that names no document of a KPZ: in the project, or, with no
// project, by the KPZ's absolute path.
var ErrNotKpzDocument = errors.New("the reference names no document of a .kpz in the project")

// projectKpzDocuments is the workspace home of the KPZ a reference into the
// project at recipe names, the key of the document it names, and the KPZ
// file. With recipe empty the reference names the KPZ by its absolute path.
func (a *App) projectKpzDocuments(ctx context.Context, recipe, doc string) (*workhome.Documents, string, string, error) {
	root := ""
	if recipe != "" {
		root = filepath.Dir(recipe)
	} else if i := strings.Index(strings.ToLower(doc), workspaceExt+"!"); i < 0 || !filepath.IsAbs(doc[:i]) {
		return nil, "", "", ErrNotKpzDocument
	}
	h := &kpzHomes{app: a, ctx: ctx, root: root}
	ref, path, ok := h.kpzRef(doc)
	if !ok {
		return nil, "", "", ErrNotKpzDocument
	}
	d, err := a.kpzDocuments(ctx, ref, path)
	if err != nil {
		return nil, "", "", err
	}
	key, ok := d.Key(doc)
	if !ok {
		return nil, "", "", ErrNotKpzDocument
	}
	return d, key, path, nil
}

// RebaseKeptDocument rebases the divergent write op to doc, a document of a
// KPZ in the project at recipe, onto the document's head, as a person
// through origin (workhome.Documents.Rebase). The changes go through the
// project's change service, with its gates. With recipe empty, doc names the
// KPZ by its absolute path, and the changes go through the change service of
// the project the KPZ sits in, if any.
func (a *App) RebaseKeptDocument(ctx context.Context, recipe, doc, op, origin string) (DocumentRebase, error) {
	d, key, kpzPath, err := a.projectKpzDocuments(ctx, recipe, doc)
	if err != nil {
		return DocumentRebase{}, err
	}
	svc, err := a.ChangeService(ctx, kpzServiceOptions(recipe, kpzPath, origin))
	if err != nil {
		return DocumentRebase{}, err
	}
	rb, err := d.Rebase(ctx, svc, key, op, change.Actor{Kind: change.ActorPerson}, origin)
	if err != nil {
		return DocumentRebase{}, err
	}
	out := DocumentRebase{Carried: rb.Carried, Contested: len(rb.Contested)}
	if rb.Result != nil && rb.Result.Status != change.SetApplied {
		out.Refused = refusalLine(rb.Result)
	}
	return out, nil
}

// DiscardKeptDocument discards the divergent write op to doc, a document of a
// KPZ in the project at recipe (by its absolute path with recipe empty),
// keeping the document's head, as a person through origin.
func (a *App) DiscardKeptDocument(ctx context.Context, recipe, doc, op, origin string) error {
	d, key, _, err := a.projectKpzDocuments(ctx, recipe, doc)
	if err != nil {
		return err
	}
	return d.Discard(ctx, key, op, change.Actor{Kind: change.ActorPerson}, origin)
}

// refusalLine is why the change service refused a change set, in one line.
func refusalLine(res *change.Result) string {
	for _, o := range res.Ops {
		if o.Error != nil {
			return o.Error.Message
		}
	}
	return "the change set was refused"
}

// KpzDocumentService is the change service that edits doc, a document of a
// KPZ named by its absolute path, outside any open project: the service of
// the project the KPZ sits in, or of the KPZ's directory.
func (a *App) KpzDocumentService(ctx context.Context, doc, origin string) (*change.Service, error) {
	_, _, kpzPath, err := a.projectKpzDocuments(ctx, "", doc)
	if err != nil {
		return nil, err
	}
	return a.ChangeService(ctx, kpzServiceOptions("", kpzPath, origin))
}
