package tools

import (
	"context"
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"
	"sync"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/model"
)

// DocumentSegCount holds segment counts for a single document.
type DocumentSegCount struct {
	URI            string `json:"uri"`
	SourceSegments int    `json:"source_segments"`
	TargetSegments int    `json:"target_segments,omitempty"`
	BlockCount     int    `json:"block_count"`
}

// SegCountSummary is the aggregated result from the segment-count collectors.
type SegCountSummary struct {
	TotalSourceSegments int                         `json:"total_source_segments"`
	TotalTargetSegments int                         `json:"total_target_segments,omitempty"`
	DocumentCount       int                         `json:"document_count"`
	Documents           map[string]DocumentSegCount `json:"documents"`
}

// FormatTable writes an aligned text table to w.
func (s *SegCountSummary) FormatTable(w io.Writer) {
	hasTarget := s.TotalTargetSegments > 0
	for _, doc := range s.Documents {
		if doc.TargetSegments > 0 {
			hasTarget = true
		}
	}

	fileWidth := len("FILE")
	for _, doc := range s.Documents {
		if len(doc.URI) > fileWidth {
			fileWidth = len(doc.URI)
		}
	}
	fileWidth += 4

	// Header.
	fmt.Fprintf(w, "%-*s %6s  %15s", fileWidth, "FILE", "BLOCKS", "SOURCE SEGMENTS")
	if hasTarget {
		fmt.Fprintf(w, "  %15s", "TARGET SEGMENTS")
	}
	fmt.Fprintln(w)

	uris := make([]string, 0, len(s.Documents))
	for uri := range s.Documents {
		uris = append(uris, uri)
	}
	slices.Sort(uris)

	for _, uri := range uris {
		doc := s.Documents[uri]
		fmt.Fprintf(w, "%-*s %6d  %15d", fileWidth, doc.URI, doc.BlockCount, doc.SourceSegments)
		if hasTarget {
			fmt.Fprintf(w, "  %15d", doc.TargetSegments)
		}
		fmt.Fprintln(w)
	}

	totalWidth := fileWidth + 6 + 2 + 15
	if hasTarget {
		totalWidth += 2 + 15
	}
	fmt.Fprintln(w, strings.Repeat("─", totalWidth))

	fmt.Fprintf(w, "%-*s %6s  %15d", fileWidth,
		fmt.Sprintf("Total (%d files)", s.DocumentCount), "", s.TotalSourceSegments)
	if hasTarget {
		fmt.Fprintf(w, "  %15d", s.TotalTargetSegments)
	}
	fmt.Fprintln(w)
}

// SegCountCollector aggregates segment counts from documents processed by the
// segment-count tool. It reads PropSegCountSource and PropSegCountTarget
// properties from blocks. It implements flow.Collector and is safe for
// concurrent use.
type SegCountCollector struct {
	mu          sync.Mutex
	perDocument map[string]DocumentSegCount
	totalSource int
	totalTarget int
}

// NewSegCountCollector creates a new SegCountCollector.
func NewSegCountCollector() *SegCountCollector {
	return &SegCountCollector{
		perDocument: make(map[string]DocumentSegCount),
	}
}

// Collect reads segment count properties from block parts and aggregates them.
func (sc *SegCountCollector) Collect(_ context.Context, item *flow.Item, parts []*model.Part) error {
	doc := DocumentSegCount{URI: item.Input.URI}

	for _, p := range parts {
		if p.Type != model.PartBlock {
			continue
		}
		block, ok := p.Resource.(*model.Block)
		if !ok || !block.Translatable {
			continue
		}
		doc.BlockCount++
		if sf, ok := model.AnnoAs[*SegCountFacet](block, string(model.FacetSegCount)); ok {
			doc.SourceSegments += sf.Source
			doc.TargetSegments += sf.Target
		}
	}

	sc.mu.Lock()
	defer sc.mu.Unlock()
	sc.perDocument[doc.URI] = doc
	sc.totalSource += doc.SourceSegments
	sc.totalTarget += doc.TargetSegments
	return nil
}

// Result returns the aggregated segment count summary.
func (sc *SegCountCollector) Result() (flow.CollectorResult, error) {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	docs := make(map[string]DocumentSegCount, len(sc.perDocument))
	maps.Copy(docs, sc.perDocument)

	return flow.CollectorResult{
		Name: "segment-count",
		Data: &SegCountSummary{
			TotalSourceSegments: sc.totalSource,
			TotalTargetSegments: sc.totalTarget,
			DocumentCount:       len(docs),
			Documents:           docs,
		},
	}, nil
}

// StreamingSegCountCollector counts segments inline as Parts flow through the
// pipeline via Observe(), without buffering the full Part stream. It implements
// flow.StreamingCollector and is safe for concurrent use across files.
type StreamingSegCountCollector struct {
	mu          sync.Mutex
	currentURI  string
	perDocument map[string]*DocumentSegCount
	totalSource int
	totalTarget int
}

// NewStreamingSegCountCollector creates a new StreamingSegCountCollector.
func NewStreamingSegCountCollector() *StreamingSegCountCollector {
	return &StreamingSegCountCollector{
		perDocument: make(map[string]*DocumentSegCount),
	}
}

// Observe is called inline as each Part flows through the pipeline. It reads
// segment count properties set by the segment-count tool and accumulates them.
func (sc *StreamingSegCountCollector) Observe(part *model.Part) {
	if part.Type != model.PartBlock {
		return
	}
	block, ok := part.Resource.(*model.Block)
	if !ok || !block.Translatable {
		return
	}

	sc.mu.Lock()
	defer sc.mu.Unlock()

	uri := sc.currentURI
	doc := sc.perDocument[uri]
	if doc == nil {
		doc = &DocumentSegCount{URI: uri}
		sc.perDocument[uri] = doc
	}

	doc.BlockCount++
	if sf, ok := model.AnnoAs[*SegCountFacet](block, string(model.FacetSegCount)); ok {
		doc.SourceSegments += sf.Source
		sc.totalSource += sf.Source
		doc.TargetSegments += sf.Target
		sc.totalTarget += sf.Target
	}
}

// Collect sets the document context for subsequent Observe() calls. The parts
// parameter is ignored since counting happens inline via Observe().
func (sc *StreamingSegCountCollector) Collect(_ context.Context, item *flow.Item, _ []*model.Part) error {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	sc.currentURI = item.Input.URI
	return nil
}

// Result returns the aggregated segment count summary.
func (sc *StreamingSegCountCollector) Result() (flow.CollectorResult, error) {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	docs := make(map[string]DocumentSegCount, len(sc.perDocument))
	for k, v := range sc.perDocument {
		docs[k] = *v
	}

	return flow.CollectorResult{
		Name: "segment-count",
		Data: &SegCountSummary{
			TotalSourceSegments: sc.totalSource,
			TotalTargetSegments: sc.totalTarget,
			DocumentCount:       len(docs),
			Documents:           docs,
		},
	}, nil
}

// Verify collector interface conformance.
var (
	_ flow.Collector          = (*SegCountCollector)(nil)
	_ flow.StreamingCollector = (*StreamingSegCountCollector)(nil)
)
