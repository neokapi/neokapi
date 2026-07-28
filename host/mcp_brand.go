package host

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/profile/packs"
	coretools "github.com/neokapi/neokapi/core/tools"
	"github.com/neokapi/neokapi/memory"
	"github.com/neokapi/neokapi/terms"
)

// init registers the offline brand/terminology/content memory tools on the shared `mcp`
// stdio server. These mirror the cloud bowrain MCP brand tools so non-Claude
// MCP clients (Cursor, generic) get parity locally. They are hand-authored
// because they wrap resources (a brand profile, a terms/content-memory file) rather than
// a single processing tool; the registry's processing tools are exposed
// generically alongside them (see mcp_tools.go), so the MCP surface now mirrors
// the CLI rather than being a curated subset of it.
func init() {
	RegisterMCPToolFactory(registerBrandMCPTools)
}

func registerBrandMCPTools(server *mcp.Server, a *App) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "brand_guide",
		Description: "Context retrieval: render the voice guidance (markdown) that applies here, from a built-in pack or a profile YAML, to put in context BEFORE generating content. Retrieve first, then write — a check that fails afterwards is the expensive way to learn the same fact",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in brandGuideInput) (*mcp.CallToolResult, brandGuideMCPOutput, error) {
		p, err := loadProfileForMCP(in.ProfilePack, in.ProfileFile)
		if err != nil {
			return nil, brandGuideMCPOutput{}, err
		}
		return nil, brandGuideMCPOutput{Profile: p.Name, Guide: profile.RenderVoiceGuide(p)}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "brand_check",
		Description: "Score text against a brand voice profile using deterministic vocabulary rules; returns a 0-100 compliance score and findings",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in brandCheckInput) (*mcp.CallToolResult, brandCheckMCPOutput, error) {
		p, err := loadProfileForMCP(in.ProfilePack, in.ProfileFile)
		if err != nil {
			return nil, brandCheckMCPOutput{}, err
		}
		findings, err := RunBlockTool(ctx, coretools.NewBrandVocabCheckTool(p, nil), in.Text)
		if err != nil {
			return nil, brandCheckMCPOutput{}, err
		}
		score := profile.CalculateScore(findings)
		score.ProfileID = p.ID
		return nil, brandCheckMCPOutput{
			Profile:    p.Name,
			Score:      score.Overall,
			Dimensions: score.Dimensions,
			Findings:   findings,
		}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "brand_rewrite",
		Description: "Rewrite text to comply with a brand voice profile by substituting forbidden/competitor terms (deterministic, offline)",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in brandCheckInput) (*mcp.CallToolResult, brandRewriteMCPOutput, error) {
		p, err := loadProfileForMCP(in.ProfilePack, in.ProfileFile)
		if err != nil {
			return nil, brandRewriteMCPOutput{}, err
		}
		rewritten, changes := RuleRewrite(p, in.Text)
		out := brandRewriteMCPOutput{Profile: p.Name, Original: in.Text, Rewritten: rewritten}
		for _, c := range changes {
			out.Changes = append(out.Changes, brandChangeMCP{From: c.From, To: c.To, Count: c.Count})
		}
		return nil, out, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "term_lookup",
		Description: "Context retrieval: look up what a term should be in this project, so terminology is consistent by construction rather than corrected afterwards",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in termLookupInput) (*mcp.CallToolResult, termLookupMCPOutput, error) {
		path := in.Terms
		if path == "" {
			path = "terms.db"
		}
		tb, err := terms.NewSQLiteStore(path)
		if err != nil {
			return nil, termLookupMCPOutput{}, fmt.Errorf("open terms: %w", err)
		}
		defer tb.Close()
		opts := terms.LookupOptions{
			SourceLocale: model.LocaleID(in.SourceLang),
			TargetLocale: model.LocaleID(in.TargetLang),
			MatchModes:   []model.MatchStrategy{model.MatchStrategyExact, model.MatchStrategyNormalized},
		}
		matches, err := tb.Lookup(ctx, in.Term, opts)
		if err != nil {
			return nil, termLookupMCPOutput{}, fmt.Errorf("term lookup: %w", err)
		}
		var out termLookupMCPOutput
		for _, m := range matches {
			out.Matches = append(out.Matches, termMatchMCP{
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
		Description: "Context retrieval: search this project's content memory for wording already approved for the same source text, so approved phrasing is reused rather than re-invented",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in tMSearchInput) (*mcp.CallToolResult, tMSearchMCPOutput, error) {
		path := in.Memory
		if path == "" {
			path = "memory.db"
		}
		tm, err := memory.NewSQLiteStore(path)
		if err != nil {
			return nil, tMSearchMCPOutput{}, fmt.Errorf("open content memory: %w", err)
		}
		defer tm.Close()
		minScore := in.MinScore
		if minScore == 0 {
			minScore = 0.7
		}
		src := model.LocaleID(in.SourceLang)
		tgt := model.LocaleID(in.TargetLang)
		matches, err := tm.LookupText(ctx, in.Text, src, tgt, memory.LookupOptions{MinScore: minScore, MaxResults: 10})
		if err != nil {
			return nil, tMSearchMCPOutput{}, fmt.Errorf("tm lookup: %w", err)
		}
		var out tMSearchMCPOutput
		for _, m := range matches {
			out.Matches = append(out.Matches, tMMatchMCP{
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

// loadProfileForMCP resolves a profile from a built-in pack name or a profile YAML path.
func loadProfileForMCP(pack, file string) (*profile.VoiceProfile, error) {
	if file != "" {
		f, err := os.Open(file)
		if err != nil {
			return nil, fmt.Errorf("open profile: %w", err)
		}
		defer f.Close()
		return profile.LoadProfileYAML(f)
	}
	if pack != "" {
		return packs.Load(pack)
	}
	return nil, errors.New("specify profile_pack or profile_file")
}

// --- MCP input/output types ---

type brandGuideInput struct {
	ProfilePack string `json:"profile_pack,omitempty" jsonschema:"built-in profile pack name (e.g. marketing-blog, technical-docs)"`
	ProfileFile string `json:"profile_file,omitempty" jsonschema:"path to a profile YAML"`
}

type brandGuideMCPOutput struct {
	Profile string `json:"profile"`
	Guide   string `json:"guide"`
}

type brandCheckInput struct {
	Text        string `json:"text" jsonschema:"the text to check or rewrite"`
	ProfilePack string `json:"profile_pack,omitempty" jsonschema:"built-in profile pack name"`
	ProfileFile string `json:"profile_file,omitempty" jsonschema:"path to a profile YAML"`
}

type brandCheckMCPOutput struct {
	Profile    string                      `json:"profile"`
	Score      int                         `json:"score"`
	Dimensions []profile.DimensionScore    `json:"dimensions"`
	Findings   []profile.BrandVoiceFinding `json:"findings"`
}

type brandChangeMCP struct {
	From  string `json:"from"`
	To    string `json:"to"`
	Count int    `json:"count"`
}

type brandRewriteMCPOutput struct {
	Profile   string           `json:"profile"`
	Original  string           `json:"original"`
	Rewritten string           `json:"rewritten"`
	Changes   []brandChangeMCP `json:"changes,omitempty"`
}

type termLookupInput struct {
	Term       string `json:"term" jsonschema:"the term to look up"`
	SourceLang string `json:"source_lang,omitempty" jsonschema:"source locale (e.g. en)"`
	TargetLang string `json:"target_lang,omitempty" jsonschema:"target locale (e.g. fr)"`
	Terms      string `json:"termbase,omitempty" jsonschema:"path to the terms store db (default: terms.db)"`
}

type termMatchMCP struct {
	Term      string `json:"term"`
	Locale    string `json:"locale"`
	Status    string `json:"status,omitempty"`
	MatchType string `json:"match_type,omitempty"`
}

type termLookupMCPOutput struct {
	Matches []termMatchMCP `json:"matches"`
	Total   int            `json:"total"`
}

type tMSearchInput struct {
	Text       string  `json:"text" jsonschema:"source text to search for"`
	SourceLang string  `json:"source_lang" jsonschema:"source locale (e.g. en)"`
	TargetLang string  `json:"target_lang" jsonschema:"target locale (e.g. fr)"`
	MinScore   float64 `json:"min_score,omitempty" jsonschema:"minimum match score 0-1 (default 0.7)"`
	Memory     string  `json:"memory,omitempty" jsonschema:"path to the content memory db (default: memory.db)"`
}

type tMMatchMCP struct {
	Source    string  `json:"source"`
	Target    string  `json:"target"`
	Score     float64 `json:"score"`
	MatchType string  `json:"match_type,omitempty"`
}

type tMSearchMCPOutput struct {
	Matches []tMMatchMCP `json:"matches"`
	Total   int          `json:"total"`
}
