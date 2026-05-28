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

// Annotate computes a Score from findings and writes a FindingsAnnotation onto
// the block under AnnotationKey, returning the Score. Checkers call this at the
// end of their handler so every producer surfaces findings the same way. Pass
// WithWordCount(v.WordCount()) to length-normalize.
func Annotate(v tool.BlockView, source string, findings []Finding, opts ...ScoreOption) Score {
	score := CalculateScore(findings, opts...)
	v.Annotate(AnnotationKey, &FindingsAnnotation{
		Source:   source,
		Score:    score.Overall,
		Findings: findings,
	})
	return score
}
