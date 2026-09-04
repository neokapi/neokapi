package host

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/neokapi/neokapi/core/profile"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// northwindProfileYAML forbids three terms and names no replacement for any of
// them: the profile from issue #2219, where the rewrite used to return the
// text unchanged and say nothing.
const northwindProfileYAML = `name: Northwind
tone:
    personality: [plain, direct]
    formality: neutral
    emotion: neutral
    humor: none
style:
    active_voice: true
vocabulary:
    forbidden_terms:
        - term: utilize
          severity: major
        - term: leverage
          severity: major
        - term: cutting-edge
          severity: major
`

// mixedProfileYAML names a replacement for one term and none for the others.
const mixedProfileYAML = `name: Mixed
tone:
    formality: neutral
vocabulary:
    forbidden_terms:
        - term: utilize
          replacement: use
        - term: leverage
          severity: minor
          note: say what the reader does
    competitor_terms:
        - term: Globex
`

func writeVoiceYAMLFixture(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "voice.yaml")
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
	return path
}

func skippedTerms(skipped []profile.RewriteSkip) []string {
	out := make([]string, 0, len(skipped))
	for _, s := range skipped {
		out = append(out, s.Term)
	}
	return out
}

func TestVoiceRewriteMCP_ReportsSkipped(t *testing.T) {
	path := writeVoiceYAMLFixture(t, northwindProfileYAML)
	text := "Leverage our cutting-edge workspace to utilize your content."

	_, out, err := voiceRewriteMCP(context.Background(), nil, voiceCheckInput{Text: text, ProfileFile: path})
	require.NoError(t, err)

	assert.Equal(t, "Northwind", out.Profile)
	assert.Equal(t, text, out.Rewritten, "nothing to substitute, so the text is unchanged")
	assert.Empty(t, out.Changes)
	assert.Equal(t, []string{"utilize", "leverage", "cutting-edge"}, skippedTerms(out.Skipped))
	for _, s := range out.Skipped {
		assert.Equal(t, profile.RewriteSkipNoReplacement, s.Reason, s.Term)
		assert.Equal(t, profile.SeverityMajor, s.Severity, s.Term)
		assert.Equal(t, "forbidden", s.List, s.Term)
		assert.Equal(t, 1, s.Count, s.Term)
	}
	assert.Equal(t, []string{"Leverage"}, out.Skipped[1].Matched, "the spelling in the text, for the hand edit")
}

func TestVoiceRewriteMCP_MixedProfile(t *testing.T) {
	path := writeVoiceYAMLFixture(t, mixedProfileYAML)

	_, out, err := voiceRewriteMCP(context.Background(), nil, voiceCheckInput{
		Text:        "Leverage Globex to utilize your content.",
		ProfileFile: path,
	})
	require.NoError(t, err)

	assert.Equal(t, "Leverage Globex to use your content.", out.Rewritten)
	assert.Equal(t, []voiceChangeMCP{{From: "utilize", To: "use", Count: 1}}, out.Changes)
	require.Len(t, out.Skipped, 2)
	assert.Equal(t, profile.RewriteSkip{
		Term: "leverage", List: "forbidden", Severity: profile.SeverityMinor,
		Note: "say what the reader does", Matched: []string{"Leverage"}, Count: 1,
		Reason: profile.RewriteSkipNoReplacement,
	}, out.Skipped[0])
	assert.Equal(t, profile.RewriteSkip{
		Term: "Globex", List: "competitor", Severity: profile.SeverityCritical,
		Matched: []string{"Globex"}, Count: 1, Reason: profile.RewriteSkipNoReplacement,
	}, out.Skipped[1])
}

func TestVoiceRewriteMCP_CleanText(t *testing.T) {
	path := writeVoiceYAMLFixture(t, northwindProfileYAML)

	_, out, err := voiceRewriteMCP(context.Background(), nil, voiceCheckInput{Text: "Use the workspace.", ProfileFile: path})
	require.NoError(t, err)
	assert.Empty(t, out.Changes)
	assert.Empty(t, out.Skipped, "a clean text reports nothing skipped")
}

// TestVoiceRewriteMCP_SurfaceDeclaresSkipped proves what an agent sees: the
// tool's description says the skipped list exists and what to do about it,
// and the output schema carries the list with its reason.
func TestVoiceRewriteMCP_SurfaceDeclaresSkipped(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "kapi", Version: "test"}, nil)
	registerVoiceMCPTools(server, &App{})

	ctx := t.Context()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	require.NoError(t, err)
	defer serverSession.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "test"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	require.NoError(t, err)
	defer session.Close()

	res, err := session.ListTools(ctx, &mcp.ListToolsParams{})
	require.NoError(t, err)
	var tool *mcp.Tool
	for _, tl := range res.Tools {
		if tl.Name == "voice_rewrite" {
			tool = tl
		}
	}
	require.NotNil(t, tool, "voice_rewrite is registered")
	assert.Contains(t, tool.Description, "skipped")
	assert.Contains(t, tool.Description, "voice_check")

	raw, err := json.Marshal(tool.OutputSchema)
	require.NoError(t, err)
	var schema struct {
		Properties map[string]struct {
			Items struct {
				Properties map[string]json.RawMessage `json:"properties"`
			} `json:"items"`
		} `json:"properties"`
	}
	require.NoError(t, json.Unmarshal(raw, &schema), "output schema: %s", raw)
	skipped, ok := schema.Properties["skipped"]
	require.True(t, ok, "output schema lists skipped: %s", raw)
	for _, field := range []string{"term", "list", "severity", "matched", "count", "reason"} {
		assert.Contains(t, skipped.Items.Properties, field)
	}
}
