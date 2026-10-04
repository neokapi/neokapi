package workhome

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/change/filehome"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/workspace"
)

// Log is where the workspace home records its writes, and what its
// projection catches up from: the project's projector.
type Log interface {
	// CommitWorkspace appends the record of a write to an edition the
	// workspace home keeps, only while the edition's head is still at
	// c.Expect, and folds it into the projection. A head that moved is
	// workspace.ErrHeadMoved, and nothing is appended.
	CommitWorkspace(ctx context.Context, c Commit) (string, error)
	// CatchUp folds into the projection what the log holds that it has not
	// yet folded: writes another process made, and writes merged in from
	// another machine.
	CatchUp(ctx context.Context) error
	// SubjectHead is the local position of the latest operation the log
	// holds on one edition, zero for none: the head a conditional record
	// expects.
	SubjectHead(ctx context.Context, doc, edition string) (int64, error)
	// Blob reads runs a row keeps in a blob.
	Blob(ctx context.Context, address string) ([]byte, error)
	// Writes reads every write the log holds to one edition, as the fold
	// reads them.
	Writes(ctx context.Context, doc, edition string) ([]Write, error)
}

// Redaction keeps the values a project's redaction policy withholds out of
// what the log records of a write to the workspace home, which travels to
// every context backend the project shares its context through, and puts them
// back where the workspace home reads an edition. The originals stay on the
// machine that withheld them, so another machine reads the placeholders.
type Redaction interface {
	// Redact rewrites, in place, the runs and the note a write records, and
	// drops the change set as sent, whose free text no rule reaches. An
	// error refuses the write: a policy that cannot redact a record keeps
	// no edition in the log.
	Redact(ctx context.Context, c *Commit) error
	// Restore returns runs as the edition held them before Redact, for the
	// row that keeps them.
	Restore(ctx context.Context, r Row, runs []model.Run) []model.Run
}

// Commit is one write to one edition the workspace home keeps, as the log
// records it: a content.edit operation that carries the result.
type Commit struct {
	// Doc is the document's key, Path where it is, and Edition the edition
	// in its text form.
	Doc     string
	Path    string
	Edition string
	// Expect is the edition's head in the log (Log.SubjectHead) the write was
	// staged on, and Base the operation the folded head is at. Cause names the divergent
	// operation a rebase carries over.
	Expect int64
	Base   string
	Cause  string

	Actor change.Actor
	// Origin names the surface that made the write: apply, desktop, mcp,
	// flow:<name>, merge, rebase.
	Origin      string
	Fingerprint string
	Note        string
	// Set is the change set as sent, for a person's or an agent's write.
	Set        *change.Set
	Overridden []change.Finding
	// DocBefore and DocAfter are the edition's digests around the write.
	DocBefore string
	DocAfter  string
	// Release says the write is a delivery's release of the whole edition,
	// which settles every write a merge left divergent on it.
	Release bool
	Blocks  []CommitBlock
}

// CommitBlock is one block's edition as a write leaves it.
type CommitBlock struct {
	// Block is the document's block key.
	Block string
	// Before and After are the edition's revisions around the write; After
	// is model.AbsentRevision for a write that removes it.
	Before string
	After  string
	// BeforeRuns are the runs the write replaced, kept for a person's or an
	// agent's write; Edition is what the write leaves, nil for a removal.
	BeforeRuns []model.Run
	Edition    *model.Edition
	// Basis is the authoritative edition's revision the edition was made
	// from.
	Basis string
	// Stamp is what the edition's producer recognizes it by.
	Stamp json.RawMessage
	// Key is the block's durable key where reconciliation assigned one, and
	// ContentHash and ContextHash are its identity signals.
	Key         string
	ContentHash string
	ContextHash string
	// Ops are the kinds of the operations that changed the edition, in the
	// order they applied, and Tool the tool in a flow that changed it, where
	// the write can name one: the block history keeps both, as it does for a
	// write to a file.
	Ops  []change.Kind
	Tool string
}

// Home is the workspace home over one project's log and projection. It keeps
// the editions of the project's documents that have no file yet
// (filehome.Keeper), and a flow writes its drafts into it (Produce).
type Home struct {
	Store *Store
	Log   Log
	// DocKey names the document a reference names by the key it keeps it
	// under, which a rename leaves as it was. Nil keeps the reference.
	DocKey func(ref string) string
	// Redaction applies the project's redaction policy to what the log
	// records; nil records the runs as they are.
	Redaction Redaction
}

