package host

import (
	"context"
	"errors"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/contextgraph"
	coregraph "github.com/neokapi/neokapi/core/graph"
	"github.com/neokapi/neokapi/core/occurrence"
	"github.com/neokapi/neokapi/terms"
)

// Where a term is used, read from the context graph.
//
// Extraction writes one uses_term edge per (block, term, language of the text)
// with the number of uses on it (occurrence.BuildGraph, through
// MaterializeContextGraph), and every surface that reports a usage count reads
// those edges back: the context search behind `kapi context search` and the
// context_search tool, the desktop explorer's relations pane, and the
// platform's concept page through the same contextgraph.UsesByProject. One
// producer, so the number cannot differ by where it was asked; and because the
// producer is extraction, the number is as of the last extraction rather than
// of the working tree. `kapi terms occurrences` is the surface that lists uses
// live, with their positions.

// ContextUse is one recorded use of a term: the edge, and the passage it sits
// in where the block cache still holds the block.
type ContextUse struct {
	ConceptID  string `json:"concept_id"`
	Term       string `json:"term"`
	TermLocale string `json:"term_locale,omitempty"`
	// Status is the term's lifecycle status as the edge recorded it, and
	// Discouraged whether that status is one a writer must act on.
	Status      string `json:"status,omitempty"`
	Discouraged bool   `json:"discouraged,omitempty"`
	// ContentKey is the block's durable identity, BlockID its structural name
	// within Document, and Collection where the block sits.
	ContentKey string `json:"content_key"`
	BlockID    string `json:"block_id,omitempty"`
	Document   string `json:"document,omitempty"`
	Collection string `json:"collection,omitempty"`
	// Locale is the language of the text the term was found in: empty for the
	// block's own source text, otherwise the target locale.
	Locale string `json:"locale,omitempty"`
	// Occurrences is how many times the term is used in that text.
	Occurrences int `json:"occurrences"`
	// Snippet is the first use in context, read from the block cache. Empty
	// when the cache no longer holds the block: the graph is a projection, and
	// a projection may name a row that has since moved.
	Snippet string `json:"snippet,omitempty"`
}

// ContextUses answers "where is this used" for a term or a concept.
type ContextUses struct {
	// Subject is the question as asked.
	Subject string `json:"subject"`
	// ConceptIDs are the concepts the subject resolved to, in id order.
	ConceptIDs []string `json:"concept_ids,omitempty"`
	// Uses are the recorded uses, ordered by document, block, language and
	// term, capped at the limit the caller asked for.
	Uses []ContextUse `json:"uses"`
	// Total is the number of uses across every edge before the cap.
	Total int `json:"total"`
	// Blocks is how many distinct blocks the uses fall in.
	Blocks int `json:"blocks"`
	// Notes says what could not be reached, so an empty answer is never
	// ambiguous between "unused" and "nowhere to look".
	Notes []string `json:"notes,omitempty"`
}

// FindContextUses reads where a term or concept is used in the project the
// sources describe. The subject resolves the way `kapi terms occurrences`
// resolves it, a concept id first and then a term text across every concept,
// and the answer is the edges for those concepts: for a term text, only the
// edges of that term, so "widget" is where the project writes widget and not
// wherever the concept appears. The passage is attached from the block cache
// for the first limit rows; zero means no cap.
//
// An unknown subject is occurrence.ErrUnknownSubject, distinct from a term
// nobody uses: a typo is not an answer.
func FindContextUses(ctx context.Context, src ContextSearchSources, subject string, limit int) (*ContextUses, error) {
	if src.Terms == nil {
		return nil, errors.New("context uses: no terms store")
	}
	resolved, err := occurrence.Find(ctx, occurrence.Sources{Terms: src.Terms}, occurrence.Query{Subject: subject})
	if err != nil {
		return nil, err
	}
	out := &ContextUses{Subject: resolved.Subject, ConceptIDs: resolved.ConceptIDs, Uses: []ContextUse{}}
	if note := usesUnavailable(src); note != "" {
		out.Notes = append(out.Notes, note)
		return out, nil
	}

	wanted := make(map[string]bool, len(resolved.Terms))
	for _, t := range resolved.Terms {
		wanted[terms.NormalizeTerm(t)] = true
	}
	wording := newUseWording(ctx, src.Blocks)
	defer wording.close()
	blocks := map[string]bool{}
	for _, conceptID := range resolved.ConceptIDs {
		uses, err := conceptUses(ctx, src.Graph, src.GraphScope, conceptID)
		if err != nil {
			return nil, err
		}
		for _, u := range uses {
			if !wanted[terms.NormalizeTerm(u.Term)] {
				continue
			}
			out.Total += u.Occurrences
			blocks[u.ContentKey] = true
			if limit > 0 && len(out.Uses) >= limit {
				continue
			}
			out.Uses = append(out.Uses, contextUse(conceptID, u, wording))
		}
	}
	out.Blocks = len(blocks)
	return out, nil
}

