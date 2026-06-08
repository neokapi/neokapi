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

// ScopingCategory holds aggregated counts for a single scoping category.
type ScopingCategory struct {
	Name       string `json:"name"`
	WordCount  int    `json:"word_count"`
	BlockCount int    `json:"block_count"`
}

// ScopingSummary is the aggregated result from ScopingCollector.
type ScopingSummary struct {
	Categories    map[string]*ScopingCategory `json:"categories"`
	TotalWords    int                         `json:"total_words"`
	TotalBlocks   int                         `json:"total_blocks"`
	DocumentCount int                         `json:"document_count"`
}

// FormatTable writes an aligned text table to w.
func (s *ScopingSummary) FormatTable(w io.Writer) {
	// Sort categories for deterministic output.
	names := make([]string, 0, len(s.Categories))
	for name := range s.Categories {
		names = append(names, name)
	}
	slices.Sort(names)

	// Header.
	fmt.Fprintf(w, "%-16s %8s  %12s\n", "CATEGORY", "BLOCKS", "WORDS")

	// Data rows.
	for _, name := range names {
		cat := s.Categories[name]
		fmt.Fprintf(w, "%-16s %8d  %12d\n", cat.Name, cat.BlockCount, cat.WordCount)
	}

	// Separator.
	fmt.Fprintln(w, strings.Repeat("\u2500", 38))

	// Total row.
	fmt.Fprintf(w, "%-16s %8d  %12d\n",
		fmt.Sprintf("Total (%d files)", s.DocumentCount), s.TotalBlocks, s.TotalWords)
}

// ScopingCollector aggregates scoping categories from documents processed
// by the ScopingReportTool. It reads PropScopingCategory and
// PropWordCountSource properties from blocks.
//
// It implements flow.Collector and is safe for concurrent use.
type ScopingCollector struct {
	mu          sync.Mutex
	categories  map[string]*ScopingCategory
	totalWords  int
	totalBlocks int
	documents   map[string]bool
}

// NewScopingCollector creates a new ScopingCollector.
func NewScopingCollector() *ScopingCollector {
	return &ScopingCollector{
		categories: make(map[string]*ScopingCategory),
		documents:  make(map[string]bool),
	}
}

// Collect reads scoping category and word count properties from block parts
// and aggregates them.
func (sc *ScopingCollector) Collect(_ context.Context, item *flow.Item, parts []*model.Part) error {
	// Local accumulators to minimize lock time.
	localCats := make(map[string]*ScopingCategory)

	for _, p := range parts {
		if p.Type != model.PartBlock {
			continue
		}
		block, ok := p.Resource.(*model.Block)
		if !ok || !block.Translatable {
			continue
		}

		category := block.Properties[PropScopingCategory]
		if category == "" {
			category = ScopingNew
		}

		wordCount := 0
		if wc, ok := model.AnnoAs[*WordCountFacet](block, string(model.AnnoWordCount)); ok {
			wordCount = wc.Source
		}

		cat, exists := localCats[category]
		if !exists {
			cat = &ScopingCategory{Name: category}
			localCats[category] = cat
		}
		cat.BlockCount++
		cat.WordCount += wordCount
	}

	sc.mu.Lock()
	defer sc.mu.Unlock()

	sc.documents[item.Input.URI] = true

	for name, local := range localCats {
		cat, exists := sc.categories[name]
		if !exists {
			cat = &ScopingCategory{Name: name}
			sc.categories[name] = cat
		}
		cat.BlockCount += local.BlockCount
		cat.WordCount += local.WordCount
		sc.totalBlocks += local.BlockCount
		sc.totalWords += local.WordCount
	}

	return nil
}

// Result returns the aggregated scoping summary.
func (sc *ScopingCollector) Result() (flow.CollectorResult, error) {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	// Copy the map to avoid external mutation.
	cats := make(map[string]*ScopingCategory, len(sc.categories))
	for k, v := range sc.categories {
		cp := *v
		cats[k] = &cp
	}

	docs := make(map[string]bool, len(sc.documents))
	maps.Copy(docs, sc.documents)

	return flow.CollectorResult{
		Name: "scoping-report",
		Data: &ScopingSummary{
			Categories:    cats,
			TotalWords:    sc.totalWords,
			TotalBlocks:   sc.totalBlocks,
			DocumentCount: len(docs),
		},
	}, nil
}
