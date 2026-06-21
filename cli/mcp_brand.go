package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/neokapi/neokapi/core/brand"
	"github.com/neokapi/neokapi/core/brand/packs"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/tool"
	coretools "github.com/neokapi/neokapi/core/tools"
	"github.com/neokapi/neokapi/sievepen"
	"github.com/neokapi/neokapi/termbase"
	"github.com/pmezard/go-difflib/difflib"
)

// init registers the offline brand/terminology/TM tools on the shared `mcp`
// stdio server. These mirror the cloud bowrain MCP brand tools so non-Claude
// MCP clients (Cursor, generic) get parity locally. They are hand-authored
// because they wrap resources (a brand profile, a termbase/TM file) rather than
// a single processing tool; the registry's processing tools are exposed
// generically alongside them (see mcp_tools.go), so the MCP surface now mirrors
// the CLI rather than being a curated subset of it.
func init() {
	RegisterMCPToolFactory(registerBrandMCPTools)
}

func registerBrandMCPTools(server *mcp.Server, a *App) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "brand_guide",
		Description: "Render a brand voice guide (markdown) from a starter pack or a profile YAML, to inject into context before generating content",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in BrandGuideInput) (*mcp.CallToolResult, BrandGuideMCPOutput, error) {
		p, err := loadProfileForMCP(in.ProfilePack, in.ProfileFile)
		if err != nil {
			return nil, BrandGuideMCPOutput{}, err
		}
		return nil, BrandGuideMCPOutput{Profile: p.Name, Guide: brand.RenderVoiceGuide(p)}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "brand_check",
		Description: "Score text against a brand voice profile using deterministic vocabulary rules; returns a 0-100 compliance score and findings",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in BrandCheckInput) (*mcp.CallToolResult, BrandCheckMCPOutput, error) {
		p, err := loadProfileForMCP(in.ProfilePack, in.ProfileFile)
		if err != nil {
			return nil, BrandCheckMCPOutput{}, err
		}
		findings, err := runBlockTool(ctx, coretools.NewBrandVocabCheckTool(p, nil), in.Text)
		if err != nil {
			return nil, BrandCheckMCPOutput{}, err
		}
		score := brand.CalculateScore(findings)
		score.ProfileID = p.ID
		return nil, BrandCheckMCPOutput{
			Profile:    p.Name,
			Score:      score.Overall,
			Dimensions: score.Dimensions,
			Findings:   findings,
		}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "brand_rewrite",
		Description: "Rewrite text to comply with a brand voice profile by substituting forbidden/competitor terms (deterministic, offline)",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in BrandCheckInput) (*mcp.CallToolResult, BrandRewriteMCPOutput, error) {
		p, err := loadProfileForMCP(in.ProfilePack, in.ProfileFile)
		if err != nil {
			return nil, BrandRewriteMCPOutput{}, err
		}
		rewritten, changes := ruleRewrite(p, in.Text)
		out := BrandRewriteMCPOutput{Profile: p.Name, Original: in.Text, Rewritten: rewritten}
		for _, c := range changes {
			out.Changes = append(out.Changes, BrandChangeMCP{From: c.From, To: c.To, Count: c.Count})
		}
		return nil, out, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "rewrite_file",
		Description: "Rewrite the text inside a file (Word, PowerPoint, JSON, XLIFF, Markdown, …) following an instruction and/or a brand voice profile, preserving the document's format and structure; returns the rewritten document content",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in RewriteFileInput) (*mcp.CallToolResult, RewriteFileMCPOutput, error) {
		return a.rewriteFileMCP(ctx, in)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "term_lookup",
		Description: "Look up a term in a local termbase to enforce consistent terminology",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in TermLookupInput) (*mcp.CallToolResult, TermLookupMCPOutput, error) {
		path := in.Termbase
		if path == "" {
			path = "termbase.db"
		}
		tb, err := termbase.NewSQLiteTermBase(path)
		if err != nil {
			return nil, TermLookupMCPOutput{}, fmt.Errorf("open termbase: %w", err)
		}
		defer tb.Close()
		opts := termbase.LookupOptions{
			SourceLocale: model.LocaleID(in.SourceLang),
			TargetLocale: model.LocaleID(in.TargetLang),
			MatchModes:   []model.MatchStrategy{model.MatchStrategyExact, model.MatchStrategyNormalized},
		}
		matches, err := tb.Lookup(ctx, in.Term, opts)
		if err != nil {
			return nil, TermLookupMCPOutput{}, fmt.Errorf("term lookup: %w", err)
		}
		var out TermLookupMCPOutput
		for _, m := range matches {
			out.Matches = append(out.Matches, TermMatchMCP{
				Term:      m.Term.Text,
				Locale:    string(m.Term.Locale),
				Status:    string(m.Term.Status),
				MatchType: string(m.MatchType),
			})
		}
		out.Total = len(out.Matches)
		return nil, out, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "tm_search",
		Description: "Search a local translation memory for prior translations of source text",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in TMSearchInput) (*mcp.CallToolResult, TMSearchMCPOutput, error) {
		path := in.TM
		if path == "" {
			path = "tm.db"
		}
		tm, err := sievepen.NewSQLiteTM(path)
		if err != nil {
			return nil, TMSearchMCPOutput{}, fmt.Errorf("open TM: %w", err)
		}
		defer tm.Close()
		minScore := in.MinScore
		if minScore == 0 {
			minScore = 0.7
		}
		src := model.LocaleID(in.SourceLang)
		tgt := model.LocaleID(in.TargetLang)
		matches, err := tm.LookupText(ctx, in.Text, src, tgt, sievepen.LookupOptions{MinScore: minScore, MaxResults: 10})
		if err != nil {
			return nil, TMSearchMCPOutput{}, fmt.Errorf("tm lookup: %w", err)
		}
		var out TMSearchMCPOutput
		for _, m := range matches {
			out.Matches = append(out.Matches, TMMatchMCP{
				Source:    m.Entry.VariantText(src),
				Target:    m.Entry.VariantText(tgt),
				Score:     m.Score,
				MatchType: string(m.MatchType),
			})
		}
		out.Total = len(out.Matches)
		return nil, out, nil
	})
}