var _ filehome.Keeper = (*Home)(nil)

// Name is the home as a change result reports it.
func (h *Home) Name() string { return Name }

func (h *Home) docKey(ref string) string {
	if h.DocKey == nil {
		return ref
	}
	return h.DocKey(ref)
}

// editionText is an edition key in the text form the workspace home keeps it
// under.
func editionText(k model.EditionKey) string {
	text, _ := k.Canonical().MarshalText()
	return string(text)
}

// token names a head for a commit to find again: the log's head on the
// edition and the operation the folded head is at.
func token(seq int64, h Head) string { return strconv.FormatInt(seq, 10) + " " + h.Op }

// EmptyKept is what a keeper reads of an edition in a project whose log holds
// nothing yet.
func EmptyKept() filehome.Kept { return filehome.Kept{Token: token(0, Head{})} }

// parseToken reads a token back into the head position and operation it
// names.
func parseToken(t string) (int64, string, error) {
	seq, op, _ := strings.Cut(t, " ")
	n, err := strconv.ParseInt(seq, 10, 64)
	if err != nil {
		return 0, "", fmt.Errorf("workhome: %q is not a head the workspace home read", t)
	}
	return n, op, nil
}

// read reads one edition: the log's head on it, the folded head, and the
// edition of each block it keeps, by block key. The log's head is read before
// the projection catches up, so the projection holds at least every write up
// to it, and a write that lands after it moves the head a commit expects.
func (h *Home) read(ctx context.Context, doc, edition string) (int64, Head, map[string]Row, map[string]model.Edition, error) {
	seq, err := h.Log.SubjectHead(ctx, doc, edition)
	if err != nil {
		return 0, Head{}, nil, nil, err
	}
	if err := h.Log.CatchUp(ctx); err != nil {
		return 0, Head{}, nil, nil, err
	}
	head, _, err := h.Store.Head(ctx, doc, edition)
	if err != nil {
		return 0, Head{}, nil, nil, err
	}
	rows, err := h.Store.Rows(ctx, doc, edition)
	if err != nil {
		return 0, Head{}, nil, nil, err
	}
	blocks := make(map[string]model.Edition, len(rows))
	for key, r := range rows {
		ed, err := h.edition(ctx, r)
		if err != nil {
			return 0, Head{}, nil, nil, err
		}
		blocks[key] = ed
	}
	return seq, head, rows, blocks, nil
}

// Holds reports whether the workspace home keeps any block of edition k of
// the document ref names, as its projection stands.
func (h *Home) Holds(ctx context.Context, ref string, k model.EditionKey) (bool, error) {
	return h.Store.Held(ctx, h.docKey(ref), editionText(k))
}

// EditionOf is the edition a row keeps, its runs read from the blob that
// holds them where the row keeps none inline.
func (h *Home) EditionOf(ctx context.Context, r Row) (model.Edition, error) { return h.edition(ctx, r) }

// edition is the edition a row keeps.
func (h *Home) edition(ctx context.Context, r Row) (model.Edition, error) {
	data := r.Runs
	if data == nil && r.Blob != "" {
		var err error
		if data, err = h.Log.Blob(ctx, r.Blob); err != nil {
			return model.Edition{}, fmt.Errorf("workhome: read the runs of %s in %s: %w", r.Block, Subject(r.Doc, r.Edition), err)
		}
	}
	var runs []model.Run
	if len(data) > 0 {
		if err := json.Unmarshal(data, &runs); err != nil {
			return model.Edition{}, fmt.Errorf("workhome: read the runs of %s in %s: %w", r.Block, Subject(r.Doc, r.Edition), err)
		}
	}
	if h.Redaction != nil {
		runs = h.Redaction.Restore(ctx, r, runs)
	}
	return model.Edition{Runs: runs, Status: r.Status, Origin: r.Origin}, nil
}

// Edition reads what the workspace home keeps of edition k of the document
// ref names.
func (h *Home) Edition(ctx context.Context, ref string, k model.EditionKey) (filehome.Kept, error) {
	seq, head, rows, blocks, err := h.read(ctx, h.docKey(ref), editionText(k))
	if err != nil {
		return filehome.Kept{}, err
	}
	return filehome.Kept{Token: token(seq, head), Digest: Digest(rows), Blocks: blocks}, nil
}

// Rows reads every block of edition k of the document ref names, as the
// workspace home keeps it, with the projection caught up first.
func (h *Home) Rows(ctx context.Context, ref string, k model.EditionKey) (map[string]Row, map[string]model.Edition, error) {
	_, _, rows, blocks, err := h.read(ctx, h.docKey(ref), editionText(k))
	return rows, blocks, err
}

