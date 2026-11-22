package mcptools

import (
	"context"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/host"
)

// pseudoTranslateOver calls pseudo_translate through a real MCP session whose
// client declares roots (nil declares no roots capability at all). It returns
// the text of the result and whether the call failed.
func pseudoTranslateOver(t *testing.T, roots []string, args map[string]any) (string, bool) {
	t.Helper()
	return pseudoTranslateOn(t, rootsProtocol, roots, args)
}

// rootsProtocol is the newest protocol version on which a server may list a
// client's roots while it serves a call.
const rootsProtocol = "2025-11-25"

func pseudoTranslateOn(t *testing.T, protocol string, roots []string, args map[string]any) (string, bool) {
	t.Helper()
	a := testApp()
	a.MCPSurface.AllTools = true
	server := mcp.NewServer(&mcp.Implementation{Name: "kapi-test", Version: "0"}, nil)
	registerKapiTools(server, a)
	// As `kapi mcp` serves it: the roots middleware in front, the start
	// directory the working directory.
	require.NoError(t, a.ResolveMCPProject(host.NewEnvCommand(t.Context(), "mcp")))
	server.AddReceivingMiddleware(a.MCPRootsMiddleware())

	opts := &mcp.ClientOptions{Capabilities: &mcp.ClientCapabilities{}}
	if roots != nil {
		opts = nil // the SDK's default capabilities declare roots
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "client", Version: "0"}, opts)
	for _, r := range roots {
		client.AddRoots(&mcp.Root{URI: (&url.URL{Scheme: "file", Path: filepath.ToSlash(r)}).String()}) //nolint:staticcheck // roots are what is under test
	}

	ctx := context.Background()
	st, ct := mcp.NewInMemoryTransports()
	ss, err := server.Connect(ctx, st, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = ss.Close() })
	cs, err := client.Connect(ctx, ct, &mcp.ClientSessionOptions{ProtocolVersion: protocol})
	require.NoError(t, err)
	t.Cleanup(func() { _ = cs.Close() })

	res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "pseudo_translate", Arguments: args})
	require.NoError(t, err)
	var text strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			text.WriteString(tc.Text)
		}
	}
	return text.String(), res.IsError
}

func writeInput(t *testing.T, dir string) string {
	t.Helper()
	p := filepath.Join(dir, "in.json")
	require.NoError(t, os.WriteFile(p, []byte(`{"greeting": "Hello"}`), 0o644))
	return p
}

