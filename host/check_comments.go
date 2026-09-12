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
// Both halves are analyzers with canaries. The provider must locate its canary
// file's comment and the hygiene check must flag it, or nothing it located in
// the real file can be trusted. The formatter must report its canary's
// comment. The formatter compares and never writes; when it cannot finish, the
// run did not run.
//
// A nil execution reads the layer for a caller that records no analyzers, and
// gives no canaries.
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
		canary, cerr := probeCommentExtraction(ctx, p)
		if cerr != nil {
			return nil, fmt.Errorf("comment extraction %s: %w", DisplayName(file), cerr)
		}
		execution.completed(commentsAnalyzer(p), file, 0, start, canary, true)
	}

	f, ok := p.(comment.Formatter)
	if !ok {
		execution.unsupported(formatterCheck, file, fmt.Sprintf("No formatter is known for %s.", p.Language()))
		return layer, nil
	}
	id := check.RuleID(formatterCheck, f.FormatterName())
	start = time.Now()
	disagreements, err := f.Disagreements(file, src, located)
	if err != nil {
		execution.notRun(id, file, err.Error())
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
	if execution != nil {
		canary, cerr := probeFormatter(p, f)
		if cerr != nil {
			return nil, fmt.Errorf("%s %s: %w", id, DisplayName(file), cerr)
		}
		execution.completed(id, file, len(layer.formatter), start, canary, true)
	}
	return layer, nil
}

// probeCommentExtraction gives the provider its canary file, through the same
// provider and the same hygiene checker the real comments went through.
func probeCommentExtraction(ctx context.Context, p comment.Provider) (check.CanaryOutcome, error) {
	c := p.Canary()
	canary := check.Canary{Name: c.Name, Block: check.CanaryBlock(string(c.Source)), Expect: "doubled-word"}
	hygiene := hygieneTool()
	return check.Probe([]check.Canary{canary}, "", func(b *model.Block) ([]check.Finding, error) {
		located, err := p.Locate("canary", []byte(model.RunsText(b.SourceRuns())))
		if err != nil {
			return nil, err
		}
		for _, blk := range located.Blocks() {
			if blk.ID != c.Block {
				continue
			}
			if err := RunCheckTool(ctx, hygiene, blk); err != nil {
				return nil, err
			}
			return FindingsFromBlock(blk, false), nil
		}
		return nil, nil
	})
}

// probeFormatter gives the formatter a file holding a comment it rewrites,
// located by the same provider the real file was.
func probeFormatter(p comment.Provider, f comment.Formatter) (check.CanaryOutcome, error) {
	canary := check.Canary{Name: "a comment " + f.FormatterName() + " rewrites", Block: check.CanaryBlock(string(f.FormatterCanary()))}
	return check.Probe([]check.Canary{canary}, "", func(b *model.Block) ([]check.Finding, error) {
		src := []byte(model.RunsText(b.SourceRuns()))
		located, err := p.Locate("canary", src)
		if err != nil {
			return nil, err
		}
		disagreements, err := f.Disagreements("canary", src, located)
		if err != nil {
			return nil, err
		}
		findings := make([]check.Finding, len(disagreements))
		for i := range disagreements {
			findings[i] = check.Finding{Category: f.FormatterName()}
		}
		return findings, nil
	})
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
		reason := fmt.Sprintf("%s: %s would rewrite a comment in %s", formatterCheck, f.Rule, f.Location.File)
		if !seen[reason] {
			seen[reason] = true
			report.Gate.Failed = append(report.Gate.Failed, reason)
		}
	}
	report.Decide()
}

// notRun records a required analysis that started and could not finish, which
// leaves the run unverified.
func (e *checkExecution) notRun(id, file, reason string) {
	if e == nil {
		return
	}
	e.Analyzers = append(e.Analyzers, check.AnalyzerExecution{
		ID: id, File: DisplayName(file), Status: check.AnalyzerDidNotRun, Required: true, Reason: reason,
	})
}
