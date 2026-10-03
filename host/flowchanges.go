package host

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"sync"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/change/filehome"
	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
)

// What a flow writes, as the kapi host commits and records it.
//
// Every flow run the host starts (kapi translate, pseudo-translate, run, exec,
// up, and the same runs in Kapi Desktop and over MCP) writes its documents
// through the file home its change service commits through (flowHome): the
// same lock directory and the same if-unchanged rename. Inside a project the
// run also records what it changed, one content.edit operation per document
// it wrote, with the actor tool:<flow> and the origin flow:<flow>, through the
// recorder a change set's commit records through (App.EditRecorder). A flow's
// record keeps revisions and hashes and no text: the file holds the text.
//
// The transitions are the run's effect on the file as the change service reads
// it. The document is read through the service before the run (the revision
// of each edition it holds, or, for a run that writes a target-language file,
// of that edition joined in from its file) and again after the commit, and a
// transition is recorded for each edition whose revision moved. Reading the
// file rather than the blocks the run held is what makes a recorded revision
// the one a later read finds, which is how a read, kapi status and the next
// pass know the translation on disk is the one the run made and which source
// revision it was made from (its basis).
//
// Under --print-ops the run writes nothing: its home stages no file
// (filehome.Options.WriteNothing), nothing is recorded, and the operations the
// run would apply are collected into one change set (printedOps), which
// kapi apply applies to the same bytes.
//
// A document that moved while the run worked (a person saved it, a change set
// committed first) is applied again through the change service, from the
// operations the run's blocks describe, each guarded by the revision read
// before the run. The service reads the file under its lock and refuses the
// whole document stale when an edition the run changed has moved too.

// flowActor is the actor a flow writes as.
func flowActor(flowName string) change.Actor {
	return change.Actor{Kind: change.ActorTool, Name: flowName}
}

// flowOrigin is the origin a flow's record names.
func flowOrigin(flowName string) string { return "flow:" + flowName }

// flowHome is the file home a flow run commits the documents it writes
// through: the home of the project at root ("" outside one), with the lock
// directory a change service of that project takes its locks in. Under
// --print-ops it writes nothing but a convergence pass's drafts, which stay
// in the run's own tree.
func (a *App) flowHome(root string) *filehome.Home {
	opts := filehome.Options{LockDir: filepath.Join(DataDir(), "locks"), WriteNothing: a.printOps != nil, WriteUnder: a.convergeDraftDir}
	if root != "" {
		l := project.LayoutAt(root)
		opts.LockDir = filepath.Join(l.WorkDir(), "locks")
		opts.PrepareLocks = sync.OnceValue(func() error { return project.EnsureLayout(l) })
	}
	return filehome.New(nil, opts)
}

// flowDocuments returns the home a flow run commits through and the follower
// that records or prints what it changed, for the run cmd starts. root is the
// project's root, "" outside a project. A run outside a project that prints
// nothing has no follower: there is no log to record into.
func (a *App) flowDocuments(ctx context.Context, cmd Command, root string) (*filehome.Home, flow.Documents) {
	return a.flowDocumentsIn(ctx, cmd, root, model.LocaleID(a.SourceLocale()))
}

// FlowDocuments is flowDocuments for a surface that names the language the
// documents are written in itself (an MCP call, which serves many projects).
func (a *App) FlowDocuments(ctx context.Context, root string, source model.LocaleID) (*filehome.Home, flow.Documents) {
	return a.flowDocumentsIn(ctx, nil, root, source)
}

func (a *App) flowDocumentsIn(ctx context.Context, cmd Command, root string, source model.LocaleID) (*filehome.Home, flow.Documents) {
	home := a.flowHome(root)
	if root == "" && a.printOps == nil {
		return home, nil
	}
	recipe := ""
	if root != "" {
		recipe = project.LayoutAt(root).RecipePath
	}
	fc := &flowChanges{app: a, ctx: ctx, root: root, print: a.printOps, home: home, source: source}
	fc.changes = a.newCommandChanges(flowHookCommand(ctx, cmd, recipe), recipe, ChangeServiceOptions{
		Format:       a.FormatFlag,
		SourceLocale: source,
		// The follower reads revisions and applies the run's own
		// operations; it shows no edition's basis.
		revisionsOnly: true,
	})
	if root != "" && a.printOps == nil {
		fc.rec = a.changeRecorder(ctx, root)
	}
	return home, fc
}

