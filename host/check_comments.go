package host

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/comment"
	"github.com/neokapi/neokapi/core/comment/golang"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/registry"
)

// commentProviders read the comment layer of files that no format reader
// covers. The Go provider uses only the standard library, so every kapi binary
// carries it. It is a variable so a test can put a faulty provider in its place
// and show that the canary invalidates the run.
var commentProviders = comment.NewRegistry(golang.Provider{})

// formatterCheck is the check family a formatter's disagreement is reported
// under. The rule is `formatter.<formatter>`, such as `formatter.gofmt`.
const formatterCheck = "formatter"

// commentsAnalyzer is the analyzer a language's comment extraction is recorded
// under, such as `comments.go`.
func commentsAnalyzer(p comment.Provider) string { return "comments." + p.Language() }

// commentLayerFor returns the provider that reads a file for its comments,
// when the comments are all a check can read in it: no format was declared for
// the file, no reader claims its extension, and a language provider does.
func (a *App) commentLayerFor(file, fmtName string) (comment.Provider, bool) {
	if fmtName != "" {
		return nil, false
	}
	p, ok := commentProviders.For(file)
	if !ok {
		return nil, false
	}
	// Detection by extension fails exactly when no format claims the extension.
	if _, err := a.FormatReg.Detect(file, registry.DetectOptions{ExtensionOnly: true}); err == nil {
		return nil, false
	}
	return p, true
}

// commentLayer is one file's comments, read for checking.
type commentLayer struct {
	blocks []*model.Block
	// extents locate each block in the file, in the order of blocks.
	extents []format.Extent
	// lines maps each block's location key to the lines it spans in the file.
	lines map[string]format.LineRange
	// analyzers are the comment extraction and the language's formatter, run
	// over whichever comments a check has in scope.
	analyzers []providerAnalyzer
}

// locate gives every diagnostic on a comment block the lines that comment
// spans, so a finding reads as a place in the file as well as a block.
func (l *commentLayer) locate(diags []check.Diagnostic) {
	for i := range diags {
		if diags[i].Location.Lines != nil {
			continue
		}
		if r, ok := l.lines[diags[i].Location.Block]; ok {
			diags[i].Location.Lines = &r
		}
	}
}

// checkCommentFile is checkFileBlocks for a file read for its comment layer.
// The comments become blocks and go through collectFileDiagnostics, the same
// checkset, governance and report as any other content. The layer's analyzers
// run over every comment in the file, and their findings join the checkset's.
func (a *App) checkCommentFile(ctx context.Context, file string, p comment.Provider, validateMode format.ValidationMode, opts checkRunOptions) ([]*model.Block, []check.Diagnostic, error) {
	layer, err := a.readCommentLayer(ctx, file, p, opts.execution)
	if err != nil {
		return nil, nil, err
	}
	layerDiags, err := recordProviderAnalyzers(ctx, layer.analyzers, layer.blocks, file, opts.execution)
	if err != nil {
		return nil, nil, err
	}
	if validateMode == format.ValidationOff {
		opts.execution.skipped("reader.validation", file, "Reader validation was not requested.")
	} else {
		opts.execution.unsupported("reader.validation", file, "The file is read for its comments, and no format reader parses it.")
	}
	fileDiags, err := a.collectFileDiagnostics(ctx, layer.blocks, file, opts)
	if err != nil {
		return nil, nil, err
	}
	diags := make([]check.Diagnostic, 0, len(layerDiags)+len(fileDiags))
	diags = append(diags, layerDiags...)
	diags = append(diags, fileDiags...)
	layer.locate(diags)
	return layer.blocks, diags, nil
}

// readCommentLayer reads a file from disk and locates its comments, counting
// the time as extraction. It records no analyzer: the caller runs the layer's
// analyzers over the comments it has in scope.
func (a *App) readCommentLayer(ctx context.Context, file string, p comment.Provider, execution *checkExecution) (*commentLayer, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	start := time.Now()
	src, err := os.ReadFile(file)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", DisplayName(file), err)
	}
	layer, err := locateComments(file, src, p)
	if err != nil {
		return nil, err
	}
	if execution != nil {
		execution.Timings.ExtractionMS += elapsedMS(start)
	}
	return layer, nil
}

// locateComments reads a file's bytes through its language provider into
// blocks and gives the layer its analyzers. The formatter compares the file
// here and never writes it.
func locateComments(file string, src []byte, p comment.Provider) (*commentLayer, error) {
	located, err := p.Locate(file, src)
	if err != nil {
		return nil, fmt.Errorf("locate the comments in %s: %w", DisplayName(file), err)
	}
	layer := &commentLayer{blocks: located.Blocks(), extents: located.Extents(), lines: map[string]format.LineRange{}}
	for i, extent := range layer.extents {
		layer.lines[blockKey(layer.blocks[i])] = extent.Lines
	}
	layer.analyzers = []providerAnalyzer{extractionAnalyzer(p)}
	f, ok := p.(comment.Formatter)
	if !ok {
		layer.analyzers = append(layer.analyzers, providerAnalyzer{
			id:          formatterCheck,
			unsupported: fmt.Sprintf("No formatter is known for %s.", p.Language()),
		})
		return layer, nil
	}
	layer.analyzers = append(layer.analyzers, formatterAnalyzer(file, src, located, layer.blocks, p, f))
	return layer, nil
}