// Outside a project an MCP call writes only under the directory the server
// was started in and the roots its client declares.
func TestMCPWritesOnlyUnderTheServerDirectoryAndTheClientRoots(t *testing.T) {
	start := t.TempDir()
	t.Chdir(start)
	declared := t.TempDir()
	elsewhere := t.TempDir()
	input := writeInput(t, start)

	t.Run("a path under the working directory is written", func(t *testing.T) {
		out := filepath.Join(start, "out", "qps.json")
		msg, failed := pseudoTranslateOver(t, nil, map[string]any{"path": input, "output_path": out})
		require.False(t, failed, msg)
		assert.FileExists(t, out)
	})

	t.Run("a path outside it is refused when the client declares no roots", func(t *testing.T) {
		out := filepath.Join(declared, "qps.json")
		msg, failed := pseudoTranslateOver(t, nil, map[string]any{"path": input, "output_path": out})
		require.True(t, failed)
		assert.Contains(t, msg, "output_path")
		assert.Contains(t, msg, "lies outside the directories this server may reach")
		assert.NoFileExists(t, out)
	})

	t.Run("a path under a root the client declares is written", func(t *testing.T) {
		out := filepath.Join(declared, "qps.json")
		msg, failed := pseudoTranslateOver(t, []string{declared}, map[string]any{"path": input, "output_path": out})
		require.False(t, failed, msg)
		assert.FileExists(t, out)
	})

	t.Run("a path under neither is refused when the client declares roots", func(t *testing.T) {
		out := filepath.Join(elsewhere, "qps.json")
		msg, failed := pseudoTranslateOver(t, []string{declared}, map[string]any{"path": input, "output_path": out})
		require.True(t, failed)
		assert.Contains(t, msg, "output_path")
		assert.NoFileExists(t, out)
	})

	t.Run("roots are not listed on a protocol that forbids it, so only the working directory is written", func(t *testing.T) {
		out := filepath.Join(declared, "late.json")
		msg, failed := pseudoTranslateOn(t, "", []string{declared}, map[string]any{"path": input, "output_path": out})
		require.True(t, failed)
		assert.Contains(t, msg, "output_path")
		assert.NoFileExists(t, out)
	})

	t.Run("a path beneath a link out of the working directory is refused", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("creating a symbolic link needs a privilege on Windows")
		}
		require.NoError(t, os.Symlink(elsewhere, filepath.Join(start, "site")))
		msg, failed := pseudoTranslateOver(t, nil, map[string]any{"path": input, "output_path": filepath.Join(start, "site", "qps.json")})
		require.True(t, failed)
		assert.Contains(t, msg, "leads outside the root")
		assert.NoFileExists(t, filepath.Join(elsewhere, "qps.json"))
	})

	t.Run("an input outside the roots is refused, saying where the call may reach, and nothing is written beside it", func(t *testing.T) {
		outsideInput := writeInput(t, elsewhere)
		msg, failed := pseudoTranslateOver(t, nil, map[string]any{"path": outsideInput})
		require.True(t, failed)
		assert.Contains(t, msg, "path:")
		assert.Contains(t, msg, "lies outside the directories this server may reach")
		assert.Contains(t, msg, filepath.Base(filepath.Dir(start)), "the message names where the call may reach")
		assert.NoFileExists(t, filepath.Join(elsewhere, "in_qps.json"))
	})
}

// A root reached through a link is the directory it resolves to, so a path
// spelled through either name is written: a server started in /var/... takes
// an output_path spelled /private/var/....
func TestMCPWriteRootReachedThroughALink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("creating a symbolic link needs a privilege on Windows")
	}
	base := t.TempDir()
	real := filepath.Join(base, "real")
	require.NoError(t, os.MkdirAll(real, 0o755))
	via := filepath.Join(base, "via")
	require.NoError(t, os.Symlink(real, via))
	input := writeInput(t, real)

	for name, tc := range map[string]struct{ root, out string }{
		"root named through the link, path spelled by its real name": {via, filepath.Join(real, "a.json")},
		"root named by its real name, path spelled through the link": {real, filepath.Join(via, "b.json")},
	} {
		t.Run(name, func(t *testing.T) {
			ctx := withWriteRoots(t.Context(), []string{tc.root})
			_, _, err := handlePseudoTranslate(ctx, testApp(), PseudoTranslateInput{Path: input, OutputPath: tc.out})
			require.NoError(t, err)
			assert.FileExists(t, tc.out)
		})
	}
}

// Only a file URI on this machine names a root: one on another host is not
// the local directory of the same path.
func TestFileRootPath(t *testing.T) {
	for uri, want := range map[string]string{
		"file:///srv/work":          "/srv/work",
		"file://localhost/srv/work": "/srv/work",
		"file://LOCALHOST/srv/work": "/srv/work",
		"file://evil.example/etc":   "",
		"https://example.com/etc":   "",
		"/srv/work":                 "",
		"file:":                     "",
	} {
		got, ok := fileRootPath(uri)
		if want == "" {
			assert.False(t, ok, uri)
			continue
		}
		require.True(t, ok, uri)
		if runtime.GOOS != "windows" {
			assert.Equal(t, want, got, uri)
		}
	}
}

// With no directory to write under, nothing is written.
func TestMCPWritesNothingWithoutADirectory(t *testing.T) {
	dir := t.TempDir()
	input := writeInput(t, dir)
	ctx := withWriteRoots(t.Context(), nil)
	_, _, err := handlePseudoTranslate(ctx, testApp(), PseudoTranslateInput{Path: input})
	require.ErrorContains(t, err, "this server has no directory it may write")
	assert.NoFileExists(t, filepath.Join(dir, "in_qps.json"))
}
