package backend

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/host"
)

// assistantsApp is an app on a workspace of its own, with Claude Desktop's
// folder and the person's Claude Code settings pointed at places of the
// test's own, so nothing reads or writes the person's configuration.
func assistantsApp(t *testing.T) *App {
	t.Helper()
	claudeDesktopAt(t, filepath.Join(t.TempDir(), "missing"))
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())
	return newWorkspaceApp(t)
}

// assistantRow finds one assistant's row.
func assistantRow(t *testing.T, rows []AssistantHostView, name string) AssistantHostView {
	t.Helper()
	for _, r := range rows {
		if r.Host == name {
			return r
		}
	}
	require.Failf(t, "no row", "no row for %s", name)
	return AssistantHostView{}
}

// TestAssistantsView_NothingConnected: a fresh project lists every assistant
// kapi connect knows, then Claude Desktop, none of them connected, and Claude
// Code's check waits on the plugin install kapi connect prints.
func TestAssistantsView_NothingConnected(t *testing.T) {
	app := assistantsApp(t)
	tab, recipe := openSharingProject(t, app, "Assistants")
	root := filepath.Dir(recipe)
	require.NoDirExists(t, filepath.Join(root, project.StateDirName), "opening the project left no state")

	view, err := app.AssistantsView(tab)
	require.NoError(t, err)
	assert.False(t, view.NeedsCheckout)
	require.NotNil(t, view.Rules)
	assert.Equal(t, 0, *view.Rules)
	assert.Empty(t, view.HostsError)
	assert.Empty(t, view.SessionsError)
	assert.Empty(t, view.Sessions)
	assert.NotEmpty(t, view.ProjectKey)

	names := make([]string, 0, len(view.Hosts))
	for _, h := range view.Hosts {
		names = append(names, h.Host)
		assert.False(t, h.Connected, h.Host)
		assert.False(t, h.Check, h.Host)
	}
	assert.Equal(t, []string{"claude-code", "cursor", "vscode", "codex", "agents", "claude-desktop"}, names)

	claude := assistantRow(t, view.Hosts, "claude-code")
	assert.Equal(t, "Claude Code", claude.Label)
	assert.True(t, claude.Available)
	assert.Equal(t, []string{
		"/plugin marketplace add neokapi/claude-plugins",
		"/plugin install kapi@neokapi-plugins",
	}, claude.Pending)
	assert.Empty(t, assistantRow(t, view.Hosts, "cursor").Pending, "only Claude Code has the check")

	desktop := assistantRow(t, view.Hosts, "claude-desktop")
	assert.True(t, desktop.Personal)
	assert.False(t, desktop.Available, "its configuration folder does not exist")

	// Reading the page writes nothing, not even the project's state.
	for _, rel := range []string{".mcp.json", ".claude", "AGENTS.md", "CLAUDE.md", ".cursor", ".vscode", ".codex", ".agents", project.StateDirName} {
		_, serr := os.Stat(filepath.Join(root, rel))
		assert.True(t, os.IsNotExist(serr), rel)
	}
}

