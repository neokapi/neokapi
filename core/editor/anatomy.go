package editor

import (
	"fmt"
	"sort"

	"github.com/neokapi/neokapi/core/model"
)

// ContentTree is a hierarchical, JSON-serializable view of a document's Part
// stream, purpose-built for the "Anatomy" learning explorer in the docs site.
// It shows how a format reader decomposes raw bytes into the content model:
// nested Layers and Groups containing Blocks (with their run sequences), Data
// (non-translatable skeleton structure) and Media.
//
// Unlike [BlockIndex] — which flattens a Block's source to plain strings for
// reconstruction — ContentTree preserves the run sequence so a reader can see
// where inline placeholders, paired codes and plurals live. The Block atom is
// the run sequence ([]model.Run, via Block.SourceRuns / TargetRuns), matching
// the run-native content model (RFC 0001). Segment boundaries are exposed only
// as a secondary overlay view ([]SegmentSpan, by run-index range), sourced from
// the Block's stand-off segmentation overlay (AD-002).
type ContentTree struct {
	Format string         `json:"format"`
	Root   []*ContentNode `json:"root"`
	Stats  ContentStats   `json:"stats"`
}

// ContentStats are document-wide counts, handy for a learner's at-a-glance
// "what's in this file" summary.
type ContentStats struct {
	Layers int `json:"layers"`
	Groups int `json:"groups"`
	Blocks int `json:"blocks"`
	Data   int `json:"data"`
	Media  int `json:"media"`
	Runs   int `json:"runs"`
}

// ContentNode is one node in a ContentTree. Kind discriminates: container
// kinds ("layer", "group") carry Children; leaf kinds ("block", "data",
// "media") carry their own payload.
type ContentNode struct {
	Kind       string            `json:"kind"` // layer | group | block | data | media
	ID         string            `json:"id"`
	Name       string            `json:"name,omitempty"`
	Properties map[string]string `json:"properties,omitempty"`

	// Layer fields.
	Format   string `json:"format,omitempty"`
	Locale   string `json:"locale,omitempty"`
	ParentID string `json:"parentId,omitempty"`

	// Block fields. Source/Targets are flattened run sequences (the Block
	// atom); Segments is the run-index boundary overlay view.
	Type         string                 `json:"type,omitempty"`
	Translatable bool                   `json:"translatable,omitempty"`
	SourceLocale string                 `json:"sourceLocale,omitempty"`
	Source       []model.Run            `json:"source,omitempty"`
	Targets      map[string][]model.Run `json:"targets,omitempty"`
	// TargetMeta carries each variant's lifecycle status and provenance,
	// keyed identically to Targets.
	TargetMeta map[string]*TargetMeta `json:"targetMeta,omitempty"`
	Segments   []SegmentSpan          `json:"segments,omitempty"`
	// Overlays are the block's stand-off interpretations (segmentation, terms,
	// entities, QA findings, alignment), each with its spans' extracted text.
	Overlays []OverlayView `json:"overlays,omitempty"`
	// Annotations are block-level metadata (alt-translations, notes, generic).
	Annotations        []AnnotationView `json:"annotations,omitempty"`
	HasSkeleton        bool             `json:"hasSkeleton,omitempty"`
	IsReferent         bool             `json:"isReferent,omitempty"`
	PreserveWhitespace bool             `json:"preserveWhitespace,omitempty"`
	Identity           string           `json:"identity,omitempty"`

	// Leaf summary (data / media): a short human-readable label.
	MimeType string `json:"mimeType,omitempty"`
	Summary  string `json:"summary,omitempty"`

	// Container children (layer / group).
	Children []*ContentNode `json:"children,omitempty"`
}

// SegmentSpan is the overlay view of a Block's segment boundaries, expressed as
// a half-open run-index range [Start, End) over the flattened Source runs,
// derived from the Block's stand-off segmentation overlay (AD-002).
type SegmentSpan struct {
	ID    string `json:"id"`
	Start int    `json:"start"` // first run index, inclusive
	End   int    `json:"end"`   // one past the last run index
}

