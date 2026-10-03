package changes

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"

	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/change/filehome"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/venue"
)

// Home is the change service's stream home: the documents of one stream of
// one project, each an item whose blocks are rows. A row holds every edition
// of its block, so every edition lives in the document.
//
// A stage reads the blocks a change set addresses and applies its operations
// in memory. The commit lock is the rows: Lock holds the rows the stage read
// on the change set's transaction, Settle applies the operations again to the
// held rows when they moved since the stage, so every if_match is checked
// against the rows the write stores, and Commit stores them, with their
// history and change-log rows, and commits the transaction.
//
// Every item of one change set is held and stored on one transaction, which
// the first item's Lock begins and the first Commit commits, storing every
// item: a change set takes one database connection whatever the number of
// items it names, and lands whole or not at all. A Home therefore commits one
// change set at a time; the change sets applied through it run one after
// another.
type Home struct {
	// Store keeps the stream. It must be a store.BlockWriteStore.
	Store store.ContentStore
	// ProjectID and Stream name the stream.
	ProjectID string
	Stream    string
	// SourceLocale is the language the project's content is written in.
	SourceLocale model.LocaleID
	// Locales are the languages the project translates into. An edition in
	// any other language has no home in the stream. Empty admits every
	// language.
	Locales []model.LocaleID
	// Registry binds an item's format to the writer that declares what the
	// format can hold beyond what its reader read (set_attribute, mark, new
	// codes). Nil declares nothing.
	Registry *registry.FormatRegistry
	// Stamp, when set, is called with each block a stage changed, once the
	// operations have applied and before the block is stored.
	Stamp func(b *model.Block)
	// BeforeLock, when set, is called once a document of a change set is
	// staged and before its rows are held.
	BeforeLock func(doc string)
	// Committed, when set, is called with the row ids of each item a commit
	// wrote, once the write is committed.
	Committed func(doc string, ids []string)

	// mu guards w, the write of the change set whose rows are held.
	mu sync.Mutex
	w  *streamWrite
}

// streamWrite is the one transaction a change set's items are held and stored
// on.
type streamWrite struct {
	bw store.BlockWrite
	// members are the items held on the write, in the order they were held.
	members []*staged
	// done says the write was committed or discarded, and err why it did not
	// commit.
	done bool
	err  error
}

// errWriteDiscarded is what an item of a change set whose write was discarded
// answers a commit with.
var errWriteDiscarded = errors.New("the change set's write was discarded")

// write returns the write of the change set being committed, beginning it on
// the first item's lock.
func (h *Home) write(ctx context.Context, ws store.BlockWriteStore) (*streamWrite, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.w != nil && !h.w.done {
		return h.w, nil
	}
	bw, err := ws.BeginBlockWrite(ctx, h.ProjectID, h.stream())
	if err != nil {
		return nil, err
	}
	h.w = &streamWrite{bw: bw}
	return h.w, nil
}

// commit stores every item held on w and commits it, once: the first item's
// Commit commits the change set, and each later one answers with its outcome.
func (h *Home) commit(ctx context.Context, w *streamWrite) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if w.done {
		return w.err
	}
	w.done = true
	if h.w == w {
		h.w = nil
	}
	for _, m := range w.members {
		if len(m.changed) == 0 {
			continue
		}
		if err := w.bw.Store(ctx, m.changed); err != nil {
			_ = w.bw.Rollback()
			w.err = err
			return err
		}
	}
	if err := w.bw.Commit(); err != nil {
		w.err = err
		return err
	}
	return nil
}

// discard rolls w back unless it was committed. cause is what the items held
// on it answer a later commit with.
func (h *Home) discard(w *streamWrite, cause error) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if w.done {
		return nil
	}
	w.done, w.err = true, cause
	if h.w == w {
		h.w = nil
	}
	return w.bw.Rollback()
}

var _ change.Home = (*Home)(nil)

// Name is the home as a result reports it.
func (h *Home) Name() string { return "stream:" + h.stream() }

func (h *Home) stream() string {
	if h.Stream == "" {
		return "main"
	}
	return h.Stream
}