// flowHookCommand is the command a flow's change service resolves its project
// from: one that names the project at recipe, or none.
func flowHookCommand(ctx context.Context, cmd Command, recipe string) Command {
	if recipe == "" {
		return detachedCommand(ctx, "flow")
	}
	if cmd != nil {
		if p, err := ResolveProjectPath(cmd); err == nil && p != "" && samePath(p, recipe) {
			return cmd
		}
	}
	return projectCommand(ctx, "flow", recipe)
}

// flowChanges follows the documents one flow run writes.
type flowChanges struct {
	app     *App
	ctx     context.Context
	root    string
	home    *filehome.Home
	changes *commandChanges
	rec     change.Recorder
	print   *printedOps
	// source is the language the documents are written in.
	source model.LocaleID
	// history is the project's block history, read when a document is
	// recorded.
	history *loopWrites

	mu sync.Mutex
}

var _ flow.Documents = (*flowChanges)(nil)

// Open starts following one document. A document the change service cannot
// read (no format reads it, or it lies where the recipe does not reach) is
// committed and neither recorded nor printed; a run over the source alone
// that writes elsewhere (an export) and a run that writes another format are
// the same.
func (fc *flowChanges) Open(ctx context.Context, d flow.Document) (flow.DocumentRun, error) {
	doc := &flowDoc{fc: fc, d: d, dest: d.OutputPath, inPlace: d.InPlace()}
	if dest, ok := fc.app.draftDestination(d.TargetLocale, d.OutputPath); ok {
		// The destination as the run first read it, whichever pass this is:
		// the drafts are the run's, and the destination keeps what it held.
		// Delivery commits the draft only while the destination still holds
		// it, whether or not the run follows the document.
		dd, err := fc.app.convergeDeliveries.first(dest)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", dest, err)
		}
		doc.dest, doc.draft, doc.delivery = dest, true, dd
	}
	switch {
	case d.OutputFormat != "" && d.OutputFormat != d.Format:
		// A conversion: the output is another document.
		fc.print.note(fmt.Sprintf("%s: a conversion of %s into another format, which no change set writes; it is not printed",
			fc.displayPath(doc.dest), fc.displayPath(d.InputPath)))
		return doc, nil
	case !doc.inPlace && d.TargetLocale == "":
		// The source, written somewhere else: an export.
		fc.print.note(fmt.Sprintf("%s: a copy of %s written elsewhere, which no change set writes; it is not printed",
			fc.displayPath(doc.dest), fc.displayPath(d.InputPath)))
		return doc, nil
	}
	fc.mu.Lock()
	if fc.changes.project == nil && fc.changes.dir == nil {
		// What the service records when it applies the run's changes to a
		// document that moved names the run's flow.
		fc.changes.opts.Origin = flowOrigin(d.Flow)
	}
	svc, ref, inProject, err := fc.locate(ctx, d.InputPath)
	fc.mu.Unlock()
	if err != nil {
		fc.print.note(fmt.Sprintf("%s: kapi apply reads no document there (%v); it is not printed", fc.displayPath(doc.dest), err))
		return doc, nil
	}
	if fc.print == nil && (fc.rec == nil || !inProject) {
		return doc, nil
	}
	doc.svc, doc.ref = svc, ref
	if !doc.inPlace {
		doc.edition = model.EditionKey{Locale: d.TargetLocale}.Canonical()
		if !fc.editionLivesAt(ctx, ref, doc.edition, doc.dest) {
			// The run writes the edition somewhere the change service does
			// not keep it, so a read of the document does not find it there
			// and a change set cannot write it there: nothing is recorded,
			// and a printing run names the file it leaves out.
			if fc.print != nil {
				fc.print.leaveOut(fc.displayPath(doc.dest), ref, doc.edition)
			}
			return doc, nil
		}
	}
	doc.track = true
	var rerr error
	switch {
	case doc.delivery != nil:
		// The destination as the run first read it guards the delivery; the
		// source this pass translates is this pass's read of it.
		var fresh revisions
		doc.before, rerr = doc.delivery.firstRevisions(func() (revisions, error) {
			r, err := doc.readRevisions(ctx)
			fresh = r
			return r, err
		})
		if rerr == nil && fresh == nil {
			fresh, rerr = doc.readRevisions(ctx)
		}
		doc.sources = fresh
	default:
		// The blocks are kept while the run works on the document, for a
		// commit that leaves the file as it was read.
		doc.before, doc.read, rerr = doc.readDocument(ctx, fc.rec != nil && fc.print == nil)
	}
	if rerr != nil {
		doc.track = false
		fc.print.note(fmt.Sprintf("%s: kapi apply cannot read %s (%v); it is not printed", fc.displayPath(doc.dest), ref, rerr))
		return doc, nil
	}
	if fc.print != nil {
		doc.entered = map[string]*model.Block{}
	}
	return doc, nil
}

