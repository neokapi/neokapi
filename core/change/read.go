package change

import (
	"context"
	"encoding/base64"
	"errors"
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
	Rev    string `json:"rev"`
	Text   string `json:"text"`
	Status string `json:"status,omitempty"`
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
	desc := s.describe(info)

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

	page := &Page{Doc: info.Doc, Home: h.Name(), Format: info.Format, Blocks: []BlockRead{}}
	index := 0
	more := false
	head, err := sess.Read(ctx, want, func(b *model.Block) error {
		if len(blocks) > 0 && !slices.ContainsFunc([]string{b.Unit, b.Name, b.ID}, func(k string) bool {
			return k != "" && slices.Contains(blocks, k)
		}) {
			return nil
		}
		if each != nil {
			return each(b, s.readBlock(ctx, info, desc, b, editions))
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
		page.Blocks = append(page.Blocks, s.readBlock(ctx, info, desc, b, editions))
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
// included.
func (s *Service) readBlock(ctx context.Context, info DocInfo, desc Description, b *model.Block, editions []model.EditionKey) BlockRead {
	var primary model.EditionKey
	if info.Edition != nil {
		primary = info.Edition.Canonical()
	}
	ed, _ := b.Edition(primary)
	ref := Ref{Doc: info.Doc, Block: BlockKey(b)}
	if !b.IsSourceEdition(primary) {
		ref.Edition = primary
	}
	out := BlockRead{
		Ref:  ref,
		Rev:  model.EditionRevision(b, primary),
		Text: model.RunsEditText(ed.Runs),
		Ops:  desc.blockOps(b.Translatable),
	}
	out.Codes = codesOf(ed.Runs, desc)
	out.Structures = structuresOf(ed.Runs)
	authRev := model.EditionRevision(b, b.Authoritative(model.AuthorityPolicy{}))
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
			out.Editions[keyText(b.EditionKeyOf(k))] = EditionRead{Rev: model.EditionRevision(b, k), Text: model.RunsEditText(own.Runs), Status: string(own.Status)}
			continue
		}
		ed, _ := b.Edition(k)
		er := EditionRead{Rev: model.EditionRevision(b, k), Text: model.RunsEditText(ed.Runs), Status: string(ed.Status)}
		if s.states != nil {
			if st, ok := s.states.EditionState(ctx, info, b, k); ok {
				if st.Status != "" {
					er.Status = string(st.Status)
				}
				er.Basis = st.Basis
				er.Stale = st.Basis != "" && st.Basis != authRev
			}
		}
		if out.Editions == nil {
			out.Editions = map[string]EditionRead{}
		}
		out.Editions[keyText(k)] = er
	}
	return out
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
				out[r.PcOpen.ID] = CodeRead{Kind: "paired", Type: r.PcOpen.Type, Attrs: r.PcOpen.Attrs, Writable: writable(r.PcOpen.Type, r.PcOpen.Attrs)}
			case r.Ph != nil:
				out[r.Ph.ID+"/"] = CodeRead{Kind: "placeholder", Type: r.Ph.Type, Attrs: r.Ph.Attrs, Writable: writable(r.Ph.Type, r.Ph.Attrs)}
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

// structuresOf lists the plurals and selects at the top level of runs.
func structuresOf(runs []model.Run) []StructureRead {
	var out []StructureRead
	for i, r := range runs {
		path := model.RunPath{{Kind: model.StepIndex, Index: i}}
		switch {
		case r.Plural != nil:
			st := StructureRead{Path: path, Kind: "plural", Pivot: r.Plural.Pivot, Branches: map[string]string{}}
			for form, rs := range r.Plural.Forms {
				st.Branches[string(form)] = model.RunsEditText(rs)
			}
			out = append(out, st)
		case r.Select != nil:
			st := StructureRead{Path: path, Kind: "select", Pivot: r.Select.Pivot, Branches: map[string]string{}}
			for c, rs := range r.Select.Cases {
				st.Branches[c] = model.RunsEditText(rs)
			}
			out = append(out, st)
		}
	}
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
