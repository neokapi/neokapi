package changes

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"strings"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/model"
)

// BlockState is a block as a server job read it, before a tool ran over it:
// the revision and content of each edition, the overlays on each, and the
// block's annotations and properties. A tool changes the block it is given in
// place, so a job records the state first and compares the block the tool
// hands back against it (Ops, Meta).
type BlockState struct {
	id       string
	source   model.EditionKey
	authRev  string
	editions map[model.EditionKey]editionSnap
	overlays map[overlayKey][]byte
	annos    map[string][]byte
	props    map[string]string
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
	s.annos = map[string][]byte{}
	for key, v := range b.Annos() {
		s.annos[key] = payloadJSON(v)
	}
	s.props = maps.Clone(b.Properties)
	return s
}

func payloadJSON(v model.Payload) []byte {
	raw, err := json.Marshal(v)
	if err != nil {
		return []byte(err.Error())
	}
	return raw
}

// BlockMeta is what a tool wrote on a block beyond its content and overlays:
// the block annotations it added or changed, and the properties it set or
// removed. A row keeps them beside the content, and no change set operation
// carries them (Home.WriteMeta).
type BlockMeta struct {
	Annotations map[string]model.Payload
	Set         map[string]string
	Removed     []string
}

// Empty reports whether the tool wrote nothing beyond the content.
func (m BlockMeta) Empty() bool {
	return len(m.Annotations) == 0 && len(m.Set) == 0 && len(m.Removed) == 0
}

// apply writes m onto b.
func (m BlockMeta) apply(b *model.Block) {
	for key, v := range m.Annotations {
		b.SetAnno(key, v)
	}
	if len(m.Set) > 0 && b.Properties == nil {
		b.Properties = map[string]string{}
	}
	maps.Copy(b.Properties, m.Set)
	for _, name := range m.Removed {
		delete(b.Properties, name)
	}
}