// locate finds the service that holds the file at path and the reference it
// names the file by, and reports whether the file is a document of the
// project.
func (fc *flowChanges) locate(ctx context.Context, path string) (*change.Service, string, bool, error) {
	cs, ref, inProject, err := fc.changes.forFile(ctx, path)
	if err != nil {
		return nil, "", false, err
	}
	return cs.svc, ref, inProject, nil
}

// editionLivesAt reports whether the change service keeps edition k of the
// document ref in the file at path.
func (fc *flowChanges) editionLivesAt(ctx context.Context, ref string, k model.EditionKey, path string) bool {
	fc.mu.Lock()
	cs := fc.changes.project
	if cs == nil {
		cs = fc.changes.dir
	}
	fc.mu.Unlock()
	if cs == nil {
		return false
	}
	d, err := cs.home.layout.Locate(ctx, ref)
	if err != nil || d.EditionFile == nil {
		return false
	}
	f, ok := d.EditionFile(k)
	return ok && samePath(f.Path, path)
}

// flowDoc follows one document through a flow run.
type flowDoc struct {
	fc *flowChanges
	d  flow.Document
	// dest is the file the document's bytes land in: the output, or, for a
	// convergence pass that drafts, the file its draft is delivered to.
	dest  string
	draft bool
	// delivery is a draft's delivery: the destination as the run first read
	// it, and the follower of the pass that drafted it last.
	delivery *draftDelivery
	// svc reads and edits the document; ref names it there.
	svc *change.Service
	ref string
	// inPlace says the run writes the file it reads. Otherwise the run
	// writes edition, a target-language file of the document.
	inPlace bool
	edition model.EditionKey
	// track says the run's changes are recorded or printed.
	track bool
	// before is each block's editions as the service read them before the
	// run: block key, then edition text, then revision. read is that read's
	// blocks, for a document the run records and a commit that finds the
	// file as the run read it.
	before revisions
	read   *readBlocks
	// sources is a draft's document as the pass that drafted it read it,
	// for the source each translation was made from.
	sources revisions

	mu sync.Mutex
	// left are the blocks as the writer received them, by block key, in
	// document order; entered (under --print-ops) the blocks as the reader
	// gave them, by block id.
	left    map[string]*leftBlock
	order   []string
	entered map[string]*model.Block
	// producers are the stamps the run's tools left on each derived edition
	// it wrote, by block key and edition text, for the record.
	producers map[string]model.Origin
}

// producerKey names one edition of one block among a document's producers.
func producerKey(block, edition string) string { return block + "\x00" + edition }

// revisions maps a block key to its editions as the change service read
// them.
type revisions map[string]blockRevs

// blockRevs is one block's editions as the change service read them: each
// tracked edition's revision by edition text ("" is the document's own), and
// the authoritative edition, by the key the service names it with, its
// revision, which is the basis of a derived edition made from it, and the
// content hash of its text.
type blockRevs struct {
	editions    map[string]string
	auth        model.EditionKey
	authRev     string
	contentHash string
}

// revision is the revision edition text held, AbsentRevision for one the
// block did not hold.
func (r blockRevs) revision(text string) (string, bool) {
	rev, ok := r.editions[text]
	if !ok {
		return model.AbsentRevision, false
	}
	return rev, true
}