// Open starts work on the item doc names.
func (h *Home) Open(ctx context.Context, doc string) (change.Session, error) {
	ws, ok := h.Store.(store.BlockWriteStore)
	if !ok {
		return nil, &change.Error{Code: change.CodeUnsupported, Capability: "stream",
			Message: fmt.Sprintf("the content store %T keeps no held writes, so the stream takes no change set", h.Store)}
	}
	if doc == "" {
		return nil, &change.Error{Code: change.CodeInvalid, Field: "at/doc", Message: "the reference names no item"}
	}
	item, err := h.Store.GetItem(ctx, h.ProjectID, h.stream(), doc)
	switch {
	case errors.Is(err, sql.ErrNoRows) || (err == nil && item == nil):
		return nil, &change.Error{Code: change.CodeNotFound, Field: "at/doc",
			Message: fmt.Sprintf("stream %s holds no item %s", h.stream(), doc)}
	case err != nil:
		return nil, fmt.Errorf("read item %s: %w", doc, err)
	}
	info := change.DocInfo{
		Doc:          item.Name,
		Format:       item.Format,
		SourceLocale: h.SourceLocale,
		Editions:     change.EditionsInFile,
		Capabilities: h.capabilities(item.Format),
	}
	return &session{h: h, ws: ws, item: item.Name, info: info}, nil
}

// capabilities is what the writer of format can write beyond what its reader
// read, as the export of an item writes it.
func (h *Home) capabilities(format string) change.Capabilities {
	if h.Registry == nil || format == "" || h.Registry.FormatInfo(registry.FormatID(format)) == nil {
		return change.Capabilities{Format: format}
	}
	return filehome.RegistryBinding(h.Registry, format, "").Capabilities()
}

// admits reports whether the stream keeps editions in locale.
func (h *Home) admits(locale model.LocaleID) bool {
	if len(h.Locales) == 0 || model.NormalizeLocale(locale) == model.NormalizeLocale(h.SourceLocale) {
		return true
	}
	return slices.ContainsFunc(h.Locales, func(l model.LocaleID) bool {
		return model.NormalizeLocale(l) == model.NormalizeLocale(locale)
	})
}

// session is one item open in the stream home.
type session struct {
	h    *Home
	ws   store.BlockWriteStore
	item string
	info change.DocInfo
}

func (s *session) Info() change.DocInfo { return s.info }

func (s *session) Place(k model.EditionKey) change.Place {
	if k.IsZero() || s.h.admits(k.Locale) {
		return change.Place{Kind: change.PlaceInDocument}
	}
	return change.Place{Kind: change.PlaceNone, Why: fmt.Sprintf("the project does not translate into %s", k.Locale)}
}

// prepare gives each block read from a row the project's source language,
// which no row keeps, so an edition key in that language names the source.
func (s *session) prepare(rows []*venue.StoredBlock) {
	for _, sb := range rows {
		if sb != nil && sb.Block != nil && sb.Block.SourceLocale == "" {
			sb.Block.SourceLocale = s.h.SourceLocale
		}
	}
}

func (s *session) Read(ctx context.Context, want change.Want, fn func(*model.Block) error) (string, error) {
	rows, err := s.ws.ItemBlocks(ctx, s.h.ProjectID, s.h.stream(), s.item, want.Blocks)
	if err != nil {
		return "", err
	}
	s.prepare(rows)
	head := digest(rows)
	for _, sb := range rows {
		if err := fn(sb.Block); err != nil {
			if errors.Is(err, change.ErrStop) {
				break
			}
			return "", err
		}
	}
	return head, nil
}

func (s *session) Stage(ctx context.Context, want change.Want, e change.Editor) (change.Staged, error) {
	rows, err := s.ws.ItemBlocks(ctx, s.h.ProjectID, s.h.stream(), s.item, want.Blocks)
	if err != nil {
		return nil, err
	}
	st := &staged{s: s, want: want, e: e}
	if err := st.pass(rows); err != nil {
		return nil, err
	}
	return st, nil
}

func (s *session) Close() error { return nil }

// staged is a change to one item, held ready to commit.
type staged struct {
	s    *session
	want change.Want
	e    change.Editor

	// rows are the blocks the last pass ran over, and changed the ones it
	// changed. before is the text of each edition of a changed block as the
	// pass found it, for the preview.
	rows    []*venue.StoredBlock
	changed []*venue.StoredBlock
	before  map[string]map[string]string
	// head and after are the digests of the rows around the pass.
	head, after string

	// w is the change set's write the item's rows are held on, and held the
	// rows as Lock read them.
	w       *streamWrite
	held    []*venue.StoredBlock
	written bool
}