// TestConnectAssistant_WritesWhatKapiConnectWrites: Connect for Claude Code
// writes the files `kapi connect claude` writes in an identical project, byte
// for byte, the row then reads connected, and a second Connect writes
// nothing.
func TestConnectAssistant_WritesWhatKapiConnectWrites(t *testing.T) {
	app := assistantsApp(t)
	tab, recipe := openSharingProject(t, app, "Assistants")
	root := filepath.Dir(recipe)

	got, err := app.ConnectAssistant(tab, "claude")
	require.NoError(t, err)

	// The command line's run: its own host.App, over a project of the same
	// name in another folder.
	_, other := scaffoldNamedProject(t, filepath.Join(t.TempDir(), "Assistants"), project.NewID(), "Assistants")
	cli := &host.App{}
	cli.InitRegistries()
	ctx := context.Background()
	want, err := cli.ConnectHosts(ctx, connectCommand(ctx, other), host.ConnectOptions{
		Hosts: []host.AgentHost{host.AgentHostClaudeCode},
	})
	require.NoError(t, err)

	assert.Equal(t, connectSummary(want), connectSummary(got))
	written := 0
	for _, f := range connectFiles(got) {
		if f.Path == "" || f.Kind == host.ConnectRecipe || f.Action == host.ConnectKept {
			continue
		}
		written++
		theirs := filepath.Join(filepath.Dir(other), filepath.FromSlash(f.Path))
		require.NoError(t, filepath.WalkDir(theirs, func(p string, d fs.DirEntry, werr error) error {
			if werr != nil || d.IsDir() {
				return werr
			}
			rel, _ := filepath.Rel(filepath.Dir(other), p)
			want, _ := os.ReadFile(p)
			mine, rerr := os.ReadFile(filepath.Join(root, rel))
			require.NoError(t, rerr, rel)
			assert.Equal(t, string(want), string(mine), rel)
			return nil
		}))
	}
	assert.Positive(t, written, "connecting wrote files")

	view, err := app.AssistantsView(tab)
	require.NoError(t, err)
	claude := assistantRow(t, view.Hosts, "claude-code")
	assert.True(t, claude.Connected)
	assert.Contains(t, claude.Files, ".mcp.json")
	assert.False(t, assistantRow(t, view.Hosts, "cursor").Connected, "only the assistant named is connected")

	again, err := app.ConnectAssistant(tab, "claude-code")
	require.NoError(t, err)
	for _, f := range connectFiles(again) {
		assert.Contains(t, []host.ConnectAction{host.ConnectKept, host.ConnectPending}, f.Action, f.Path)
	}

	// With the rules files gone the assistant reads nothing of the project's
	// rules, so its row is not connected and offers Connect again.
	require.NoError(t, os.Remove(filepath.Join(root, "AGENTS.md")))
	require.NoError(t, os.Remove(filepath.Join(root, "CLAUDE.md")))
	view, err = app.AssistantsView(tab)
	require.NoError(t, err)
	claude = assistantRow(t, view.Hosts, "claude-code")
	assert.False(t, claude.Connected)
	assert.Equal(t, "rules files missing", claude.Reason)
	assert.True(t, claude.Available)
}

// TestAssistantsView_CheckFromTheProjectSettings: a project whose Claude Code
// settings enable the kapi plugin reads as checked, with nothing pending.
func TestAssistantsView_CheckFromTheProjectSettings(t *testing.T) {
	app := assistantsApp(t)
	tab, recipe := openSharingProject(t, app, "Assistants")
	root := filepath.Dir(recipe)
	_, err := app.ConnectAssistant(tab, "claude")
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".claude"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".claude", "settings.json"),
		[]byte(`{"enabledPlugins":{"kapi@neokapi-plugins":true}}`), 0o644))

	view, err := app.AssistantsView(tab)
	require.NoError(t, err)
	claude := assistantRow(t, view.Hosts, "claude-code")
	assert.True(t, claude.Connected)
	assert.True(t, claude.Check)
	assert.Empty(t, claude.Pending)
}

// TestAssistantsView_Sessions: what an agent recorded in the project is a
// session on the page, with its counts.
func TestAssistantsView_Sessions(t *testing.T) {
	app := assistantsApp(t)
	tab, recipe := openSharingProject(t, app, "Assistants")
	engine := agentProcess(t, app)
	agentSuggest(t, engine, recipe, "sess-1", "sign in", "log in", "docs/pricing.md", "Please sign in.")

	view, err := app.AssistantsView(tab)
	require.NoError(t, err)
	require.Len(t, view.Sessions, 1)
	s := view.Sessions[0]
	assert.Equal(t, "session:sess-1", s.ID)
	assert.Equal(t, "agent", s.Actor.Kind)
	assert.Equal(t, "claude", s.Actor.Name)
	assert.Equal(t, 1, s.Operations)
	assert.Equal(t, 1, s.Suggestions)
	assert.Equal(t, 1, s.Awaiting)
	assert.NotEmpty(t, s.Last)
	assert.Zero(t, s.Checked, "a session that ran no check counts none")
}