// leftBlock is what the run left in one block: each tracked edition the run
// changed, and the block itself under --print-ops.
type leftBlock struct {
	block    *model.Block
	editions map[string]leftEdition
}

// basis is the revision of the authoritative edition a derived edition the
// run wrote in the block was made from, as the change service names it: the
// source the service read, or, when the run rewrote the document's own edition
// too, that edition as the run left it.
func (doc *flowDoc) basis(key string, lb *leftBlock) string {
	br := doc.sourceOf(key)
	if own, ok := lb.editions[""]; ok && doc.inPlace {
		return model.RunsRevision(br.auth, own.runs)
	}
	if br.authRev != "" {
		return br.authRev
	}
	if lb.block != nil {
		return doc.flowRevision(br, lb.block, lb.block.Authoritative(model.AuthorityPolicy{}))
	}
	return ""
}

// sourceOf is the block keyed key as the pass that wrote the document read
// it, for the source a translation was made from: a draft's pass reads it
// afresh, while its destination is the one the run first read.
func (doc *flowDoc) sourceOf(key string) blockRevs {
	if br, ok := doc.sources[key]; ok {
		return br
	}
	return doc.before[key]
}

// flowRevision is the revision of edition k of a block the run holds, under
// the key the change service names the edition with. A reader the run drives
// may leave the language of the document's own edition unset, where the
// service's read names it; the content, not the spelling of the key, is what
// a revision compares.
func (doc *flowDoc) flowRevision(br blockRevs, b *model.Block, k model.EditionKey) string {
	ed, ok := b.Edition(k)
	if !ok {
		return model.AbsentRevision
	}
	key := b.EditionKeyOf(k)
	if key.IsZero() {
		key = br.auth
		if key.IsZero() && doc.fc.source != "" {
			key = model.EditionKey{Locale: doc.fc.source}
		}
	}
	return model.RunsRevision(key, ed.Runs)
}

// leftEdition is an edition as the run left it.
type leftEdition struct {
	key  model.EditionKey
	runs []model.Run
	rev  string
	// status and origin are the stamp the run's tool left on a derived
	// edition.
	status model.Status
	origin model.Origin
}

// editionText is the text form of an edition key, "" for the zero key.
func editionText(k model.EditionKey) string {
	if k.IsZero() {
		return ""
	}
	b, err := k.MarshalText()
	if err != nil {
		return ""
	}
	return string(b)
}

// tracked lists the editions of b the run's record covers: the document's own
// and every edition the block holds in the file for a run over the file it
// reads, else the edition the run writes.
func (doc *flowDoc) tracked(b *model.Block) []model.EditionKey {
	if !doc.inPlace {
		return []model.EditionKey{doc.edition}
	}
	keys := []model.EditionKey{{}}
	for _, k := range b.Editions() {
		if !b.IsSourceEdition(k) {
			keys = append(keys, k.Canonical())
		}
	}
	return keys
}

// readRevisions reads the document through the change service and returns
// the revision of every tracked edition of each block.
func (doc *flowDoc) readRevisions(ctx context.Context) (revisions, error) {
	revs, _, err := doc.readDocument(ctx, false)
	return revs, err
}

