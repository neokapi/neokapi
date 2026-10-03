package host

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/change/filehome"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/workhome"
)

// The workspace home of a project (core/workhome) keeps the editions that
// have no file yet: a parked locale's drafts under `materialize: on-converge`,
// the drafts a gated run produced that kapi merge has not delivered, and every
// edit a person or an agent makes to them. They live in the project's
// operation log, outside the checkout, so deleting `.kapi/work/` loses none of
// them and costs no provider call to rebuild.
//
// An edition's home follows where it is. A gated run that parks a locale
// writes its drafts into the workspace home (keepParkedDrafts); while the file
// the recipe's target template names does not exist, an edition the
// workspace keeps lives there, and every read and change set reaches it there.
// Once the file exists, the file is its home and the workspace's copy is never
// read or written. A delivery (kapi merge, or kapi up for a locale that
// cleared its gate) writes the file and then releases the edition from the
// workspace home, so the workspace never keeps a second copy of an edition a
// file holds. An edition with neither a file nor a kept draft is written to
// its file, as a delivery writes it.

// keptEditions is the workspace home of the project rooted at root, as a file
// home reaches it for the editions that have no file (filehome.Keeper). It
// opens the project store only when it has to: a project with no store keeps
// nothing, and the first write creates the store.
type keptEditions struct {
	app  *App
	root string

	mu   sync.Mutex
	home *workhome.Home
}

var _ filehome.Keeper = (*keptEditions)(nil)

// keptEditions returns the workspace home of the project rooted at root.
func (a *App) keptEditions(root string) *keptEditions {
	return &keptEditions{app: a, root: root}
}

// Name is the home as a change result reports it.
func (k *keptEditions) Name() string { return workhome.Name }

// open returns the workspace home over the project's store and log. Without
// create, a project with no store answers nil. A store whose workspace keeps
// no log (a workspace opened read-only) has nowhere to keep an edition, and
// answers nil too.
func (k *keptEditions) open(ctx context.Context, create bool) (*workhome.Home, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.home != nil {
		return k.home, nil
	}
	db := k.app.existingProjectDB(ctx, k.root)
	if db == nil && create {
		var err error
		if db, err = k.app.ProjectDB(ctx, k.root); err != nil {
			return nil, err
		}
	}
	if db == nil || db.Heads() == nil {
		return nil, nil
	}
	p, err := k.app.Projector(ctx, k.root)
	if err != nil {
		return nil, err
	}
	if !p.Logged() {
		return nil, nil
	}
	keys := k.app.keptDocKeysFor(k.root)
	keyCtx := context.WithoutCancel(ctx)
	k.home = &workhome.Home{Store: db.Heads(), Log: p, DocKey: func(ref string) string { return keys.key(keyCtx, ref) }}
	return k.home, nil
}

// keptDocKeys names the documents of one project by the keys the workspace
// home files their editions under (DocumentIndex.Key). The index is read once
// per App and read again only for a path it does not name, which a document
// adopted since then is: every read surface asks it once per unit, and the
// index lists every document of the project.
type keptDocKeys struct {
	app  *App
	root string

	mu     sync.Mutex
	docs   *DocumentIndex
	missed map[string]bool
}

// keptDocKeysFor returns the document keys of the project rooted at root.
func (a *App) keptDocKeysFor(root string) *keptDocKeys {
	s := a.ensureProjectStores()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.keptKeys == nil {
		s.keptKeys = map[string]*keptDocKeys{}
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		abs = root
	}
	k, ok := s.keptKeys[abs]
	if !ok {
		k = &keptDocKeys{app: a, root: root, missed: map[string]bool{}}
		s.keptKeys[abs] = k
	}
	return k
}

// key is the key of the document at the project-relative path ref.
func (k *keptDocKeys) key(ctx context.Context, ref string) string {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.docs == nil || (!k.docs.Holds(ref) && !k.missed[ref]) {
		docs := k.app.documentIndexOrEmpty(ctx, k.root)
		k.docs = &docs
		if !k.docs.Holds(ref) {
			k.missed[ref] = true
		}
	}
	return k.docs.Key(ref)
}