// extractionAnalyzer records the comment extraction, which reports no finding
// of its own. Its canary is the provider's canary file, located by the same
// provider and flagged by the same hygiene checker as the real comments, so a
// provider that loses prose it should locate invalidates the run.
func extractionAnalyzer(p comment.Provider) providerAnalyzer {
	c := p.Canary()
	return providerAnalyzer{
		id:       commentsAnalyzer(p),
		run:      func(context.Context, []*model.Block) ([]check.Diagnostic, error) { return nil, nil },
		canaries: []check.Canary{{Name: c.Name, Block: check.CanaryBlock(string(c.Source)), Expect: "doubled-word"}},
		probe: func(ctx context.Context, b *model.Block) ([]check.Finding, error) {
			located, err := p.Locate("canary", []byte(model.RunsText(b.SourceRuns())))
			if err != nil {
				return nil, err
			}
			for _, blk := range located.Blocks() {
				if blk.ID != c.Block {
					continue
				}
				if err := RunCheckTool(ctx, hygieneTool(), blk); err != nil {
					return nil, err
				}
				return FindingsFromBlock(blk, false), nil
			}
			return nil, nil
		},
	}
}

// formatterAnalyzer compares the file with its language's formatter and reports
// a major finding on each comment in scope that the formatter would rewrite. Its
// canary is a file holding a comment the formatter rewrites, located by the same
// provider. A formatter that cannot compare this file has nothing it can be
// shown to catch, so it has no canary and did not run.
func formatterAnalyzer(file string, src []byte, located *comment.File, blocks []*model.Block, p comment.Provider, f comment.Formatter) providerAnalyzer {
	id := check.RuleID(formatterCheck, f.FormatterName())
	disagreements, err := f.Disagreements(file, src, located)
	if err != nil {
		return providerAnalyzer{
			id:          id,
			run:         func(context.Context, []*model.Block) ([]check.Diagnostic, error) { return nil, nil },
			uncheckable: f.FormatterName() + " could not compare " + DisplayName(file) + ": " + err.Error(),
		}
	}
	return providerAnalyzer{
		id: id,
		run: func(_ context.Context, scope []*model.Block) ([]check.Diagnostic, error) {
			inScope := make(map[string]bool, len(scope))
			for _, b := range scope {
				inScope[blockKey(b)] = true
			}
			var diags []check.Diagnostic
			for _, d := range disagreements {
				key := blockKey(blocks[d.Comment])
				if !inScope[key] {
					continue
				}
				diags = append(diags, check.Diagnostic{
					Rule:       id,
					Check:      formatterCheck,
					Severity:   check.SeverityMajor,
					Message:    f.FormatterName() + " would rewrite this comment",
					Suggestion: d.Formatted,
					Location:   check.Location{Block: key},
				})
			}
			return diags, nil
		},
		canaries: []check.Canary{{Name: "a comment " + f.FormatterName() + " rewrites", Block: check.CanaryBlock(string(f.FormatterCanary()))}},
		probe: func(_ context.Context, b *model.Block) ([]check.Finding, error) {
			canary := []byte(model.RunsText(b.SourceRuns()))
			located, err := p.Locate("canary", canary)
			if err != nil {
				return nil, err
			}
			found, err := f.Disagreements("canary", canary, located)
			if err != nil {
				return nil, err
			}
			findings := make([]check.Finding, len(found))
			for i := range found {
				findings[i] = check.Finding{Category: f.FormatterName()}
			}
			return findings, nil
		},
	}
}

// readSourceForCheck reads a unit's source for the source-side checks: through
// its format reader, or through its language's comment provider when the
// comments are all a check can read in it. A comment layer's analyzers run over
// every comment, and only they carry diagnostics.
func (a *App) readSourceForCheck(ctx context.Context, u VerifyUnit, execution *checkExecution) ([]*model.Block, []check.Diagnostic, error) {
	name, _ := a.unitFormat(u.SourceFormat, u.SourceConfig)
	if p, ok := a.commentLayerFor(u.SourcePath, name); ok {
		layer, err := a.readCommentLayer(ctx, u.SourcePath, p, execution)
		if err != nil {
			return nil, nil, err
		}
		diags, err := recordProviderAnalyzers(ctx, layer.analyzers, layer.blocks, u.SourcePath, execution)
		if err != nil {
			return nil, nil, err
		}
		return layer.blocks, diags, nil
	}
	blocks, err := a.readSource(ctx, u)
	return blocks, nil, err
}

// applyFormatterGate fails a report in which a formatter would rewrite a checked
// comment. An edit the project's formatter reflows churns the next commit, so it
// does not pass whatever the severity thresholds allow. --lenient turns every
// limit off, and this one with them.
func applyFormatterGate(report *check.Report) {
	seen := map[string]bool{}
	for _, f := range report.Findings {
		if f.Check != formatterCheck {
			continue
		}
		reason := fmt.Sprintf("%s: %s would rewrite a comment in %s", formatterCheck, f.Rule, f.Location.File)
		if !seen[reason] {
			seen[reason] = true
			report.Gate.Failed = append(report.Gate.Failed, reason)
		}
	}
	report.Decide()
}
