package tools

import (
	"fmt"
	"strings"

	"github.com/neokapi/neokapi/core/brand"
	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/schema"
	"github.com/neokapi/neokapi/core/tool"
)

// SourceCheckConfig configures the source-readiness stamp.
type SourceCheckConfig struct {
	// BlockSeverity is the lowest finding severity that keeps a block from being
	// `checked`. Findings at or above it leave the source at the `authored`
	// baseline; anything below is tolerated. Default "major": minor/style nits do
	// not block source readiness, but a clear violation (major) or a
	// release-blocker (critical) does. Set "minor" to require a spotless source,
	// or "critical" to tolerate everything short of a release-blocker.
	BlockSeverity string `json:"blockSeverity,omitempty" schema:"title=Blocking severity,description=Lowest finding severity that keeps a block from being marked checked (minor|major|critical),enum=minor|major|critical"`
}

// ToolName returns the tool name this config applies to.
func (c *SourceCheckConfig) ToolName() string { return "source-check" }

// Reset restores default values.
func (c *SourceCheckConfig) Reset() { c.BlockSeverity = "" }

// Validate checks configuration validity.
func (c *SourceCheckConfig) Validate() error {
	switch strings.ToLower(strings.TrimSpace(c.BlockSeverity)) {
	case "", "minor", "major", "critical":
		return nil
	default:
		return fmt.Errorf("source-check: blockSeverity must be one of minor|major|critical, got %q", c.BlockSeverity)
	}
}

// blockThreshold returns the configured blocking severity weight (default major).
func (c *SourceCheckConfig) blockThreshold() int {
	sev := check.SeverityMajor
	switch strings.ToLower(strings.TrimSpace(c.BlockSeverity)) {
	case "minor":
		sev = check.SeverityMinor
	case "critical":
		sev = check.SeverityCritical
	}
	return check.SeverityWeight(sev)
}

// NewSourceCheckFromConfig creates a source-check tool from a config map.
func NewSourceCheckFromConfig(config map[string]any, _ string) (tool.Tool, error) {
	cfg := &SourceCheckConfig{}
	if err := schema.ApplyConfig(config, cfg); err != nil {
		return nil, fmt.Errorf("source-check config: %w", err)
	}
	return NewSourceCheckTool(cfg), nil
}

// NewSourceCheckTool creates the source-readiness stamp: a terminal check tool
// that promotes a block's SourceStatus from the `authored` baseline to `checked`
// once the source is clean, and demotes it back to `authored` when it is not.
// It is the source-side counterpart of a translation producer stamping a target
// — what keeps the author "in check".
//
// It is derived, not a checker itself: it reads the findings the upstream source
// checks already left on the block (the unified check.Findings annotation —
// terminology, do-not-translate, pattern, length, … — plus the brand-voice
// annotation) and decides readiness from their severities. So it belongs LAST in
// a source-check flow, after the checks that produce the findings. A block whose
// worst finding is below the configured blocking severity (default major) is
// `checked`; otherwise it falls to `authored`. An already-`approved` source that
// is still clean keeps its approval (a clean re-check never downgrades a human
// sign-off); a clean source below approval is raised to `checked`.
//
// It is read-only with respect to content (Annotate): SourceStatus is metadata
// about the source, not a rewrite of its runs.
func NewSourceCheckTool(cfg *SourceCheckConfig) *tool.BaseTool {
	t := &tool.BaseTool{
		ToolName:        "source-check",
		ToolDescription: "Marks source content checked once it clears its brand/terminology checks",
		Cfg:             cfg,
	}
	t.Annotate = func(v tool.BlockView) error {
		if !v.Translatable() || strings.TrimSpace(v.SourceText()) == "" {
			return nil
		}
		conf := t.Cfg.(*SourceCheckConfig)

		worst := worstSourceFindingWeight(v)
		blocked := worst >= conf.blockThreshold()

		switch {
		case blocked:
			// A blocking finding regresses readiness to the authored baseline,
			// even if the source was previously checked or approved — the source
			// changed (or a rule did) and no longer clears its checks.
			v.SetSourceStatus(model.SourceStatusAuthored)
		case v.SourceStatus().Rank() >= model.SourceStatusApproved.Rank():
			// Clean and already approved: a re-check never undoes a human
			// sign-off. Leave it as-is.
		default:
			v.SetSourceStatus(model.SourceStatusChecked)
		}
		return nil
	}
	return t
}

// worstSourceFindingWeight returns the highest finding-severity weight recorded
// on the block by upstream source checks, across both the unified check.Findings
// annotation and the brand-voice annotation. 0 means "no findings" (clean).
func worstSourceFindingWeight(v tool.BlockView) int {
	worst := 0
	for _, f := range check.Findings(v) {
		if w := check.SeverityWeight(f.Severity); w > worst {
			worst = w
		}
	}
	if a, ok := v.Annotations()["brand-voice"].(*brand.BrandVoiceAnnotation); ok {
		for _, f := range a.Findings {
			if w := check.SeverityWeight(f.Severity); w > worst {
				worst = w
			}
		}
	}
	return worst
}
