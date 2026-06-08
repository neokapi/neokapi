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

// DocumentWordCount holds word counts for a single document.
type DocumentWordCount struct {
	URI         string                 `json:"uri"`
	SourceWords int                    `json:"source_words"`
	TargetWords map[model.LocaleID]int `json:"target_words,omitempty"`
	BlockCount  int                    `json:"block_count"`
}

// WordCountSummary is the aggregated result from WordCountCollector.
type WordCountSummary struct {
	TotalSourceWords int                          `json:"total_source_words"`
	TotalTargetWords map[model.LocaleID]int       `json:"total_target_words,omitempty"`
	DocumentCount    int                          `json:"document_count"`
	Documents        map[string]DocumentWordCount `json:"documents"`
}

// FormatTable writes an aligned text table to w.
func (s *WordCountSummary) FormatTable(w io.Writer) {
	// Collect all target locales across all documents.
	localeSet := make(map[model.LocaleID]bool)
	for _, doc := range s.Documents {
		for loc := range doc.TargetWords {
			localeSet[loc] = true
		}
	}
	locales := make([]model.LocaleID, 0, len(localeSet))
	for loc := range localeSet {
		locales = append(locales, loc)
	}
	slices.Sort(locales)

	// Determine column widths.
	fileWidth := len("FILE")
	for _, doc := range s.Documents {
		if len(doc.URI) > fileWidth {
			fileWidth = len(doc.URI)
		}
	}
	// Add padding.
	fileWidth += 4

	// Header.
	fmt.Fprintf(w, "%-*s %6s  %12s", fileWidth, "FILE", "BLOCKS", "SOURCE WORDS")
	for _, loc := range locales {
		fmt.Fprintf(w, "  %12s", fmt.Sprintf("TARGET (%s)", loc))
	}
	fmt.Fprintln(w)

	// Sort documents by URI for deterministic output.
	uris := make([]string, 0, len(s.Documents))
	for uri := range s.Documents {
		uris = append(uris, uri)
	}
	slices.Sort(uris)

	// Data rows.
	for _, uri := range uris {
		doc := s.Documents[uri]
		fmt.Fprintf(w, "%-*s %6d  %12d", fileWidth, doc.URI, doc.BlockCount, doc.SourceWords)
		for _, loc := range locales {
			if n, ok := doc.TargetWords[loc]; ok {
				fmt.Fprintf(w, "  %12d", n)
			} else {
				fmt.Fprintf(w, "  %12s", "-")
			}
		}
		fmt.Fprintln(w)
	}

	// Separator line.
	totalWidth := fileWidth + 6 + 2 + 12
	for range locales {
		totalWidth += 2 + 12
	}
	fmt.Fprintln(w, strings.Repeat("\u2500", totalWidth))

	// Total row.
	fmt.Fprintf(w, "%-*s %6s  %12d", fileWidth,
		fmt.Sprintf("Total (%d files)", s.DocumentCount), "", s.TotalSourceWords)
	for _, loc := range locales {
		if n, ok := s.TotalTargetWords[loc]; ok {
			fmt.Fprintf(w, "  %12d", n)
		} else {
			fmt.Fprintf(w, "  %12s", "-")
		}
	}
	fmt.Fprintln(w)
}

// WordCountCollector aggregates word counts from documents processed
// by the WordCountTool. It reads PropWordCountSource and
// PropWordCountTargetPrefix properties from blocks.
//
// It implements flow.Collector and is safe for concurrent use.
type WordCountCollector struct {
	mu          sync.Mutex
	perDocument map[string]DocumentWordCount
	totalSource int
	totalTarget map[model.LocaleID]int
}

// NewWordCountCollector creates a new WordCountCollector.
func NewWordCountCollector() *WordCountCollector {
	return &WordCountCollector{
		perDocument: make(map[string]DocumentWordCount),
		totalTarget: make(map[model.LocaleID]int),
	}
}

// Collect reads word count properties from block parts and aggregates them.
func (wc *WordCountCollector) Collect(_ context.Context, item *flow.Item, parts []*model.Part) error {
	doc := DocumentWordCount{
		URI:         item.Input.URI,
		TargetWords: make(map[model.LocaleID]int),
	}

	for _, p := range parts {
		if p.Type != model.PartBlock {
			continue
		}
		block, ok := p.Resource.(*model.Block)
		if !ok || !block.Translatable {
			continue
		}
		doc.BlockCount++

		if wcf, ok := model.AnnoAs[*WordCountFacet](block, string(model.FacetWordCount)); ok {
			doc.SourceWords += wcf.Source
			for locale, n := range wcf.Targets {
				doc.TargetWords[locale] += n
			}
		}
	}

	wc.mu.Lock()
	defer wc.mu.Unlock()

	wc.perDocument[doc.URI] = doc
	wc.totalSource += doc.SourceWords
	for loc, n := range doc.TargetWords {
		wc.totalTarget[loc] += n
	}
	return nil
}

// Result returns the aggregated word count summary.
func (wc *WordCountCollector) Result() (flow.CollectorResult, error) {
	wc.mu.Lock()
	defer wc.mu.Unlock()

	// Copy the map to avoid external mutation.
	docs := make(map[string]DocumentWordCount, len(wc.perDocument))
	for k, v := range wc.perDocument {
		tw := make(map[model.LocaleID]int, len(v.TargetWords))
		maps.Copy(tw, v.TargetWords)
		v.TargetWords = tw
		docs[k] = v
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
