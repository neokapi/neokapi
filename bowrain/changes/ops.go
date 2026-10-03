package changes

import (
	"bytes"
	"context"
	"encoding/json"
	"slices"
	"strings"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/model"
)

// BlockState is a block as a server job read it, before a tool ran over it:
// the revision and content of each edition, and the overlays on each. A tool
// changes the block it is given in place, so a job records the state first and
// compares the block the tool hands back against it (Ops).
type BlockState struct {
	id       string
	source   model.EditionKey
	authRev  string
	editions map[model.EditionKey]editionSnap
	overlays map[overlayKey][]byte
}

type editionSnap struct {
	rev    string
	runs   []byte
	status model.Status
	origin model.Origin
	score  float64
}

// overlayKey names one overlay of a block: its type, the edition it lies on
// (empty for the source) and its layer.
type overlayKey struct {
	typ     model.OverlayType
	edition string
	layer   string
}

// Snapshot records b's state. A row keeps no source language, so b takes the
// project's, source, when it has none: the revisions the state records are
// then the ones the stream home computes for the row.
func Snapshot(b *model.Block, source model.LocaleID) BlockState {
	if b.SourceLocale == "" {
		b.SourceLocale = source
	}
	s := BlockState{id: b.ID, source: b.EditionKeyOf(model.EditionKey{}), editions: map[model.EditionKey]editionSnap{}, overlays: map[overlayKey][]byte{}}
	for k, e := range b.EachEdition {
		s.editions[k] = editionSnap{rev: model.RunsRevision(k, e.Runs), runs: model.CanonicalRunsJSON(e.Runs),
			status: e.Status, origin: e.Origin, score: e.Score}
	}
	s.authRev = model.EditionRevision(b, b.Authoritative(model.AuthorityPolicy{}))
	for _, o := range b.Overlays {
		s.overlays[keyOfOverlay(o)] = overlayJSON(o)
	}
	return s
}

func keyOfOverlay(o model.Overlay) overlayKey {
	k := overlayKey{typ: o.Type, layer: o.Layer}
	if o.Variant != nil {
		text, _ := o.Variant.Canonical().MarshalText()
		k.edition = string(text)
	}
	return k
}

func overlayJSON(o model.Overlay) []byte {
	raw, err := json.Marshal(o.Spans)
	if err != nil {
		return []byte(err.Error())
	}
	return raw
}

// ID is the row id of the block the state was recorded from.
func (s BlockState) ID() string { return s.id }

// Ops returns the operations that make the block the state was recorded from
// into after, which a tool in a server job produced from it, for the item doc.
// Each is guarded by the revision the state recorded, so a change made to the
// block since the read refuses the job's operations on it.
//
//   - A derived edition after adds, or whose content it changed, is a
//     set_content of after's runs, with the authoritative edition's revision
//     the job read as its basis; one after drops is a remove_edition.
//   - The status, origin and score after gives a derived edition the job
//     wrote are a provenance operation, which records how the tool produced
//     it.
//   - The source, when a tool changed it, is a set_content of after's runs.
//   - An overlay after adds, changes or drops is an annotate of its spans
//     that takes the overlay's place.
//
// Block annotations and properties are not operations and are not written.
func (s BlockState) Ops(doc string, after *model.Block) []change.Op {
	at := func(k model.EditionKey) change.Ref {
		r := change.Ref{Doc: doc, Block: s.id}
		if k != s.source {
			r.Edition = k
		}
		return r
	}
	var ops []change.Op
	now := map[model.EditionKey]model.Edition{}
	for k, e := range after.EachEdition {
		now[k] = e
	}
	keys := make([]model.EditionKey, 0, len(now)+len(s.editions))
	for k := range now {
		keys = append(keys, k)
	}
	for k := range s.editions {
		if _, ok := now[k]; !ok {
			keys = append(keys, k)
		}
	}
	slices.SortFunc(keys, func(a, b model.EditionKey) int { return strings.Compare(keyText(a), keyText(b)) })
	for _, k := range keys {
		was, inBefore := s.editions[k]
		cur, inAfter := now[k]
		source := k == s.source
		var content bool
		switch {
		case !inBefore && inAfter:
			content = true
			ops = append(ops, change.Op{Kind: change.KindSetContent, At: at(k), IfMatch: model.AbsentRevision,
				Basis: s.authRev, Body: &change.SetContent{Content: change.Content{Runs: slices.Clone(cur.Runs)}}})
		case inBefore && !inAfter:
			if !source {
				ops = append(ops, change.Op{Kind: change.KindRemoveEdition, At: at(k), IfMatch: was.rev, Body: &change.RemoveEdition{}})
			}
			continue
		case !bytes.Equal(was.runs, model.CanonicalRunsJSON(cur.Runs)):
			content = true
			op := change.Op{Kind: change.KindSetContent, At: at(k), IfMatch: was.rev,
				Body: &change.SetContent{Content: change.Content{Runs: slices.Clone(cur.Runs)}}}
			if !source {
				op.Basis = s.authRev
			}
			ops = append(ops, op)
		}
		if source {
			continue
		}
		if content || was.status != cur.Status || was.origin != cur.Origin || was.score != cur.Score {
			score := cur.Score
			ops = append(ops, change.Op{Kind: change.KindProvenance, At: at(k),
				Body: &change.Provenance{Status: cur.Status, Origin: cur.Origin, Score: &score}})
		}
	}
	ops = append(ops, s.overlayOps(doc, after)...)
	return ops
}

