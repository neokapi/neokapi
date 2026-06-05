package cli

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/neokapi/neokapi/core/brand"
	"github.com/neokapi/neokapi/core/brand/packs"
	"github.com/neokapi/neokapi/core/model"
	coretools "github.com/neokapi/neokapi/core/tools"
	"github.com/neokapi/neokapi/sievepen"
	"github.com/neokapi/neokapi/termbase"
)

// init registers the offline brand/terminology/TM tools on the shared `mcp`
// stdio server. These mirror the cloud bowrain MCP brand tools so non-Claude
// MCP clients (Cursor, generic) get parity locally — kapi skills themselves use
// the CLI, per the CLI-vs-MCP boundary.
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
