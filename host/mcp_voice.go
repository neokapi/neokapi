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

// init registers the offline voice/terminology/content memory tools on the shared `mcp`
// stdio server. These mirror the cloud bowrain MCP voice tools so non-Claude
// MCP clients (Cursor, generic) get parity locally. They are hand-authored
// because they wrap resources (a voice profile, a terms/content-memory file) rather than
// a single processing tool; the registry's processing tools are exposed
// generically alongside them (see mcp_tools.go), so the MCP surface now mirrors
// the CLI rather than being a curated subset of it.
func init() {
	RegisterMCPToolFactory(registerVoiceMCPTools)
}

func registerVoiceMCPTools(server *mcp.Server, a *App) {

	mcp.AddTool(server, &mcp.Tool{
		Name:        "voice_check",
		Description: "Score text against a voice profile using deterministic vocabulary rules; returns a 0-100 compliance score and findings",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in voiceCheckInput) (*mcp.CallToolResult, voiceCheckMCPOutput, error) {
		p, err := loadProfileForMCP(in.ProfilePack, in.ProfileFile)
		if err != nil {
			return nil, voiceCheckMCPOutput{}, err
		}
		findings, err := RunBlockTool(ctx, coretools.NewVoiceVocabCheckTool(p, nil), in.Text)
		if err != nil {
			return nil, voiceCheckMCPOutput{}, err
		}
		score := profile.CalculateScore(findings)
		score.ProfileID = p.ID
		return nil, voiceCheckMCPOutput{
			Profile:    p.Name,
			Score:      score.Overall,
			Dimensions: score.Dimensions,
			Findings:   findings,
		}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "voice_rewrite",
		Description: "Rewrite text to comply with a voice profile by substituting forbidden/competitor terms " +
			"(deterministic, offline). A rule that names no replacement, and a match on an inflected form of " +
			"a term, are left in place and listed under skipped with the term, its list, severity and reason; " +
			"rewrite those by hand and verify with voice_check.",
	}, voiceRewriteMCP)

}

// voiceRewriteMCP is the voice_rewrite tool: the rule-based rewrite over the
// profile the input names, with the substitutions made and the rules that
// matched and were left in place.
func voiceRewriteMCP(_ context.Context, _ *mcp.CallToolRequest, in voiceCheckInput) (*mcp.CallToolResult, voiceRewriteMCPOutput, error) {
	p, err := loadProfileForMCP(in.ProfilePack, in.ProfileFile)
	if err != nil {
		return nil, voiceRewriteMCPOutput{}, err
	}
	rewritten, changes, skipped := RuleRewrite(p, in.Text)
	out := voiceRewriteMCPOutput{Profile: p.Name, Original: in.Text, Rewritten: rewritten, Skipped: skipped}
	for _, c := range changes {
		out.Changes = append(out.Changes, voiceChangeMCP{From: c.From, To: c.To, Count: c.Count})
	}
	return nil, out, nil
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

type voiceCheckInput struct {
	Text        string `json:"text" jsonschema:"the text to check or rewrite"`
	ProfilePack string `json:"profile_pack,omitempty" jsonschema:"built-in profile pack name"`
	ProfileFile string `json:"profile_file,omitempty" jsonschema:"path to a profile YAML"`
}

type voiceCheckMCPOutput struct {
	Profile    string                   `json:"profile"`
	Score      int                      `json:"score"`
	Dimensions []profile.DimensionScore `json:"dimensions"`
	Findings   []profile.VoiceFinding   `json:"findings"`
}

type voiceChangeMCP struct {
	From  string `json:"from"`
	To    string `json:"to"`
	Count int    `json:"count"`
}

// voiceRewriteMCPOutput is the voice_rewrite result. Skipped lists the
// vocabulary rules that matched and were left in place, with the reason
// (no_replacement, or inflected_form), so an agent knows the text still
// carries violations it has to edit by hand.
type voiceRewriteMCPOutput struct {
	Profile   string                `json:"profile"`
	Original  string                `json:"original"`
	Rewritten string                `json:"rewritten"`
	Changes   []voiceChangeMCP      `json:"changes,omitempty"`
	Skipped   []profile.RewriteSkip `json:"skipped,omitempty"`
}