// rewriteFileMCP rewrites the content inside a file through the format-aware
// rewrite tool and returns the reconstructed document. The faithful round-trip
// (editDocument into a buffer) keeps the document's structure and inline codes
// intact and rewrites only the editable text — the safe way for an MCP client to
// edit a Word/PowerPoint/JSON/XLIFF document it cannot open directly. An
// optional brand profile folds its voice guide into the instruction.
func (a *App) rewriteFileMCP(ctx context.Context, in RewriteFileInput) (*mcp.CallToolResult, RewriteFileMCPOutput, error) {
	if in.File == "" {
		return nil, RewriteFileMCPOutput{}, errors.New("file is required")
	}
	instruction := strings.TrimSpace(in.Instruction)

	var profileName string
	if in.ProfilePack != "" || in.ProfileFile != "" {
		p, err := loadProfileForMCP(in.ProfilePack, in.ProfileFile)
		if err != nil {
			return nil, RewriteFileMCPOutput{}, err
		}
		profileName = p.Name
		guide := brand.RenderVoiceGuide(p)
		if instruction == "" {
			instruction = "Rewrite the text so it complies with the following brand voice guide. " +
				"Preserve meaning and any placeholders or markup.\n\n" + guide
		} else {
			instruction += "\n\nAlso comply with the following brand voice guide:\n\n" + guide
		}
	}
	if instruction == "" {
		return nil, RewriteFileMCPOutput{}, errors.New("provide an instruction or a profile (profile_pack/profile_file)")
	}

	original, err := os.ReadFile(in.File)
	if err != nil {
		return nil, RewriteFileMCPOutput{}, fmt.Errorf("read file: %w", err)
	}

	// Build the rewrite tool through the registry so saved credentials and the
	// configured provider/model default are resolved the same way the CLI does.
	t, err := a.ToolReg.NewToolWithConfig(registry.ToolID("rewrite"), map[string]any{"instruction": instruction}, "")
	if err != nil {
		return nil, RewriteFileMCPOutput{}, err
	}
	bt, ok := t.(*tool.BaseTool)
	if !ok {
		return nil, RewriteFileMCPOutput{}, fmt.Errorf("rewrite: unexpected tool type %T", t)
	}

	var buf bytes.Buffer
	if err := a.editDocument(ctx, in.File, bt, "", false, "", &buf); err != nil {
		return nil, RewriteFileMCPOutput{}, err
	}
	newContent := buf.Bytes()

	return nil, RewriteFileMCPOutput{
		File:    in.File,
		Profile: profileName,
		Changed: !bytes.Equal(original, newContent),
		Content: string(newContent),
		Summary: rewriteFileSummary(original, newContent),
	}, nil
}

// rewriteFileSummary describes the change between two document byte slices. For
// text documents it reports added/removed line counts; for binary documents
// (e.g. .docx) it reports only that the content was updated.
func rewriteFileSummary(before, after []byte) string {
	if bytes.Equal(before, after) {
		return "no changes"
	}
	if !utf8.Valid(before) || !utf8.Valid(after) {
		return "document content updated"
	}
	d := difflib.UnifiedDiff{
		A:       difflib.SplitLines(string(before)),
		B:       difflib.SplitLines(string(after)),
		Context: 0,
	}
	s, err := difflib.GetUnifiedDiffString(d)
	if err != nil {
		return "document content updated"
	}
	added, removed := 0, 0
	for line := range strings.SplitSeq(s, "\n") {
		switch {
		case strings.HasPrefix(line, "+++"), strings.HasPrefix(line, "---"):
			// file headers, not content
		case strings.HasPrefix(line, "+"):
			added++
		case strings.HasPrefix(line, "-"):
			removed++
		}
	}
	return fmt.Sprintf("%d line(s) added, %d removed", added, removed)
}

