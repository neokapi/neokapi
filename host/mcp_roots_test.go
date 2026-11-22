package host

import (
	"context"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// rootsSession connects a client to a server that carries the roots
// middleware `kapi mcp` installs. The client declares roots (none when nil)
// on a protocol version that lets the server list them during a call.
func rootsSession(t *testing.T, app *App, roots []string) (*mcp.ClientSession, context.Context) {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)

	server := mcp.NewServer(&mcp.Implementation{Name: "kapi", Version: "test"}, nil)
	ApplyMCPToolFactories(server, app)
	server.AddReceivingMiddleware(app.MCPRootsMiddleware())
	st, ct := mcp.NewInMemoryTransports()
	ss, err := server.Connect(ctx, st, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = ss.Close() })

	opts := &mcp.ClientOptions{Capabilities: &mcp.ClientCapabilities{}}
	if roots != nil {
		opts = nil
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "roots-test", Version: "test"}, opts)
	for _, r := range roots {
		client.AddRoots(&mcp.Root{URI: (&url.URL{Scheme: "file", Path: filepath.ToSlash(r)}).String()}) //nolint:staticcheck // roots are what is under test
	}
	cs, err := client.Connect(ctx, ct, &mcp.ClientSessionOptions{ProtocolVersion: "2025-11-25"})
	require.NoError(t, err)
	t.Cleanup(func() { _ = cs.Close() })
	return cs, ctx
}

// A project, a file to read, or a profile named in a call is an agent's
// argument. It reaches only the directory the server was started in and the
// roots the client declares; the server's own project stays the default.
func TestMCPCallsReachOnlyTheStartDirectoryAndDeclaredRoots(t *testing.T) {
	isolateCheckExecution(t)
	alpha := scopeProject(t, "alpha", "translation memory", "content memory", "nb")
	beta := scopeProject(t, "beta", "termbase", "terms store", "fr")
	t.Chdir(alpha)
	app := scopeApp(t, alpha)

	t.Run("the server's own project is the default", func(t *testing.T) {
		session, ctx := rootsSession(t, app, nil)
		report := reportOf(t, callTool(t, ctx, session, "check_file", map[string]any{
			"file": filepath.Join(alpha, "docs", "bad.md"),
		}))
		assert.Positive(t, ruleCounts(report)["terms.vocabulary"])
	})

	t.Run("a project outside the roots is refused, naming the argument", func(t *testing.T) {
		session, ctx := rootsSession(t, app, nil)
		res := callTool(t, ctx, session, "check_text", map[string]any{
			"text":    "the termbase",
			"project": filepath.Join(beta, "kapi.yaml"),
		})
		require.True(t, res.IsError)
		assert.Contains(t, resultText(res), "project")
		assert.Contains(t, resultText(res), "lies outside the directories this server may reach")
	})

	t.Run("a file to read outside the roots is refused", func(t *testing.T) {
		session, ctx := rootsSession(t, app, nil)
		res := callTool(t, ctx, session, "check_file", map[string]any{
			"file": filepath.Join(beta, "docs", "bad.md"),
		})
		require.True(t, res.IsError)
		assert.Contains(t, resultText(res), "file:")
	})

	t.Run("a profile file outside the roots is refused", func(t *testing.T) {
		session, ctx := rootsSession(t, app, nil)
		profile := filepath.Join(beta, "voice.yaml")
		require.NoError(t, os.WriteFile(profile, []byte("name: v\n"), 0o600))
		res := callTool(t, ctx, session, "check_text", map[string]any{
			"text": "hello", "profile_file": profile,
		})
		require.True(t, res.IsError)
		assert.Contains(t, resultText(res), "profile_file")
	})

	t.Run("a project under a root the client declares is reached", func(t *testing.T) {
		session, ctx := rootsSession(t, app, []string{beta})
		report := reportOf(t, callTool(t, ctx, session, "check_file", map[string]any{
			"file":    filepath.Join(beta, "docs", "bad.md"),
			"project": filepath.Join(beta, "kapi.yaml"),
		}))
		assert.Positive(t, ruleCounts(report)["terms.vocabulary"])
	})

	t.Run("a project reached through a link out of the start directory is refused", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("creating a symbolic link needs a privilege on Windows")
		}
		link := filepath.Join(alpha, "elsewhere")
		require.NoError(t, os.Symlink(beta, link))
		t.Cleanup(func() { _ = os.Remove(link) })
		session, ctx := rootsSession(t, app, nil)
		res := callTool(t, ctx, session, "check_text", map[string]any{
			"text": "the termbase", "project": filepath.Join(link, "kapi.yaml"),
		})
		require.True(t, res.IsError)
		assert.Contains(t, resultText(res), "project")
	})
}