// Meta returns the block annotations and properties after carries that the
// state did not: an annotation added or changed, and a property set, changed
// or removed. An annotation the tool removed is not among them; a row keeps
// the annotations it has.
func (s BlockState) Meta(after *model.Block) BlockMeta {
	var m BlockMeta
	for key, v := range after.Annos() {
		if was, ok := s.annos[key]; ok && bytes.Equal(was, payloadJSON(v)) {
			continue
		}
		if m.Annotations == nil {
			m.Annotations = map[string]model.Payload{}
		}
		m.Annotations[key] = v
	}
	for name, v := range after.Properties {
		if was, ok := s.props[name]; ok && was == v {
			continue
		}
		if m.Set == nil {
			m.Set = map[string]string{}
		}
		m.Set[name] = v
	}
	for name := range s.props {
		if _, ok := after.Properties[name]; !ok {
			m.Removed = append(m.Removed, name)
		}
	}
	slices.Sort(m.Removed)
	return m
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
//     that takes the overlay's place, guarded by the revision of the edition
//     it lies on.
//
// Block annotations and properties are not operations: Meta returns them, and
// the stream home writes them (Home.WriteMeta).
func (s BlockState) Ops(doc string, after *model.Block) []change.Op {
	at := func(k model.EditionKey) change.Ref {
		r := change.Ref{Doc: doc, Block: s.id}
		if k != s.source {
			r.Edition = k
		}
		return r
	}
	var ops []change.Op
	now := maps.Collect(after.EachEdition)
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
				Basis: s.authRev, Body: &change.SetContent{Runs: slices.Clone(cur.Runs)}})
		case inBefore && !inAfter:
			if !source {
				ops = append(ops, change.Op{Kind: change.KindRemoveEdition, At: at(k), IfMatch: was.rev, Body: &change.RemoveEdition{}})
			}
			continue
		case !bytes.Equal(was.runs, model.CanonicalRunsJSON(cur.Runs)):
			content = true
			op := change.Op{Kind: change.KindSetContent, At: at(k), IfMatch: was.rev,
				Body: &change.SetContent{Runs: slices.Clone(cur.Runs)}}
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
// An overlay marks positions in the content of the edition it lies on, so
// each is guarded by the revision of that edition the state recorded: an
// overlay a tool made on content that moved since the read does not land.
func (s BlockState) overlayOps(doc string, after *model.Block) []change.Op {
	seen := map[overlayKey]bool{}
	var ops []change.Op
	annotate := func(k overlayKey, spans []model.Span) {
		ref := change.Ref{Doc: doc, Block: s.id}
		on := s.source
		if k.edition != "" {
			if ek, err := model.ParseEditionKey(k.edition); err == nil {
				ref.Edition, on = ek, ek
			}
		}
		op := change.Op{Kind: change.KindAnnotate, At: ref,
			Body: &change.Annotate{Type: string(k.typ), Layer: k.layer, Replace: true, Spans: spans}}
		if was, ok := s.editions[on]; ok {
			op.IfMatch = was.rev
		}
		ops = append(ops, op)
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

// Draft is one block a tool in a server job produced: the item it belongs to,
// the state the job read it in, and the block the tool handed back.
type Draft struct {
	Doc    string
	Before BlockState
	After  *model.Block
}

// CommitDrafts writes what a tool produced through the change service of
// home, as the tool: the operations that turn each draft's block from the
// state the job read into what the tool made of it (BlockState.Ops), under
// gate report, so the drafts land with their findings and meet the ship gates
// later, and with require_basis, so a translation drafted from a source that
// moved since the job read it does not land. A block a person changed since
// the read keeps the person's change (ApplyEach). The block annotations and
// properties the tool wrote then land on the rows that hold what the tool
// produced (Home.WriteMeta). It returns the result, the blocks left out, and
// the row ids of the drafts the stream now holds: those that landed, and those
// that changed nothing because the stream already held what the tool
// produced.
func CommitDrafts(ctx context.Context, home *Home, svc *change.Service, tool string, drafts []Draft) (*change.Result, []Refusal, []string, error) {
	set := change.Set{Gate: change.GateReport, RequireBasis: true}
	byBlock := map[[2]string]bool{}
	for _, d := range drafts {
		byBlock[[2]string{d.Doc, d.Before.ID()}] = true
		set.Ops = append(set.Ops, d.Before.Ops(d.Doc, d.After)...)
	}
	res, _, refused, err := ApplyEach(ctx, svc, set, change.Actor{Kind: change.ActorTool, Name: tool})
	if err != nil || (res != nil && res.Status == change.SetRefused) {
		return res, refused, nil, err
	}
	for _, r := range refused {
		delete(byBlock, [2]string{r.Doc, r.Block})
	}
	var meta []Draft
	for _, d := range drafts {
		if byBlock[[2]string{d.Doc, d.Before.ID()}] && !d.Before.Meta(d.After).Empty() {
			meta = append(meta, d)
		}
	}
	moved, err := home.WriteMeta(ctx, meta)
	if err != nil {
		return res, refused, nil, err
	}
	for _, r := range moved {
		delete(byBlock, [2]string{r.Doc, r.Block})
	}
	refused = append(refused, moved...)
	logLeftOut(ctx, home, tool, refused)
	landed := make([]string, 0, len(byBlock))
	for _, d := range drafts {
		if byBlock[[2]string{d.Doc, d.Before.ID()}] {
			landed = append(landed, d.Before.ID())
		}
	}
	return res, refused, landed, nil
}

// logLeftOut records the blocks a tool's commit left out, counted by the code
// that left them out, so what a job reports it produced can be read beside
// what landed.
func logLeftOut(ctx context.Context, home *Home, tool string, refused []Refusal) {
	if len(refused) == 0 {
		return
	}
	codes := map[change.Code]int{}
	for _, r := range refused {
		var code change.Code
		if r.Error != nil {
			code = r.Error.Code
		}
		codes[code]++
	}
	first := refused[0]
	reason := ""
	if first.Error != nil {
		reason = first.Error.Message
	}
	slog.InfoContext(ctx, "changes: a tool's commit left blocks out",
		"tool", tool, "project", home.ProjectID, "stream", home.stream(), "left_out", len(refused),
		"codes", fmt.Sprint(codes), "first_item", first.Doc, "first_block", first.Block, "first_reason", reason)
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
// was left out, the operations that change set held (in the order of its
// result), and the blocks it left out. A refusal that names no block (the
// change set as a whole) ends it with that result.
func ApplyEach(ctx context.Context, svc *change.Service, set change.Set, actor change.Actor) (*change.Result, []change.Op, []Refusal, error) {
	var refused []Refusal
	for {
		if len(set.Ops) == 0 {
			return nil, nil, refused, nil
		}
		res, err := svc.Apply(ctx, set, actor)
		if err != nil {
			return nil, nil, refused, err
		}
		if res.Status != change.SetRefused {
			return res, set.Ops, refused, nil
		}
		type blockRef struct{ doc, block string }
		drop := map[blockRef]bool{}
		for i, r := range res.Ops {
			if r.Status != change.OpRefused {
				continue
			}
			at := set.Ops[i].At
			if at.Block == "" {
				return res, set.Ops, refused, nil
			}
			ref := blockRef{at.Doc, at.Block}
			if !drop[ref] {
				drop[ref] = true
				refused = append(refused, Refusal{Doc: at.Doc, Block: at.Block, Error: r.Error})
			}
		}
		if len(drop) == 0 {
			return res, set.Ops, refused, nil
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
