package host

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"slices"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/projector"
	"github.com/neokapi/neokapi/core/reconcile"
)

// The record of an applied edit.
//
// The change service hands every change set it applied to a recorder
// (change.Recorder) once the homes committed. This one records it in the
// workspace's log as one content.edit operation per document
// (projector.RecordEdit), which the projector writes into the block history:
// who changed which edition, from which revision to which, through which
// surface, under which governance.
//
// What a record keeps depends on who made the change and where the text
// lives. A person's or an agent's edit keeps the runs around each change and
// the change set as sent, which review, a revert by session and "who wrote
// this" read back. A tool's edit, which a flow makes by the thousand, keeps
// the revisions and hashes only: the file holds the text. A write to the
// workspace home keeps the edition it leaves, whoever made it, because the log
// is that home.
//
// A project that declares redaction (defaults.redaction) keeps no withheld
// value in a record, because a record travels to every context backend the
// project syncs with (C-10). The runs a record keeps, and its note, are
// redacted with the project's rules first, the originals going to the project
// vault as ingest puts them, and the change set as sent, whose free text no
// rule reaches through its structure, is left out. A policy that detects
// entities needs the entity annotations of a read, which a record's runs do
// not carry, so under one the record keeps no runs and no note.

// homeWorkspace is the home whose text the log holds (change.DocResult.Home).
const homeWorkspace = "workspace"

// EditRecorder returns the recorder for the project rooted at root. It
// records through the project's projector, so an edit reaches the same log
// and the same context store as the project's decisions, and it applies the
// redaction policy the project's recipe declares.
func (a *App) EditRecorder(ctx context.Context, root string) (change.Recorder, error) {
	p, err := a.Projector(ctx, root)
	if err != nil {
		return nil, err
	}
	r := &editRecorder{app: a, root: root, p: p}
	layout := project.LayoutAt(root)
	proj, err := project.Load(layout.RecipePath)
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		// A policy that cannot be read is not one to assume away: the record
		// would carry whatever the edit held.
		return nil, fmt.Errorf("record edits: read the redaction policy: %w", err)
	default:
		if spec := ProjectRedaction(proj); spec != nil {
			r.redaction = &recordRedaction{spec: spec, root: layout.Root, vault: layout.RedactionVaultPath()}
		}
	}
	return r, nil
}

type editRecorder struct {
	app       *App
	root      string
	p         *projector.Projector
	redaction *recordRedaction
}

var _ change.Recorder = (*editRecorder)(nil)

