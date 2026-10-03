package host_test

import (
	"context"
	"encoding/json"
	"flag"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/neokapi/neokapi/host"
	"github.com/neokapi/neokapi/host/facetest"
	"github.com/stretchr/testify/require"
)

// The MCP leg of the face parity contract, and the record the other legs read.
//
// Every face answers from the host layer, so the reference here is what that
// layer says about the fixture. The record is committed rather than recomputed
// per suite because the CLI and the desktop live in other modules and cannot
// call into this test binary; see host/facetest.
//
// Regenerate with: make face-parity-update

var updateGolden = flag.Bool("update-face-golden", false,
	"rewrite host/facetest/testdata/answers.json from this run")

// hostCommand builds a flag-carrying command bound to the fixture's recipe, the
// way every embedded surface builds one.
func hostCommand(t *testing.T, ctx context.Context, name string, p facetest.Project) host.Command {
	t.Helper()
	cmd := host.NewEnvCommand(ctx, name)
	host.AddProjectFlag(cmd)
	host.AddResourceFlags(cmd)
	require.NoError(t, cmd.Flags().Set("project", p.Recipe))
	return cmd
}

// hostStatusFacts reads the two-axis status the CLI prints, through the same
// RunStatus the verb calls, with its structured rendering captured.
func hostStatusFacts(t *testing.T, p facetest.Project) facetest.StatusFacts {
	t.Helper()
	a := &host.App{}
	a.InitRegistries()

	// AddStatusFlags declares the verb's own --json, so the persistent one is
	// not added here: pflag panics on a redefinition.
	cmd := host.NewEnvCommand(t.Context(), "status")
	host.AddProjectFlag(cmd)
	host.AddStatusFlags(cmd)
	require.NoError(t, cmd.Flags().Set("project", p.Recipe))
	require.NoError(t, cmd.Flags().Set("json", "true"))

	var buf capturingWriter
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	require.NoError(t, a.RunStatus(cmd, nil))

	var out host.StatusOutput
	require.NoError(t, json.Unmarshal(buf.Bytes(), &out), "status --json is a structured document")
	return statusFactsFrom(out)
}

func statusFactsFrom(out host.StatusOutput) facetest.StatusFacts {
	f := facetest.StatusFacts{Project: out.Project}
	seen := map[string]bool{}
	for _, lc := range out.Locales {
		if lc.Collection != "" && !seen[lc.Collection] {
			seen[lc.Collection] = true
			f.Collections = append(f.Collections, lc.Collection)
		}
		f.Locales = append(f.Locales, facetest.LocaleFacts{
			Locale:     lc.Locale,
			Translated: lc.Pct["translated"],
		})
	}
	facetest.SortLocales(f.Locales)
	return f
}

// capturingWriter collects what a command printed.
type capturingWriter struct{ buf []byte }

func (w *capturingWriter) Write(p []byte) (int, error) {
	w.buf = append(w.buf, p...)
	return len(p), nil
}
func (w *capturingWriter) Bytes() []byte { return w.buf }

// structuredJSON returns a tool result's structured content as JSON.
func structuredJSON(t *testing.T, res *mcp.CallToolResult) []byte {
	t.Helper()
	require.NotNil(t, res.StructuredContent, "the tool returns a structured result")
	raw, err := json.Marshal(res.StructuredContent)
	require.NoError(t, err)
	return raw
}