// Edition reads what the workspace home keeps of edition ed of the document
// ref names.
func (k *keptEditions) Edition(ctx context.Context, ref string, ed model.EditionKey) (filehome.Kept, error) {
	h, err := k.open(ctx, false)
	if err != nil {
		return filehome.Kept{}, err
	}
	if h == nil {
		return workhome.EmptyKept(), nil
	}
	return h.Edition(ctx, ref, ed)
}

// Commit stores a change a file home staged on a kept edition.
func (k *keptEditions) Commit(ctx context.Context, w filehome.KeptWrite) (string, error) {
	h, err := k.open(ctx, true)
	if err != nil {
		return "", err
	}
	if h == nil {
		return "", &change.Error{Code: change.CodeUnsupported, Capability: "edition",
			Message: fmt.Sprintf("edition %s of %s has no file yet, and the workspace is open for reading only, so it has nowhere to keep it", editionText(w.Edition), w.Doc)}
	}
	return h.Commit(ctx, w)
}

// holds reports whether the workspace home keeps any block of edition ed of
// the document ref names.
func (k *keptEditions) holds(ctx context.Context, ref string, ed model.EditionKey) bool {
	h, err := k.open(ctx, false)
	if err != nil || h == nil {
		return false
	}
	held, err := h.Holds(ctx, ref, ed)
	return err == nil && held
}

// keptPolicy says when an edition of a project's document lives in the
// workspace home.
type keptPolicy struct {
	keeper *keptEditions
}

// keeps returns the keeper of edition ed of the document ref names, whose
// file is path, or nil when the edition's home is its file: the file exists,
// or the workspace keeps nothing of the edition.
func (p *keptPolicy) keeps(ctx context.Context, ref string, ed model.EditionKey, path string) filehome.Keeper {
	if p == nil || p.keeper == nil {
		return nil
	}
	if _, err := os.Stat(path); err == nil || !errors.Is(err, os.ErrNotExist) {
		// The file is the edition's home; whatever the workspace still keeps
		// of the edition is never read beside it.
		return nil
	}
	if p.keeper.holds(ctx, ref, ed) {
		return p.keeper
	}
	return nil
}

// draftStamp is what a producer recognizes a draft it made by, kept beside
// the draft in the workspace home: the block-store key of its overlay, and
// the overlay's own reuse fields (blockstore.TargetOverlay). A run whose
// `.kapi/work/` was deleted writes the overlay again from it, so the producer
// serves the draft rather than paying a provider for it again.
type draftStamp struct {
	Key      string `json:"key"`
	Provider string `json:"provider,omitempty"`
	Config   string `json:"config,omitempty"`
	Source   string `json:"source,omitempty"`
}

// keptTargetBlocks reads a unit's source blocks with the edition the
// workspace home keeps for its locale joined in, which is where a parked
// locale's drafts are: the read surfaces (coverage, status, the review queue,
// checks) measure and review the drafts where they live. found is false when
// the workspace home keeps nothing of the edition.
func (a *App) keptTargetBlocks(ctx context.Context, u VerifyUnit) ([]*model.Block, bool, error) {
	if u.ProjectRoot == "" || u.Locale == "" {
		return nil, false, nil
	}
	h, err := a.keptEditions(u.ProjectRoot).open(ctx, false)
	if err != nil || h == nil {
		return nil, false, err
	}
	ref, err := filepath.Rel(u.ProjectRoot, u.SourcePath)
	if err != nil {
		return nil, false, nil
	}
	loc := model.LocaleID(u.Locale)
	_, kept, err := h.Rows(ctx, filepath.ToSlash(ref), model.EditionKey{Locale: loc})
	if err != nil {
		return nil, false, fmt.Errorf("read the %s drafts of %s: %w", u.Locale, u.DisplayPath, err)
	}
	if len(kept) == 0 {
		return nil, false, nil
	}
	blocks, err := a.readSource(ctx, u)
	if err != nil {
		return nil, false, fmt.Errorf("read source %s: %w", u.SourcePath, err)
	}
	found := false
	for _, b := range blocks {
		b.SourceLocale = model.LocaleID(a.SourceLocale())
		ed, ok := kept[change.BlockKey(b)]
		if !ok || !model.RunsHaveContent(ed.Runs) {
			continue
		}
		b.SetEdition(model.EditionKey{Locale: loc}, ed)
		normalizeKeptStatus(b, loc)
		found = true
	}
	if !found {
		return nil, false, nil
	}
	return blocks, true, nil
}