// overlayOps returns an annotate for each overlay after adds, changes or
// drops, taking the place of the overlay of its type and layer on its edition.
func (s BlockState) overlayOps(doc string, after *model.Block) []change.Op {
	seen := map[overlayKey]bool{}
	var ops []change.Op
	annotate := func(k overlayKey, spans []model.Span) {
		ref := change.Ref{Doc: doc, Block: s.id}
		if k.edition != "" {
			if ek, err := model.ParseEditionKey(k.edition); err == nil {
				ref.Edition = ek
			}
		}
		ops = append(ops, change.Op{Kind: change.KindAnnotate, At: ref,
			Body: &change.Annotate{Type: string(k.typ), Layer: k.layer, Replace: true, Spans: spans}})
	}
	for _, o := range after.Overlays {
		k := keyOfOverlay(o)
		seen[k] = true
		if was, ok := s.overlays[k]; ok && bytes.Equal(was, overlayJSON(o)) {
			continue
		}
		annotate(k, slices.Clone(o.Spans))
	}
	keys := make([]overlayKey, 0, len(s.overlays))
	for k := range s.overlays {
		if !seen[k] {
			keys = append(keys, k)
		}
	}
	slices.SortFunc(keys, func(a, b overlayKey) int {
		return strings.Compare(string(a.typ)+"\x00"+a.edition+"\x00"+a.layer, string(b.typ)+"\x00"+b.edition+"\x00"+b.layer)
	})
	for _, k := range keys {
		annotate(k, nil)
	}
	return ops
}

func keyText(k model.EditionKey) string {
	text, _ := k.MarshalText()
	return string(text)
}

// Refusal is a block whose operations a change set could not apply, and why.
type Refusal struct {
	Doc   string
	Block string
	Error *change.Error
}

// ApplyEach applies set as actor and, when it is refused, leaves out every
// operation on each block a refused operation names and applies the rest
// again, until what remains applies or nothing does. It is how a server job
// writes what a tool produced: a block a person changed since the job read it
// keeps the person's change, and the job's other blocks land.
//
// It returns the result of the change set that applied, nil when every block
// was left out, and the blocks it left out. A refusal that names no block
// (the change set as a whole) ends it with that result.
func ApplyEach(ctx context.Context, svc *change.Service, set change.Set, actor change.Actor) (*change.Result, []Refusal, error) {
	var refused []Refusal
	for {
		if len(set.Ops) == 0 {
			return nil, refused, nil
		}
		res, err := svc.Apply(ctx, set, actor)
		if err != nil {
			return nil, refused, err
		}
		if res.Status != change.SetRefused {
			return res, refused, nil
		}
		type blockRef struct{ doc, block string }
		drop := map[blockRef]bool{}
		for i, r := range res.Ops {
			if r.Status != change.OpRefused {
				continue
			}
			at := set.Ops[i].At
			if at.Block == "" {
				return res, refused, nil
			}
			ref := blockRef{at.Doc, at.Block}
			if !drop[ref] {
				drop[ref] = true
				refused = append(refused, Refusal{Doc: at.Doc, Block: at.Block, Error: r.Error})
			}
		}
		if len(drop) == 0 {
			return res, refused, nil
		}
		kept := set.Ops[:0:0]
		for _, op := range set.Ops {
			if !drop[blockRef{op.At.Doc, op.At.Block}] {
				kept = append(kept, op)
			}
		}
		set.Ops = kept
	}
}
