package cli

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/host"
	"github.com/neokapi/neokapi/host/facetest"
	"github.com/neokapi/neokapi/host/output"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The CLI leg of the face parity contract.
//
// Each question is asked through the verb a person types, parsed from the
// structured document the verb prints, projected into the contract's shapes and
// compared against the record every face is held to (host/facetest). The verb is
// driven rather than the host function it calls, because a face's own entry
// point is where a face drifts: a flag defaulted differently, a field dropped
// on the way to the renderer, a limit applied twice.

// runVerb executes one kapi verb against the fixture and returns its stdout.
func runVerb(t *testing.T, a *App, build func(*App) *cobra.Command, args ...string) string {
	t.Helper()
	root := &cobra.Command{Use: "kapi"}
	AddCommandGroups(a, root)
	output.AddPersistentFlags(root.PersistentFlags())
	root.AddCommand(build(a))
	root.SetArgs(args)
	root.SilenceUsage, root.SilenceErrors = true, true

	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	require.NoError(t, root.Execute(), "verb failed: %s", out.String())
	return out.String()
}

func TestFaceParity_CLIMatchesTheRecord(t *testing.T) {
	p := facetest.Write(t)
	want := facetest.Golden(t)

	t.Run("context <path>", func(t *testing.T) {
		out := runVerb(t, &App{}, NewContextCmd, "context", p.ContextPath, "--json",
			"--limit", "10")
		var answer host.ContextAnswer
		require.NoError(t, json.Unmarshal([]byte(out), &answer), "context --json is a structured document")
		assert.Equal(t, want.ContextAt, facetest.ContextFactsFrom(&answer))
	})

	t.Run("context search", func(t *testing.T) {
		out := runVerb(t, &App{}, NewContextCmd, "context", "search", p.SearchQuery, "--json",
			"--limit", "10")
		var result host.ContextSearchResult
		require.NoError(t, json.Unmarshal([]byte(out), &result))
		assert.Equal(t, want.ContextSearch, facetest.SearchFactsFrom(&result))
	})

	t.Run("check", func(t *testing.T) {
		out := runVerb(t, &App{}, NewCheckCmd, "check", p.CheckPath,
			"--profile-file", ".kapi/voice.yaml", "--json")
		var report check.Report
		require.NoError(t, json.Unmarshal([]byte(out), &report))
		assert.Equal(t, want.Check, facetest.CheckFactsFrom(report))
	})
}

// Status is driven separately: the verb declares its own --json, and it is one
// of the two questions the MCP surface does not answer at all.
func TestFaceParity_CLIStatusMatchesTheRecord(t *testing.T) {
	facetest.Write(t)
	want := facetest.Golden(t)

	out := runVerb(t, &App{}, NewStatusCmd, "status", "--json")
	var status host.StatusOutput
	require.NoError(t, json.Unmarshal([]byte(out), &status), "status --json is a structured document")

	got := facetest.StatusFacts{Project: status.Project}
	seen := map[string]bool{}
	for _, lc := range status.Locales {
		if lc.Collection != "" && !seen[lc.Collection] {
			seen[lc.Collection] = true
			got.Collections = append(got.Collections, lc.Collection)
		}
		got.Locales = append(got.Locales, facetest.LocaleFacts{
			Locale:     lc.Locale,
			Translated: lc.Pct["translated"],
		})
	}
	facetest.SortLocales(got.Locales)
	assert.Equal(t, want.Status, got)
}