// Held is one edition the workspace home keeps, as one read found it.
type Held struct {
	// Token names the head the read found: a release of what the read
	// found expects it (Release.Token).
	Token string
	// Rows and Blocks are each block's row and edition, by block key.
	Rows   map[string]Row
	Blocks map[string]model.Edition
}

// Read reads edition k of the document ref names, with the head it found.
func (h *Home) Read(ctx context.Context, ref string, k model.EditionKey) (Held, error) {
	seq, head, rows, blocks, err := h.read(ctx, h.docKey(ref), editionText(k))
	if err != nil {
		return Held{}, err
	}
	return Held{Token: token(seq, head), Rows: rows, Blocks: blocks}, nil
}

// record redacts c under the project's policy and records it.
func (h *Home) record(ctx context.Context, c Commit) (string, error) {
	if h.Redaction != nil {
		if err := h.Redaction.Redact(ctx, &c); err != nil {
			return "", err
		}
	}
	return h.Log.CommitWorkspace(ctx, c)
}

// Commit stores the change a file home staged on an edition the workspace
// home keeps, as the record the change service handed the stage.
func (h *Home) Commit(ctx context.Context, w filehome.KeptWrite) (string, error) {
	if w.Record == nil {
		return "", errors.New("workhome: a write with no record: the workspace home keeps an edition by recording each change to it")
	}
	seq, base, err := parseToken(w.Token)
	if err != nil {
		return "", err
	}
	rec := w.Record
	c := Commit{
		Doc: h.docKey(w.Doc), Path: w.Doc, Edition: editionText(w.Edition),
		Expect: seq, Base: base,
		Actor: rec.Actor, Origin: rec.Origin, Fingerprint: rec.Fingerprint, Set: rec.Set,
		DocBefore: w.Before, DocAfter: w.After,
	}
	if rec.Set != nil {
		c.Note = rec.Set.Note
	}
	for _, f := range rec.Overridden {
		if f.At == nil || f.At.Doc == w.Doc {
			c.Overridden = append(c.Overridden, f)
		}
	}
	byBlock := map[string]change.Transition{}
	for _, t := range rec.Transitions {
		if t.Ref.Edition.Canonical() == w.Edition.Canonical() {
			byBlock[t.Ref.Block] = t
		}
	}
	keep := rec.Actor.Kind == change.ActorPerson || rec.Actor.Kind == change.ActorAgent
	for _, ch := range w.Changes {
		b := CommitBlock{Block: ch.Block, Before: ch.Before, After: model.AbsentRevision, Edition: ch.Edition}
		if ch.Edition != nil {
			b.After = model.RunsRevision(w.Edition, ch.Edition.Runs)
		}
		if t, ok := byBlock[ch.Block]; ok {
			b.Basis, b.Key, b.ContentHash, b.ContextHash = t.Basis, t.Key, t.ContentHash, t.ContextHash
			b.Ops, b.Tool = slices.Clone(t.Ops), t.Tool
			if blk := t.Block; blk != nil {
				if b.Key == "" {
					b.Key = blk.Unit
				}
				if b.ContentHash == "" || b.ContextHash == "" {
					id := model.ComputeIdentity(blk)
					b.ContentHash = cmp.Or(b.ContentHash, id.ContentHash)
					b.ContextHash = cmp.Or(b.ContextHash, id.ContextHash)
				}
			}
			if keep {
				b.BeforeRuns = t.Before
			}
		}
		c.Blocks = append(c.Blocks, b)
	}
	id, err := h.record(ctx, c)
	if errors.Is(err, workspace.ErrHeadMoved) {
		return "", &change.Error{Code: change.CodeDocChanged,
			Message: fmt.Sprintf("edition %s of %s changed in the workspace while the edit was committed; read it and send the change again", c.Edition, w.Doc)}
	}
	return id, err
}

// Produce is a flow's drafts of one edition, written into the workspace home.
type Produce struct {
	// Doc is the document's reference, and Edition the edition drafted.
	Doc     string
	Edition model.EditionKey
	Blocks  []Produced
	// Actor and Origin are the flow's: the tool and flow:<name>.
	Actor  change.Actor
	Origin string
}

