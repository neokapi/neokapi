//go:build !js

package host

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/contextop"
	"github.com/neokapi/neokapi/core/workspace"
)

// callSessionSummary calls context_session_summary for one session of the
// project at root, and returns the answer decoded and as the JSON it arrived in.
func callSessionSummary(t *testing.T, app *App, root, session string) (contextSessionOutput, string) {
	t.Helper()
	res, err := growthSession(t, app).CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "context_session_summary",
		Arguments: map[string]any{"project": recipeOf(root), "session": session},
	})
	require.NoError(t, err)
	require.False(t, res.IsError, resultText(res))
	body, err := json.Marshal(res.StructuredContent)
	require.NoError(t, err)
	var out contextSessionOutput
	require.NoError(t, json.Unmarshal(body, &out))
	return out, string(body)
}

func TestContextSessionSummary_ReportsWhatTheHookCaught(t *testing.T) {
	app := hookApp(t)
	root := hookProject(t, app, "mcpcaught")
	guide := filepath.Join(root, "docs", "guide.md")
	twice := strings.Replace(hookGuide, "Log in with your account.", "Log in with your account, then Log in again.", 1)
	require.NoError(t, os.WriteFile(guide, []byte(twice), 0o644))
	tr := filepath.Join(t.TempDir(), "transcript.jsonl")

	// Two failing findings, then the assistant fixes one of them.
	transcript(t, tr, "turn-1", guide)
	runStop(t, app, root, "s1", tr, false)
	once := strings.Replace(twice, "then Log in again", "then Sign in again", 1)
	require.NoError(t, os.WriteFile(guide, []byte(once), 0o644))
	transcript(t, tr, "turn-2", guide)
	runStop(t, app, root, "s1", tr, false)

	out, raw := callSessionSummary(t, app, root, "s1")
	t.Logf("context_session_summary: %s", raw)
	assert.Equal(t, "s1", out.Session)
	assert.Equal(t, 2, out.Checked, "two hook runs")
	assert.Equal(t, 1, out.Failing, "the latest run left one finding")
	assert.Equal(t, 1, out.Caught, "the second run no longer found one of the first run's two")
	assert.Equal(t, 1, out.Files, "both turns edited the guide")
	assert.Equal(t, 2, out.Operations, "the session's operations are its two check runs")
	assert.Zero(t, out.Observed)
	assert.Equal(t,
		"Recorded nothing about this project's context (session s1). Checks ran 2 times on 1 edited file: 1 failing, 1 caught.",
		out.Report, "check runs are not notes, and the report says what they found")

	// The same counts the desktop feed and `kapi context log` derive.
	want, err := app.SessionOutcomes(t.Context(), ContextSessionRequest{Project: recipeOf(root), Session: "s1"})
	require.NoError(t, err)
	assert.Equal(t, SessionOutcomes{Session: "s1", Checked: out.Checked, Failing: out.Failing, Caught: out.Caught, Files: out.Files}, want)
}

func TestContextSessionSummary_ASessionWithNoRunReportsZeros(t *testing.T) {
	app := hookApp(t)
	root := hookProject(t, app, "mcpnoruns")

	out, raw := callSessionSummary(t, app, root, "s-none")
	assert.Equal(t, contextSessionOutput{
		Session: "s-none",
		Review:  "kapi context review --session s-none",
		Report:  "Recorded nothing about this project's context (session s-none).",
	}, out)

	var fields map[string]any
	require.NoError(t, json.Unmarshal([]byte(raw), &fields))
	for _, name := range []string{"checked", "failing", "caught", "files"} {
		require.Containsf(t, fields, name, "%s is present at zero, so no run reads apart from a clean one", name)
		assert.EqualValues(t, 0, fields[name])
	}
}

// callSessionSummaryDefault calls context_session_summary without a session,
// the way an assistant that knows neither its host's id nor the server's does.
func callSessionSummaryDefault(t *testing.T, app *App, root string) (contextSessionOutput, string) {
	t.Helper()
	res, err := growthSession(t, app).CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "context_session_summary",
		Arguments: map[string]any{"project": recipeOf(root)},
	})
	require.NoError(t, err)
	require.False(t, res.IsError, resultText(res))
	body, err := json.Marshal(res.StructuredContent)
	require.NoError(t, err)
	var out contextSessionOutput
	require.NoError(t, json.Unmarshal(body, &out))
	return out, string(body)
}

