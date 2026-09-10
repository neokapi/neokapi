package main

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPairedMCPDiscovery(t *testing.T) {
	for _, tc := range []struct {
		mode, failure string
	}{
		{mode: "ready"},
		{mode: "pagination"},
		{mode: "missing-tool", failure: "check_text"},
		{mode: "missing-context", failure: "context://"},
		{mode: "missing-capability", failure: "advertise tools and resources"},
		{mode: "malformed", failure: "malformed MCP message"},
		{mode: "wrong-id", failure: "unexpected MCP response identity"},
		{mode: "disconnect", failure: "disconnected"},
		{mode: "cycle", failure: "repeated a pagination cursor"},
		{mode: "rpc-error", failure: "discovery disabled"},
		{mode: "wrong-version", failure: "unsupported MCP protocol"},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			launch := pairedMCPPeerLaunch(t, tc.mode)
			t.Setenv("OPENAI_API_KEY", "private-test-key")
			t.Setenv("CLAUDE_CODE_OAUTH_TOKEN", "private-test-token")
			prepared := PairedPrepared{Launch: launch, Blockers: []string{}, IsolationNotes: []string{}}
			preparePairedMCPReadiness(t.Context(), &prepared)
			result := prepared.MCPReadiness
			require.NotNil(t, result)
			assert.Equal(t, "direct-server-discovery", result.Evidence)
			assert.Equal(t, "unverified", result.AgentHostExposure)
			if tc.failure != "" {
				assert.Equal(t, "failed", result.Status)
				assert.Contains(t, result.Error, tc.failure)
				assert.NotEmpty(t, prepared.Blockers)
				return
			}
			assert.Equal(t, "ready", result.Status, result.Error)
			assert.Empty(t, prepared.Blockers)
			assert.Equal(t, "test-kapi", result.ServerName)
			assert.Equal(t, "1", result.ServerVersion)
			assert.Equal(t, []string{"resources", "tools"}, result.Capabilities)
			assert.Equal(t, []string{"check_file", "check_text", "zzz"}, result.Tools)
			assert.Equal(t, []string{"context://{+path}{?format}"}, result.ResourceTemplates)
			log, err := os.ReadFile(filepath.Join(launch.Workspace, "peer-log"))
			require.NoError(t, err)
			assert.NotContains(t, string(log), "tools/call")
			assert.NotContains(t, string(log), "resources/read")
			assert.Contains(t, string(log), "notifications/initialized")
			assert.Contains(t, string(log), "sampling refused")
		})
	}
}

func TestPairedMCPReadinessCancellation(t *testing.T) {
	launch := pairedMCPPeerLaunch(t, "timeout")
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	started := time.Now()
	result := probePairedMCP(ctx, launch)
	assert.Equal(t, "timeout", result.Status)
	assert.Less(t, time.Since(started), time.Second)
}

func TestPairedMCPReadinessRequiredBeforeInference(t *testing.T) {
	prepared := PairedPrepared{Launch: PairedLaunch{Condition: "mcp"}}
	result, err := runPairedAgent(t.Context(), prepared)
	require.ErrorContains(t, err, "readiness has not been established")
	assert.Equal(t, "blocked", result.Status)
	prepared.Launch.Condition = "baseline"
	preparePairedMCPReadiness(t.Context(), &prepared)
	assert.Nil(t, prepared.MCPReadiness)
}

func TestPairedMCPReadinessWithBuiltKapi(t *testing.T) {
	binary := os.Getenv("PAIRED_TEST_KAPI")
	if binary == "" {
		t.Skip("set PAIRED_TEST_KAPI to a built kapi binary for offline MCP discovery")
	}
	binary, err := filepath.Abs(binary)
	require.NoError(t, err)
	task, err := findPairedTask("audience-child")
	require.NoError(t, err)
	launch := PairedLaunch{Workspace: t.TempDir(), StateDir: t.TempDir(), Condition: "mcp", KapiBin: binary}
	require.NoError(t, materializePairedTask(launch.Workspace, task))
	files, err := pairedTaskFiles(task)
	require.NoError(t, err)
	result := probePairedMCP(t.Context(), launch)
	require.Equal(t, "ready", result.Status, result.Error)
	assert.Contains(t, result.Tools, "check_file")
	assert.Contains(t, result.Tools, "check_text")
	assert.Contains(t, result.ResourceTemplates, "context://{+path}{?format}")
	for path, expected := range files {
		actual, err := os.ReadFile(filepath.Join(launch.Workspace, path))
		require.NoError(t, err)
		assert.Equal(t, expected, actual, path)
	}
	t.Logf("direct discovery: server=%s version=%s protocol=%s tools=%d templates=%d duration=%dms",
		result.ServerName, result.ServerVersion, result.ProtocolVersion,
		len(result.Tools), len(result.ResourceTemplates), result.DurationMS)
}

