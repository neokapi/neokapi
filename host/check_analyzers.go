package host

import (
	"context"
	"fmt"
	"time"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/model"
)

// providerAnalyzer is an analysis that the provider of a file's blocks brings
// beside the checkset, such as a language's formatter. A whole-file check and a
// diff-scoped check record it the same way, with its canaries, so a provider
// adds one without either check path knowing what it does.
type providerAnalyzer struct {
	// id names the analyzer in the report, such as formatter.gofmt.
	id string
	// unsupported, when set, says why the analysis does not apply to the file.
	// Nothing runs, and the report lists the analyzer as unsupported.
	unsupported string
	// run reports the analyzer's findings on blocks, the file's blocks in scope.
	// A diagnostic names its block, and the recorder names the file.
	run func(ctx context.Context, blocks []*model.Block) ([]check.Diagnostic, error)
	// canaries are the known-bad inputs probe must report, each evaluated by the
	// same analysis run applies. With none, the analyzer did not run, and
	// uncheckable says why.
	canaries    []check.Canary
	uncheckable string
	probe       func(ctx context.Context, b *model.Block) ([]check.Finding, error)
}

// recordProviderAnalyzers runs each analyzer over blocks, records it with the
// outcome of its canaries, and returns its findings. file is the name the report
// gives the file. Every analyzer is required, so one whose canary is missed or
// impossible leaves the run invalid or not run, never passed. A nil execution
// records nothing and probes no canary.
func recordProviderAnalyzers(ctx context.Context, analyzers []providerAnalyzer, blocks []*model.Block, file string, execution *checkExecution) ([]check.Diagnostic, error) {
	var diags []check.Diagnostic
	for _, an := range analyzers {
		if an.unsupported != "" {
			execution.unsupported(an.id, file, an.unsupported)
			continue
		}
		start := time.Now()
		found, err := an.run(ctx, blocks)
		if err != nil {
			return nil, fmt.Errorf("%s %s: %w", an.id, DisplayName(file), err)
		}
		for i := range found {
			found[i].Location.File = DisplayName(file)
		}
		if execution != nil {
			canary, err := check.Probe(an.canaries, an.uncheckable, func(b *model.Block) ([]check.Finding, error) {
				return an.probe(ctx, b)
			})
			if err != nil {
				return nil, fmt.Errorf("%s %s: %w", an.id, DisplayName(file), err)
			}
			execution.completed(an.id, file, len(found), start, canary, true)
		}
		diags = append(diags, found...)
	}
	return diags, nil
}