// Without `session`, the tool summarizes the session that recorded last in
// the project's log. A session the workspace knows of that recorded nothing,
// such as this MCP server's own, is passed over for the hook's session with
// runs, and an explicit `session` still wins.
func TestContextSessionSummary_DefaultIsTheLatestSessionWithOperations(t *testing.T) {
	app := hookApp(t)
	root := hookProject(t, app, "mcpdefault")
	tr := filepath.Join(t.TempDir(), "transcript.jsonl")
	transcript(t, tr, "turn-1", filepath.Join(root, "docs", "guide.md"))
	runStop(t, app, root, "s-hook", tr, false)

	// A later session, known to the workspace, that recorded nothing.
	ws, err := app.Workspace(t.Context())
	require.NoError(t, err)
	later := time.Now().UTC().Add(time.Minute)
	require.NoError(t, ws.NoteAgentSession(t.Context(), workspace.AgentSession{
		ID: "s-empty", Project: workspace.ProjectKey(projectIDFor("mcpdefault")), Agent: "claude-code",
		Started: later, LastSeen: later,
	}))

	runs := checkRuns(t, app, root)
	require.Len(t, runs, 1)
	ran := runs[0].At.UTC().Format(time.RFC3339)
	out, raw := callSessionSummaryDefault(t, app, root)
	t.Logf("context_session_summary (no session): %s", raw)
	assert.Equal(t, "s-hook", out.Session, "the session with runs, not the later one that recorded nothing")
	assert.Equal(t, 1, out.Checked)
	assert.Equal(t,
		"No `session` given: this is s-hook, the latest session in the project's log, last recorded "+ran+".",
		out.Chosen)
	assert.Equal(t, ran, out.Last)

	named, raw := callSessionSummary(t, app, root, "s-empty")
	t.Logf("context_session_summary (session s-empty): %s", raw)
	assert.Equal(t, "s-empty", named.Session, "an explicit session wins")
	assert.Zero(t, named.Operations)
	assert.Empty(t, named.Chosen, "a call that names its session is told of no choice")
	assert.Equal(t, "Recorded nothing about this project's context (session s-empty).", named.Report)
}

// A note this server records after the hook's run is the latest operation, so
// the default reads this server's session.
func TestContextSessionSummary_DefaultCountsANote(t *testing.T) {
	app := hookApp(t)
	root := hookProject(t, app, "mcpdefaultnote")
	tr := filepath.Join(t.TempDir(), "transcript.jsonl")
	transcript(t, tr, "turn-1", filepath.Join(root, "docs", "guide.md"))
	runStop(t, app, root, "s-hook", tr, false)

	recorded, res := callRecord(t, growthSession(t, app), "context_note", map[string]any{
		"text": "The guide addresses the reader as you.", "path": "docs/guide.md", "project": recipeOf(root),
	})
	require.False(t, res.IsError, resultText(res))

	out, raw := callSessionSummaryDefault(t, app, root)
	t.Logf("context_session_summary (no session, after a note): %s", raw)
	assert.Equal(t, MCPSessionID(), recorded.Session)
	assert.Equal(t, MCPSessionID(), out.Session, "the note is newer than the hook's run")
	assert.Equal(t, 1, out.Observed)
	assert.Contains(t, out.Chosen, "this is "+MCPSessionID()+", the latest session")
}

// A log that holds no session's operations answers in one line that says what
// records one.
func TestContextSessionSummary_DefaultWithAnEmptyLog(t *testing.T) {
	app, _ := contextOpsApp(t)
	root := contextOpsProject(t, "mcpdefaultempty")

	out, raw := callSessionSummaryDefault(t, app, root)
	t.Logf("context_session_summary (empty log): %s", raw)
	assert.Equal(t, contextSessionOutput{Report: noSessionReport}, out)
	assert.Equal(t,
		"The project's log holds no session's operations yet: the Stop hook records one "+
			"each time it runs the project's checks at the end of an assistant turn.",
		out.Report)
	assert.NotContains(t, out.Report, "\n")
}

// A session that recorded notes and ran checks reports both, with the command
// a person reviews it with last.
func TestSessionReport_NotesAndCheckRuns(t *testing.T) {
	out := sessionOutput(contextop.SessionSummary{
		Session:    "s4f1c2",
		Project:    workspace.ProjectKey("prj_docs"),
		Operations: 4,
		ByKind:     map[contextop.Kind]int{contextop.KindObserve: 1, contextop.KindCheck: 3},
		ByStatus:   map[contextop.Status]int{contextop.StatusSuggested: 1},
	}, SessionOutcomes{Session: "s4f1c2", Checked: 3, Failing: 0, Caught: 2, Files: 2})
	assert.Equal(t,
		"Context, session s4f1c2: 1 observation recorded. 1 suggestion is waiting for a person. "+
			"Checks ran 3 times on 2 edited files: 0 failing, 2 caught. "+
			"Review with `kapi context review --session s4f1c2`.",
		out.Report)
}