func pairedMCPPeerLaunch(t *testing.T, mode string) PairedLaunch {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("peer launcher uses a Unix shell")
	}
	binary, err := os.Executable()
	require.NoError(t, err)
	launch := PairedLaunch{Workspace: t.TempDir(), StateDir: t.TempDir(), Condition: "mcp"}
	launch.KapiBin = filepath.Join(launch.StateDir, "peer")
	wrapper := "#!/bin/sh\nexec " + pairedShellQuote(binary) + " -test.run=^TestPairedMCPDiscoveryPeer$ -- \"$@\"\n"
	require.NoError(t, os.WriteFile(launch.KapiBin, []byte(wrapper), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(launch.Workspace, "peer-mode"), []byte(mode), 0o600))
	return launch
}

// This subprocess peer checks the actual process environment and speaks only
// JSON-RPC on stdout, including an unsupported sampling request the client must
// refuse without invoking any inference route.
func TestPairedMCPDiscoveryPeer(t *testing.T) {
	data, err := os.ReadFile("peer-mode")
	if err != nil {
		return
	}
	mode := string(data)
	if os.Getenv("OPENAI_API_KEY") != "" || os.Getenv("CLAUDE_CODE_OAUTH_TOKEN") != "" {
		os.Exit(2)
	}
	if os.Getenv("KAPI_NO_PROJECT") != "1" || os.Getenv("KAPI_PLUGINS_DIR_ONLY") != "1" {
		os.Exit(2)
	}
	if len(os.Args) < 4 {
		os.Exit(2)
	}
	cwd, _ := os.Getwd()
	recipeDir, _ := filepath.EvalSymlinks(filepath.Dir(os.Args[len(os.Args)-2]))
	if os.Args[len(os.Args)-3] != "-p" || os.Args[len(os.Args)-1] != "mcp" || recipeDir != cwd {
		os.Exit(2)
	}
	if mode == "timeout" {
		child := exec.Command("/bin/sleep", "30")
		child.Stdout = os.Stdout
		_ = child.Run()
		os.Exit(0)
	}
	if mode == "disconnect" {
		os.Exit(0)
	}
	if mode == "malformed" {
		_, _ = os.Stdout.WriteString("{broken JSON\n")
		os.Exit(0)
	}
	log, _ := os.Create("peer-log")
	encoder := json.NewEncoder(os.Stdout)
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		request := map[string]any{}
		if json.Unmarshal(scanner.Bytes(), &request) != nil {
			os.Exit(2)
		}
		method := pairedString(request, "method")
		_, _ = log.WriteString(method + "\n")
		if method == "" {
			if pairedObject(request, "error") != nil {
				_, _ = log.WriteString("sampling refused\n")
			}
			continue
		}
		if method == "notifications/initialized" {
			continue
		}
		result := map[string]any{}
		switch method {
		case "initialize":
			_ = encoder.Encode(map[string]any{"jsonrpc": "2.0", "method": "notifications/message"})
			_ = encoder.Encode(map[string]any{"jsonrpc": "2.0", "id": "sample", "method": "sampling/createMessage"})
			version := "2025-11-25"
			if mode == "wrong-version" {
				version = "2099-01-01"
			}
			result = map[string]any{"protocolVersion": version,
				"serverInfo":   map[string]string{"name": "test-kapi", "version": "1"},
				"capabilities": map[string]any{"tools": map[string]any{}, "resources": map[string]any{}}}
			if mode == "missing-capability" {
				result["capabilities"] = map[string]any{}
			}
		case "tools/list":
			tools := []map[string]string{{"name": "zzz"}, {"name": "check_text"}, {"name": "check_file"}}
			if mode == "missing-tool" {
				tools = []map[string]string{{"name": "check_file"}}
			}
			if mode == "pagination" {
				tools = tools[:2]
				result["nextCursor"] = "next"
				if pairedString(pairedObject(request, "params"), "cursor") == "next" {
					tools = []map[string]string{{"name": "check_file"}, {"name": "check_text"}}
					delete(result, "nextCursor")
				}
			}
			if mode == "cycle" {
				result["nextCursor"] = "repeated"
			}
			result["tools"] = tools
		case "resources/list":
			result["resources"] = []any{}
		case "resources/templates/list":
			template := "context://{+path}{?format}"
			if mode == "missing-context" {
				template = "context://profile/{name}"
			}
			result["resourceTemplates"] = []map[string]string{{"uriTemplate": template}}
		default:
			os.Exit(2)
		}
		reply := map[string]any{"jsonrpc": "2.0", "id": request["id"], "result": result}
		if mode == "wrong-id" {
			reply["id"] = 99
		}
		if mode == "rpc-error" {
			delete(reply, "result")
			reply["error"] = map[string]any{"code": -32601, "message": "discovery disabled"}
		}
		if encoder.Encode(reply) != nil {
			os.Exit(2)
		}
	}
	os.Exit(0)
}
