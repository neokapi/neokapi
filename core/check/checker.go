package check

import (
	"context"

	"github.com/neokapi/neokapi/core/tool"
)

// Checker inspects a block read-only and emits Findings. Every verification
// producer implements it — deterministic rule, small ML model, or LLM judge —
// so a runner can compose them and aggregate one finding stream. Checkers never
// write content; they are wired as read-only Annotate tools (AD-006).
type Checker interface {
	// Name identifies the checker (used in finding metadata and logs).
	Name() string
	// Check returns findings for the block. It must not mutate source or target.
	Check(ctx context.Context, v tool.BlockView) ([]Finding, error)
}

// Annotate merges findings into the block's unified FindingsAnnotation under
// AnnotationKey and returns the Score over the combined set. Checkers call this
// at the end of their handler so every producer surfaces findings the same way.
// Pass WithWordCount(v.WordCount()) to length-normalize.
//
// Multiple checkers run on one block (qa, dnt-check, placeholder-check,
// terminology, …), so findings ACCUMULATE: any findings already written by an
// earlier checker are preserved, the new findings are appended, and the score is
// recomputed over the union. A checker that finds nothing leaves the block's
// existing annotation untouched (so it never clobbers an earlier checker's
// findings with an empty set, and a clean block stays un-annotated).
func Annotate(v tool.BlockView, source string, findings []Finding, opts ...ScoreOption) Score {
	existing := Findings(v)
	if len(findings) == 0 {
		// Nothing new to record. Return the score over whatever is already
		// there so callers still get a meaningful roll-up, but don't write
		// (or overwrite) an annotation for a checker that found nothing.
		return CalculateScore(existing, opts...)
	}

	combined := make([]Finding, 0, len(existing)+len(findings))
	combined = append(combined, existing...)
	combined = append(combined, findings...)

	score := CalculateScore(combined, opts...)
	v.Annotate(AnnotationKey, &FindingsAnnotation{
		Source:   source,
		Score:    score.Overall,
		Findings: combined,
	})
	return score
}

// Findings returns the findings already recorded on the block under
// AnnotationKey, or nil when none are present. It is the read counterpart to
// Annotate: consumers (verify gates, the QA handler, the desktop Checks panel)
// read one shape regardless of which checker produced the findings.
func Findings(v tool.BlockView) []Finding {
	if a, ok := v.Annotations()[AnnotationKey].(*FindingsAnnotation); ok {
		return a.Findings
	}
	return nil
}
