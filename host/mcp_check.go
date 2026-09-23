package host

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
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
			"kapi.check/v1 Report with findings, analyzer coverage and configuration warnings, which never " +
			"change pass; pass is not semantic approval. " +
			"After saving edits, use check_file to verify the actual file.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in checkTextInput) (*mcp.CallToolResult, check.Report, error) {
		return a.checkTextMCP(ctx, in)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "check_file",
		Description: "Check a file you changed against the voice and terms in force at its location, in any " +
			"format kapi reads (Markdown, JSON, Word, XLIFF and more). Run it on every file you changed and fix " +
			"what it reports before you say the work is done. To check only what a change touched, pass diff, " +
			"diff_against (a git revision), staged, or diff_range (A..B). Returns a kapi.check/v1 report: " +
			"findings, and which analyzers covered the content. A pass is not semantic approval.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in checkFileInput) (*mcp.CallToolResult, check.Report, error) {
		return a.checkFileMCP(ctx, in)
	})
}

// checkTextInput is the input to the check_text MCP tool.
type checkTextInput struct {
	Text        string   `json:"text" jsonschema:"the text to verify"`
	Project     string   `json:"project,omitempty" jsonschema:"the project this call acts on: its kapi.yaml recipe, its root directory, or any path inside it (default: the project the MCP server started in)"`
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
	File        string   `json:"file,omitempty" jsonschema:"path to the file whose content should be checked; with diff, diff_against, staged or diff_range it narrows the scope to this file, and may be omitted"`
	Project     string   `json:"project,omitempty" jsonschema:"the project this call acts on: its kapi.yaml recipe, its root directory, or any path inside it (default: the project the MCP server started in)"`
	Diff        string   `json:"diff,omitempty" jsonschema:"unified diff text (git diff output); only the content blocks it touches are checked"`
	DiffAgainst string   `json:"diff_against,omitempty" jsonschema:"git revision to diff the working tree against, read-only, with untracked files as added; only the content blocks changed are checked"`
	Staged      bool     `json:"staged,omitempty" jsonschema:"check the changes staged for commit: the index diffed against HEAD, with each file read from the index, leaving out unstaged edits and untracked files; only the content blocks changed are checked"`
	DiffRange   string   `json:"diff_range,omitempty" jsonschema:"two commits as A..B, or A...B for the change B made since its merge base with A; each file is read from B and nothing from the working tree; only the content blocks changed are checked"`
	MaxChars    int      `json:"max_chars,omitempty" jsonschema:"flag content longer than this many characters (0 = off)"`
	MaxWords    int      `json:"max_words,omitempty" jsonschema:"flag content with more than this many words (0 = off)"`
	Forbid      []string `json:"forbid,omitempty" jsonschema:"regex that must NOT appear in the content"`
	Require     []string `json:"require,omitempty" jsonschema:"regex that MUST appear in the content"`
	ProfilePack string   `json:"profile_pack,omitempty" jsonschema:"explicit voice override; omitting profile_pack and profile_file preserves the file-scoped project voice and channel"`
	ProfileFile string   `json:"profile_file,omitempty" jsonschema:"explicit voice override loaded from YAML; bypasses the file-scoped project voice and channel; omit to use project guidance"`
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
	opts, err := a.mcpCheckOptions(ctx, execution, in.MaxChars, in.MaxWords, in.Forbid, in.Require, in.ProfilePack, in.ProfileFile)
	if err != nil {
		return nil, check.Report{}, err
	}
	// The draft is read in the language the call's project writes its source in.
	// A call naming none reads it in the server's, which is what opts answers
	// with when it carries no language of its own.
	if in.Project != "" {
		recipe, rerr := a.ResolveMCPCallProject(in.Project)
		if rerr != nil {
			return nil, check.Report{}, rerr
		}
		opts.sourceLocale = a.mcpCallSourceLocale(recipe)
	}
	if in.ContextPath != "" {
		if err := a.resolveTextCheckContext(ctx, in.Project, in.ContextPath, &opts); err != nil {
			return nil, check.Report{}, err
		}
	}
	execution.recordContext("", in.ContextPath, opts)
	execution.Timings.ContextMS += elapsedMS(contextStart)
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
	// A draft named for a destination is held to the voice and terms in force
	// there, so the evaluation record names the project they came from: the
	// call's own project, rather than whatever the server's working directory
	// sits in. A draft checked on its own is held to the call's options alone,
	// and the record names no project.
	var cmd Command
	if in.ContextPath != "" {
		if c, _, cerr := a.mcpCallCommand(ctx, "check_text", in.Project); cerr == nil && c != nil {
			cmd = c
		}
	}
	return nil, execution.report(ctx, a, cmd, target, diags, check.DefaultGate()), nil
}

