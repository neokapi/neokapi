package host

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/neokapi/neokapi/core/check"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// One `kapi mcp` process serves an assistant that works in more than one
// project, so every project-scoped tool takes the project as an argument and
// the server's own project is only the default. These tests hold both halves:
// the resolution (a recipe, a root, a path inside, a path that holds no
// project) and the behaviour of one App answering for two projects at once.

// scopeProject writes a project whose voice forbids one term, so a check of its
// content says which project governed the run.
func scopeProject(t *testing.T, name, forbidden, replacement, target string) string {
	t.Helper()
	root := t.TempDir()
	write := func(rel, body string) {
		t.Helper()
		file := filepath.Join(root, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(file), 0o700))
		require.NoError(t, os.WriteFile(file, []byte(body), 0o600))
	}
	write("kapi.yaml", "version: v1\nname: "+name+`
defaults:
  source_language: en
  target_languages: [`+target+`]
  source_gate: none
collections:
  - name: Docs
    content:
      - path: "docs/**/*.md"
`)
	write(".kapi/voice.yaml", "name: "+name+`
description: The voice of one project and no other.
vocabulary:
  forbidden_terms:
    - term: `+forbidden+`
      replacement: `+replacement+`
      severity: major
`)
	// The violating document and the clean one, so a run has both a negative
	// and a positive fixture in every project.
	write("docs/bad.md", "# Bad\n\nWe rely on the "+forbidden+" here.\n")
	write("docs/good.md", "# Good\n\nWe rely on the "+replacement+" here.\n")
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".kapi"), 0o700))
	readProjectContext(t, root)
	return root
}

// scopeApp builds the App a `kapi mcp` process runs with, bound to root the way
// `kapi mcp -p` binds it.
func scopeApp(t *testing.T, root string) *App {
	t.Helper()
	app := &App{}
	app.InitRegistries()
	cmd := NewEnvCommand(t.Context(), "mcp")
	cmd.Flags().String(projectFlagName, filepath.Join(root, "kapi.yaml"), "")
	require.NoError(t, app.ResolveMCPProject(cmd))
	t.Cleanup(app.Shutdown)
	return app
}

func TestResolveMCPCallProject(t *testing.T) {
	isolateCheckExecution(t)
	root := scopeProject(t, "alpha", "translation memory", "content memory", "nb")
	recipe := filepath.Join(root, "kapi.yaml")
	// The walk recognises a project by its recipe and the state directory
	// beside it, so the fixture has one.
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".kapi"), 0o700))
	app := scopeApp(t, root)

	other := scopeProject(t, "beta", "termbase", "terms store", "fr")
	require.NoError(t, os.MkdirAll(filepath.Join(other, ".kapi"), 0o700))

	t.Run("no argument takes the project the server started in", func(t *testing.T) {
		got, err := app.ResolveMCPCallProject("")
		require.NoError(t, err)
		assert.Equal(t, recipe, got)
	})

	t.Run("a recipe path", func(t *testing.T) {
		got, err := app.ResolveMCPCallProject(filepath.Join(other, "kapi.yaml"))
		require.NoError(t, err)
		assert.Equal(t, filepath.Join(other, "kapi.yaml"), got)
	})

	t.Run("a project root", func(t *testing.T) {
		got, err := app.ResolveMCPCallProject(other)
		require.NoError(t, err)
		assert.Equal(t, filepath.Join(other, "kapi.yaml"), got)
	})

	t.Run("a file inside the project", func(t *testing.T) {
		got, err := app.ResolveMCPCallProject(filepath.Join(other, "docs", "bad.md"))
		require.NoError(t, err)
		assert.Equal(t, filepath.Join(other, "kapi.yaml"), got)
	})

	t.Run("a directory inside the project", func(t *testing.T) {
		got, err := app.ResolveMCPCallProject(filepath.Join(other, "docs"))
		require.NoError(t, err)
		assert.Equal(t, filepath.Join(other, "kapi.yaml"), got)
	})

	t.Run("a path that holds no project names itself in the error", func(t *testing.T) {
		outside := t.TempDir()
		_, err := app.ResolveMCPCallProject(outside)
		require.Error(t, err)
		var perr *MCPProjectError
		require.ErrorAs(t, err, &perr)
		assert.Equal(t, outside, perr.Path)
		assert.Contains(t, err.Error(), outside)
	})

	t.Run("a path that does not exist names itself in the error", func(t *testing.T) {
		missing := filepath.Join(other, "nowhere", "kapi.yaml")
		_, err := app.ResolveMCPCallProject(missing)
		require.Error(t, err)
		var perr *MCPProjectError
		require.ErrorAs(t, err, &perr)
		assert.Equal(t, missing, perr.Path)
	})

	t.Run("KAPI_NO_PROJECT leaves an unbound server with no project", func(t *testing.T) {
		bare := &App{}
		got, err := bare.ResolveMCPCallProject("")
		require.NoError(t, err)
		assert.Empty(t, got, "discovery is off, so nothing is in scope")
		_, err = bare.RequireMCPCallProject("")
		require.Error(t, err)
		var perr *MCPProjectError
		require.ErrorAs(t, err, &perr)
	})

	t.Run("an explicit path still resolves under KAPI_NO_PROJECT", func(t *testing.T) {
		bare := &App{}
		got, err := bare.RequireMCPCallProject(filepath.Join(other, "docs", "good.md"))
		require.NoError(t, err)
		assert.Equal(t, filepath.Join(other, "kapi.yaml"), got)
	})
}

