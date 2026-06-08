package tools

import (
	"fmt"
	"hash/fnv"
	"strconv"
	"strings"

	"github.com/neokapi/neokapi/core/schema"
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
	CaseSensitive bool `json:"caseSensitive,omitempty" schema:"title=Case Sensitive,description=Whether comparison is case-sensitive,default=true"`
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

// RepetitionAnalysisSchema returns the auto-generated schema for the repetition-analysis tool.
func RepetitionAnalysisSchema() *schema.ComponentSchema {
	return schema.FromStruct(&RepetitionAnalysisConfig{}, schema.ToolMeta{
		ID:          "repetition-analysis",
		Category:    schema.CategoryAnalysis,
		DisplayName: "Repetition Analysis",
		Description: "Identify repeated segments across files for TM leverage",
	})
}

// NewRepetitionAnalysisFromConfig creates a repetition-analysis tool from a config map.
func NewRepetitionAnalysisFromConfig(config map[string]any, targetLang string) (tool.Tool, error) {
	var cfg RepetitionAnalysisConfig
	if err := schema.ApplyConfig(config, &cfg); err != nil {
		return nil, fmt.Errorf("repetition-analysis config: %w", err)
	}
	return NewRepetitionAnalysisTool(&cfg), nil
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

	t.Annotate = func(v tool.BlockView) error {
		// Lazy-init on first block (Process creates a fresh closure scope per run
		// via BaseTool, but Annotate is reused — so we init on nil).
		if groups == nil {
			groups = make(map[string]*repGroup)
		}

		if !v.Translatable() {
			return nil
		}

		conf := t.Cfg.(*RepetitionAnalysisConfig)

		normalized := strings.TrimSpace(v.SourceText())
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
		g.blockIDs = append(g.blockIDs, v.ID())

		if g.count == 1 {
			v.SetProperty(PropRepetitionStatus, "first-occurrence")
		} else {
			v.SetProperty(PropRepetitionStatus, "repetition")
		}

		v.SetProperty(PropRepetitionGroup, groupKey)
		v.SetProperty(PropRepetitionCount, strconv.Itoa(g.count))
		v.SetProperty(PropRepetitionIndex, strconv.Itoa(g.count))

		return nil
	}

	return t
}

// hashText returns a compact hex hash of the given string using FNV-64a.
func hashText(s string) string {
	h := fnv.New64a()
	h.Write([]byte(s))
	return fmt.Sprintf("%016x", h.Sum64())
}
