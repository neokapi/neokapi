package cli

import "github.com/neokapi/neokapi/core/convergence"

// The convergence report MODEL and the per-block ladder helpers live in the
// framework (core/convergence) so any surface derives the same shape from the
// same rules. The CLI owns the file-IO orchestration that feeds them
// (unitsFromProject, readBlocks, bilingualBlocks, the state-store review index)
// and re-exports the types via aliases so existing CLI + desktop callers — and
// the generated Wails bindings — are unchanged.
type (
	// ConvergenceReport is the full derived convergence picture (alias of
	// convergence.Report). Returned by ProjectConvergence and consumed by the
	// Kapi Desktop.
	ConvergenceReport = convergence.Report
	LocaleCoverage    = convergence.LocaleCoverage
	SourceCoverage    = convergence.SourceCoverage
	ReviewItem        = convergence.ReviewItem
)

// Per-block ladder helpers, framework-owned. Kept as package-level aliases so the
// many CLI call sites are untouched.
var (
	blockKey        = convergence.BlockKey
	preview         = convergence.Preview
	unitState       = convergence.TargetState
	sourceUnitState = convergence.SourceState
)