// resolveTextCheckContext resolves a lexical destination, not an input file.
// Anchoring it to the recipe root keeps collection matching independent of cwd
// and permits a draft for a file that has not yet been created.
func (a *App) resolveTextCheckContext(ctx context.Context, projectPath, contextPath string, opts *checkRunOptions) error {
	validPath := fs.ValidPath(contextPath) && contextPath != "." && !filepath.IsAbs(contextPath)
	if !validPath || strings.ContainsAny(contextPath, "\\\x00") {
		return errors.New("context_path must be a clean project-relative file path")
	}
	cmd, recipe, err := a.mcpCallCommand(ctx, "check_text", projectPath)
	if err != nil {
		return err
	}
	if recipe == "" {
		return &MCPProjectError{Reason: "context_path requires a project: pass `project`, or start the MCP server with -p"}
	}
	destination := filepath.Join(filepath.Dir(recipe), filepath.FromSlash(contextPath))
	voice, err := a.newCheckVoice(cmd, opts.execution.warningSink())
	if err != nil {
		return fmt.Errorf("resolve context_path voice: %w", err)
	}
	defer voice.close()
	vocab, err := a.newCheckTerms(cmd)
	if err != nil {
		return fmt.Errorf("resolve context_path terms: %w", err)
	}
	g, err := a.governFile(ctx, voice, vocab, destination, atPoint{})
	if err != nil {
		return fmt.Errorf("resolve context_path governance: %w", err)
	}
	// A draft is text for the destination rather than a comment in it, so it
	// sits at the destination's own point.
	*opts = opts.govern(g)
	opts.comments = nil
	return nil
}

// checkFileMCP runs the content checkset over a file's content, optionally with
// the bilingual checks when a target is supplied.
func (a *App) checkFileMCP(ctx context.Context, in checkFileInput) (*mcp.CallToolResult, check.Report, error) {
	execution := newCheckExecution()
	contextStart := time.Now()
	a.InitRegistries()
	scoped := in.Diff != "" || in.DiffAgainst != "" || in.Staged || in.DiffRange != ""
	if in.File == "" && !scoped {
		return nil, check.Report{}, errors.New("file is required unless diff, diff_against, staged or diff_range is given")
	}
	var named []string
	for _, source := range []struct {
		name string
		set  bool
	}{{"diff", in.Diff != ""}, {"diff_against", in.DiffAgainst != ""}, {"staged", in.Staged}, {"diff_range", in.DiffRange != ""}} {
		if source.set {
			named = append(named, source.name)
		}
	}
	if err := oneDiffSource(named...); err != nil {
		return nil, check.Report{}, err
	}
	opts, err := a.mcpCheckOptions(ctx, execution, in.MaxChars, in.MaxWords, in.Forbid, in.Require, in.ProfilePack, in.ProfileFile)
	if err != nil {
		return nil, check.Report{}, err
	}
	// Check context is required when bound. A resolution failure is an operation
	// error, not permission to omit the governing rules from a successful report.
	cmd, recipe, err := a.mcpCallCommand(ctx, "check_file", in.Project)
	if err != nil {
		return nil, check.Report{}, err
	}
	// The file is read in the language its own project writes source in, which
	// is what every vocabulary and terminology lookup below keys on.
	opts.sourceLocale = a.mcpCallSourceLocale(recipe)
	var voice *checkVoice
	if opts.profile == nil {
		if voice, err = a.newCheckVoice(cmd, opts.execution.warningSink()); err != nil {
			return nil, check.Report{}, err
		}
		defer voice.close()
	}
	vocab, err := a.newCheckTerms(cmd)
	if err != nil {
		return nil, check.Report{}, err
	}
	g, err := a.governFile(ctx, voice, vocab, in.File, opts.here())
	if err != nil {
		return nil, check.Report{}, err
	}
	opts = opts.govern(g)
	opts.formats, err = a.newCheckFormats(cmd)
	if err != nil {
		return nil, check.Report{}, err
	}
	if scoped {
		return a.checkDiffMCP(ctx, cmd, recipe, in, opts)
	}
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
			return nil, check.Report{}, NoReaderError(berr, in.File, fmtName, a.discoveredPlugins()...)
		}
		if missing {
			return nil, check.Report{}, fmt.Errorf("target file %q does not exist", in.Target)
		}
		target.Blocks = len(blocks)
		// A translated rendering holds no comment layer, so its blocks sit at
		// the source file's own point.
		opts.comments = nil
		execution.recordContext(in.File, "", opts)
		fd, ferr := a.collectFileDiagnostics(ctx, blocks, in.File, opts)
		if ferr != nil {
			return nil, check.Report{}, ferr
		}
		diags = fd
		termRules, terr := vocab.rulesFor(in.File, lang)
		if terr != nil {
			return nil, check.Report{}, terr
		}
		biDiags, bderr := a.collectBilingualDiagnostics(ctx, blocks, in.File, model.LocaleID(lang), in.DNT, termRules, opts)
		if bderr != nil {
			return nil, check.Report{}, bderr
		}
		opts.stampPoints(biDiags, blocks)
		diags = append(diags, biDiags...)
	} else {
		opts.named = true
		blocks, fileDiags, ferr := a.checkFileBlocks(ctx, in.File, validateMode, opts)
		if ferr != nil {
			return nil, check.Report{}, ferr
		}
		execution.recordContexts(in.File, "", opts, blocks)
		target.Blocks = len(blocks)
		diags = fileDiags
		report := execution.report(ctx, a, cmd, target, diags, check.DefaultGate())
		if validateMode == format.ValidationStrict {
			applyStrictValidationGate(&report)
		}
		ApplyFormatterGate(&report)
		return nil, report, nil
	}
	return nil, execution.report(ctx, a, cmd, target, diags, check.DefaultGate()), nil
}

