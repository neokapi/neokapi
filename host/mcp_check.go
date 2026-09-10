package host

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/locale"
	"github.com/neokapi/neokapi/core/model"
)

// init registers the content-check MCP tools on the shared `mcp` server. These
// are the verifier half of the AI author→check→revise loop: an assistant authors
// content, calls check_text/check_file, reads the located findings by stable
// rule id, fixes the flagged block (optionally via apply_edits), and
// re-checks until the Report passes.
func init() {
	RegisterMCPToolFactory(registerCheckMCPTools)
}

func registerCheckMCPTools(server *mcp.Server, a *App) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "check_text",
		Description: "Check a draft snippet with deterministic content rules. Before drafting, read the " +
			"context://<project-relative-path> resource for the applicable guidance. Supply context_path " +
			"to check with that destination's voice and terms; without it, project guidance is not resolved. " +
			"Explicit profile_pack/profile_file are available only without context_path. Returns a " +
			"kapi.check/v1 Report with findings and analyzer coverage; pass is not semantic approval. " +
			"After saving edits, use check_file to verify the actual file.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in checkTextInput) (*mcp.CallToolResult, check.Report, error) {
		return a.checkTextMCP(ctx, in)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "check_file",
		Description: "Check the actual content inside a file (Word, PowerPoint, JSON, XLIFF, Markdown, …) " +
			"with format-aware extraction and the applicable project voice and terms. Before editing, read " +
			"the context://<project-relative-path> resource; after saving edits (including apply_edits), " +
			"run check_file and review its per-block findings and analyzer coverage. Returns a kapi.check/v1 " +
			"Report; pass is not semantic approval. Pass target/target_lang to also run bilingual checks.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in checkFileInput) (*mcp.CallToolResult, check.Report, error) {
		return a.checkFileMCP(ctx, in)
	})
}

// checkTextInput is the input to the check_text MCP tool.
type checkTextInput struct {
	Text        string   `json:"text" jsonschema:"the text to verify"`
	ContextPath string   `json:"context_path,omitempty" jsonschema:"project-relative destination whose voice and terms govern this draft; may not exist yet; requires a project; cannot combine with profile_pack or profile_file"`
	MaxChars    int      `json:"max_chars,omitempty" jsonschema:"flag content longer than this many characters (0 = off)"`
	MaxWords    int      `json:"max_words,omitempty" jsonschema:"flag content with more than this many words (0 = off)"`
	Forbid      []string `json:"forbid,omitempty" jsonschema:"regex that must NOT appear in the content"`
	Require     []string `json:"require,omitempty" jsonschema:"regex that MUST appear in the content"`
	ProfilePack string   `json:"profile_pack,omitempty" jsonschema:"built-in profile pack to check vocabulary against (e.g. marketing-blog)"`
	ProfileFile string   `json:"profile_file,omitempty" jsonschema:"path to a voice profile YAML"`
}

// checkFileInput is the input to the check_file MCP tool.
type checkFileInput struct {
	File        string   `json:"file" jsonschema:"path to the file whose content should be checked"`
	MaxChars    int      `json:"max_chars,omitempty" jsonschema:"flag content longer than this many characters (0 = off)"`
	MaxWords    int      `json:"max_words,omitempty" jsonschema:"flag content with more than this many words (0 = off)"`
	Forbid      []string `json:"forbid,omitempty" jsonschema:"regex that must NOT appear in the content"`
	Require     []string `json:"require,omitempty" jsonschema:"regex that MUST appear in the content"`
	ProfilePack string   `json:"profile_pack,omitempty" jsonschema:"built-in profile pack to check vocabulary against"`
	ProfileFile string   `json:"profile_file,omitempty" jsonschema:"path to a voice profile YAML"`
	Target      string   `json:"target,omitempty" jsonschema:"translated target file to check against the source (enables the bilingual source-against-target checks)"`
	TargetLang  string   `json:"target_lang,omitempty" jsonschema:"locale of the target file (e.g. de)"`
	DNT         []string `json:"dnt,omitempty" jsonschema:"do-not-translate terms that must survive verbatim into the target"`
	Validate    string   `json:"validate,omitempty" jsonschema:"reader structure/encoding validation: off|report|strict (report folds structure.*/encoding.* findings into the report; strict also fails on a Major+ structure/encoding problem). Default off."`
}

