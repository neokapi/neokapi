package main

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Each tool call that writes a task's files is classed by its route: the
// contract, a merge, or the host's own tools. A preview, a schema lookup and
// a write to a scratch file are no route.
func TestPairedWriteRouteOf(t *testing.T) {
	files := []string{"locales/nb/messages.po", "docs/guide.md"}
	ws := "/cell/workspace"
	shell := func(command string) map[string]any { return map[string]any{"command": command} }
	for _, tc := range []struct {
		tool  string
		input map[string]any
		want  string
	}{
		{"Bash", shell("kapi apply /tmp/x/change.json"), pairedRouteContract},
		{"Bash", shell(`printf '{"ops":[{"op":"set_content","at":{"doc":"docs/guide.md"},"text":"<x id=\"1\"/>"}]}' | kapi apply`), pairedRouteContract},
		{"shell", shell("kapi-files apply change.json --json"), pairedRouteContract},
		{"Bash", shell("kapi ksed -i 's/colour/color/' docs/guide.md"), pairedRouteContract},
		{"Bash", shell("kapi apply change.json --dry-run"), ""},
		{"Bash", shell("kapi apply --schema set_attribute"), ""},
		{"Bash", shell("kapi merge out/nb.xliff"), pairedRouteMerge},
		{"Bash", shell("sed -i '' 's/Bok/Bestill/' locales/nb/messages.po"), pairedRouteNative},
		{"Bash", shell("python3 -c \"open('docs/guide.md','w').write(x)\""), pairedRouteNative},
		{"Bash", shell("cat > /tmp/change.json <<'EOF'\n{}\nEOF"), ""},
		{"Bash", shell("grep -n Bok locales/nb/messages.po"), ""},
		{"Bash", shell("kapi check locales/nb/messages.po 2>&1"), ""},
		{"Edit", map[string]any{"file_path": "/cell/workspace/locales/nb/messages.po"}, pairedRouteNative},
		{"Write", map[string]any{"file_path": "/cell/workspace/notes.md"}, ""},
		{"file_change", map[string]any{"changes": []any{map[string]any{"path": "docs/guide.md"}}}, pairedRouteNative},
		{"mcp__kapi__apply_edits", map[string]any{"ops": []any{}}, pairedRouteContract},
		{"mcp__kapi__apply_edits", map[string]any{"mode": "preview"}, ""},
		{"mcp__kapi__read_blocks", map[string]any{"doc": "docs/guide.md"}, ""},
	} {
		assert.Equal(t, tc.want, pairedWriteRouteOf(tc.tool, tc.input, files, ws), "%s %v", tc.tool, tc.input)
	}
	assert.Equal(t, "none", pairedWriteRoute(nil))
	assert.Equal(t, "contract+native", pairedWriteRoute([]string{"native", "contract"}))
}

// The summary reports each cell's medians by route.
func TestPairedSummaryReportsMediansByRoute(t *testing.T) {
	tokens := func(n int64) *int64 { return &n }
	session := PairedSession{Task: "add-edition-markup", Agent: PairedAgentSpec{Host: "claude"}, Condition: "skill-cli"}
	row := func(route string, seconds int64, tools int) pairedScoreRow {
		return pairedScoreRow{Session: session, Phase: "main", Status: "completed", ObjectivePassed: true, WriteRoute: route,
			DurationMS: seconds * 1000, ToolCalls: tools, InputTokens: tokens(1000), OutputTokens: tokens(10)}
	}
	var markdown strings.Builder
	writePairedSummary(&markdown, []pairedScoreRow{row("contract", 100, 20), row("contract", 120, 30), row("merge", 300, 60)})
	text := markdown.String()
	assert.Contains(t, text, "## Write routes")
	assert.Contains(t, text, "| add-edition-markup | claude | skill-cli | contract | 2 | 2 | 110 | 1000 | 25 |")
	assert.Contains(t, text, "| add-edition-markup | claude | skill-cli | merge | 1 | 1 | 300 | 1000 | 60 |")
}

// The stream records the routes an attempt's tool calls took.
func TestPairedStreamRecordsWriteRoutes(t *testing.T) {
	stream := `{"type":"system","subtype":"init","model":"test","session_id":"s"}` + "\n" +
		`{"type":"assistant","message":{"model":"test","content":[{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"kapi merge out/nb.xliff"}}]}}` + "\n" +
		`{"type":"assistant","message":{"model":"test","content":[{"type":"tool_use","id":"t2","name":"Bash","input":{"command":"kapi apply change.json"}}]}}` + "\n" +
		`{"type":"result","subtype":"success","is_error":false,"result":"done","usage":{"input_tokens":1,"output_tokens":1}}`
	result, err := parsePairedAgentStream(strings.NewReader(stream), PairedLaunch{Agent: PairedAgentSpec{Host: "claude", Model: "test"}, Condition: "skill-cli"})
	assert.NoError(t, err)
	assert.Equal(t, []string{"merge", "contract"}, result.WriteRoutes)
}
