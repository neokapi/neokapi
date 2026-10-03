package host

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/change/filehome"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/workhome"
	"github.com/neokapi/neokapi/core/workspace"
)

// The workspace home of a project (core/workhome) keeps the editions that
// have no file yet: a parked locale's drafts under `materialize: on-converge`,
// the drafts a gated run produced that kapi merge has not delivered, and every
// edit a person or an agent makes to them. They live in the project's
// operation log, outside the checkout, so deleting `.kapi/work/` loses none of
// them and costs no provider call to rebuild.
//
// The recipe picks an edition's home. Under `materialize: on-converge` a
// translation whose file does not exist lives in the workspace home until a
// delivery writes its file, whether or not the workspace holds anything of it
// yet; under `manual` it lives in its file, except while the workspace still
// keeps a draft of it (a recipe switched from on-converge). Once the file
// exists, the file is the edition's home and the workspace's copy is never
// read or written there. A gated run keeps the drafts of every locale it
// parks (keepDrafts); a delivery (kapi merge, or kapi up for a locale that
// cleared its gate) writes the file from what the workspace keeps and then
// releases exactly what it read, so the workspace never keeps a second copy
// of an edition a file holds. A file that appears by any other path (a
// person, kapi pull, a checkout) is settled at the end of the next run
// (settleKept): what the file already holds and a tool's drafts are
// released, and a person's or an agent's wording the file does not hold stays
// kept, listed by kapi status, until kapi merge writes it.

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
	redact, err := k.app.keptRedactionAt(k.root)
	if err != nil {
		return nil, err
	}
	keys := k.app.keptDocKeysFor(k.root)
	keyCtx := context.WithoutCancel(ctx)
	h := &workhome.Home{Store: db.Heads(), Log: p, DocKey: func(ref string) string { return keys.key(keyCtx, ref) }}
	if redact != nil {
		h.Redaction = redact
	}
	k.home = h
	return k.home, nil
}

// keptRedactionRead is the redaction policy a recipe declared when it was
// last read, with the recipe's size and time of change then.
type keptRedactionRead struct {
	size   int64
	mod    time.Time
	policy *keptRedaction
}

// keptRedactionAt is the redaction policy the recipe of the project rooted at
// root declares, as the workspace home applies it; nil for none. The recipe
// is read again only when it changed since the last read.
func (a *App) keptRedactionAt(root string) (*keptRedaction, error) {
	layout := project.LayoutAt(root)
	info, err := os.Stat(layout.RecipePath)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("the workspace home: read the redaction policy: %w", err)
	}
	s := a.ensureProjectStores()
	s.mu.Lock()
	read, ok := s.keptRedact[layout.Root]
	s.mu.Unlock()
	if ok && read.size == info.Size() && read.mod.Equal(info.ModTime()) {
		return read.policy, nil
	}
	proj, err := project.Load(layout.RecipePath)
	if err != nil {
		// A policy that cannot be read is not one to assume away: the record
		// would carry whatever the edition held.
		return nil, fmt.Errorf("the workspace home: read the redaction policy: %w", err)
	}
	read = keptRedactionRead{size: info.Size(), mod: info.ModTime()}
	if spec := ProjectRedaction(proj); spec != nil {
		read.policy = &keptRedaction{policy: &recordRedaction{spec: spec, root: layout.Root, vault: layout.RedactionVaultPath()}}
	}
	s.mu.Lock()
	if s.keptRedact == nil {
		s.keptRedact = map[string]keptRedactionRead{}
	}
	s.keptRedact[layout.Root] = read
	s.mu.Unlock()
	return read.policy, nil
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
	// withheld says the recipe withholds a translation's file until a
	// delivery writes it (`materialize: on-converge`).
	withheld bool
}

// newKeptPolicy is the policy of a project whose recipe is proj.
func (a *App) newKeptPolicy(root string, proj *project.KapiProject) *keptPolicy {
	return &keptPolicy{keeper: a.keptEditions(root),
		withheld: proj != nil && proj.Defaults.ResolvedMaterialize() == project.MaterializeOnConverge}
}

