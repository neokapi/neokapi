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
// carries it.
var commentProviders = comment.NewRegistry(golang.Provider{})

// formatterCheck is the check family a formatter's disagreement is reported
// under. The rule is `formatter.<formatter>`, such as `formatter.gofmt`.
const formatterCheck = "formatter"

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
	// lines maps each block's location key to the lines it spans in the file.
	lines map[string]format.LineRange
	// formatter holds the formatter's disagreements, located.
	formatter []check.Diagnostic
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
// checkset, governance and report as any other content; the formatter's
// disagreements join them.
func (a *App) checkCommentFile(ctx context.Context, file string, p comment.Provider, validateMode format.ValidationMode, opts checkRunOptions) ([]*model.Block, []check.Diagnostic, error) {
	layer, err := a.readCommentLayer(ctx, file, p, opts.execution)
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
	diags := make([]check.Diagnostic, 0, len(layer.formatter)+len(fileDiags))
	diags = append(diags, layer.formatter...)
	diags = append(diags, fileDiags...)
	layer.locate(diags)
	return layer.blocks, diags, nil
}

// readCommentLayer locates a file's comments with its language provider and
// returns them as blocks, together with the formatter's disagreements as
// diagnostics.
//
// The formatter compares and never writes. A language with no formatter, or a
// comparison that cannot locate its result, is recorded as not having run: a
// formatter check that did not run is never reported as agreement.
func (a *App) readCommentLayer(ctx context.Context, file string, p comment.Provider, execution *checkExecution) (*commentLayer, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	start := time.Now()
	src, err := os.ReadFile(file)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", DisplayName(file), err)
	}
	located, err := p.Locate(file, src)
	if err != nil {
		return nil, fmt.Errorf("locate the comments in %s: %w", DisplayName(file), err)
	}
	layer := &commentLayer{blocks: located.Blocks(), lines: map[string]format.LineRange{}}
	for i, extent := range located.Extents() {
		layer.lines[blockKey(layer.blocks[i])] = extent.Lines
	}
	if execution != nil {
		execution.Timings.ExtractionMS += elapsedMS(start)
	}

	f, ok := p.(comment.Formatter)
	if !ok {
		execution.unsupported(formatterCheck, file, fmt.Sprintf("No formatter is available for %s.", p.Language()))
		return layer, nil
	}
	id := check.RuleID(formatterCheck, f.FormatterName())
	start = time.Now()
	disagreements, err := f.Disagreements(file, src, located)
	if err != nil {
		execution.failed(id, file, err.Error())
		return layer, nil
	}
	for _, d := range disagreements {
		layer.formatter = append(layer.formatter, check.Diagnostic{
			Rule:       id,
			Check:      formatterCheck,
			Severity:   check.SeverityMajor,
			Message:    f.FormatterName() + " would rewrite this comment",
			Suggestion: d.Formatted,
			Location:   check.Location{File: DisplayName(file), Block: blockKey(layer.blocks[d.Comment])},
		})
	}
	execution.completed(id, file, len(layer.formatter), start)
	return layer, nil
}

// readSourceForCheck reads a unit's source for the source-side checks: through
// its format reader, or through its language's comment provider when the
// comments are all a check can read in it. Only a comment layer carries
// formatter diagnostics.
func (a *App) readSourceForCheck(ctx context.Context, u VerifyUnit, execution *checkExecution) ([]*model.Block, []check.Diagnostic, error) {
	name, _ := a.unitFormat(u.SourceFormat, u.SourceConfig)
	if p, ok := a.commentLayerFor(u.SourcePath, name); ok {
		layer, err := a.readCommentLayer(ctx, u.SourcePath, p, execution)
		if err != nil {
			return nil, nil, err
		}
		return layer.blocks, layer.formatter, nil
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
		report.Pass = false
		reason := fmt.Sprintf("%s: %s would rewrite a comment in %s", formatterCheck, f.Rule, f.Location.File)
		if !seen[reason] {
			seen[reason] = true
			report.Gate.Failed = append(report.Gate.Failed, reason)
		}
	}
}

// failed records an analysis that started and could not finish.
func (e *checkExecution) failed(id, file, reason string) {
	if e == nil {
		return
	}
	e.Analyzers = append(e.Analyzers, check.AnalyzerExecution{
		ID: id, File: DisplayName(file), Status: check.AnalyzerError, Required: true, Reason: reason,
	})
}