// TestAssistantsView_HookOnlySession: a session that only ran the Stop hook
// is a session on the page with no operations and what its runs caught.
func TestAssistantsView_HookOnlySession(t *testing.T) {
	app := assistantsApp(t)
	tab, _ := openSharingProject(t, app, "Assistants")
	view, err := app.AssistantsView(tab)
	require.NoError(t, err)
	require.NotEmpty(t, view.ProjectKey)

	hookRun(t, app, view.ProjectKey, "sess-hook", "t1", time.Time{}, failingRun())
	hookRun(t, app, view.ProjectKey, "sess-hook", "t2", time.Time{}, cleanRun())

	view, err = app.AssistantsView(tab)
	require.NoError(t, err)
	require.Len(t, view.Sessions, 1)
	s := view.Sessions[0]
	assert.Equal(t, "session:sess-hook", s.ID)
	assert.Equal(t, "claude-code", s.Actor.Name)
	assert.Zero(t, s.Operations)
	assert.Equal(t, [4]int{1, 2, 0, 1}, [4]int{s.Files, s.Checked, s.Failing, s.Caught})
	assert.True(t, s.Failed)
	assert.False(t, s.Clean)
}

// TestConnectAssistant_RefusesWhatKapiConnectRefuses: an assistant kapi
// connect does not support is refused with its message.
func TestConnectAssistant_RefusesWhatKapiConnectRefuses(t *testing.T) {
	app := assistantsApp(t)
	tab, _ := openSharingProject(t, app, "Assistants")
	_, err := app.ConnectAssistant(tab, "notepad")
	require.ErrorIs(t, err, host.ErrConnectUnsupported)
}

// TestConnectAssistant_WritesTheRecipeOfAFolder: a folder opened without a
// recipe reads its assistants without writing one, and Connect writes it as
// the desktop writes it and reports it created.
func TestConnectAssistant_WritesTheRecipeOfAFolder(t *testing.T) {
	app := assistantsApp(t)
	dir := contentFolder(t, "Handbook")
	tab, err := app.OpenProject(dir)
	require.NoError(t, err)
	t.Cleanup(func() { app.CloseProject(tab.ID) })
	recipe := filepath.Join(dir, project.RecipeFileName)

	view, err := app.AssistantsView(tab.ID)
	require.NoError(t, err)
	assert.False(t, assistantRow(t, view.Hosts, "claude-code").Connected)
	assert.NoFileExists(t, recipe, "reading the page writes no recipe")

	res, err := app.ConnectAssistant(tab.ID, "claude")
	require.NoError(t, err)
	require.FileExists(t, recipe)
	require.NotEmpty(t, res.Files)
	assert.Equal(t, host.ConnectRecipe, res.Files[0].Kind)
	assert.Equal(t, host.ConnectCreated, res.Files[0].Action)
	assert.True(t, app.ListTabs()[0].RecipeWritten, "the tab took the recipe up")
	assert.FileExists(t, filepath.Join(dir, ".mcp.json"))
}

// connectFiles are every file a connect reported, the assistants' and the
// project's.
func connectFiles(res *host.ConnectResult) []host.ConnectFile {
	var out []host.ConnectFile
	for _, h := range res.Hosts {
		out = append(out, h.Files...)
	}
	return append(out, res.Files...)
}

// connectSummary is what a connect reported, file by file, without the
// recipe's line, whose action depends on how the project came to be.
func connectSummary(res *host.ConnectResult) []string {
	var out []string
	for _, h := range res.Hosts {
		out = append(out, string(h.Host))
		out = append(out, h.Reads...)
	}
	for _, f := range connectFiles(res) {
		if f.Kind == host.ConnectRecipe {
			continue
		}
		out = append(out, string(f.Kind)+" "+f.Path+" "+string(f.Action))
	}
	return out
}