// Produced is one block's draft.
type Produced struct {
	Block string
	// Before is the revision the producer read the edition at
	// (model.AbsentRevision for none): the draft lands only while the
	// workspace home still holds it.
	Before  string
	Edition model.Edition
	Basis   string
	Stamp   json.RawMessage
	// Key is the block's durable key where reconciliation assigned one, and
	// ContentHash and ContextHash are its identity signals.
	Key         string
	ContentHash string
	ContextHash string
	// Tool is the tool in the flow that drafted the edition, where the flow
	// can name one.
	Tool string
}

// ProduceResult says what a Produce wrote.
type ProduceResult struct {
	// Record is the id of the operation that recorded the write, empty when
	// nothing changed.
	Record string
	// Written counts the blocks written; Unchanged the drafts the workspace
	// home already held; Moved the blocks another writer changed since the
	// producer read them, which keep what that writer left; and Kept the
	// blocks whose wording a person or an agent wrote from the source the
	// draft was made from, which keep that wording.
	Written   int
	Unchanged int
	Moved     int
	Kept      int
}

// Produce writes a flow's drafts of one edition into the workspace home, one
// operation for the edition. A block another writer changed since the flow
// read it keeps that writer's edition, and so does a block a person or an
// agent wrote from the same source the draft was made from: the flow's
// output replaces a tool's draft or nothing.
func (h *Home) Produce(ctx context.Context, p Produce) (ProduceResult, error) {
	doc, edition := h.docKey(p.Doc), editionText(p.Edition)
	for attempt := 0; ; attempt++ {
		seq, head, rows, eds, err := h.read(ctx, doc, edition)
		if err != nil {
			return ProduceResult{}, err
		}
		var res ProduceResult
		c := Commit{Doc: doc, Path: p.Doc, Edition: edition, Expect: seq, Base: head.Op,
			Actor: p.Actor, Origin: p.Origin, DocBefore: Digest(rows)}
		after := maps.Clone(rows)
		for _, d := range p.Blocks {
			now := model.AbsentRevision
			held, ok := rows[d.Block]
			if ok {
				now = held.Rev
			}
			if now != d.Before {
				res.Moved++
				continue
			}
			rev := model.RunsRevision(p.Edition, d.Edition.Runs)
			if ok && byWriter(held.Origin) && held.Basis != "" && held.Basis == d.Basis {
				res.Kept++
				continue
			}
			if ok && rev == held.Rev && held.Status == d.Edition.Status && held.Origin == d.Edition.Origin &&
				held.Basis == d.Basis && string(held.Stamp) == string(d.Stamp) {
				res.Unchanged++
				continue
			}
			ed := d.Edition
			c.Blocks = append(c.Blocks, CommitBlock{Block: d.Block, Before: now, After: rev, Edition: &ed,
				Basis: d.Basis, Stamp: d.Stamp, Key: d.Key, ContentHash: d.ContentHash, ContextHash: d.ContextHash,
				Ops: []change.Kind{draftKind(eds[d.Block].Runs, ok, ed.Runs)}, Tool: d.Tool})
			after[d.Block] = Row{Rev: rev, Status: ed.Status, Origin: ed.Origin}
			res.Written++
		}
		if len(c.Blocks) == 0 {
			return res, nil
		}
		c.DocAfter = Digest(after)
		id, err := h.record(ctx, c)
		if errors.Is(err, workspace.ErrHeadMoved) && attempt < 2 {
			continue
		}
		if err != nil {
			return ProduceResult{}, err
		}
		res.Record = id
		return res, nil
	}
}

// draftKind is the kind of the operation a draft comes to, as change.Diff
// would send it: set_content for an edition created or rewritten, and
// replace_text for one whose text moved around the same codes. A draft that
// gives a held edition its own runs again (a new stamp or status) is set
// again.
func draftKind(was []model.Run, had bool, now []model.Run) change.Kind {
	if k, ok := change.EditionKind(was, had, now, true); ok {
		return k
	}
	return change.KindSetContent
}

// byWriter reports whether an edition's origin is a person's or an agent's.
func byWriter(o model.Origin) bool {
	return o.Kind == model.OriginHuman || o.Kind == model.OriginAgent
}

// Release is what a delivery releases from the workspace home once an
// edition's file holds it.
type Release struct {
	// Token is Held.Token as the delivery read the edition: the release lands
	// only while the edition's head is still the one the delivery wrote out,
	// so a write that lands in between is never released unread.
	Token string
	// Blocks are the blocks to release; nil releases every block.
	Blocks []string
	// Actor and Origin are the delivery's: the tool and the surface.
	Actor  change.Actor
	Origin string
}