// mcpCheckOptions resolves the shared content-check options for the MCP tools,
// loading a voice profile from a pack/file when one is named and collecting its
// warnings into execution.
func (a *App) mcpCheckOptions(ctx context.Context, execution *checkExecution, maxChars, maxWords int, forbid, require []string, pack, file string) (checkRunOptions, error) {
	opts := checkRunOptions{maxChars: maxChars, maxWords: maxWords, forbid: forbid, require: require,
		voiceContext: check.VoiceContext{Selection: "none"}, execution: execution}
	if pack != "" || file != "" {
		p, err := loadProfileForMCP(pack, file)
		if err != nil {
			return opts, err
		}
		opts.profile = p
		source := file
		if source == "" {
			source = "pack:" + pack
		}
		opts.voiceContext = check.VoiceContext{Selection: "override", Applied: p != nil, Source: source}
		if p != nil {
			opts.voiceContext.Name = p.Name
			if err := execution.warningSink().note(ctx, a, nil, source); err != nil {
				return opts, err
			}
		}
	}
	return opts, nil
}

// checkDiffMCP is check_file scoped to a diff: the same run `kapi check
// --diff-file`, `--diff-against`, `--staged` and `--diff-range` make, with
// governance resolved per changed file unless the call named a profile.
func (a *App) checkDiffMCP(ctx context.Context, cmd Command, recipe string, in checkFileInput, opts checkRunOptions) (*mcp.CallToolResult, check.Report, error) {
	if in.Target != "" {
		return nil, check.Report{}, errors.New("target checks a source against its translation and cannot be scoped to a diff")
	}
	if mode, err := parseValidationMode(in.Validate); err != nil {
		return nil, check.Report{}, err
	} else if mode != format.ValidationOff {
		return nil, check.Report{}, errors.New("reader validation is unavailable with a diff scope; validate the files separately")
	}
	dir, err := os.Getwd()
	if err != nil {
		return nil, check.Report{}, err
	}
	if recipe != "" {
		dir = filepath.Dir(recipe)
	}
	var src *diffSource
	switch {
	case in.Staged:
		src, err = gitDiffStaged(ctx, dir)
	case in.DiffRange != "":
		src, err = gitDiffRange(ctx, dir, in.DiffRange)
	case in.DiffAgainst != "":
		src, err = gitDiffAgainst(ctx, dir, in.DiffAgainst)
	default:
		src, err = parsedDiff(ctx, "diff", []byte(in.Diff), dir)
	}
	if err != nil {
		return nil, check.Report{}, err
	}
	run := diffCheckRun{src: src, cmd: cmd, opts: opts, gate: check.DefaultGate()}
	if in.File != "" {
		run.named = []string{in.File}
	}
	// check_file resolved the file's own voice before it knew the call was
	// scoped to a diff; a diff can name many files, so voice resolves per file
	// unless the call named a profile.
	if in.ProfilePack == "" && in.ProfileFile == "" {
		voice, err := a.newCheckVoice(cmd, opts.execution.warningSink())
		if err != nil {
			return nil, check.Report{}, err
		}
		defer voice.close()
		run.voice = voice
	}
	if run.vocab, err = a.newCheckTerms(cmd); err != nil {
		return nil, check.Report{}, err
	}
	report, err := a.runDiffCheck(ctx, run)
	return nil, report, err
}