// readDocument reads the document through the change service: the revision
// of every tracked edition of each block, and, with keep, the blocks too. The
// read records what it finds changed outside kapi, as every read does, except
// in a run that prints its change set, which records nothing.
func (doc *flowDoc) readDocument(ctx context.Context, keep bool) (revisions, *readBlocks, error) {
	if doc.fc.print != nil {
		ctx = change.Unobserved(ctx)
	}
	out := revisions{}
	var blocks *readBlocks
	if keep {
		blocks = &readBlocks{blocks: map[string]*model.Block{}}
	}
	q := change.ReadRequest{Doc: doc.ref}
	if !doc.inPlace {
		q.Editions = []model.EditionKey{doc.edition}
	}
	_, err := doc.svc.ReadEach(ctx, q, func(b *model.Block, _ change.BlockRead) error {
		key := change.BlockKey(b)
		br := blockRevs{editions: map[string]string{}}
		for _, k := range doc.tracked(b) {
			br.editions[editionText(k)] = model.EditionRevision(b, k)
		}
		br.auth = b.EditionKeyOf(b.Authoritative(model.AuthorityPolicy{}))
		br.authRev = model.EditionRevision(b, br.auth)
		br.contentHash = model.ComputeContentHash(b.SourceText())
		out[key] = br
		if blocks != nil {
			if _, seen := blocks.blocks[key]; !seen {
				blocks.order = append(blocks.order, key)
			}
			blocks.blocks[key] = b
		}
		return nil
	})
	if err != nil {
		var ce *change.Error
		if errors.As(err, &ce) && ce.Code == change.CodeNotFound && !doc.inPlace {
			// A target file that does not exist yet holds no edition.
			return out, nil, nil
		}
		return nil, nil, err
	}
	return out, blocks, nil
}

// Enter keeps a copy of the block as the reader gave it, for the operations
// --print-ops prints.
func (doc *flowDoc) Enter(b *model.Block) {
	if !doc.track || doc.entered == nil {
		return
	}
	c := snapshotBlock(b)
	doc.mu.Lock()
	doc.entered[b.ID] = c
	doc.mu.Unlock()
}

// Leave keeps each tracked edition of the block the run changed from what the
// service read before the run.
func (doc *flowDoc) Leave(b *model.Block) {
	if !doc.track {
		return
	}
	key := change.BlockKey(b)
	lb := &leftBlock{editions: map[string]leftEdition{}}
	before := doc.before[key]
	for _, k := range doc.tracked(b) {
		text := editionText(k)
		if ed, ok := b.Edition(k); ok && !k.IsZero() && ed.Origin != (model.Origin{}) {
			doc.mu.Lock()
			if doc.producers == nil {
				doc.producers = map[string]model.Origin{}
			}
			doc.producers[producerKey(key, text)] = ed.Origin
			doc.mu.Unlock()
		}
		rev := doc.flowRevision(before, b, k)
		if was, _ := before.revision(text); rev == was {
			continue
		}
		if rev == model.AbsentRevision && !doc.inPlace {
			// The run wrote no translation for the block, and the writer
			// renders the block from its source: a target file has no way to
			// hold the block without an edition.
			continue
		}
		ed, _ := b.Edition(k)
		lb.editions[text] = leftEdition{key: k, runs: slices.Clone(ed.Runs), rev: rev, status: ed.Status, origin: ed.Origin}
	}
	if len(lb.editions) == 0 && doc.entered == nil {
		return
	}
	if doc.entered != nil {
		lb.block = snapshotBlock(b)
	}
	doc.mu.Lock()
	defer doc.mu.Unlock()
	if doc.left == nil {
		doc.left = map[string]*leftBlock{}
	}
	if _, seen := doc.left[key]; !seen {
		doc.order = append(doc.order, key)
	}
	doc.left[key] = lb
}

// Abort ends a document the run did not produce.
func (doc *flowDoc) Abort() {}

// Commit commits the produced document and records what it changed; under
// --print-ops it adds the document's operations to the run's change set and
// writes nothing.
func (doc *flowDoc) Commit(ctx context.Context, p *filehome.Produced) error {
	fc := doc.fc
	if fc.print != nil && !doc.draft {
		if doc.track {
			return doc.printDocument(ctx, doc.dest, p.After())
		}
		return nil
	}
	// A draft is written into the run's own tree whether or not the run
	// prints: its delivery gate reads it, and delivery prints it or commits
	// it.
	err := p.Commit(ctx)
	if errors.Is(err, filehome.ErrMoved) && doc.track && doc.svc != nil && !doc.draft {
		return doc.applyAgain(ctx, err)
	}
	if err != nil {
		return err
	}
	if doc.draft {
		fc.app.noteDraft(doc, p)
		return nil
	}
	if !doc.track || fc.rec == nil {
		return nil
	}
	return doc.record(ctx, p.Written(), p.Before(), p.After())
}