// TargetMeta is the lifecycle + provenance view of one committed target variant.
type TargetMeta struct {
	Status  string      `json:"status,omitempty"`
	Score   float64     `json:"score,omitempty"`
	Origin  *OriginView `json:"origin,omitempty"`
	Tone    string      `json:"tone,omitempty"`
	Channel string      `json:"channel,omitempty"`
}

// OriginView mirrors model.Origin for the wire.
type OriginView struct {
	Kind      string `json:"kind,omitempty"`
	Engine    string `json:"engine,omitempty"`
	Tool      string `json:"tool,omitempty"`
	Reference string `json:"reference,omitempty"`
	Timestamp string `json:"timestamp,omitempty"`
}

// OverlaySpanView is one run-anchored span of an overlay with its extracted text.
type OverlaySpanView struct {
	ID        string            `json:"id,omitempty"`
	Range     model.RunRange    `json:"range"`
	Props     map[string]string `json:"props,omitempty"`
	Text      string            `json:"text,omitempty"`
	Ignorable bool              `json:"ignorable,omitempty"`
}

// OverlayView is the wire view of a stand-off overlay over one side of a block.
type OverlayView struct {
	Type  string            `json:"type"`
	Side  string            `json:"side"` // "source" or a variant key
	Layer string            `json:"layer,omitempty"`
	Spans []OverlaySpanView `json:"spans,omitempty"`
}

// AnnotationView is the wire view of a block-level annotation.
type AnnotationView struct {
	Key     string         `json:"key"`
	Type    string         `json:"type"`
	Summary string         `json:"summary,omitempty"`
	Fields  map[string]any `json:"fields,omitempty"`
}

// BuildContentTree walks a Part stream (in document order, as emitted by a
// format reader) and assembles the hierarchical ContentTree. LayerStart and
// GroupStart open containers; their matching End parts close them; Block, Data
// and Media attach to the innermost open container (or the root when none is
// open). Malformed (unbalanced) streams are tolerated: an End with no matching
// open container is ignored rather than panicking.
func BuildContentTree(parts []*model.Part, format string) *ContentTree {
	tree := &ContentTree{Format: format, Root: []*ContentNode{}}

	// stack holds the currently-open container nodes. attach appends a node
	// to the innermost open container, or to the root when the stack is empty.
	var stack []*ContentNode
	attach := func(n *ContentNode) {
		if len(stack) == 0 {
			tree.Root = append(tree.Root, n)
			return
		}
		top := stack[len(stack)-1]
		top.Children = append(top.Children, n)
	}
	// popKind closes the innermost open container if it matches kind.
	popKind := func(kind string) {
		if len(stack) > 0 && stack[len(stack)-1].Kind == kind {
			stack = stack[:len(stack)-1]
		}
	}

	for _, p := range parts {
		if p == nil || p.Resource == nil {
			continue
		}
		switch p.Type {
		case model.PartLayerStart:
			l, ok := p.Resource.(*model.Layer)
			if !ok {
				continue
			}
			n := &ContentNode{
				Kind:       "layer",
				ID:         l.ID,
				Name:       l.Name,
				Format:     l.Format,
				Locale:     string(l.Locale),
				ParentID:   l.ParentID,
				Properties: nonEmptyProps(l.Properties),
			}
			attach(n)
			stack = append(stack, n)
			tree.Stats.Layers++

		case model.PartLayerEnd:
			popKind("layer")

		case model.PartGroupStart:
			g, ok := p.Resource.(*model.GroupStart)
			if !ok {
				continue
			}
			n := &ContentNode{
				Kind:       "group",
				ID:         g.ID,
				Name:       g.Name,
				Properties: nonEmptyProps(g.Properties),
			}
			attach(n)
			stack = append(stack, n)
			tree.Stats.Groups++

		case model.PartGroupEnd:
			popKind("group")

		case model.PartBlock:
			b, ok := p.Resource.(*model.Block)
			if !ok {
				continue
			}
			attach(blockNode(b))
			tree.Stats.Blocks++
			tree.Stats.Runs += len(b.SourceRuns())

		case model.PartData:
			d, ok := p.Resource.(*model.Data)
			if !ok {
				continue
			}
			attach(&ContentNode{
				Kind:        "data",
				ID:          d.ID,
				Name:        d.Name,
				Properties:  nonEmptyProps(d.Properties),
				HasSkeleton: d.Skeleton != nil,
				Summary:     orFallback(d.Name, "structural data"),
			})
			tree.Stats.Data++

		case model.PartMedia:
			m, ok := p.Resource.(*model.Media)
			if !ok {
				continue
			}
			attach(&ContentNode{
				Kind:       "media",
				ID:         m.ID,
				MimeType:   m.MimeType,
				Properties: nonEmptyProps(m.Properties),
				Summary:    mediaSummary(m),
			})
			tree.Stats.Media++
		}
	}

	return tree
}