// Record records rec, one operation per document it changed, in the order the
// record lists them and in one write to the log, and returns the id of the
// first. The others name the same change set.
func (r *editRecorder) Record(ctx context.Context, rec change.Record) (string, error) {
	keepRuns := rec.Actor.Kind == change.ActorPerson || rec.Actor.Kind == change.ActorAgent
	var setJSON []byte
	if keepRuns && rec.Set != nil && r.redaction == nil {
		var err error
		if setJSON, err = json.Marshal(rec.Set); err != nil {
			return "", fmt.Errorf("record edit: encode the change set: %w", err)
		}
	}
	note := ""
	if rec.Set != nil {
		note = rec.Set.Note
	}

	stated, statesBases := ctx.Value(statedBasesKey{}).(map[basisAt]bool)

	var order []string
	results := map[string]change.DocResult{}
	byDoc := map[string][]change.Transition{}
	for _, d := range rec.Docs {
		if _, seen := results[d.Doc]; !seen {
			order = append(order, d.Doc)
		}
		results[d.Doc] = d
	}
	for _, t := range rec.Transitions {
		if _, listed := results[t.Ref.Doc]; !listed {
			if _, seen := byDoc[t.Ref.Doc]; !seen {
				order = append(order, t.Ref.Doc)
			}
		}
		byDoc[t.Ref.Doc] = append(byDoc[t.Ref.Doc], t)
	}

	var edits []projector.Edit
	for _, doc := range order {
		transitions := byDoc[doc]
		if len(transitions) == 0 {
			continue
		}
		res := results[doc]
		e := projector.Edit{
			Doc:         r.editDoc(ctx, doc),
			Home:        res.Home,
			Actor:       rec.Actor,
			Origin:      projector.Origin{By: rec.Origin},
			Fingerprint: rec.Fingerprint,
			Note:        note,
			DocBefore:   res.Before,
			Overridden:  overriddenIn(doc, rec.Overridden),
			SetJSON:     setJSON,
		}
		if res.After != nil {
			e.DocAfter = *res.After
		}
		keep := keptRuns{before: keepRuns, after: keepRuns || res.Home == homeWorkspace}
		for _, t := range transitions {
			if statesBases && !stated[basisAt{t.Ref.Block, t.Ref.EditionText()}] {
				// The writer named the basis of every edition whose source it
				// knows (WithStatedBases). The service's basis for any other,
				// the source the checkout holds now, would claim it current.
				t.Basis = ""
			}
			e.Transitions = append(e.Transitions, editTransition(t, keep))
		}
		edits = append(edits, e)
	}
	if len(edits) == 0 {
		return "", nil
	}
	if err := r.redaction.apply(ctx, edits); err != nil {
		return "", fmt.Errorf("record edit: redact: %w", err)
	}
	ids, err := r.p.RecordEdits(ctx, edits)
	if err != nil {
		return "", fmt.Errorf("record edit: %w", err)
	}
	return ids[0], nil
}

// basisAt names an edition of a block a change set's operation addresses.
type basisAt struct{ block, edition string }

// statedBasesKey marks the context of a change set that states its bases.
type statedBasesKey struct{}

// WithStatedBases returns ctx for applying set, a change set whose writer
// knows the source of some of the derived editions it writes and of no other:
// kapi pull, writing a venue's translations, knows the source of those the
// venue's record says were made from the source the checkout holds. Each
// operation names that source as its basis, and the record of the set keeps
// the basis of those editions and of no other. Without it the record keeps
// the basis the service gives every derived edition an operation names none
// for, the authoritative edition the change set found.
func WithStatedBases(ctx context.Context, set change.Set) context.Context {
	stated := map[basisAt]bool{}
	for _, op := range set.Ops {
		if op.Basis != "" {
			stated[basisAt{op.At.Block, op.At.EditionText()}] = true
		}
	}
	return context.WithValue(ctx, statedBasesKey{}, stated)
}

// editDoc names the document a change addressed. A change addresses a
// document by its project-relative path, or by its key where the document
// lives in the workspace home and has no path; a key is kept as it is.
func (r *editRecorder) editDoc(ctx context.Context, doc string) projector.EditDoc {
	if reconcile.IsDocumentKey(doc) {
		return projector.EditDoc{Key: doc}
	}
	return projector.EditDoc{Key: r.app.documentKey(ctx, r.root, doc), Path: doc}
}

// keptRuns says which of an edition's run sequences a record keeps.
type keptRuns struct {
	before, after bool
}

// editTransition renders one recorded transition. Its identity evidence comes
// from the block after the change where the transition carries none, so every
// record can re-attach history after a reorder.
func editTransition(t change.Transition, keep keptRuns) projector.EditTransition {
	edition := t.Ref.EditionText()
	key, contentHash, contextHash := t.Key, t.ContentHash, t.ContextHash
	if b := t.Block; b != nil {
		if text, err := b.EditionKeyOf(t.Ref.Edition).MarshalText(); err == nil && len(text) > 0 {
			edition = string(text)
		}
		if key == "" {
			key = b.Unit
		}
		if contentHash == "" || contextHash == "" {
			id := model.ComputeIdentity(b)
			if contentHash == "" {
				contentHash = id.ContentHash
			}
			if contextHash == "" {
				contextHash = id.ContextHash
			}
		}
	}
	out := projector.EditTransition{
		Block: t.Ref.Block, Key: key, Edition: edition,
		Before: t.BeforeRev, After: t.AfterRev, Basis: t.Basis,
		ContentHash: contentHash, ContextHash: contextHash,
		Tool: t.Tool,
	}
	for _, k := range t.Ops {
		out.Ops = append(out.Ops, string(k))
	}
	if o := producerOf(t); o != (model.Origin{}) {
		// How a tool produced the edition, as it stamped it.
		out.Producer = &o
	}
	if keep.before {
		out.BeforeRuns = t.Before
	}
	if keep.after {
		out.AfterRuns = t.After
	}
	return out
}