// loadProfileForMCP resolves a profile from a starter pack name or a profile YAML path.
func loadProfileForMCP(pack, file string) (*brand.VoiceProfile, error) {
	if file != "" {
		f, err := os.Open(file)
		if err != nil {
			return nil, fmt.Errorf("open profile: %w", err)
		}
		defer f.Close()
		return brand.LoadProfileYAML(f)
	}
	if pack != "" {
		return packs.Load(pack)
	}
	return nil, errors.New("specify profile_pack or profile_file")
}

// --- MCP input/output types ---

type BrandGuideInput struct {
	ProfilePack string `json:"profile_pack,omitempty" jsonschema:"starter pack name (e.g. marketing-blog, technical-docs)"`
	ProfileFile string `json:"profile_file,omitempty" jsonschema:"path to a profile YAML"`
}

type BrandGuideMCPOutput struct {
	Profile string `json:"profile"`
	Guide   string `json:"guide"`
}

type BrandCheckInput struct {
	Text        string `json:"text" jsonschema:"the text to check or rewrite"`
	ProfilePack string `json:"profile_pack,omitempty" jsonschema:"starter pack name"`
	ProfileFile string `json:"profile_file,omitempty" jsonschema:"path to a profile YAML"`
}

type BrandCheckMCPOutput struct {
	Profile    string                    `json:"profile"`
	Score      int                       `json:"score"`
	Dimensions []brand.DimensionScore    `json:"dimensions"`
	Findings   []brand.BrandVoiceFinding `json:"findings"`
}

type BrandChangeMCP struct {
	From  string `json:"from"`
	To    string `json:"to"`
	Count int    `json:"count"`
}

type BrandRewriteMCPOutput struct {
	Profile   string           `json:"profile"`
	Original  string           `json:"original"`
	Rewritten string           `json:"rewritten"`
	Changes   []BrandChangeMCP `json:"changes,omitempty"`
}

type RewriteFileInput struct {
	File        string `json:"file" jsonschema:"path to the file whose content should be rewritten"`
	Instruction string `json:"instruction,omitempty" jsonschema:"plain-language instruction describing how to rewrite the text (optional if a profile is given)"`
	ProfilePack string `json:"profile_pack,omitempty" jsonschema:"starter pack name to rewrite on-brand (e.g. marketing-blog)"`
	ProfileFile string `json:"profile_file,omitempty" jsonschema:"path to a brand voice profile YAML to rewrite on-brand"`
}

type RewriteFileMCPOutput struct {
	File    string `json:"file"`
	Profile string `json:"profile,omitempty"`
	Changed bool   `json:"changed"`
	Content string `json:"content"`
	Summary string `json:"summary"`
}

type TermLookupInput struct {
	Term       string `json:"term" jsonschema:"the term to look up"`
	SourceLang string `json:"source_lang,omitempty" jsonschema:"source locale (e.g. en)"`
	TargetLang string `json:"target_lang,omitempty" jsonschema:"target locale (e.g. fr)"`
	Termbase   string `json:"termbase,omitempty" jsonschema:"path to the termbase db (default: termbase.db)"`
}

type TermMatchMCP struct {
	Term      string `json:"term"`
	Locale    string `json:"locale"`
	Status    string `json:"status,omitempty"`
	MatchType string `json:"match_type,omitempty"`
}

type TermLookupMCPOutput struct {
	Matches []TermMatchMCP `json:"matches"`
	Total   int            `json:"total"`
}

type TMSearchInput struct {
	Text       string  `json:"text" jsonschema:"source text to search for"`
	SourceLang string  `json:"source_lang" jsonschema:"source locale (e.g. en)"`
	TargetLang string  `json:"target_lang" jsonschema:"target locale (e.g. fr)"`
	MinScore   float64 `json:"min_score,omitempty" jsonschema:"minimum match score 0-1 (default 0.7)"`
	TM         string  `json:"tm,omitempty" jsonschema:"path to the TM db (default: tm.db)"`
}

type TMMatchMCP struct {
	Source    string  `json:"source"`
	Target    string  `json:"target"`
	Score     float64 `json:"score"`
	MatchType string  `json:"match_type,omitempty"`
}

type TMSearchMCPOutput struct {
	Matches []TMMatchMCP `json:"matches"`
	Total   int          `json:"total"`
}