// keeps returns the keeper of edition ed of the document ref names, whose
// file is path, or nil when the edition's home is its file: the file exists,
// or the recipe delivers every translation as it is written and the
// workspace keeps nothing of the edition.
func (p *keptPolicy) keeps(ctx context.Context, ref string, ed model.EditionKey, path string) filehome.Keeper {
	if p == nil || p.keeper == nil {
		return nil
	}
	if _, err := os.Stat(path); err == nil || !errors.Is(err, os.ErrNotExist) {
		// The file is the edition's home; whatever the workspace still keeps
		// of the edition is never read beside it.
		return nil
	}
	if p.withheld || p.keeper.holds(ctx, ref, ed) {
		return p.keeper
	}
	return nil
}

// draftStamp is what a producer recognizes a draft it made by, kept beside
// the draft in the workspace home: the block-store key of its overlay, the
// overlay's own reuse fields (blockstore.TargetOverlay), and the overlay's
// runs where they differ from the draft's (a step after the producer, such as
// unredact, changed the draft). A run whose `.kapi/work/` was deleted writes
// the overlay again from it, so the producer serves the draft rather than
// paying a provider for it again.
type draftStamp struct {
	Key      string      `json:"key"`
	Provider string      `json:"provider,omitempty"`
	Config   string      `json:"config,omitempty"`
	Source   string      `json:"source,omitempty"`
	Runs     []model.Run `json:"runs,omitempty"`
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

// keepDrafts writes the drafts a gated convergence run produced for a
// locale into the workspace home, one write per document, and returns how
// many blocks it wrote. The drafts are the run's own (what each document's
// latest pass left, against the edition as the run first read it), with the
// basis each was made from and the stamp its producer serves it again by. A
// block a person or an agent changed while the run worked keeps their
// edition, and so does one they wrote from the source the draft was made
// from. A translation whose file exists is never kept: its file is its home.
//
// A parked locale keeps every draft. A locale about to be delivered keeps
// drafts only of the editions the workspace already holds (held), so the
// delivery writes the edits made to them over the run's fresh drafts, and
// never a draft of a source that has changed since.
func (a *App) keepDrafts(ctx context.Context, root string, locale model.LocaleID, held bool) (int, error) {
	if a.convergeDeliveries == nil || root == "" {
		return 0, nil
	}
	docs := a.convergeDeliveries.drafts(locale)
	if len(docs) == 0 {
		return 0, nil
	}
	redact, err := a.keptRedactionAt(root)
	if err != nil {
		return 0, err
	}
	if redact != nil {
		if err := redact.keeps(); err != nil {
			// The policy cannot redact what a kept draft records, so the
			// drafts stay in the producer's cache, as an ungated run leaves
			// them.
			fmt.Fprintf(os.Stderr, "Warning: the %s drafts are not kept in the workspace: %v\n", locale, err)
			return 0, nil
		}
	}
	kept := a.keptEditions(root)
	h, err := kept.open(ctx, !held)
	if err != nil || h == nil {
		return 0, err
	}
	var store blockstore.Store
	if db := a.existingProjectDB(ctx, root); db != nil {
		store = a.projectBlocksAutocommit(db)
	}
	written := 0
	for _, doc := range docs {
		if _, err := os.Stat(doc.dest); err == nil {
			continue
		}
		if held && !kept.holds(ctx, doc.ref, doc.edition) {
			continue
		}
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
			Key:         lb.unit,
			ContentHash: doc.sourceOf(key).contentHash,
			ContextHash: lb.contextHash,
		}
		pr.Stamp = draftStampOf(sess, doc, lb, ed)
		p.Blocks = append(p.Blocks, pr)
	}
	return p
}

// draftStampOf reads the overlay the run's producer left for a block's draft
// and returns the stamp the workspace home keeps beside the draft, or nil
// when the store holds no overlay for the block's source. The overlay's runs
// are kept in the stamp when a later step of the flow changed the draft.
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
	if json.Unmarshal(o.Payload, &stored) != nil {
		return nil
	}
	st := draftStamp{Key: key, Provider: stored.Provider, Config: stored.Config, Source: stored.Source}
	if stored.TargetText() != model.RunsText(ed.runs) {
		st.Runs = stored.Runs
		if len(st.Runs) == 0 && stored.TargetText() != "" {
			st.Runs = []model.Run{model.TextR(stored.TargetText())}
		}
	}
	data, err := json.Marshal(st)
	if err != nil {
		return nil
	}
	return data
}