// overriddenIn returns the overridden findings that concern one document: the
// ones placed in it, and the ones placed nowhere.
func overriddenIn(doc string, findings []change.Finding) []change.Finding {
	var out []change.Finding
	for _, f := range findings {
		if f.At == nil || f.At.Doc == doc {
			out = append(out, f)
		}
	}
	return out
}

// recordRedaction is a project's declared redaction policy as the recorder
// applies it to what a record keeps.
type recordRedaction struct {
	spec  *project.RedactionSpec
	root  string
	vault string
}

// apply redacts the runs and the notes of a record's edits in place, in one
// pass of the redact tool. A nil policy leaves them as they are.
func (r *recordRedaction) apply(ctx context.Context, edits []projector.Edit) error {
	if r == nil {
		return nil
	}
	if slices.Contains(r.spec.Detectors, "entities") {
		for i := range edits {
			edits[i].Note = ""
			for j := range edits[i].Transitions {
				edits[i].Transitions[j].BeforeRuns, edits[i].Transitions[j].AfterRuns = nil, nil
			}
		}
		return nil
	}
	// Each sequence is redacted as the source of a block of its own, named by
	// the content it holds, so the originals the vault keeps under that name
	// are always the ones the placeholders replaced.
	var (
		blocks []*model.Block
		sinks  []*[]model.Run
	)
	add := func(id string, runs *[]model.Run) {
		if len(*runs) == 0 {
			return
		}
		b := &model.Block{ID: id, Translatable: true}
		b.SetSourceRuns(slices.Clone(*runs))
		blocks = append(blocks, b)
		sinks = append(sinks, runs)
	}
	notes := make([][]model.Run, len(edits))
	for i := range edits {
		e := &edits[i]
		for j := range e.Transitions {
			t := &e.Transitions[j]
			at := "edit:" + e.Doc.Key + "#" + t.Block + "@" + t.Edition + ":"
			add(at+t.Before, &t.BeforeRuns)
			add(at+t.After, &t.AfterRuns)
		}
		if e.Note != "" {
			notes[i] = []model.Run{{Text: &model.TextRun{Text: e.Note}}}
			add("note:"+model.ComputeContentHash(e.Note), &notes[i])
		}
	}
	if len(blocks) == 0 {
		return nil
	}
	if err := RedactAtIngest(ctx, blocks, r.spec, r.root, r.vault, ""); err != nil {
		return err
	}
	for i, b := range blocks {
		src, _ := b.Edition(model.EditionKey{})
		*sinks[i] = src.Runs
	}
	for i := range edits {
		if notes[i] != nil {
			edits[i].Note = model.RenderRunsWithData(notes[i])
		}
	}
	return nil
}

// producerOf is the stamp a tool left on the derived edition a transition
// leaves, or the zero origin.
func producerOf(t change.Transition) model.Origin {
	if t.Block == nil || t.Role != change.RoleDerived || t.Ref.Edition.IsZero() {
		return model.Origin{}
	}
	ed, ok := t.Block.Edition(t.Ref.Edition)
	if !ok || ed.Origin.Kind == model.OriginHuman {
		return model.Origin{}
	}
	return ed.Origin
}