// normalizeKeptStatus grades a kept draft at the rung its delivered copy
// would read. A JSON or YAML target file carries no lifecycle metadata, so a
// delivered translation reads as `translated` (convergence.TargetState); a
// producer stamps `draft` on what it writes, and honoring that stamp would
// grade one record of the work a rung below the other, so the locale would
// read `blocked: translate` while holding a translation of every unit. A rung
// above translated is a decision, and it is kept.
func normalizeKeptStatus(b *model.Block, loc model.LocaleID) {
	t := b.Target(loc)
	if t == nil {
		return
	}
	if t.Status.Rank() < model.TargetStatusTranslated.Rank() {
		t.Status = ""
	}
}

// keepParkedDrafts writes the drafts a gated convergence run produced for a
// locale it did not deliver into the workspace home, one write per
// document, and returns how many blocks it wrote. The drafts are the run's
// own (what each document's latest pass left, against the edition as the run
// first read it), with the basis each was made from and the stamp its
// producer serves it again by. A block a person or an agent changed while
// the run worked keeps their edition.
func (a *App) keepParkedDrafts(ctx context.Context, root string, locale model.LocaleID) (int, error) {
	if a.convergeDeliveries == nil || root == "" {
		return 0, nil
	}
	h, err := a.keptEditions(root).open(ctx, true)
	if err != nil || h == nil {
		return 0, err
	}
	var store blockstore.Store
	if db := a.existingProjectDB(ctx, root); db != nil {
		store = a.projectBlocksAutocommit(db)
	}
	written := 0
	for _, doc := range a.convergeDeliveries.drafts(locale) {
		p := doc.parked(ctx, store)
		if len(p.Blocks) == 0 {
			continue
		}
		res, err := h.Produce(ctx, p)
		if err != nil {
			return written, fmt.Errorf("keep the %s drafts of %s: %w", locale, doc.ref, err)
		}
		written += res.Written
	}
	return written, nil
}

// parked is the run's drafts of the document's edition, as the workspace
// home takes them.
func (doc *flowDoc) parked(ctx context.Context, store blockstore.Store) workhome.Produce {
	p := workhome.Produce{Doc: doc.ref, Edition: doc.edition, Actor: flowActor(doc.d.Flow), Origin: flowOrigin(doc.d.Flow)}
	text := editionText(doc.edition)
	var sess blockstore.Session
	if store != nil {
		if s, err := store.Begin(ctx); err == nil {
			sess = s
			defer s.Close()
		}
	}
	doc.mu.Lock()
	defer doc.mu.Unlock()
	for _, key := range doc.order {
		lb := doc.left[key]
		ed, ok := lb.editions[text]
		if !ok || ed.rev == model.AbsentRevision {
			continue
		}
		before, _ := doc.before[key].revision(text)
		pr := workhome.Produced{
			Block:       key,
			Before:      before,
			Edition:     model.Edition{Runs: ed.runs, Status: ed.status, Origin: ed.origin},
			Basis:       doc.basis(key, lb),
			ContentHash: doc.sourceOf(key).contentHash,
		}
		pr.Stamp = draftStampOf(sess, doc, lb, ed)
		p.Blocks = append(p.Blocks, pr)
	}
	return p
}

