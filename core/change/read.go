package change

import (
	"context"
	"encoding/base64"
	"errors"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// ReadRequest names what to read: a document, optionally only some of its
// blocks and the editions that live in files of their own, a page at a time.
type ReadRequest struct {
	Doc string `json:"doc"`
	// Blocks names blocks by key; empty reads every block.
	Blocks []string `json:"blocks,omitempty"`
	// Editions names the editions to show that live in files of their own.
	// The editions a document holds itself are always shown.
	Editions []model.EditionKey `json:"editions,omitempty"`
	// Cursor continues a read where the page that returned it ended.
	Cursor string `json:"cursor,omitempty"`
	// Limit is the most blocks a page holds; zero is DefaultReadLimit.
	Limit int `json:"limit,omitempty"`
	// OwnEdition names the document's own edition in each block's ref, by
	// the language the document is written in, where a ref would otherwise
	// leave the edition out. A sender copies it into insert_block's
	// editions, and the service takes it as the document's own edition. A
	// surface sets it when it knows the document's language, as a project's
	// recipe gives it.
	OwnEdition bool `json:"own_edition,omitempty"`
}

// DefaultReadLimit and MaxReadLimit bound a page.
const (
	DefaultReadLimit = 100
	MaxReadLimit     = 1000
)

// Page is one page of a document's blocks.
type Page struct {
	Doc    string `json:"doc"`
	Home   string `json:"home"`
	Format string `json:"format"`
	// Head is the digest of the document the page was read from.
	Head   string      `json:"head"`
	Blocks []BlockRead `json:"blocks"`
	// Next continues the read; empty on the last page.
	Next string `json:"next,omitempty"`
}

// BlockRead is a block as a read shows it: the reference to copy into an
// operation, the revision to send as if_match, the content in placeholder
// text, the inline codes and structures it holds, its other editions, and the
// operations it accepts.
type BlockRead struct {
	Ref  Ref    `json:"ref"`
	Rev  string `json:"rev"`
	Text string `json:"text"`
	// Codes maps the id a code shows in placeholder text (1, /1 is its
	// closing half, 1/ a placeholder, sub:1 a subblock) to what it is.
	Codes map[string]CodeRead `json:"codes,omitempty"`
	// Structures lists the plurals and selects, each with the path an
	// operation names to reach one of its branches.
	Structures []StructureRead        `json:"structures,omitempty"`
	Editions   map[string]EditionRead `json:"editions,omitempty"`
	Ops        []Kind                 `json:"ops"`
}

// CodeRead is an inline code as a read shows it.
type CodeRead struct {
	Kind  string            `json:"kind"`
	Type  string            `json:"type,omitempty"`
	Attrs map[string]string `json:"attrs,omitempty"`
	// Equiv is the code's equivalent text, such as the name of the variable a
	// placeholder stands for, and Disp the short label an editor shows on it.
	// Both are labels: a read leaves out either one that repeats the code's
	// native form (an ICU argument, a printf specifier, a tag), which no read
	// shows.
	Equiv string `json:"equiv,omitempty"`
	Disp  string `json:"disp,omitempty"`
	// Writable are the attributes set_attribute can change on the code.
	Writable []string `json:"writable,omitempty"`
}

// StructureRead is a plural or a select.
type StructureRead struct {
	Path  model.RunPath `json:"path"`
	Kind  string        `json:"kind"`
	Pivot string        `json:"pivot,omitempty"`
	// Branches maps each plural form or select case to its text.
	Branches map[string]string `json:"branches"`
}

// EditionRead is an edition other than the document's own.
type EditionRead struct {
	Rev  string `json:"rev"`
	Text string `json:"text"`
	// Codes lists the edition's inline codes where they differ from the
	// block's; absent, the block's codes are the edition's.
	Codes map[string]CodeRead `json:"codes,omitempty"`
	// Structures lists the edition's own plurals and selects, each with the
	// path an operation on the edition names to reach one of its branches.
	Structures []StructureRead `json:"structures,omitempty"`
	// Status is where the edition stands: draft, translated or established
	// for a translation, written or established for the document's own
	// edition, new for an edition with no recorded status, and untranslated
	// for one with no recorded status whose text is the authoritative
	// edition's, as a file the source filled holds it.
	Status string `json:"status"`
	// Basis is the authoritative edition's revision the edition was made
	// from, where the host keeps it.
	Basis string `json:"basis,omitempty"`
	// Stale says the authoritative edition has moved since the basis.
	Stale bool `json:"stale"`
}

// Read reads a page of a document's blocks.
func (s *Service) Read(ctx context.Context, q ReadRequest) (*Page, error) {
	return s.read(ctx, q, nil)
}

// ReadEach reads every block of a document in one pass and hands each to fn
// as a read shows it, beside the block it was read from. It is the read a
// command line streams: the document is read once and no page is held, so
// Limit and Cursor are not used. fn returning ErrStop ends the read early.
// The page it returns names the document, its home, format and head, and
// holds no blocks.
func (s *Service) ReadEach(ctx context.Context, q ReadRequest, fn func(b *model.Block, r BlockRead) error) (*Page, error) {
	q.Cursor = ""
	return s.read(ctx, q, fn)
}

// read reads q's document: a page of its blocks, or with each set, every
// block handed to each.
func (s *Service) read(ctx context.Context, q ReadRequest, each func(b *model.Block, r BlockRead) error) (*Page, error) {
	if q.Doc == "" {
		return nil, &Error{Code: CodeInvalid, Field: "doc", Message: "name the document to read"}
	}
	limit := q.Limit
	switch {
	case limit <= 0:
		limit = DefaultReadLimit
	case limit > MaxReadLimit:
		limit = MaxReadLimit
	}
	wantHead, skip, err := decodeCursor(q.Cursor)
	if err != nil {
		return nil, err
	}
	h, err := s.homeFor(q.Doc)
	if err != nil {
		return nil, err
	}
	sess, err := h.Open(ctx, q.Doc)
	if err != nil {
		if e := asError(err); e != nil {
			return nil, e
		}
		return nil, err
	}
	defer sess.Close()
	info := sess.Info()
	desc := s.describe(sess)

	editions := slices.Clone(q.Editions)
	if info.Edition != nil {
		editions = append(editions, *info.Edition)
	}
	var want Want
	for _, k := range editions {
		if sess.Place(k).Kind == PlaceOwnFile {
			want.Editions = append(want.Editions, k.Canonical())
		}
	}
	blocks := slices.Clone(q.Blocks)
	if resolver, ok := sess.(EditionKeyResolver); ok && info.Edition != nil && sess.Place(*info.Edition).Kind == PlaceOwnFile {
		// A block named by its key in the edition's own file, as a person who
		// opened that file reads it.
		for i, key := range blocks {
			if k, found, rerr := resolver.DocumentBlockKey(ctx, *info.Edition, key); rerr == nil && found {
				blocks[i] = k
			}
		}
	}
	want.Blocks = blocks

	var states DocumentStates
	if s.states != nil {
		shows := 0
		if each == nil {
			shows = limit
		}
		if len(blocks) > 0 && (shows == 0 || len(blocks) < shows) {
			shows = len(blocks)
		}
		states = s.states.Document(ctx, info, shows)
	}
	page := &Page{Doc: info.Doc, Home: h.Name(), Format: info.Format, Blocks: []BlockRead{}}
	if info.Edition != nil {
		// A read of the file one edition lives in reports the home that
		// keeps that edition.
		if pl := sess.Place(*info.Edition); pl.Home != "" {
			page.Home = pl.Home
		}
	}
	index := 0
	more := false
	// The observer sees each block the read shows, whatever page it falls
	// on, and records once the read ends.
	obs := s.observe(ctx, info)
	if obs != nil {
		defer obs.Done(ctx)
	}
	head, err := sess.Read(ctx, want, func(b *model.Block) error {
		if len(blocks) > 0 && !slices.ContainsFunc([]string{b.Unit, b.Name, b.ID}, func(k string) bool {
			return k != "" && slices.Contains(blocks, k)
		}) {
			return nil
		}
		if obs != nil {
			obs.Saw(b, coveredEditions(b, editions))
		}
		if each != nil {
			return each(b, readBlock(info, states, desc, b, editions, q.OwnEdition))
		}
		i := index
		index++
		if i < skip {
			return nil
		}
		if len(page.Blocks) == limit {
			more = true
			return ErrStop
		}
		page.Blocks = append(page.Blocks, readBlock(info, states, desc, b, editions, q.OwnEdition))
		return nil
	})
	if err != nil && !errors.Is(err, ErrStop) {
		if e := asError(err); e != nil {
			return nil, e
		}
		return nil, err
	}
	if wantHead != "" && wantHead != head {
		return nil, &Error{Code: CodeStale, Field: "cursor",
			Message: info.Doc + " changed since the page the cursor came from; read it again from the start"}
	}
	page.Head = head
	if more {
		page.Next = encodeCursor(head, skip+len(page.Blocks))
	}
	return page, nil
}

// homeFor picks the home of doc.
func (s *Service) homeFor(doc string) (Home, error) {
	if s.homes == nil {
		return nil, &Error{Code: CodeUnreachable, Message: "the change service has no home for documents"}
	}
	return s.homes.For(doc)
}

// readBlock is b as a read shows it. The block's reference, revision and text
// are those of the edition the read was opened on: the document's own, or,
// for a read of the file one edition lives in, that edition, so a person who
// opened the German file and copies a reference edits the German. Every other
// edition the block holds is listed among its editions, the document's own
// included, with the status and basis states gives it (nil gives none).
func readBlock(info DocInfo, states DocumentStates, desc Description, b *model.Block, editions []model.EditionKey, ownEdition bool) BlockRead {
	var primary model.EditionKey
	if info.Edition != nil {
		primary = info.Edition.Canonical()
	}
	ed, _ := b.Edition(primary)
	ref := Ref{Doc: info.Doc, Block: BlockKey(b)}
	switch {
	case !b.IsSourceEdition(primary):
		ref.Edition = primary
	case ownEdition:
		// The document's own edition, by its language.
		ref.Edition = b.EditionKeyOf(primary)
	}
	out := BlockRead{
		Ref:  ref,
		Rev:  model.EditionRevision(b, primary),
		Text: model.RunsEditText(ed.Runs),
		Ops:  desc.blockOps(b.Translatable),
	}
	if !runsUTF8(ed.Runs) {
		// The edition holds bytes that are not UTF-8, which an operation that
		// rebuilds its text refuses (workset.utf8Text).
		out.Ops = slices.DeleteFunc(out.Ops, rebuildsText)
	}
	out.Codes = codesOf(ed.Runs, desc)
	out.Structures = structuresOf(ed.Runs)
	authKey := b.Authoritative(model.AuthorityPolicy{})
	authRev := model.EditionRevision(b, authKey)
	authEd, _ := b.Edition(authKey)
	authText := model.RunsEditText(authEd.Runs)
	primaryKey := b.EditionKeyOf(primary)
	for _, k := range b.Editions() {
		if b.EditionKeyOf(k) == primaryKey {
			continue
		}
		if b.IsSourceEdition(k) {
			own, _ := b.Edition(k)
			if out.Editions == nil {
				out.Editions = map[string]EditionRead{}
			}
			er := out.editionRead(model.EditionRevision(b, k), own, desc)
			if er.Status == "" {
				er.Status = StatusNew
			}
			out.Editions[keyText(b.EditionKeyOf(k))] = er
			continue
		}
		ed, _ := b.Edition(k)
		er := out.editionRead(model.EditionRevision(b, k), ed, desc)
		if states != nil {
			if st, ok := states.EditionState(b, k); ok {
				if st.Status != "" {
					er.Status = string(st.Status)
				}
				er.Basis = st.Basis
				er.Stale = st.Basis != "" && st.Basis != authRev
			}
		}
		if er.Status == "" {
			// No status is recorded: the edition is new, or holds the
			// authoritative edition's text, as a file the source filled
			// holds it until someone translates it.
			er.Status = StatusNew
			if er.Text == authText {
				er.Status = StatusUntranslated
			}
		}
		if out.Editions == nil {
			out.Editions = map[string]EditionRead{}
		}
		out.Editions[keyText(k)] = er
	}
	if info.Editions == EditionsInFile && len(out.Editions) == 0 {
		// A file that keeps its translations in it lists every one it holds,
		// so a block that holds none has no edition to remove.
		out.Ops = slices.DeleteFunc(out.Ops, func(k Kind) bool { return k == KindRemoveEdition })
	}
	return out
}

// The statuses a read gives an edition that has none recorded.
const (
	// StatusNew is an edition with no recorded status.
	StatusNew = "new"
	// StatusUntranslated is an edition with no recorded status that holds
	// the authoritative edition's text.
	StatusUntranslated = "untranslated"
)

// editionRead is ed, another edition of the block out reads, at revision rev:
// its text and status, its plurals and selects, and its codes where they
// differ from the block's.
func (out BlockRead) editionRead(rev string, ed model.Edition, desc Description) EditionRead {
	er := EditionRead{Rev: rev, Text: model.RunsEditText(ed.Runs), Status: string(ed.Status), Structures: structuresOf(ed.Runs)}
	if codes := codesOf(ed.Runs, desc); !reflect.DeepEqual(codes, out.Codes) {
		er.Codes = codes
	}
	return er
}

// codesOf lists the inline codes of runs, branches of plurals and selects
// included, keyed by the id each shows in placeholder text. Every kind of run
// is answered: text carries no code, and a plural or select is listed among
// the structures rather than here.
func codesOf(runs []model.Run, desc Description) map[string]CodeRead {
	out := map[string]CodeRead{}
	declared := format.EditCapabilities{WritableAttrs: desc.Ops.SetAttribute}
	// writable lists the attributes set_attribute may change on a code: what
	// the format declares for its type, and, where it declares every
	// attribute (format.AnyAttr), each one the code holds.
	writable := func(typ string, attrs map[string]string) []string {
		w := declared.Writable(typ)
		if !slices.Contains(w, format.AnyAttr) {
			return w
		}
		w = slices.DeleteFunc(w, func(n string) bool { return n == format.AnyAttr })
		for n := range attrs {
			if !slices.Contains(w, n) {
				w = append(w, n)
			}
		}
		slices.Sort(w)
		return w
	}
	var walk func([]model.Run)
	walk = func(rs []model.Run) {
		for _, r := range rs {
			switch {
			case r.Text != nil, r.PcClose != nil:
			case r.PcOpen != nil:
				out[r.PcOpen.ID] = CodeRead{Kind: "paired", Type: r.PcOpen.Type, Attrs: r.PcOpen.Attrs,
					Equiv: label(r.PcOpen.Equiv, r.PcOpen.Data), Disp: label(r.PcOpen.Disp, r.PcOpen.Data),
					Writable: writable(r.PcOpen.Type, r.PcOpen.Attrs)}
			case r.Ph != nil:
				out[r.Ph.ID+"/"] = CodeRead{Kind: "placeholder", Type: r.Ph.Type, Attrs: r.Ph.Attrs,
					Equiv: label(r.Ph.Equiv, r.Ph.Data), Disp: label(r.Ph.Disp, r.Ph.Data),
					Writable: writable(r.Ph.Type, r.Ph.Attrs)}
			case r.Sub != nil:
				out["sub:"+r.Sub.ID] = CodeRead{Kind: "subblock", Attrs: map[string]string{"ref": r.Sub.Ref}}
			case r.Plural != nil:
				for _, form := range r.Plural.Forms {
					walk(form)
				}
			case r.Select != nil:
				for _, c := range r.Select.Cases {
					walk(c)
				}
			}
		}
	}
	walk(runs)
	if len(out) == 0 {
		return nil
	}
	return out
}

// label is a code's equiv or disp as a read shows it: s, unless s repeats
// data, the code's native form, which no read shows.
func label(s, data string) string {
	if s == data {
		return ""
	}
	return s
}

// structuresOf lists the plurals and selects of runs, each with the path that
// reaches it. A structure inside a branch is listed after the one that holds
// it, with the path through that branch, so every branch is reachable from a
// read.
func structuresOf(runs []model.Run) []StructureRead {
	var out []StructureRead
	var walk func(seq []model.Run, prefix model.RunPath)
	walk = func(seq []model.Run, prefix model.RunPath) {
		for i, r := range seq {
			path := append(slices.Clone(prefix), model.RunPathStep{Kind: model.StepIndex, Index: i})
			switch {
			case r.Plural != nil:
				st := StructureRead{Path: path, Kind: "plural", Pivot: r.Plural.Pivot, Branches: map[string]string{}}
				for form, rs := range r.Plural.Forms {
					st.Branches[string(form)] = model.RunsEditText(rs)
				}
				out = append(out, st)
				for _, form := range sortedKeys(pluralNames(r.Plural.Forms)) {
					walk(r.Plural.Forms[model.PluralForm(form)],
						append(slices.Clone(path), model.RunPathStep{Kind: model.StepPlural, PluralForm: model.PluralForm(form)}))
				}
			case r.Select != nil:
				st := StructureRead{Path: path, Kind: "select", Pivot: r.Select.Pivot, Branches: map[string]string{}}
				for c, rs := range r.Select.Cases {
					st.Branches[c] = model.RunsEditText(rs)
				}
				out = append(out, st)
				for _, c := range sortedKeys(r.Select.Cases) {
					walk(r.Select.Cases[c], append(slices.Clone(path), model.RunPathStep{Kind: model.StepSelect, SelectValue: c}))
				}
			}
		}
	}
	walk(runs, nil)
	return out
}

// cursorPrefix versions the cursor's encoding.
const cursorPrefix = "c1:"

// encodeCursor is an opaque cursor: the head a page was read from and the
// number of blocks read so far.
func encodeCursor(head string, next int) string {
	return base64.RawURLEncoding.EncodeToString([]byte(cursorPrefix + head + "|" + strconv.Itoa(next)))
}

// decodeCursor reads a cursor encodeCursor wrote; the empty cursor starts at
// the beginning.
func decodeCursor(c string) (head string, next int, err error) {
	if c == "" {
		return "", 0, nil
	}
	bad := &Error{Code: CodeInvalid, Field: "cursor", Message: "the cursor is not one a read returned"}
	raw, derr := base64.RawURLEncoding.DecodeString(c)
	if derr != nil {
		return "", 0, bad
	}
	body, ok := strings.CutPrefix(string(raw), cursorPrefix)
	if !ok {
		return "", 0, bad
	}
	head, count, found := strings.CutLast(body, "|")
	if !found {
		return "", 0, bad
	}
	n, aerr := strconv.Atoi(count)
	if aerr != nil || n < 0 {
		return "", 0, bad
	}
	return head, n, nil
}