// record reads the document as the commit left it and records, as one
// content.edit, each tracked edition whose revision moved, and each
// translation the run produced that the file already held, unless the block
// history already says a flow wrote that translation from the source the block
// holds now. The second kind keeps the run's basis: a pass that reproduces a
// translation (content memory recycling the wording a file holds) still made
// it from the source in front of it, and a later source edit under it is drift
// the loop owes a draft for. The content is written whatever happens here; a
// failure is reported as the record's and leaves the next read to find a
// transition no record explains.
func (doc *flowDoc) record(ctx context.Context, written bool, before, after string) error {
	doc.mu.Lock()
	produced := len(doc.producers) > 0
	doc.mu.Unlock()
	if !written && !produced {
		return nil
	}
	pre, now := doc.read, doc.read
	if written || before != after || now == nil {
		// The file holds what the run wrote, and a read of it names each
		// revision as a later read finds it. A file that already held the
		// run's bytes when the run read it reads as it read then.
		read, err := doc.readRevisionsWithBlocks(ctx)
		if err != nil {
			return fmt.Errorf("record what %s wrote to %s: %w", doc.d.Flow, doc.ref, err)
		}
		now = &read
	}
	doc.read = nil
	loop := doc.fc.loop()
	docKey := doc.fc.docKey(doc.ref)
	var transitions []change.Transition
	for _, key := range now.order {
		b := now.blocks[key]
		basis, content := doc.madeFrom(key, b)
		for _, k := range doc.tracked(b) {
			text := editionText(k)
			was, _ := doc.before[key].revision(text)
			rev := model.EditionRevision(b, k)
			o, byTool := doc.producer(key, text)
			derived := !b.IsSourceEdition(k) && !k.IsZero()
			if rev == was {
				if !derived || !byTool || rev == model.AbsentRevision {
					continue
				}
				if r, ok := loop.at(docKey, key, text, rev); ok && r.After == rev &&
					(!loopWrite(r) || r.ContentHash == content) {
					// Recorded already: a tool wrote this translation from
					// this source, or a writer whose source nobody recorded
					// wrote it (a person, an agent, a venue) and the run
					// reproduced their wording, which stays theirs.
					continue
				}
			}
			t := change.Transition{
				Ref:       change.Ref{Doc: doc.ref, Block: key, Edition: k},
				Role:      change.RoleAuthoritative,
				BeforeRev: was,
				AfterRev:  rev,
				Block:     b,
				Ops:       doc.kinds(pre, key, k, b, was, rev),
				Tool:      doc.toolOf(o),
			}
			if derived {
				t.Role = change.RoleDerived
				t.Basis, t.ContentHash = basis, content
				// A file of strings keeps no stamp, so the record carries
				// the one the run's tool left.
				if byTool {
					ed, _ := b.Edition(k)
					ed.Origin = o
					b.SetEdition(k, ed)
				}
			}
			transitions = append(transitions, t)
		}
	}
	if len(transitions) == 0 {
		return nil
	}
	res := change.DocResult{Doc: doc.ref, Home: "file", Written: written, Before: before}
	if written {
		res.After = &after
	}
	if !doc.inPlace {
		res.Edition = editionText(doc.edition)
		if rel, rerr := filepath.Rel(doc.fc.root, doc.dest); rerr == nil {
			res.File = filepath.ToSlash(rel)
		}
	}
	_, err := doc.fc.rec.Record(ctx, change.Record{
		Actor:       flowActor(doc.d.Flow),
		Origin:      flowOrigin(doc.d.Flow),
		Docs:        []change.DocResult{res},
		Transitions: transitions,
	})
	if err != nil {
		return fmt.Errorf("record what %s wrote to %s: %w", doc.d.Flow, doc.ref, err)
	}
	return nil
}