// blockNode builds the leaf node for a translatable Block, capturing its run
// sequence, every committed target variant (with lifecycle + provenance), the
// segment boundary overlay, all stand-off overlays (with extracted span text)
// and block-level annotations — the full content-model view of the part.
func blockNode(b *model.Block) *ContentNode {
	n := &ContentNode{
		Kind:               "block",
		ID:                 b.ID,
		Name:               b.Name,
		Type:               b.Type,
		Translatable:       b.Translatable,
		SourceLocale:       string(b.SourceLocale),
		Properties:         nonEmptyProps(b.Properties),
		HasSkeleton:        b.Skeleton != nil,
		IsReferent:         b.IsReferent,
		PreserveWhitespace: b.PreserveWhitespace,
		Source:             b.SourceRuns(),
		Segments:           segmentSpans(b),
		Overlays:           overlayViews(b),
		Annotations:        annotationViews(b),
	}
	if b.Identity != nil {
		n.Identity = b.Identity.ContentHash
	}
	if len(b.Targets) > 0 {
		n.Targets = make(map[string][]model.Run, len(b.Targets))
		n.TargetMeta = make(map[string]*TargetMeta, len(b.Targets))
		for _, key := range sortedVariantKeys(b.Targets) {
			t := b.Targets[key]
			if t == nil {
				continue
			}
			label := variantLabel(key)
			n.Targets[label] = t.Runs
			n.TargetMeta[label] = targetMeta(key, t)
		}
	}
	return n
}

// variantLabel renders a VariantKey as its wire/text form (locale, or
// "locale;tone=…;channel=…"), matching the key used for Targets/TargetMeta.
func variantLabel(k model.VariantKey) string {
	b, err := k.MarshalText()
	if err != nil {
		return string(k.Locale)
	}
	return string(b)
}

// sortedVariantKeys returns the target variant keys in a stable order so the
// serialized view is deterministic.
func sortedVariantKeys(targets map[model.VariantKey]*model.Target) []model.VariantKey {
	keys := make([]model.VariantKey, 0, len(targets))
	for k := range targets {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return variantLabel(keys[i]) < variantLabel(keys[j]) })
	return keys
}

func targetMeta(key model.VariantKey, t *model.Target) *TargetMeta {
	m := &TargetMeta{Status: string(t.Status), Score: t.Score, Tone: key.Tone, Channel: key.Channel}
	if o := t.Origin; o != (model.Origin{}) {
		m.Origin = &OriginView{Kind: o.Kind, Engine: o.Engine, Tool: o.Tool, Reference: o.Reference, Timestamp: o.Timestamp}
	}
	return m
}

