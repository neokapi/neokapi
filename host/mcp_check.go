package host

import (
	"context"
	"errors"
	"fmt"
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
// rule id, fixes the flagged block (optionally via the rewrite_file moat), and
// re-checks until the Report passes.
func init() {
	RegisterMCPToolFactory(registerCheckMCPTools)
}

func registerCheckMCPTools(server *mcp.Server, a *App) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "check_text",
		Description: "Verify a text snippet against the content checkset (text hygiene, length limits, " +
			"forbidden/required patterns, and voice vocabulary when a profile is given) and return a " +
			"kapi.check/v1 Report: pass, a 0-100 score, the gate, and a finding per stable rule id. Use it to " +
			"check content you authored, then fix and re-check until it passes.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in checkTextInput) (*mcp.CallToolResult, check.Report, error) {
		return a.checkTextMCP(ctx, in)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "check_file",
		Description: "Verify the content inside a file (Word, PowerPoint, JSON, XLIFF, Markdown, …) against the " +
			"content checkset and return a kapi.check/v1 Report with per-block locations. The check counterpart to " +
			"rewrite_file: author → check_file → fix the flagged block (optionally via rewrite_file) → re-check " +
			"until pass. Pass target/target_lang to also run bilingual checks.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in checkFileInput) (*mcp.CallToolResult, check.Report, error) {
		return a.checkFileMCP(ctx, in)
	})
}

// checkTextInput is the input to the check_text MCP tool.
type checkTextInput struct {
	Text        string   `json:"text" jsonschema:"the text to verify"`
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
	opts, err := a.mcpCheckOptions(in.MaxChars, in.MaxWords, in.Forbid, in.Require, in.ProfilePack, in.ProfileFile)
	if err != nil {
		return nil, check.Report{}, err
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
	return nil, execution.report(check.Target{Kind: "text", Blocks: 1}, diags, check.DefaultGate()), nil
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