// checkTextMCP runs the source-side content checkset over a text snippet.
func (a *App) checkTextMCP(ctx context.Context, in checkTextInput) (*mcp.CallToolResult, check.Report, error) {
	execution := newCheckExecution()
	contextStart := time.Now()
	a.InitRegistries()
	if in.ContextPath != "" && (in.ProfilePack != "" || in.ProfileFile != "") {
		return nil, check.Report{}, errors.New("context_path cannot be combined with profile_pack or profile_file")
	}
	opts, err := a.mcpCheckOptions(in.MaxChars, in.MaxWords, in.Forbid, in.Require, in.ProfilePack, in.ProfileFile)
	if err != nil {
		return nil, check.Report{}, err
	}
	if in.ContextPath != "" {
		if err := a.resolveTextCheckContext(ctx, in.ContextPath, &opts); err != nil {
			return nil, check.Report{}, err
		}
	}
	execution.Timings.ContextMS += elapsedMS(contextStart)
	opts.execution = execution
	block := &model.Block{ID: "text", Translatable: true, Source: []model.Run{{Text: &model.TextRun{Text: in.Text}}}}
	diags, err := a.collectFileDiagnostics(ctx, []*model.Block{block}, "text", opts)
	if err != nil {
		return nil, check.Report{}, err
	}
	for i := range execution.Analyzers {
		execution.Analyzers[i].File = ""
	}
	for i := range diags {
		diags[i].Location.File = ""
	}
	target := check.Target{Kind: "text", Blocks: 1, ContextPath: in.ContextPath}
	return nil, execution.report(target, diags, check.DefaultGate()), nil
}

// resolveTextCheckContext resolves a lexical destination, not an input file.
// Anchoring it to the recipe root keeps collection matching independent of cwd
// and permits a draft for a file that has not yet been created.
func (a *App) resolveTextCheckContext(ctx context.Context, contextPath string, opts *checkRunOptions) error {
	validPath := fs.ValidPath(contextPath) && contextPath != "." && !filepath.IsAbs(contextPath)
	if !validPath || strings.ContainsAny(contextPath, "\\\x00") {
		return errors.New("context_path must be a clean project-relative file path")
	}
	cmd := NewEnvCommand(ctx, "check_text")
	if a.mcpRecipePath != "" {
		cmd.Flags().String(projectFlagName, a.mcpRecipePath, "")
	}
	recipe, err := ResolveProjectPath(cmd)
	if err != nil {
		return fmt.Errorf("resolve context_path project: %w", err)
	}
	if recipe == "" {
		return errors.New("context_path requires a project; start the MCP server with -p")
	}
	// Freeze the resolution for both voice and terms, including when the server
	// was assembled by an embedded caller using environment-based discovery.
	if cmd.Flags().Lookup(projectFlagName) == nil {
		cmd.Flags().String(projectFlagName, recipe, "")
	}
	destination := filepath.Join(filepath.Dir(recipe), filepath.FromSlash(contextPath))
	voice, err := a.newCheckVoice(cmd)
	if err != nil {
		return fmt.Errorf("resolve context_path voice: %w", err)
	}
	defer voice.close()
	opts.profile, err = voice.forFile(ctx, destination)
	if err != nil {
		return fmt.Errorf("resolve context_path voice: %w", err)
	}
	opts.terms, err = a.ProjectTermsForFile(ctx, cmd, destination)
	if err != nil {
		return fmt.Errorf("resolve context_path terms: %w", err)
	}
	return nil
}

