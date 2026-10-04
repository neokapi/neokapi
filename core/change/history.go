package change

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/neokapi/neokapi/core/model"
)

// HistoryRequest names the edition whose recorded changes to list.
type HistoryRequest struct {
	// Ref is the edition as a read reports it. A reference to the file one
	// edition lives in, with no edition, names that edition, as a read of
	// that file does.
	Ref Ref `json:"ref"`
	// Limit is the most entries the history holds; zero is
	// DefaultHistoryLimit.
	Limit int `json:"limit,omitempty"`
}

// DefaultHistoryLimit and MaxHistoryLimit bound a history.
const (
	DefaultHistoryLimit = 50
	MaxHistoryLimit     = 500
)

// History is the recorded changes to one edition, most recent first.
type History struct {
	// Ref is the edition, canonical.
	Ref Ref `json:"ref"`
	// Rev is the edition's revision now, "absent" for an edition the block
	// does not hold.
	Rev     string         `json:"rev"`
	Entries []HistoryEntry `json:"entries"`
}

// HistoryEntry is one recorded change to an edition.
type HistoryEntry struct {
	// Record is the id of the recorded edit (content.edit) that made the
	// change.
	Record string `json:"record"`
	// Before and After are the edition's revisions around the change: Before
	// is "absent" for an edition the change created, and After for one it
	// removed.
	Before string `json:"before"`
	After  string `json:"after"`
	// Basis is the authoritative edition's revision a derived edition was
	// made from, when the change recorded one.
	Basis string `json:"basis,omitempty"`
	// Actor is who made the change, null when nobody knows who, as for an
	// edit made outside kapi.
	Actor *Actor `json:"actor"`
	// Origin is the surface that applied the change: apply, desktop, mcp,
	// flow:<name>, merge, pull or observed.
	Origin string `json:"origin,omitempty"`
	// At is when the change was recorded.
	At time.Time `json:"at"`
}

// EditionHistories reads the recorded changes to an edition, most recent
// first and at most limit of them. A host answers from its record of applied
// change sets (kapi's block history). Without one a history lists nothing.
type EditionHistories interface {
	EditionHistory(ctx context.Context, doc DocInfo, b *model.Block, k model.EditionKey, limit int) ([]HistoryEntry, error)
}

// WithHistories sets what a history asks for the recorded changes to an
// edition.
func WithHistories(h EditionHistories) Option { return func(s *Service) { s.histories = h } }

// History lists the recorded changes to one edition of a block, most recent
// first, beside the revision the edition holds now. The block is read from its
// home, so a reference resolves as a read resolves it and the history names the
// edition canonically.
func (s *Service) History(ctx context.Context, q HistoryRequest) (*History, error) {
	if q.Ref.Doc == "" {
		return nil, &Error{Code: CodeInvalid, Field: "ref/doc", Message: "name the document whose history to read"}
	}
	if q.Ref.Block == "" {
		return nil, &Error{Code: CodeInvalid, Field: "ref/block", Message: "name the block whose history to read"}
	}
	limit := q.Limit
	switch {
	case limit <= 0:
		limit = DefaultHistoryLimit
	case limit > MaxHistoryLimit:
		limit = MaxHistoryLimit
	}
	h, err := s.homeFor(q.Ref.Doc)
	if err != nil {
		return nil, err
	}
	sess, err := h.Open(ctx, q.Ref.Doc)
	if err != nil {
		if e := asError(err); e != nil {
			return nil, e
		}
		return nil, err
	}
	defer sess.Close()
	info := sess.Info()

	edition := q.Ref.Edition.Canonical()
	if edition.IsZero() && info.Edition != nil {
		edition = info.Edition.Canonical()
	}
	var want Want
	if !edition.IsZero() && sess.Place(edition).Kind == PlaceOwnFile {
		want.Editions = []model.EditionKey{edition}
	}
	key := q.Ref.Block
	if resolver, ok := sess.(EditionKeyResolver); ok && info.Edition != nil && q.Ref.Edition.IsZero() && sess.Place(*info.Edition).Kind == PlaceOwnFile {
		// A block named by its key in the edition's own file, as a person who
		// opened that file reads it.
		if k, found, rerr := resolver.DocumentBlockKey(ctx, *info.Edition, key); rerr == nil && found {
			key = k
		}
	}
	want.Blocks = []string{key}

	var block *model.Block
	_, err = sess.Read(ctx, want, func(b *model.Block) error {
		if slices.Contains([]string{b.Key, b.Name, b.ID}, key) {
			block = b
			return ErrStop
		}
		return nil
	})
	if err != nil && !errors.Is(err, ErrStop) {
		if e := asError(err); e != nil {
			return nil, e
		}
		return nil, err
	}
	if block == nil {
		return nil, &Error{Code: CodeNotFound, Field: "ref/block", Message: "no block " + q.Ref.Block + " in " + info.Doc}
	}

	ref := Ref{Doc: info.Doc, Block: BlockKey(block)}
	if !block.IsSourceEdition(edition) {
		ref.Edition = edition
	}
	out := &History{Ref: ref, Rev: model.EditionRevision(block, edition), Entries: []HistoryEntry{}}
	if s.histories == nil {
		return out, nil
	}
	entries, err := s.histories.EditionHistory(ctx, info, block, edition, limit)
	if err != nil {
		return nil, err
	}
	if len(entries) > limit {
		entries = entries[:limit]
	}
	if entries != nil {
		out.Entries = entries
	}
	return out, nil
}