// pass applies the change set's operations to rows.
func (st *staged) pass(rows []*venue.StoredBlock) error {
	st.s.prepare(rows)
	st.rows, st.changed = rows, nil
	st.head = digest(rows)
	st.before = map[string]map[string]string{}
	st.e.Begin()
	for _, sb := range rows {
		was := editionTexts(sb.Block)
		keys, err := st.e.Edit(sb.Block)
		if err != nil {
			return err
		}
		if len(keys) > 0 {
			st.changed = append(st.changed, sb)
			st.before[sb.Block.ID] = was
		}
	}
	if err := st.e.End(); err != nil {
		return err
	}
	if st.s.h.Stamp != nil {
		for _, sb := range st.changed {
			st.s.h.Stamp(sb.Block)
		}
	}
	st.after = digest(rows)
	return nil
}

func (st *staged) Files() []change.StagedFile {
	return []change.StagedFile{{File: st.s.info.Doc, Before: st.head, After: st.after, Written: st.written}}
}

// Diff renders each edition the change rewrites, before and after.
func (st *staged) Diff() string {
	if len(st.changed) == 0 {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "--- %s\n+++ %s\n", st.s.info.Doc, st.s.info.Doc)
	for _, sb := range st.changed {
		was := st.before[sb.Block.ID]
		now := editionTexts(sb.Block)
		keys := make([]string, 0, len(now))
		for k := range now {
			keys = append(keys, k)
		}
		for k := range was {
			if _, ok := now[k]; !ok {
				keys = append(keys, k)
			}
		}
		slices.Sort(keys)
		for _, k := range keys {
			if was[k] == now[k] {
				continue
			}
			fmt.Fprintf(&b, "@@ %s %s @@\n", change.BlockKey(sb.Block), k)
			if old, ok := was[k]; ok {
				fmt.Fprintf(&b, "-%s\n", old)
			}
			if cur, ok := now[k]; ok {
				fmt.Fprintf(&b, "+%s\n", cur)
			}
		}
	}
	return b.String()
}

// LockKeys names the item's rows.
func (st *staged) LockKeys() []string {
	return []string{"stream:" + st.s.h.ProjectID + "/" + st.s.h.stream() + "/" + st.s.item}
}

// Lock holds the rows the stage read on the change set's write, beginning the
// write when this is the change set's first item.
func (st *staged) Lock(ctx context.Context, _ string) error {
	if st.w != nil {
		return nil
	}
	if hook := st.s.h.BeforeLock; hook != nil {
		hook(st.s.info.Doc)
	}
	w, err := st.s.h.write(ctx, st.s.ws)
	if err != nil {
		return err
	}
	held, err := w.bw.Hold(ctx, st.s.item, st.want.Blocks)
	if err != nil {
		_ = st.s.h.discard(w, err)
		return err
	}
	st.s.prepare(held)
	st.held = held
	w.members = append(w.members, st)
	st.w = w
	return nil
}

// Settle applies the operations again to the held rows when they moved since
// the stage. Every if_match is then checked against the rows the commit
// stores.
func (st *staged) Settle(ctx context.Context) error {
	if err := st.Lock(ctx, ""); err != nil {
		return err
	}
	if digest(st.held) == st.head {
		// The rows are as the stage read them: what it changed is what the
		// held rows become. The content hash each row holds is the base the
		// write guards on.
		hashes := make(map[string]string, len(st.held))
		for _, sb := range st.held {
			hashes[sb.Block.ID] = sb.ContentHash
		}
		for _, sb := range st.changed {
			sb.ContentHash = hashes[sb.Block.ID]
		}
		return nil
	}
	return st.pass(st.held)
}

// Commit stores the changed rows of every item of the change set and commits
// the write, when no item of it has; it then reports this item's rows.
func (st *staged) Commit(ctx context.Context) error {
	if st.w == nil {
		return errors.New("commit before settle")
	}
	if err := st.s.h.commit(ctx, st.w); err != nil {
		return err
	}
	st.written = len(st.changed) > 0
	if st.written && st.s.h.Committed != nil {
		ids := make([]string, 0, len(st.changed))
		for _, sb := range st.changed {
			ids = append(ids, sb.Block.ID)
		}
		st.s.h.Committed(st.s.info.Doc, ids)
	}
	return nil
}

// Release discards the change set's write when it did not commit.
func (st *staged) Release() error {
	w := st.w
	if w == nil {
		return nil
	}
	st.w = nil
	return st.s.h.discard(w, errWriteDiscarded)
}