// draftStampOf reads the overlay the run's producer left for a block's draft
// and returns the stamp the workspace home keeps beside the draft, or nil
// when the store holds no overlay for this very draft.
func draftStampOf(sess blockstore.Session, doc *flowDoc, lb *leftBlock, ed leftEdition) json.RawMessage {
	if sess == nil || lb.id == "" {
		return nil
	}
	key := blockstore.StoreKey(filepath.FromSlash(doc.ref), lb.id, lb.source)
	o, err := sess.GetOverlay(blockstore.TargetOverlayKind(doc.edition.Locale), key)
	if err != nil || len(o.Payload) == 0 {
		return nil
	}
	var stored blockstore.TargetOverlay
	if json.Unmarshal(o.Payload, &stored) != nil || stored.TargetText() != model.RunsText(ed.runs) {
		return nil
	}
	data, err := json.Marshal(draftStamp{Key: key, Provider: stored.Provider, Config: stored.Config, Source: stored.Source})
	if err != nil {
		return nil
	}
	return data
}

// restoreKeptOverlays writes into the project's block store the overlay of
// every draft the workspace home keeps with a stamp, where the store holds
// none: the block store is a cache under `.kapi/work/`, and a draft whose
// overlay is gone would otherwise cost a provider call in the next pass. It
// returns how many overlays it wrote.
func (a *App) restoreKeptOverlays(ctx context.Context, root string) (int, error) {
	h, err := a.keptEditions(root).open(ctx, true)
	if err != nil || h == nil {
		return 0, err
	}
	db, err := a.ProjectDB(ctx, root)
	if err != nil {
		return 0, err
	}
	if err := h.Log.CatchUp(ctx); err != nil {
		return 0, err
	}
	heads, err := h.Store.Heads(ctx)
	if err != nil {
		return 0, err
	}
	sess, err := a.projectBlocksAutocommit(db).Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer sess.Close()
	written := 0
	for _, head := range heads {
		key, err := model.ParseEditionKey(head.Edition)
		if err != nil || key.Locale == "" {
			continue
		}
		kind := blockstore.TargetOverlayKind(key.Locale)
		rows, err := h.Store.Rows(ctx, head.Doc, head.Edition)
		if err != nil {
			return written, err
		}
		for _, r := range rows {
			var st draftStamp
			if len(r.Stamp) == 0 || json.Unmarshal(r.Stamp, &st) != nil || st.Key == "" {
				continue
			}
			if _, err := sess.GetOverlay(kind, st.Key); err == nil {
				continue
			} else if !errors.Is(err, blockstore.ErrNotFound) {
				return written, err
			}
			ed, err := h.EditionOf(ctx, r)
			if err != nil {
				return written, err
			}
			runs := ed.Runs
			payload, err := json.Marshal(blockstore.TargetOverlay{
				Runs: runs, Text: model.RunsText(runs), Status: string(r.Status),
				Provider: st.Provider, Config: st.Config, Source: st.Source,
				Origin: blockstore.OverlayOrigin(r.Origin),
			})
			if err != nil {
				return written, err
			}
			if err := sess.PutOverlay(blockstore.Overlay{Kind: kind, BlockHash: st.Key, Payload: payload}); err != nil {
				if errors.Is(err, blockstore.ErrReadOnly) {
					return written, nil
				}
				return written, err
			}
			written++
		}
	}
	return written, sess.Commit()
}

// releaseKept removes edition ed of the document ref names from the
// workspace home once its file holds it, recorded as actor's write through
// origin. It is what every delivery does after it writes an edition's file.
func (a *App) releaseKept(ctx context.Context, root, ref string, ed model.EditionKey, actor change.Actor, origin string) error {
	h, err := a.keptEditions(root).open(ctx, false)
	if err != nil || h == nil {
		return err
	}
	_, err = h.Release(ctx, ref, ed, actor, origin)
	return err
}

