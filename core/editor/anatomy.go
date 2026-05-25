package editor

import "github.com/neokapi/neokapi/core/model"

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
	Translatable bool                   `json:"translatable,omitempty"`
	Source       []model.Run            `json:"source,omitempty"`
	Targets      map[string][]model.Run `json:"targets,omitempty"`
	Segments     []SegmentSpan          `json:"segments,omitempty"`
	HasSkeleton  bool                   `json:"hasSkeleton,omitempty"`

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
// sequence and (when the block is multi-segment) the segment boundary overlay.
func blockNode(b *model.Block) *ContentNode {
	n := &ContentNode{
		Kind:         "block",
		ID:           b.ID,
		Name:         b.Name,
		Translatable: b.Translatable,
		Properties:   nonEmptyProps(b.Properties),
		HasSkeleton:  b.Skeleton != nil,
		Source:       b.SourceRuns(),
		Segments:     segmentSpans(b),
	}
	if len(b.Targets) > 0 {
		n.Targets = make(map[string][]model.Run, len(b.Targets))
		for _, loc := range b.TargetLocales() {
			n.Targets[string(loc)] = b.TargetRuns(loc)
		}
	}
	return n
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
