package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/neokapi/neokapi/core/check"
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
			"forbidden/required patterns, and brand vocabulary when a profile is given) and return a " +
			"kapi.check/v1 Report: pass, a 0-100 score, the gate, and a finding per stable rule id. Use it to " +
			"check content you authored, then fix and re-check until it passes.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in CheckTextInput) (*mcp.CallToolResult, check.Report, error) {
		return a.checkTextMCP(ctx, in)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "check_file",
		Description: "Verify the content inside a file (Word, PowerPoint, JSON, XLIFF, Markdown, …) against the " +
			"content checkset and return a kapi.check/v1 Report with per-block locations. The check counterpart to " +
			"rewrite_file: author → check_file → fix the flagged block (optionally via rewrite_file) → re-check " +
			"until pass. Pass target/target_lang to also run bilingual localization checks.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in CheckFileInput) (*mcp.CallToolResult, check.Report, error) {
		return a.checkFileMCP(ctx, in)
	})
}

// CheckTextInput is the input to the check_text MCP tool.
type CheckTextInput struct {
	Text        string   `json:"text" jsonschema:"the text to verify"`
	MaxChars    int      `json:"max_chars,omitempty" jsonschema:"flag content longer than this many characters (0 = off)"`
	MaxWords    int      `json:"max_words,omitempty" jsonschema:"flag content with more than this many words (0 = off)"`
	Forbid      []string `json:"forbid,omitempty" jsonschema:"regex that must NOT appear in the content"`
	Require     []string `json:"require,omitempty" jsonschema:"regex that MUST appear in the content"`
	ProfilePack string   `json:"profile_pack,omitempty" jsonschema:"brand starter pack to check vocabulary against (e.g. marketing-blog)"`
	ProfileFile string   `json:"profile_file,omitempty" jsonschema:"path to a brand voice profile YAML"`
}

// CheckFileInput is the input to the check_file MCP tool.
type CheckFileInput struct {
	File        string   `json:"file" jsonschema:"path to the file whose content should be checked"`
	MaxChars    int      `json:"max_chars,omitempty" jsonschema:"flag content longer than this many characters (0 = off)"`
	MaxWords    int      `json:"max_words,omitempty" jsonschema:"flag content with more than this many words (0 = off)"`
	Forbid      []string `json:"forbid,omitempty" jsonschema:"regex that must NOT appear in the content"`
	Require     []string `json:"require,omitempty" jsonschema:"regex that MUST appear in the content"`
	ProfilePack string   `json:"profile_pack,omitempty" jsonschema:"brand starter pack to check vocabulary against"`
	ProfileFile string   `json:"profile_file,omitempty" jsonschema:"path to a brand voice profile YAML"`
	Target      string   `json:"target,omitempty" jsonschema:"translated target file to check against the source (enables bilingual l10n checks)"`
	TargetLang  string   `json:"target_lang,omitempty" jsonschema:"locale of the target file (e.g. de)"`
	DNT         []string `json:"dnt,omitempty" jsonschema:"do-not-translate terms that must survive verbatim into the target"`
}

// checkTextMCP runs the source-side content checkset over a text snippet.
func (a *App) checkTextMCP(ctx context.Context, in CheckTextInput) (*mcp.CallToolResult, check.Report, error) {
	a.InitRegistries()
	opts, err := a.mcpCheckOptions(in.MaxChars, in.MaxWords, in.Forbid, in.Require, in.ProfilePack, in.ProfileFile)
	if err != nil {
		return nil, check.Report{}, err
	}
	block := &model.Block{ID: "text", Translatable: true, Source: []model.Run{{Text: &model.TextRun{Text: in.Text}}}}
	diags, err := a.collectFileDiagnostics(ctx, []*model.Block{block}, "text", opts)
	if err != nil {
		return nil, check.Report{}, err
	}
	return nil, check.BuildReport(check.Target{Kind: "text", Blocks: 1}, diags, check.DefaultGate()), nil
}

// checkFileMCP runs the content checkset over a file's content, optionally with
// the bilingual localization checks when a target is supplied.
func (a *App) checkFileMCP(ctx context.Context, in CheckFileInput) (*mcp.CallToolResult, check.Report, error) {
	a.InitRegistries()
	if in.File == "" {
		return nil, check.Report{}, errors.New("file is required")
	}
	opts, err := a.mcpCheckOptions(in.MaxChars, in.MaxWords, in.Forbid, in.Require, in.ProfilePack, in.ProfileFile)
	if err != nil {
		return nil, check.Report{}, err
	}
	srcLang := firstNonEmpty(a.SourceLang, "en")
	target := check.Target{Kind: "file", File: in.File}
	var diags []check.Diagnostic

	if in.Target != "" {
		lang := in.TargetLang
		if lang == "" {
			lang = "und"
		}
		unit := verifyUnit{sourcePath: in.File, targetPath: in.Target, locale: lang, displayPath: in.Target}
		blocks, missing, berr := a.bilingualBlocks(ctx, unit)
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
		diags = append(diags, a.collectBilingualDiagnostics(ctx, blocks, in.File, model.LocaleID(lang), in.DNT)...)
	} else {
		blocks, rerr := a.readBlocks(ctx, in.File, srcLang)
		if rerr != nil {
			return nil, check.Report{}, rerr
		}
		target.Blocks = len(blocks)
		diags, err = a.collectFileDiagnostics(ctx, blocks, in.File, opts)
		if err != nil {
			return nil, check.Report{}, err
		}
	}
	return nil, check.BuildReport(target, diags, check.DefaultGate()), nil
}

// mcpCheckOptions resolves the shared content-check options for the MCP tools,
// loading a brand profile from a pack/file when one is named.
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