// kinds names the operation the run's change to edition k of the block keyed
// key comes to, as change.Diff would send it: from the edition the service
// read before the run (pre, where the follower kept that read) to b, the
// block as the commit left it. A translation the run reproduced was set
// again. Without the earlier read, a change between two editions that both
// exist is named set_content.
func (doc *flowDoc) kinds(pre *readBlocks, key string, k model.EditionKey, b *model.Block, was, rev string) []change.Kind {
	if was == rev {
		return []change.Kind{change.KindSetContent}
	}
	had, has := was != model.AbsentRevision, rev != model.AbsentRevision
	now, _ := b.Edition(k)
	var before []model.Run
	if pre != nil {
		if pb := pre.blocks[key]; pb != nil {
			ed, _ := pb.Edition(k)
			before = ed.Runs
		}
	}
	if had && has && before == nil {
		return []change.Kind{change.KindSetContent}
	}
	kind, ok := change.EditionKind(before, had, now.Runs, has)
	if !ok {
		return nil
	}
	return []change.Kind{kind}
}

// toolOf names the tool that changed an edition: the one whose stamp the
// edition carries, else the run's only tool. A run of several tools that
// stamped nothing names none.
func (doc *flowDoc) toolOf(stamp model.Origin) string {
	if stamp.Tool != "" {
		return stamp.Tool
	}
	if len(doc.d.Tools) == 1 {
		return doc.d.Tools[0]
	}
	return ""
}

// madeFrom is the source a derived edition of the block keyed key was made
// from: its revision, the basis, and its content hash. A run that writes a
// target-language file read the source before it ran, and the commit guards
// the file it writes, not the source, so a source edited while the run worked
// is drift against what the run read rather than the basis of its
// translation. A run over the file it reads commits that file only while it
// still holds what the run read, so the source b holds after the commit is the
// one the run made its editions from.
func (doc *flowDoc) madeFrom(key string, b *model.Block) (basis, content string) {
	if br := doc.sourceOf(key); !doc.inPlace && br.authRev != "" && br.authRev != model.AbsentRevision {
		return br.authRev, br.contentHash
	}
	return model.EditionRevision(b, b.EditionKeyOf(b.Authoritative(model.AuthorityPolicy{}))), model.ComputeContentHash(b.SourceText())
}

// loop is the block history of the run's project, read once per run.
func (fc *flowChanges) loop() *loopWrites {
	fc.mu.Lock()
	defer fc.mu.Unlock()
	if fc.history == nil {
		fc.history = fc.app.newLoopWrites(fc.ctx, fc.root)
	}
	return fc.history
}

// docKey is the key the project knows the document ref by.
func (fc *flowChanges) docKey(ref string) string {
	return fc.app.documentKey(fc.ctx, fc.root, ref)
}

// readBlocks is a read of the document's blocks, by key, in document order.
type readBlocks struct {
	order  []string
	blocks map[string]*model.Block
}

// readRevisionsWithBlocks reads the document through the change service and
// returns its blocks. It reads what the run has just committed, which the run
// records itself, so the read records nothing as changed outside kapi.
func (doc *flowDoc) readRevisionsWithBlocks(ctx context.Context) (readBlocks, error) {
	ctx = change.Unobserved(ctx)
	out := readBlocks{blocks: map[string]*model.Block{}}
	q := change.ReadRequest{Doc: doc.ref}
	if !doc.inPlace {
		q.Editions = []model.EditionKey{doc.edition}
	}
	_, err := doc.svc.ReadEach(ctx, q, func(b *model.Block, _ change.BlockRead) error {
		key := change.BlockKey(b)
		if _, seen := out.blocks[key]; !seen {
			out.order = append(out.order, key)
		}
		out.blocks[key] = b
		return nil
	})
	return out, err
}

// applyOps are the operations that write what the run left in each block it
// changed, each guarded by the revision the service read before the run. The
// runs are the run's own, which an in-process sender may send whole.
func (doc *flowDoc) applyOps() []change.Op {
	doc.mu.Lock()
	defer doc.mu.Unlock()
	var ops []change.Op
	for _, key := range doc.order {
		lb := doc.left[key]
		for _, text := range sortedEditions(lb.editions) {
			ed := lb.editions[text]
			was, _ := doc.before[key].revision(text)
			at := change.Ref{Doc: doc.ref, Block: key, Edition: ed.key}
			if ed.rev == model.AbsentRevision {
				ops = append(ops, change.Op{Kind: change.KindRemoveEdition, At: at, IfMatch: was, Body: &change.RemoveEdition{}})
				continue
			}
			op := change.Op{Kind: change.KindSetContent, At: at, IfMatch: was, Body: &change.SetContent{Runs: ed.runs}}
			if !ed.key.IsZero() {
				op.Basis = doc.basis(key, lb)
			}
			ops = append(ops, op)
			if !ed.key.IsZero() && ed.origin != (model.Origin{}) {
				// The stamp the tool left, which a tool actor records with
				// the provenance operation after the content: the edition is
				// the tool's draft, as the run's own commit leaves it.
				ops = append(ops, change.Op{Kind: change.KindProvenance, At: at,
					Body: &change.Provenance{Status: ed.status, Origin: ed.origin}})
			}
		}
	}
	return ops
}