// releaseDelivered releases from the workspace home every edition whose file
// the recipe's target template names now exists: a pass that writes where the
// recipe points delivered it, and the file is its home from then on. It is
// what keeps the workspace from holding a second copy of an edition a file
// holds whichever path wrote the file.
func (a *App) releaseDelivered(ctx context.Context, recipe string) error {
	root := filepath.Dir(recipe)
	h, err := a.keptEditions(root).open(ctx, false)
	if err != nil || h == nil {
		return err
	}
	if err := h.Log.CatchUp(ctx); err != nil {
		return err
	}
	heads, err := h.Store.Heads(ctx)
	if err != nil || len(heads) == 0 {
		return err
	}
	// The layout of a service that writes every edition to its file names
	// each edition's file and keeps none.
	ch, err := a.changeHome(ChangeServiceOptions{Project: recipe, Materialize: true})
	if err != nil {
		return err
	}
	for _, head := range heads {
		held, err := h.Store.Held(ctx, head.Doc, head.Edition)
		if err != nil {
			return err
		}
		if !held || head.Path == "" {
			continue
		}
		key, err := model.ParseEditionKey(head.Edition)
		if err != nil {
			continue
		}
		d, err := ch.layout.Locate(ctx, head.Path)
		if err != nil || d.EditionFile == nil {
			continue
		}
		f, ok := d.EditionFile(key)
		if !ok {
			continue
		}
		if _, err := os.Stat(f.Path); err != nil {
			continue
		}
		if _, err := h.Release(ctx, head.Path, key, change.Actor{Kind: change.ActorTool, Name: "up"}, "flow:up"); err != nil {
			return err
		}
	}
	return nil
}

// keptRuns reads what the workspace home keeps of edition ed of the document
// ref names, by block key; nil when it keeps nothing.
func (a *App) keptRuns(ctx context.Context, root, ref string, ed model.EditionKey) (map[string]model.Edition, error) {
	h, err := a.keptEditions(root).open(ctx, false)
	if err != nil || h == nil {
		return nil, err
	}
	_, kept, err := h.Rows(ctx, ref, ed)
	if err != nil || len(kept) == 0 {
		return nil, err
	}
	return kept, nil
}

// keptLocales reports which of locales the workspace home keeps an edition
// of, for any document of the project.
func (a *App) keptLocales(ctx context.Context, root string) (map[model.LocaleID]bool, error) {
	h, err := a.keptEditions(root).open(ctx, false)
	if err != nil || h == nil {
		return nil, err
	}
	if err := h.Log.CatchUp(ctx); err != nil {
		return nil, err
	}
	heads, err := h.Store.Heads(ctx)
	if err != nil {
		return nil, err
	}
	out := map[model.LocaleID]bool{}
	for _, head := range heads {
		held, err := h.Store.Held(ctx, head.Doc, head.Edition)
		if err != nil {
			return nil, err
		}
		if !held {
			continue
		}
		if key, err := model.ParseEditionKey(head.Edition); err == nil && key.Locale != "" {
			out[model.NormalizeLocale(key.Locale)] = true
		}
	}
	return out, nil
}

// keptConflicts lists the writes to editions the workspace home keeps that a
// merge of two machines' logs left divergent and the rebase cannot carry
// over, for kapi status.
func (a *App) keptConflicts(ctx context.Context, root string) ([]workhome.Conflict, error) {
	h, err := a.keptEditions(root).open(ctx, false)
	if err != nil || h == nil {
		return nil, err
	}
	return h.Conflicts(ctx)
}

// statusConflicts is the conflicts kapi status lists, or none when they
// cannot be read: a status report never fails on the workspace home.
func (a *App) statusConflicts(ctx context.Context, root string) []StatusConflict {
	conflicts, err := a.keptConflicts(ctx, root)
	if err != nil {
		return nil
	}
	var out []StatusConflict
	for _, c := range conflicts {
		locale := c.Edition
		if key, err := model.ParseEditionKey(c.Edition); err == nil && key.Locale != "" {
			locale = string(key.Locale)
		}
		doc := c.Path
		if doc == "" {
			doc = c.Doc
		}
		out = append(out, StatusConflict{Doc: doc, Locale: locale, Edit: c.Op, Blocks: c.Blocks})
	}
	return out
}
