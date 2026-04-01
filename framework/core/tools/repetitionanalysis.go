package tools

import (
	"fmt"
	"hash/fnv"
	"strconv"
	"strings"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
)

// Repetition analysis property keys stored on Block.Properties.
const (
	PropRepetitionStatus = "repetition-status" // "unique", "first-occurrence", "repetition"
	PropRepetitionGroup  = "repetition-group"  // hash key linking repeated segments
	PropRepetitionCount  = "repetition-count"  // total occurrences of this text (updated on each occurrence)
	PropRepetitionIndex  = "repetition-index"  // 1-based index within the repetition group
)

// RepetitionAnalysisConfig holds configuration for the repetition analysis tool.
type RepetitionAnalysisConfig struct {
	CaseSensitive bool `schema:"description=Whether comparison is case-sensitive,default=true"` // Whether comparison is case-sensitive (default: true)
}

// ToolName returns the tool name this config applies to.
func (c *RepetitionAnalysisConfig) ToolName() string { return "repetition-analysis" }

// Reset restores default values.
func (c *RepetitionAnalysisConfig) Reset() {
	c.CaseSensitive = true
}

// Validate checks configuration validity.
func (c *RepetitionAnalysisConfig) Validate() error {
	return nil
}

// repGroup tracks a group of blocks that share the same normalized source text.
type repGroup struct {
	count    int
	blockIDs []string
}

// NewRepetitionAnalysisTool creates a new repetition analysis tool.
// It tracks seen source text segments across all blocks in the pipeline
// and tags each block with its repetition status.
func NewRepetitionAnalysisTool(cfg *RepetitionAnalysisConfig) *tool.BaseTool {
	t := &tool.BaseTool{
		ToolName:        "repetition-analysis",
		ToolDescription: "Analyzes source text repetitions across blocks in the pipeline",
		Cfg:             cfg,
	}

	// Stateful: captured by the closure and reset on each Process() call.
	var groups map[string]*repGroup

	t.HandleBlockFn = func(part *model.Part) (*model.Part, error) {
		// Lazy-init on first block (Process creates a fresh closure scope per run
		// via BaseTool, but HandleBlockFn is reused — so we init on nil).
		if groups == nil {
			groups = make(map[string]*repGroup)
		}

		block, ok := part.Resource.(*model.Block)
		if !ok {
			return part, nil
		}
		if !block.Translatable {
			return part, nil
		}

		conf := t.Cfg.(*RepetitionAnalysisConfig)

		if block.Properties == nil {
			block.Properties = make(map[string]string)
		}

		sourceText := block.SourceText()
		normalized := strings.TrimSpace(sourceText)
		if !conf.CaseSensitive {
			normalized = strings.ToLower(normalized)
		}

		groupKey := hashText(normalized)

		g, seen := groups[groupKey]
		if !seen {
			g = &repGroup{}
			groups[groupKey] = g
		}
		g.count++
		g.blockIDs = append(g.blockIDs, block.ID)

		if g.count == 1 {
			block.Properties[PropRepetitionStatus] = "first-occurrence"
		} else {
			block.Properties[PropRepetitionStatus] = "repetition"
		}

		block.Properties[PropRepetitionGroup] = groupKey
		block.Properties[PropRepetitionCount] = strconv.Itoa(g.count)
		block.Properties[PropRepetitionIndex] = strconv.Itoa(g.count)

		return part, nil
	}

	return t
}

// hashText returns a compact hex hash of the given string using FNV-64a.
func hashText(s string) string {
	h := fnv.New64a()
	h.Write([]byte(s))
	return fmt.Sprintf("%016x", h.Sum64())
}
