package host

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/profile/packs"
	coretools "github.com/neokapi/neokapi/core/tools"
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