// ErrReleaseMoved is what Release returns when the edition's head moved
// since the delivery read it. It matches workspace.ErrHeadMoved.
var ErrReleaseMoved = fmt.Errorf("workhome: the edition changed in the workspace after the delivery read it: %w", workspace.ErrHeadMoved)

// Release removes the blocks r names of edition k of the document ref names
// from the workspace home, recorded as r.Actor's write through r.Origin: what
// a delivery does once the edition's file holds them, so the workspace never
// keeps a second copy of what a file holds. A release of every block the
// edition holds settles every write a merge left divergent on it. It returns
// the id of the operation, empty when there was nothing to release, and
// ErrReleaseMoved when the head is no longer at r.Token.
func (h *Home) Release(ctx context.Context, ref string, k model.EditionKey, r Release) (string, error) {
	doc, edition := h.docKey(ref), editionText(k)
	seq, head, rows, _, err := h.read(ctx, doc, edition)
	if err != nil {
		return "", err
	}
	if token(seq, head) != r.Token {
		return "", ErrReleaseMoved
	}
	keys := r.Blocks
	if keys == nil {
		keys = slices.Sorted(maps.Keys(rows))
	}
	c := Commit{Doc: doc, Path: ref, Edition: edition, Expect: seq, Base: head.Op,
		Actor: r.Actor, Origin: r.Origin, DocBefore: Digest(rows)}
	after := maps.Clone(rows)
	for _, key := range keys {
		row, ok := rows[key]
		if !ok {
			continue
		}
		c.Blocks = append(c.Blocks, CommitBlock{Block: key, Before: row.Rev, After: model.AbsentRevision, Basis: row.Basis,
			Ops: []change.Kind{change.KindRemoveEdition}})
		delete(after, key)
	}
	if len(c.Blocks) == 0 {
		return "", nil
	}
	c.Release = len(after) == 0
	c.DocAfter = Digest(after)
	id, err := h.record(ctx, c)
	if errors.Is(err, workspace.ErrHeadMoved) {
		return "", ErrReleaseMoved
	}
	return id, err
}

// Conflict is a write to an edition the workspace home keeps that did not
// advance its head and that the rebase cannot carry over, because a block it
// changed has moved since: what two machines that wrote one edition from one
// head leave, once their logs merge.
type Conflict struct {
	Doc     string
	Path    string
	Edition string
	// Op is the write that did not land, and Blocks the blocks it changed
	// that have moved since.
	Op     string
	Blocks []string
}

// Conflicts lists the writes that did not advance an edition's head, that
// the head does not already hold, and that the rebase cannot carry over, in
// document, edition and id order. A release that did not land is not one:
// the delivery that made it handles what the head still holds.
func (h *Home) Conflicts(ctx context.Context) ([]Conflict, error) {
	if err := h.Log.CatchUp(ctx); err != nil {
		return nil, err
	}
	heads, err := h.Store.Heads(ctx)
	if err != nil {
		return nil, err
	}
	var out []Conflict
	for _, head := range heads {
		if len(head.Divergent) == 0 {
			continue
		}
		rows, err := h.Store.Rows(ctx, head.Doc, head.Edition)
		if err != nil {
			return nil, err
		}
		for _, d := range head.Divergent {
			if d.Release || Settled(d, rows) || Rebaseable(d, rows) {
				continue
			}
			c := Conflict{Doc: head.Doc, Path: head.Path, Edition: head.Edition, Op: d.Op}
			for _, m := range d.Blocks {
				if revOf(rows, m.Block) != m.Before {
					c.Blocks = append(c.Blocks, m.Block)
				}
			}
			out = append(out, c)
		}
	}
	return out, nil
}

// revOf is the revision rows hold for a block, model.AbsentRevision for one
// they do not hold.
func revOf(rows map[string]Row, block string) string {
	if r, ok := rows[block]; ok {
		return r.Rev
	}
	return model.AbsentRevision
}

// Settled reports whether a divergent write left nothing to carry over: each
// block it changed already holds what it wrote.
func Settled(d Divergence, rows map[string]Row) bool {
	for _, m := range d.Blocks {
		if revOf(rows, m.Block) != m.After {
			return false
		}
	}
	return true
}

// Rebaseable reports whether a divergent write can be carried over onto the
// head: each block it changed still holds the revision the write started
// from.
func Rebaseable(d Divergence, rows map[string]Row) bool {
	for _, m := range d.Blocks {
		if revOf(rows, m.Block) != m.Before {
			return false
		}
	}
	return true
}
