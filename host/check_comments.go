package host

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/comment"
	"github.com/neokapi/neokapi/core/comment/golang"
	"github.com/neokapi/neokapi/core/format"
	htmlformat "github.com/neokapi/neokapi/core/formats/html"
	markdownformat "github.com/neokapi/neokapi/core/formats/markdown"
	mdxformat "github.com/neokapi/neokapi/core/formats/mdx"
	poformat "github.com/neokapi/neokapi/core/formats/po"
	propertiesformat "github.com/neokapi/neokapi/core/formats/properties"
	xmlformat "github.com/neokapi/neokapi/core/formats/xml"
	yamlformat "github.com/neokapi/neokapi/core/formats/yaml"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/registry"
)

// commentProviders supply comment layers: the Go language provider for files no
// format reader covers, and the providers formats supply for the files their
// readers parse. Both use only what every kapi binary already carries. It is a
// variable so a test can put a faulty provider in its place and show that the
// canary invalidates the run.
var commentProviders = func() *comment.Registry {
	r := comment.NewRegistry(golang.Provider{})
	r.RegisterFormat("yaml", yamlformat.CommentProvider{})
	r.RegisterFormat("html", htmlformat.CommentProvider{})
	r.RegisterFormat("markdown", markdownformat.CommentProvider{})
	r.RegisterFormat("mdx", mdxformat.CommentProvider{})
	r.RegisterFormat("po", poformat.CommentProvider{})
	r.RegisterFormat("properties", propertiesformat.CommentProvider{})
	for _, f := range xmlCommentFormats {
		r.RegisterFormat(f, xmlformat.CommentProvider{Format: f})
	}
	return r
}()

// xmlCommentFormats are the formats whose readers read a plain XML file, so the
// XML provider locates their comments. A container format such as a word
// processor document keeps its XML inside an archive, where a comment has no
// span in the file.
var xmlCommentFormats = []string{"androidxml", "doclang", "resx", "tmx", "ts", "xliff", "xliff2", "xml"}

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
	p, ok := a.commentProviderFor(file)
	if !ok {
		return nil, false
	}
	// Detection by extension fails exactly when no format claims the extension.
	if _, err := a.FormatReg.Detect(file, registry.DetectOptions{ExtensionOnly: true}); err == nil {
		return nil, false
	}
	return p, true
}

// commentsOnlyFile reports a file whose only content a check reads is its
// comments: no format reader claims its extension, and a comment provider
// reads it, or would once the plugin that reads its language is installed.
func (a *App) commentsOnlyFile(path string) bool {
	a.InitRegistries()
	if _, ok := a.commentLayerFor(path, ""); ok {
		return true
	}
	return a.missingCommentReader(path) != nil
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
	// unread says why the file's comments could not be read, and is nil when
	// blocks holds them all. An unread layer has no blocks, and its extraction
	// did not run.
	unread error
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
	layer, err := a.readCommentLayer(ctx, file, opts.execution, func(src []byte) (*commentLayer, error) {
		return locateComments(file, src, p, opts.formats.directivesFor(file))
	})
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
	opts.stampPoints(diags, layer.blocks)
	layer.locate(diags)
	return layer.blocks, diags, nil
}

// readCommentLayer reads a file from disk and locates its comments with locate,
// counting the time as extraction. It records no analyzer: the caller runs the
// layer's analyzers over the comments it has in scope.
func (a *App) readCommentLayer(ctx context.Context, file string, execution *checkExecution, locate func(src []byte) (*commentLayer, error)) (*commentLayer, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	start := time.Now()
	src, err := os.ReadFile(file)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", DisplayName(file), err)
	}
	layer, err := locate(src)
	if err != nil {
		return nil, err
	}
	if execution != nil {
		execution.Timings.ExtractionMS += elapsedMS(start)
	}
	return layer, nil
}