// overlayViews serializes every stand-off overlay on the block, resolving each
// span's covered text from the side it annotates (source or a target variant).
func overlayViews(b *model.Block) []OverlayView {
	if len(b.Overlays) == 0 {
		return nil
	}
	out := make([]OverlayView, 0, len(b.Overlays))
	for i := range b.Overlays {
		o := &b.Overlays[i]
		// Overlays are positional by construction; block-scoped annotations are
		// rendered separately by annotationViews via AnnoMap.
		side := "source"
		runs := b.Source
		if o.Variant != nil {
			side = variantLabel(*o.Variant)
			if t := b.Targets[*o.Variant]; t != nil {
				runs = t.Runs
			}
		}
		spans := make([]OverlaySpanView, 0, len(o.Spans))
		for _, s := range o.Spans {
			spans = append(spans, OverlaySpanView{
				ID:        s.ID,
				Range:     s.Range,
				Props:     nonEmptyProps(s.Props),
				Text:      model.RunsText(s.Range.ExtractRuns(runs)),
				Ignorable: s.Ignorable(),
			})
		}
		out = append(out, OverlayView{Type: string(o.Type), Side: side, Layer: o.Layer, Spans: spans})
	}
	return out
}

// annotationViews serializes the block's annotations, summarising the well-known
// kinds (alt-translation, note) and passing generic ones through as fields.
func annotationViews(b *model.Block) []AnnotationView {
	annos := b.AnnoMap()
	if len(annos) == 0 {
		return nil
	}
	keys := make([]string, 0, len(annos))
	for k := range annos {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]AnnotationView, 0, len(keys))
	for _, k := range keys {
		// An alt-translation collection expands to one view per candidate so
		// each match renders individually (keyed "alt-translation[i]").
		if alts, ok := annos[k].(*model.AltTranslations); ok {
			for i, alt := range alts.Items {
				out = append(out, altTranslationView(fmt.Sprintf("%s[%d]", k, i), alt))
			}
			continue
		}
		out = append(out, annotationView(k, annos[k]))
	}
	return out
}

// altTranslationView renders a single alt-translation candidate.
func altTranslationView(key string, t *model.AltTranslation) AnnotationView {
	return AnnotationView{
		Key:     key,
		Type:    "alt-translation",
		Summary: model.RunsText(t.Target),
		Fields: map[string]any{
			"locale":    string(t.Locale),
			"matchType": string(t.MatchType),
			"score":     t.Score,
			"origin":    t.Origin,
			"engine":    t.Engine,
			"source":    model.RunsText(t.Source),
		},
	}
}

func annotationView(key string, a any) AnnotationView {
	v := AnnotationView{Key: key}
	if at, ok := a.(interface{ AnnotationType() string }); ok {
		v.Type = at.AnnotationType()
	}
	switch t := a.(type) {
	case *model.NoteAnnotation:
		v.Summary = t.Text
		v.Fields = map[string]any{"from": t.From, "priority": t.Priority, "annotates": t.Annotates}
	case *model.GenericAnnotation:
		v.Type = t.Kind
		v.Fields = t.Fields
	}
	return v
}

// segmentSpans derives the run-index boundary spans from a Block's source
// segmentation overlay (AD-002). With no overlay (or a single span) there is no
// meaningful boundary, so it returns nil — the learner sees just the run
// sequence.
func segmentSpans(b *model.Block) []SegmentSpan {
	seg := b.SourceSegmentation()
	if seg == nil || len(seg.Spans) <= 1 {
		return nil
	}
	spans := make([]SegmentSpan, 0, len(seg.Spans))
	for _, s := range seg.Spans {
		spans = append(spans, SegmentSpan{ID: s.ID, Start: s.Range.StartRun, End: s.Range.EndRun})
	}
	return spans
}

// mediaSummary produces a short label for a Media leaf.
func mediaSummary(m *model.Media) string {
	switch {
	case m.Filename != "":
		return m.Filename
	case m.MimeType != "":
		return m.MimeType
	default:
		return "media"
	}
}

// nonEmptyProps returns props unchanged, or nil when empty, so the JSON omits
// the field rather than emitting an empty object.
func nonEmptyProps(props map[string]string) map[string]string {
	if len(props) == 0 {
		return nil
	}
	return props
}

// orFallback returns s, or fallback when s is empty.
func orFallback(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}