// restoreKeptOverlays writes into the project's block store the overlay of
// every draft the workspace home keeps, where the store holds none: the block
// store is a cache under `.kapi/work/`, and a draft whose overlay is gone
// would otherwise cost a provider call in the next pass. A block a person or
// an agent edited is restored from the latest draft a producer wrote for it,
// which the log keeps, so the producer serves that draft and the person's
// wording stays where it is kept. It returns how many overlays it wrote.
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
		var drafts map[string]keptDraftOf
		for _, block := range slices.Sorted(maps.Keys(rows)) {
			r := rows[block]
			if len(r.Stamp) == 0 {
				if !byWriter(r.Origin) {
					continue
				}
				if drafts == nil {
					if drafts, err = lastDrafts(ctx, h, head); err != nil {
						return written, err
					}
				}
				d, ok := drafts[block]
				if !ok {
					continue
				}
				r = d.row
			}
			var st draftStamp
			if json.Unmarshal(r.Stamp, &st) != nil || st.Key == "" {
				continue
			}
			if _, err := sess.GetOverlay(kind, st.Key); err == nil {
				continue
			} else if !errors.Is(err, blockstore.ErrNotFound) {
				return written, err
			}
			runs := st.Runs
			if len(runs) == 0 {
				ed, err := h.EditionOf(ctx, r)
				if err != nil {
					return written, err
				}
				runs = ed.Runs
			}
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

// byWriter reports whether an edition's origin is a person's or an agent's.
func byWriter(o model.Origin) bool {
	return o.Kind == model.OriginHuman || o.Kind == model.OriginAgent
}

// keptDraftOf is the latest draft a producer wrote for one block of a kept
// edition, as a row the workspace home would hold for it.
type keptDraftOf struct {
	row workhome.Row
}

// lastDrafts reads, from the log, the latest draft a producer wrote for each
// block of one edition the workspace home keeps: the write that carries the
// producer's stamp. A person's or an agent's edit since replaced the draft in
// the edition and left it in the log.
func lastDrafts(ctx context.Context, h *workhome.Home, head workhome.Head) (map[string]keptDraftOf, error) {
	writes, err := h.Log.Writes(ctx, head.Doc, head.Edition)
	if err != nil {
		return nil, err
	}
	slices.SortStableFunc(writes, func(x, y workhome.Write) int { return cmp.Compare(x.Op, y.Op) })
	out := map[string]keptDraftOf{}
	for _, w := range writes {
		for _, b := range w.Blocks {
			if len(b.Stamp) == 0 || b.After == model.AbsentRevision {
				continue
			}
			out[b.Block] = keptDraftOf{row: workhome.Row{Doc: w.Doc, Edition: w.Edition, Block: b.Block, Rev: b.After,
				Runs: b.Runs, Blob: b.Ref, Status: b.Status, Origin: b.Origin, Basis: b.Basis, Stamp: b.Stamp, Op: w.Op}}
		}
	}
	return out, nil
}

// keptEdition reads what the workspace home keeps of edition ed of the
// document ref names, with the head the read found; nil when it keeps
// nothing.
func (a *App) keptEdition(ctx context.Context, root, ref string, ed model.EditionKey) (*workhome.Held, error) {
	h, err := a.keptEditions(root).open(ctx, false)
	if err != nil || h == nil {
		return nil, err
	}
	held, err := h.Read(ctx, ref, ed)
	if err != nil || len(held.Rows) == 0 {
		return nil, err
	}
	return &held, nil
}

// releaseKept removes from the workspace home the blocks of edition ed of
// the document ref names that a delivery read at held and wrote into the
// edition's file (nil: every block it read), recorded as actor's write
// through origin. It returns workhome.ErrReleaseMoved when the edition
// changed in the workspace after the delivery read it.
func (a *App) releaseKept(ctx context.Context, root, ref string, ed model.EditionKey, held *workhome.Held, blocks []string, actor change.Actor, origin string) error {
	if held == nil {
		return nil
	}
	h, err := a.keptEditions(root).open(ctx, false)
	if err != nil || h == nil {
		return err
	}
	_, err = h.Release(ctx, ref, ed, workhome.Release{Token: held.Token, Blocks: blocks, Actor: actor, Origin: origin})
	return err
}

// besideFile is an edition the workspace home keeps whose file exists: what
// the file already holds or a tool drafted, which the workspace releases, and
// the wording a person or an agent wrote that the file does not hold, which
// the workspace keeps until kapi merge writes it.
type besideFile struct {
	path    string
	edition model.EditionKey
	file    string
	held    *workhome.Held
	release []string
	kept    []string
}

// keptBesideFiles lists every edition the workspace home keeps whose file,
// where the recipe at recipe points, now exists.
func (a *App) keptBesideFiles(ctx context.Context, recipe string) ([]besideFile, error) {
	root := filepath.Dir(recipe)
	h, err := a.keptEditions(root).open(ctx, false)
	if err != nil || h == nil {
		return nil, err
	}
	if err := h.Log.CatchUp(ctx); err != nil {
		return nil, err
	}
	heads, err := h.Store.Heads(ctx)
	if err != nil || len(heads) == 0 {
		return nil, err
	}
	// The layout of a service that writes every edition to its file names
	// each edition's file and keeps none.
	ch, err := a.changeHome(ChangeServiceOptions{Project: recipe, Materialize: true})
	if err != nil {
		return nil, err
	}
	var source model.LocaleID
	if pl, ok := ch.layout.(*projectChangeLayout); ok {
		source = pl.source
	}
	services := &materializeServices{app: a, ctx: ctx, recipe: recipe, source: source}
	var out []besideFile
	for _, head := range heads {
		if head.Path == "" {
			continue
		}
		held, err := h.Store.Held(ctx, head.Doc, head.Edition)
		if err != nil {
			return nil, err
		}
		if !held {
			continue
		}
		key, err := model.ParseEditionKey(head.Edition)
		if err != nil || key.Locale == "" {
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
		read, err := h.Read(ctx, head.Path, key)
		if err != nil {
			return nil, err
		}
		if len(read.Rows) == 0 {
			continue
		}
		svc, err := services.of(key.Locale)
		if err != nil {
			return nil, err
		}
		inFile := map[string]string{}
		if _, err := svc.ReadEach(ctx, change.ReadRequest{Doc: head.Path, Editions: []model.EditionKey{key}}, func(b *model.Block, _ change.BlockRead) error {
			inFile[change.BlockKey(b)] = model.EditionRevision(b, key)
			return nil
		}); err != nil {
			// A file no reader opens holds nothing to compare: what the
			// workspace keeps stays kept until the file reads.
			continue
		}
		bf := besideFile{path: head.Path, edition: key, file: f.Ref, held: &read}
		for _, block := range slices.Sorted(maps.Keys(read.Rows)) {
			r := read.Rows[block]
			if rev, ok := inFile[block]; (ok && rev == r.Rev) || !byWriter(r.Origin) {
				bf.release = append(bf.release, block)
				continue
			}
			bf.kept = append(bf.kept, block)
		}
		out = append(out, bf)
	}
	return out, nil
}

// settleKept settles every edition the workspace home keeps whose file, where
// the recipe points, now exists, whichever path wrote the file: a pass of a
// recipe that stops withholding delivery, a person, kapi pull or a checkout.
// The file is the edition's home from then on, so the workspace releases what
// the file already holds and the drafts a tool made, which the file
// supersedes. A person's or an agent's wording the file does not hold stays
// kept, and kapi status lists it until kapi merge writes it into the file.
func (a *App) settleKept(ctx context.Context, recipe string) error {
	beside, err := a.keptBesideFiles(ctx, recipe)
	if err != nil {
		return err
	}
	root := filepath.Dir(recipe)
	for _, bf := range beside {
		if len(bf.release) == 0 {
			continue
		}
		err := a.releaseKept(ctx, root, bf.path, bf.edition, bf.held, bf.release, change.Actor{Kind: change.ActorTool, Name: "up"}, "flow:up")
		if errors.Is(err, workspace.ErrHeadMoved) {
			// Someone wrote the edition after this read; the next run
			// settles what it holds then.
			continue
		}
		if err != nil {
			return err
		}
	}
	return nil
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
// cannot be read: a status report never fails on the workspace home. It
// lists the edits a merge left divergent, and the wording a person or an
// agent wrote that a translation's file, which exists now, does not hold.
func (a *App) statusConflicts(ctx context.Context, recipe string) []StatusConflict {
	root := filepath.Dir(recipe)
	var out []StatusConflict
	if conflicts, err := a.keptConflicts(ctx, root); err == nil {
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
	}
	if beside, err := a.keptBesideFiles(ctx, recipe); err == nil {
		for _, bf := range beside {
			if len(bf.kept) == 0 {
				continue
			}
			out = append(out, StatusConflict{Doc: bf.path, Locale: string(bf.edition.Locale), File: bf.file, Blocks: bf.kept})
		}
	}
	return out
}