// callTool drives one tool the way a client does: over a session, by name, with
// JSON arguments.
func callTool(t *testing.T, ctx context.Context, session *mcp.ClientSession, name string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	raw, err := json.Marshal(args)
	require.NoError(t, err)
	res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: json.RawMessage(raw)})
	require.NoError(t, err)
	return res
}

// reportOf reads the check report a tool result carries.
func reportOf(t *testing.T, res *mcp.CallToolResult) check.Report {
	t.Helper()
	require.False(t, res.IsError, "tool call reported an error: %+v", res.Content)
	raw, err := json.Marshal(res.StructuredContent)
	require.NoError(t, err)
	var report check.Report
	require.NoError(t, json.Unmarshal(raw, &report))
	return report
}

// mcpSession connects a client to a server carrying every registered factory,
// which is what `kapi mcp` serves minus the binary's own porcelain.
func mcpSession(t *testing.T, app *App) (*mcp.ClientSession, context.Context) {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)

	server := mcp.NewServer(&mcp.Implementation{Name: "kapi", Version: "test"}, nil)
	ApplyMCPToolFactories(server, app)
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = serverSession.Close() })

	client := mcp.NewClient(&mcp.Implementation{Name: "scope-test", Version: "test"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close() })
	return session, ctx
}

func TestMCPServerAnswersForTwoProjects(t *testing.T) {
	isolateCheckExecution(t)
	alpha := scopeProject(t, "alpha", "translation memory", "content memory", "nb")
	beta := scopeProject(t, "beta", "termbase", "terms store", "fr")
	app := scopeApp(t, alpha)
	// The working directory is neither project, so nothing answers by accident.
	t.Chdir(t.TempDir())
	session, ctx := mcpSession(t, app)

	t.Run("the server's own project is the default", func(t *testing.T) {
		report := reportOf(t, callTool(t, ctx, session, "check_file", map[string]any{
			"file": filepath.Join(alpha, "docs", "bad.md"),
		}))
		assert.Positive(t, ruleCounts(report)["voice.vocabulary"],
			"alpha's voice must govern a call that named no project")
	})

	t.Run("a named project governs the call", func(t *testing.T) {
		report := reportOf(t, callTool(t, ctx, session, "check_file", map[string]any{
			"file":    filepath.Join(beta, "docs", "bad.md"),
			"project": filepath.Join(beta, "kapi.yaml"),
		}))
		assert.Positive(t, ruleCounts(report)["voice.vocabulary"],
			"beta's voice must govern a call that named beta")
	})

	t.Run("a path inside the project names it", func(t *testing.T) {
		report := reportOf(t, callTool(t, ctx, session, "check_file", map[string]any{
			"file":    filepath.Join(beta, "docs", "bad.md"),
			"project": filepath.Join(beta, "docs", "good.md"),
		}))
		assert.Positive(t, ruleCounts(report)["voice.vocabulary"])
	})

	t.Run("the other project's voice does not reach this one", func(t *testing.T) {
		// beta's violating word, checked under alpha's voice: alpha forbids a
		// different term, so the finding beta reports is absent here.
		report := reportOf(t, callTool(t, ctx, session, "check_file", map[string]any{
			"file":    filepath.Join(beta, "docs", "bad.md"),
			"project": filepath.Join(alpha, "kapi.yaml"),
		}))
		assert.Zero(t, ruleCounts(report)["voice.vocabulary"],
			"must fail: alpha's voice reported beta's forbidden term")
	})

	t.Run("the clean document passes in its own project", func(t *testing.T) {
		report := reportOf(t, callTool(t, ctx, session, "check_file", map[string]any{
			"file":    filepath.Join(beta, "docs", "good.md"),
			"project": beta,
		}))
		assert.Zero(t, ruleCounts(report)["voice.vocabulary"])
	})

	t.Run("a project that holds no project is refused by name", func(t *testing.T) {
		outside := t.TempDir()
		raw, err := json.Marshal(map[string]any{
			"file":    filepath.Join(beta, "docs", "bad.md"),
			"project": outside,
		})
		require.NoError(t, err)
		res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "check_file", Arguments: json.RawMessage(raw)})
		require.NoError(t, err)
		require.True(t, res.IsError, "must fail: a project path that resolves to nothing was accepted")
		assert.Contains(t, contentText(res), outside)
	})

	t.Run("check_text resolves its destination in the named project", func(t *testing.T) {
		report := reportOf(t, callTool(t, ctx, session, "check_text", map[string]any{
			"text":         "We rely on the termbase here.",
			"context_path": "docs/new.md",
			"project":      beta,
		}))
		assert.Positive(t, ruleCounts(report)["voice.vocabulary"],
			"beta's voice must govern a draft for a beta destination")
	})
}

