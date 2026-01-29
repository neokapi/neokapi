package tools

import (
	"context"
	"strconv"
	"sync"

	"github.com/asgeirf/gokapi/core/flow"
	"github.com/asgeirf/gokapi/core/model"
)

// DocumentWordCount holds word counts for a single document.
type DocumentWordCount struct {
	URI         string
	SourceWords int
	TargetWords int
	BlockCount  int
}

// WordCountSummary is the aggregated result from WordCountCollector.
type WordCountSummary struct {
	TotalSourceWords int
	TotalTargetWords int
	DocumentCount    int
	Documents        map[string]DocumentWordCount
}

// WordCountCollector aggregates word counts from documents processed
// by the WordCountTool. It reads PropWordCountSource and PropWordCountTarget
// properties from blocks.
//
// It implements flow.Collector and is safe for concurrent use.
type WordCountCollector struct {
	mu          sync.Mutex
	perDocument map[string]DocumentWordCount
	totalSource int
	totalTarget int
}

// NewWordCountCollector creates a new WordCountCollector.
func NewWordCountCollector() *WordCountCollector {
	return &WordCountCollector{
		perDocument: make(map[string]DocumentWordCount),
	}
}

// Collect reads word count properties from block parts and aggregates them.
func (wc *WordCountCollector) Collect(_ context.Context, item *flow.FlowItem, parts []*model.Part) error {
	var doc DocumentWordCount
	doc.URI = item.Input.URI

	for _, p := range parts {
		if p.Type != model.PartBlock {
			continue
		}
		block, ok := p.Resource.(*model.Block)
		if !ok || !block.Translatable {
			continue
		}
		doc.BlockCount++

		if v, ok := block.Properties[PropWordCountSource]; ok {
			n, _ := strconv.Atoi(v)
			doc.SourceWords += n
		}
		if v, ok := block.Properties[PropWordCountTarget]; ok {
			n, _ := strconv.Atoi(v)
			doc.TargetWords += n
		}
	}

	wc.mu.Lock()
	defer wc.mu.Unlock()

	wc.perDocument[doc.URI] = doc
	wc.totalSource += doc.SourceWords
	wc.totalTarget += doc.TargetWords
	return nil
}

// Result returns the aggregated word count summary.
func (wc *WordCountCollector) Result() (flow.CollectorResult, error) {
	wc.mu.Lock()
	defer wc.mu.Unlock()

	// Copy the map to avoid external mutation.
	docs := make(map[string]DocumentWordCount, len(wc.perDocument))
	for k, v := range wc.perDocument {
		docs[k] = v
	}

	return flow.CollectorResult{
		Name: "word-count",
		Data: &WordCountSummary{
			TotalSourceWords: wc.totalSource,
			TotalTargetWords: wc.totalTarget,
			DocumentCount:    len(docs),
			Documents:        docs,
		},
	}, nil
}