// usesUnavailable is the note a usage answer carries when the graph cannot be
// read, or "" when it can. The three states need three next steps, so they are
// three sentences rather than one.
func usesUnavailable(src ContextSearchSources) string {
	switch {
	case src.Graph == nil && src.GraphErr != nil:
		return "the context graph could not be opened, so term usage was not counted: " + src.GraphErr.Error()
	case src.Graph == nil:
		return "no context graph is bound, so term usage was not counted. Run `kapi up` in a project to extract its content"
	case src.Unextracted:
		return "this project's content has not been extracted yet, so term usage was not counted. Run `kapi up`"
	}
	return ""
}

// conceptUses reads every recorded use of one concept in the project, in the
// stable order the query returns them: by document, block, content key,
// language and term. The window is left unapplied, because a count of uses is
// a fact about the text: a term deprecated from a date is used the same number
// of times before it.
func conceptUses(ctx context.Context, g contextgraph.EdgeReader, scope contextgraph.Scope, conceptID string) ([]contextgraph.ConceptUse, error) {
	rollup, err := contextgraph.UsesByProject(ctx, g, scope, conceptID, coregraph.Scope{})
	if err != nil {
		return nil, err
	}
	var uses []contextgraph.ConceptUse
	for _, p := range rollup {
		uses = append(uses, p.Uses...)
	}
	return uses, nil
}

func contextUse(conceptID string, u contextgraph.ConceptUse, wording *useWording) ContextUse {
	return ContextUse{
		ConceptID:   conceptID,
		Term:        u.Term,
		TermLocale:  u.TermLocale,
		Status:      u.Status,
		Discouraged: u.Discouraged,
		ContentKey:  u.ContentKey,
		BlockID:     u.BlockID,
		Document:    u.Document,
		Collection:  u.Collection,
		Locale:      u.Locale,
		Occurrences: u.Occurrences,
		Snippet:     wording.snippet(u),
	}
}

// useWording reads the passage a recorded use sits in, from the block cache by
// content key. The graph holds the relationship and the count; the words are
// the block's. One read session serves every lookup of an answer, opened on the
// first and released by close.
type useWording struct {
	ctx    context.Context
	blocks blockstore.Store
	sess   blockstore.Session
	failed bool
}

func newUseWording(ctx context.Context, blocks blockstore.Store) *useWording {
	return &useWording{ctx: ctx, blocks: blocks}
}

// snippet renders the first use the edge records, in context, or "" when the
// cache holds no block under the key, no text in that language, or no store is
// bound at all. A missing passage costs the snippet and nothing else.
func (w *useWording) snippet(u contextgraph.ConceptUse) string {
	if w == nil || w.blocks == nil || u.ContentKey == "" || u.Term == "" {
		return ""
	}
	if w.sess == nil && !w.failed {
		sess, err := w.blocks.Begin(w.ctx)
		if err != nil {
			w.failed = true
			return ""
		}
		w.sess = sess
	}
	if w.sess == nil {
		return ""
	}
	b, err := w.sess.GetBlock(u.ContentKey)
	if err != nil || b == nil {
		return ""
	}
	for _, text := range blockstore.BlockTexts(b) {
		if text.Locale != u.Locale {
			continue
		}
		return occurrence.Snippet(text.Text, u.Term)
	}
	return ""
}

func (w *useWording) close() {
	if w != nil && w.sess != nil {
		_ = w.sess.Close()
		w.sess = nil
	}
}