// contentText joins a result's text content, which is where a refused call
// explains itself.
func contentText(res *mcp.CallToolResult) string {
	var out strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			out.WriteString(tc.Text)
		}
	}
	return out.String()
}

func TestMCPConcurrentCallsAcrossProjects(t *testing.T) {
	isolateCheckExecution(t)
	alpha := scopeProject(t, "alpha", "translation memory", "content memory", "nb")
	beta := scopeProject(t, "beta", "termbase", "terms store", "fr")
	app := scopeApp(t, alpha)
	t.Chdir(t.TempDir())
	session, ctx := mcpSession(t, app)

	// Two projects, interleaved, on one server. Under -race this is what says
	// the per-call project never lands on the App.
	const rounds = 6
	type call struct {
		project string
		file    string
		wantHit bool
	}
	calls := []call{
		{project: "", file: filepath.Join(alpha, "docs", "bad.md"), wantHit: true},
		{project: beta, file: filepath.Join(beta, "docs", "bad.md"), wantHit: true},
		{project: alpha, file: filepath.Join(alpha, "docs", "good.md"), wantHit: false},
		{project: filepath.Join(beta, "docs"), file: filepath.Join(beta, "docs", "good.md"), wantHit: false},
	}

	var wg sync.WaitGroup
	for range rounds {
		for _, c := range calls {
			wg.Add(1)
			go func(c call) {
				defer wg.Done()
				args := map[string]any{"file": c.file}
				if c.project != "" {
					args["project"] = c.project
				}
				raw, err := json.Marshal(args)
				if !assert.NoError(t, err) {
					return
				}
				res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "check_file", Arguments: json.RawMessage(raw)})
				if !assert.NoError(t, err) {
					return
				}
				if !assert.False(t, res.IsError, contentText(res)) {
					return
				}
				body, err := json.Marshal(res.StructuredContent)
				if !assert.NoError(t, err) {
					return
				}
				var report check.Report
				if !assert.NoError(t, json.Unmarshal(body, &report)) {
					return
				}
				hits := 0
				for _, f := range report.Findings {
					if f.Rule == "voice.vocabulary" {
						hits++
					}
				}
				if c.wantHit {
					assert.Positive(t, hits, "concurrent call lost its project's voice")
				} else {
					assert.Zero(t, hits, "concurrent call picked up another project's voice")
				}
			}(c)
		}
	}
	wg.Wait()
}

func TestMCPProjectStoreHandlesArePerProjectAndClosedOnShutdown(t *testing.T) {
	isolateCheckExecution(t)
	alpha := scopeProject(t, "alpha", "translation memory", "content memory", "nb")
	beta := scopeProject(t, "beta", "termbase", "terms store", "fr")

	app := &App{}
	cmd := NewEnvCommand(t.Context(), "mcp")
	cmd.Flags().String(projectFlagName, filepath.Join(alpha, "kapi.yaml"), "")
	require.NoError(t, app.ResolveMCPProject(cmd))

	ctx := t.Context()
	alphaDB, err := app.ProjectDB(ctx, alpha)
	require.NoError(t, err)
	betaDB, err := app.ProjectDB(ctx, beta)
	require.NoError(t, err)
	assert.NotSame(t, alphaDB, betaDB, "a second project served by one server needs its own handle")

	again, err := app.ProjectDB(ctx, beta)
	require.NoError(t, err)
	assert.Same(t, betaDB, again, "one handle per project root, however many calls ask")

	app.Shutdown()

	// A handle the shutdown closed cannot serve another query; the next open
	// for the same root builds a fresh one.
	reopened, err := app.ProjectDB(ctx, beta)
	require.NoError(t, err)
	assert.NotSame(t, betaDB, reopened, "shutdown must release both projects' handles")
	app.Shutdown()
}