// applyAgain applies what the run changed to a document that moved while it
// worked, through the change service, which reads it again under the commit
// lock. moved is the commit's report. A change the service refuses (an edition
// somebody changed too, or a block the file does not hold yet) leaves the file
// as they saved it, and the run reports it: the next run writes it.
func (doc *flowDoc) applyAgain(ctx context.Context, moved error) error {
	ops := doc.applyOps()
	if len(ops) == 0 {
		return nil
	}
	res, err := doc.svc.Apply(ctx, change.Set{Gate: change.GateReport, Ops: ops}, flowActor(doc.d.Flow))
	if err != nil {
		return fmt.Errorf("%w; applying the run's changes again: %w", moved, err)
	}
	if res.Status == change.SetRefused {
		for _, r := range res.Ops {
			if r.Status == change.OpRefused && r.Error != nil {
				return fmt.Errorf("%w; the run's changes were not applied again (%s); run it again to write them", moved, r.Error.Message)
			}
		}
		return fmt.Errorf("%w; run it again to write its changes", moved)
	}
	return nil
}

// sortedEditions lists the edition texts of a block's changes in order, the
// document's own first.
func sortedEditions(m map[string]leftEdition) []string {
	out := make([]string, 0, len(m))
	for text := range m {
		out = append(out, text)
	}
	slices.Sort(out)
	return out
}

// draftDestination is the file a convergence pass's draft at path is
// delivered to, when path is one.
func (a *App) draftDestination(locale model.LocaleID, path string) (string, bool) {
	if a.convergeDraftDir == "" || a.convergeDraftRoot == "" || locale == "" {
		return "", false
	}
	base := filepath.Join(a.convergeDraftDir, string(locale))
	rel, err := filepath.Rel(base, path)
	if err != nil || !filepath.IsLocal(rel) {
		return "", false
	}
	return filepath.Join(a.convergeDraftRoot, rel), true
}

// snapshotBlock copies a block's editions deeply enough that what a tool does
// to the block afterwards leaves the copy as it was: the source runs and each
// target, runs included, are copies.
func snapshotBlock(b *model.Block) *model.Block {
	c := *b
	c.Source = copyRuns(b.Source)
	if b.Targets != nil {
		c.Targets = make(map[model.VariantKey]*model.Target, len(b.Targets))
		for k, t := range b.Targets {
			if t == nil {
				c.Targets[k] = nil
				continue
			}
			tc := *t
			tc.Runs = copyRuns(t.Runs)
			c.Targets[k] = &tc
		}
	}
	c.Properties = maps.Clone(b.Properties)
	return &c
}

// copyRuns copies a run sequence through its canonical JSON, so no run of the
// copy shares a pointer with the original.
func copyRuns(runs []model.Run) []model.Run {
	if runs == nil {
		return nil
	}
	var out []model.Run
	if err := json.Unmarshal(model.CanonicalRunsJSON(runs), &out); err != nil {
		return slices.Clone(runs)
	}
	return out
}

// FlowHome is the file home a flow, or any surface that writes a whole
// document a run produced, commits through for the project at root ("" outside
// one): the lock directory a change service of that project takes its locks
// in.
func (a *App) FlowHome(root string) *filehome.Home { return a.flowHome(root) }

// producer is the stamp the run's tool left on edition text of the block key.
func (doc *flowDoc) producer(key, text string) (model.Origin, bool) {
	doc.mu.Lock()
	defer doc.mu.Unlock()
	o, ok := doc.producers[producerKey(key, text)]
	return o, ok
}