// WriteMeta writes what a pass wrote on the drafts' blocks beyond their
// content (BlockState.Meta: block annotations, properties, and statuses moved
// on unchanged content) on one write that holds their rows. A row takes them
// only while it holds the content the pass produced or read: what a pass found
// on content that moved since stays off the row, and the draft is returned as
// left out, refused as stale. Server passes that judge content without
// changing it (source settlement, the review recheck) write through it too.
func (h *Home) WriteMeta(ctx context.Context, drafts []Draft) ([]Refusal, error) {
	if len(drafts) == 0 {
		return nil, nil
	}
	ws, ok := h.Store.(store.BlockWriteStore)
	if !ok {
		return nil, fmt.Errorf("the content store %T keeps no held writes", h.Store)
	}
	byItem := map[string][]Draft{}
	for _, d := range drafts {
		byItem[d.Doc] = append(byItem[d.Doc], d)
	}
	bw, err := ws.BeginBlockWrite(ctx, h.ProjectID, h.stream())
	if err != nil {
		return nil, err
	}
	defer func() { _ = bw.Rollback() }()
	var (
		changed []*venue.StoredBlock
		moved   []Refusal
	)
	for _, item := range slices.Sorted(maps.Keys(byItem)) {
		ds := byItem[item]
		keys := make([]string, len(ds))
		for i, d := range ds {
			keys[i] = d.Before.ID()
		}
		held, err := bw.Hold(ctx, item, keys)
		if err != nil {
			return nil, err
		}
		byID := make(map[string]*venue.StoredBlock, len(held))
		for _, sb := range held {
			if sb.Block.SourceLocale == "" {
				sb.Block.SourceLocale = h.SourceLocale
			}
			byID[sb.Block.ID] = sb
		}
		for _, d := range ds {
			sb := byID[d.Before.ID()]
			if sb == nil || !sameContent(sb.Block, d.After) {
				moved = append(moved, Refusal{Doc: item, Block: d.Before.ID(), Error: &change.Error{Code: change.CodeStale,
					Message: "the block's content moved since the tool read it, so what the tool found on it is not kept"}})
				continue
			}
			d.Before.Meta(d.After).apply(sb.Block)
			changed = append(changed, sb)
		}
	}
	if len(changed) > 0 {
		if err := bw.Store(ctx, changed); err != nil {
			return nil, err
		}
	}
	return moved, bw.Commit()
}

// sameContent reports whether a and b hold the same editions with the same
// runs.
func sameContent(a, b *model.Block) bool {
	contents := func(blk *model.Block) map[string]string {
		out := map[string]string{}
		for k, e := range blk.EachEdition {
			text, _ := k.MarshalText()
			out[string(text)] = string(model.CanonicalRunsJSON(e.Runs))
		}
		return out
	}
	return maps.Equal(contents(a), contents(b))
}

// editionTexts is the text of each edition of b, keyed by its edition key in
// text form, in the placeholder form a read shows.
func editionTexts(b *model.Block) map[string]string {
	out := map[string]string{}
	for k, e := range b.EachEdition {
		text, _ := k.MarshalText()
		name := string(text)
		if b.IsSourceEdition(k) {
			name = "(source)"
		}
		out[name] = model.RunsEditText(e.Runs)
	}
	return out
}

// rowState is what a row holds that a write stores, for the digest that tells
// whether the rows moved between a stage and its commit.
type rowState struct {
	ID           string                   `json:"id"`
	Translatable bool                     `json:"translatable"`
	Properties   map[string]string        `json:"properties,omitempty"`
	Editions     []editionState           `json:"editions"`
	Overlays     []model.Overlay          `json:"overlays,omitempty"`
	Annotations  map[string]model.Payload `json:"annotations,omitempty"`
}

type editionState struct {
	Key    string          `json:"key"`
	Runs   json.RawMessage `json:"runs"`
	Status model.Status    `json:"status,omitempty"`
	Origin model.Origin    `json:"origin,omitzero"`
	Score  float64         `json:"score,omitempty"`
}

// digest is the head of the rows: a hash over what each holds.
func digest(rows []*venue.StoredBlock) string {
	h := sha256.New()
	for _, sb := range rows {
		if sb == nil || sb.Block == nil {
			continue
		}
		b := sb.Block
		st := rowState{ID: b.ID, Translatable: b.Translatable, Properties: b.Properties, Overlays: b.Overlays, Annotations: b.AnnoMap()}
		for k, e := range b.EachEdition {
			text, _ := k.MarshalText()
			st.Editions = append(st.Editions, editionState{Key: string(text), Runs: model.CanonicalRunsJSON(e.Runs),
				Status: e.Status, Origin: e.Origin, Score: e.Score})
		}
		slices.SortFunc(st.Editions, func(a, b editionState) int { return strings.Compare(a.Key, b.Key) })
		raw, err := json.Marshal(st)
		if err != nil {
			raw = []byte(b.ID + ":" + err.Error())
		}
		h.Write(raw)
		h.Write([]byte{0})
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil))
}