// checkFileMCP runs the content checkset over a file's content, optionally with
// the bilingual checks when a target is supplied.
func (a *App) checkFileMCP(ctx context.Context, in checkFileInput) (*mcp.CallToolResult, check.Report, error) {
	execution := newCheckExecution()
	contextStart := time.Now()
	a.InitRegistries()
	if in.File == "" {
		return nil, check.Report{}, errors.New("file is required")
	}
	opts, err := a.mcpCheckOptions(in.MaxChars, in.MaxWords, in.Forbid, in.Require, in.ProfilePack, in.ProfileFile)
	if err != nil {
		return nil, check.Report{}, err
	}
	// Check context is required when bound. A resolution failure is an operation
	// error, not permission to omit the governing rules from a successful report.
	cmd := NewEnvCommand(ctx, "check_file")
	if a.mcpRecipePath != "" {
		cmd.Flags().String(projectFlagName, a.mcpRecipePath, "")
	}
	if opts.profile == nil {
		voice, err := a.newCheckVoice(cmd)
		if err != nil {
			return nil, check.Report{}, err
		}
		defer voice.close()
		opts.profile, err = voice.forFile(ctx, in.File)
		if err != nil {
			return nil, check.Report{}, err
		}
	}
	opts.terms, err = a.ProjectTermsForFile(ctx, cmd, in.File)
	if err != nil {
		return nil, check.Report{}, err
	}
	opts.formats, err = a.newCheckFormats(cmd)
	if err != nil {
		return nil, check.Report{}, err
	}
	opts.execution = execution
	execution.Timings.ContextMS += elapsedMS(contextStart)
	target := check.Target{Kind: "file", File: in.File}
	var diags []check.Diagnostic

	validateMode, err := parseValidationMode(in.Validate)
	if err != nil {
		return nil, check.Report{}, err
	}
	if in.Target != "" {
		if validateMode != format.ValidationOff {
			return nil, check.Report{}, errors.New("reader validation is unavailable with target; validate each file separately")
		}
		lang := in.TargetLang
		if lang == "" {
			lang = "und"
		}
		// The bilingual checks key on this locale, so it is canonicalized
		// before it becomes one: `nb_NO` and `nb-NO` name the same target.
		id, lerr := locale.Canonical(lang)
		if lerr != nil {
			return nil, check.Report{}, fmt.Errorf("target_lang: %w", lerr)
		}
		lang = string(id)
		fmtName, fmtConfig := opts.formats.forFile(a, in.File)
		unit := VerifyUnit{SourcePath: in.File, TargetPath: in.Target, Locale: lang, DisplayPath: in.Target,
			SourceFormat: fmtName, TargetFormat: fmtName, SourceConfig: fmtConfig, TargetConfig: fmtConfig}
		extractionStart := time.Now()
		blocks, missing, berr := a.bilingualBlocks(ctx, unit)
		execution.Timings.ExtractionMS += elapsedMS(extractionStart)
		execution.skipped("reader.validation", in.File, "Reader validation was not requested.")
		if berr != nil {
			return nil, check.Report{}, berr
		}
		if missing {
			return nil, check.Report{}, fmt.Errorf("target file %q does not exist", in.Target)
		}
		target.Blocks = len(blocks)
		fd, ferr := a.collectFileDiagnostics(ctx, blocks, in.File, opts)
		if ferr != nil {
			return nil, check.Report{}, ferr
		}
		diags = fd
		biDiags, bderr := a.collectBilingualDiagnostics(ctx, blocks, in.File, model.LocaleID(lang), in.DNT, execution)
		if bderr != nil {
			return nil, check.Report{}, bderr
		}
		diags = append(diags, biDiags...)
	} else {
		blocks, fileDiags, ferr := a.checkFileBlocks(ctx, in.File, validateMode, opts)
		if ferr != nil {
			return nil, check.Report{}, ferr
		}
		target.Blocks = len(blocks)
		diags = fileDiags
		report := execution.report(target, diags, check.DefaultGate())
		if validateMode == format.ValidationStrict {
			applyStrictValidationGate(&report)
		}
		return nil, report, nil
	}
	return nil, execution.report(target, diags, check.DefaultGate()), nil
}

// mcpCheckOptions resolves the shared content-check options for the MCP tools,
// loading a voice profile from a pack/file when one is named.
func (a *App) mcpCheckOptions(maxChars, maxWords int, forbid, require []string, pack, file string) (checkRunOptions, error) {
	opts := checkRunOptions{maxChars: maxChars, maxWords: maxWords, forbid: forbid, require: require}
	if pack != "" || file != "" {
		p, err := loadProfileForMCP(pack, file)
		if err != nil {
			return opts, err
		}
		opts.profile = p
	}
	return opts, nil
}
