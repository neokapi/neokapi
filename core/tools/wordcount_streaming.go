package tools

import (
	"context"
	"maps"
	"sync"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/model"
)

// StreamingWordCountCollector counts words inline as Parts flow through the
// pipeline via Observe(), without buffering the full Part stream. It implements
// flow.StreamingCollector.
//
// Usage: wrap the word-count tool with flow.NewTappingTool to observe output
// parts as they are emitted, then call Result() for the final summary.
//
// Safe for concurrent use across multiple files.
type StreamingWordCountCollector struct {
	mu          sync.Mutex
	currentURI  string
	perDocument map[string]*DocumentWordCount
	totalSource int
	totalTarget map[model.LocaleID]int
}

// NewStreamingWordCountCollector creates a new StreamingWordCountCollector.
func NewStreamingWordCountCollector() *StreamingWordCountCollector {
	return &StreamingWordCountCollector{
		perDocument: make(map[string]*DocumentWordCount),
		totalTarget: make(map[model.LocaleID]int),
	}
}

// Observe is called inline as each Part flows through the pipeline.
// It reads word count properties set by WordCountTool and accumulates them.
func (wc *StreamingWordCountCollector) Observe(part *model.Part) {
	if part.Type != model.PartBlock {
		return
	}
	block, ok := part.Resource.(*model.Block)
	if !ok || !block.Translatable {
		return
	}

	wc.mu.Lock()
	defer wc.mu.Unlock()

	uri := wc.currentURI
	doc := wc.perDocument[uri]
	if doc == nil {
		doc = &DocumentWordCount{
			URI:         uri,
			TargetWords: make(map[model.LocaleID]int),
		}
		wc.perDocument[uri] = doc
	}

	doc.BlockCount++

	if wcf, ok := model.AnnoAs[*WordCountFacet](block, string(model.AnnoWordCount)); ok {
		doc.SourceWords += wcf.Source
		wc.totalSource += wcf.Source
		for locale, n := range wcf.Targets {
			doc.TargetWords[locale] += n
			wc.totalTarget[locale] += n
		}
	}
}

// Collect sets the document context for subsequent Observe() calls.
// The parts parameter is ignored since counting happens inline via Observe().
func (wc *StreamingWordCountCollector) Collect(_ context.Context, item *flow.Item, _ []*model.Part) error {
	wc.mu.Lock()
	defer wc.mu.Unlock()
	wc.currentURI = item.Input.URI
	return nil
}

// Result returns the aggregated word count summary.
func (wc *StreamingWordCountCollector) Result() (flow.CollectorResult, error) {
	wc.mu.Lock()
	defer wc.mu.Unlock()

	docs := make(map[string]DocumentWordCount, len(wc.perDocument))
	for k, v := range wc.perDocument {
		tw := make(map[model.LocaleID]int, len(v.TargetWords))
		maps.Copy(tw, v.TargetWords)
		cp := *v
		cp.TargetWords = tw
		docs[k] = cp
	}

	totalTarget := make(map[model.LocaleID]int, len(wc.totalTarget))
	maps.Copy(totalTarget, wc.totalTarget)

	return flow.CollectorResult{
		Name: "word-count",
		Data: &WordCountSummary{
			TotalSourceWords: wc.totalSource,
			TotalTargetWords: totalTarget,
			DocumentCount:    len(docs),
			Documents:        docs,
		},
	}, nil
}

// Verify StreamingWordCountCollector implements flow.StreamingCollector.
var _ flow.StreamingCollector = (*StreamingWordCountCollector)(nil)