// locateComments reads a file's bytes through its comment provider into blocks,
// setting aside the directives the recipe declares, and gives the layer its
// analyzers. The formatter compares the file here and never writes it.
func locateComments(file string, src []byte, p comment.Provider, directives comment.Directives) (*commentLayer, error) {
	located, err := comment.Locate(p, file, src, directives)
	if errors.Is(err, comment.ErrUnlocated) {
		// The provider read the file and could not place its comments exactly,
		// so none are checked and the extraction did not run.
		return &commentLayer{
			lines:     map[string]format.LineRange{},
			analyzers: []providerAnalyzer{{id: commentsAnalyzer(p), uncheckable: err.Error()}},
			unread:    err,
		}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("locate the comments in %s: %w", DisplayName(file), err)
	}
	layer := &commentLayer{blocks: located.Blocks(), extents: located.Extents(), lines: map[string]format.LineRange{}}
	for i, extent := range layer.extents {
		layer.lines[blockKey(layer.blocks[i])] = extent.Lines
	}
	layer.analyzers = []providerAnalyzer{extractionAnalyzer(p, directives)}
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

// declaredComments locates the comments of a file its format reader also reads,
// for a recipe that declares them as content. The format supplies the comments,
// or, when it supplies none, the comment provider for the file's language, such
// as the one the sourcecode plugin supplies for the Ruby files its format reads.
// A file neither supplies gives a layer whose comment analyzer did not run,
// which is never a pass.
func (a *App) declaredComments(file, fmtName string, src []byte, directives comment.Directives) (*commentLayer, error) {
	p, ok := commentProviders.ForFormat(fmtName)
	if !ok {
		p, ok = a.commentProviderFor(file)
	}
	if !ok {
		return &commentLayer{
			lines:     map[string]format.LineRange{},
			analyzers: []providerAnalyzer{{id: "comments." + fmtName, uncheckable: "the " + fmtName + " format supplies no comments to check"}},
		}, nil
	}
	return locateComments(file, src, p, directives)
}

// readDeclaredComments reads the comment layer of a file its format reader also
// reads, or returns nil when the recipe does not declare the file's comments.
func (a *App) readDeclaredComments(ctx context.Context, file, fmtName string, opts checkRunOptions) (*commentLayer, error) {
	if !opts.formats.commentsFor(file) {
		return nil, nil
	}
	name := fmtName
	if name == "" {
		id, err := a.FormatReg.Detect(file, registry.DetectOptions{ExtensionOnly: true})
		if err != nil {
			return nil, fmt.Errorf("detect format for %s: %w", DisplayName(file), err)
		}
		name = string(id)
	}
	return a.readCommentLayer(ctx, file, opts.execution, func(src []byte) (*commentLayer, error) {
		return a.declaredComments(file, name, src, opts.formats.directivesFor(file))
	})
}

// extractionAnalyzer records the comment extraction, which reports no finding
// of its own. Its canary is the provider's canary file, located by the same
// provider with the same directives set aside and flagged by the same hygiene
// checker as the real comments, so an extraction that loses prose it should
// locate invalidates the run.
func extractionAnalyzer(p comment.Provider, directives comment.Directives) providerAnalyzer {
	c := p.Canary()
	return providerAnalyzer{
		id:       commentsAnalyzer(p),
		canaries: []check.Canary{{Name: c.Name, Block: check.CanaryBlock(string(c.Source)), Expect: "doubled-word"}},
		probe: func(ctx context.Context, b *model.Block) ([]check.Finding, error) {
			located, err := comment.Locate(p, "canary", []byte(model.RunsText(b.SourceRuns())), directives)
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
		return providerAnalyzer{id: id, uncheckable: f.FormatterName() + " could not compare " + DisplayName(file) + ": " + err.Error()}
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
// its format reader, through its language's comment provider when the comments
// are all a check can read in it, or through both when the recipe declares the
// comments of a file a reader parses. A comment layer's analyzers run over every
// comment, and only they carry diagnostics.
func (a *App) readSourceForCheck(ctx context.Context, u VerifyUnit, execution *checkExecution) ([]*model.Block, []check.Diagnostic, error) {
	name, _ := a.unitFormat(u.SourceFormat, u.SourceConfig)
	if p, ok := a.commentLayerFor(u.SourcePath, name); ok {
		layer, err := a.readCommentLayer(ctx, u.SourcePath, execution, func(src []byte) (*commentLayer, error) {
			return locateComments(u.SourcePath, src, p, u.Directives)
		})
		if err != nil {
			return nil, nil, err
		}
		diags, err := recordProviderAnalyzers(ctx, layer.analyzers, layer.blocks, u.SourcePath, execution)
		if err != nil {
			return nil, nil, err
		}
		return layer.blocks, diags, nil
	}
	var blocks []*model.Block
	if !u.OnlyComments {
		var err error
		if blocks, err = a.readSource(ctx, u); err != nil {
			return nil, nil, err
		}
	}
	if !u.Comments {
		return blocks, nil, nil
	}
	if name == "" {
		id, err := a.FormatReg.Detect(u.SourcePath, registry.DetectOptions{ExtensionOnly: true})
		if err != nil {
			return nil, nil, fmt.Errorf("detect format for %s: %w", DisplayName(u.SourcePath), err)
		}
		name = string(id)
	}
	layer, err := a.readCommentLayer(ctx, u.SourcePath, execution, func(src []byte) (*commentLayer, error) {
		return a.declaredComments(u.SourcePath, name, src, u.Directives)
	})
	if err != nil {
		return nil, nil, err
	}
	diags, err := recordProviderAnalyzers(ctx, layer.analyzers, layer.blocks, u.SourcePath, execution)
	if err != nil {
		return nil, nil, err
	}
	return append(blocks, layer.blocks...), diags, nil
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
