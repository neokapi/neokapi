package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/host"
	"github.com/neokapi/neokapi/host/output"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// `kapi context <path>` — the by-location half of the retrieval surface
// (AD-037), CLI leg. These assert that the verb prints the host answer's own
// rendering rather than a second one: the `context://` MCP resources serve the
// same bytes, and the parity between the two surfaces is a test, not a
// convention.

// writeGovernedProject builds one governed collection with a voice and a
// vocabulary, plus a file no collection claims.
func writeGovernedProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	write := func(rel, body string) {
		p := filepath.Join(root, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(body), 0o644))
	}

	write("kapi.yaml", `version: v1
name: acme
defaults:
  source_language: en
  target_languages: [nb]
  voice: .kapi/voice.yaml
  terms_source: .kapi/terms.json
profiles:
  acme:
    channels: [docs]
collections:
  - name: acme-docs
    channel: acme/docs
    content:
      - path: "docs/**/*.md"
`)
	write(".kapi/voice.yaml", `name: Acme
description: Plain, exact writing.
tone:
  personality: [clear, restrained]
  formality: neutral
`)
	write(".kapi/terms.json", `{
  "schemaVersion": "1.0",
  "kind": "kapi-terms",
  "concepts": [
    {
      "id": "c-memory",
      "terms": [
        {"text": "content memory", "locale": "en", "status": "preferred"},
        {"text": "translation memory", "locale": "en", "status": "deprecated"}
      ],
      "created_at": "2026-01-01T00:00:00Z",
      "updated_at": "2026-01-01T00:00:00Z"
    }
  ]
}
`)
	write("docs/guide.md", "# Guide\n")
	t.Chdir(root)
	return root
}

// runContext executes `kapi context` with the given arguments and returns what
// it wrote to stdout.
func runContext(t *testing.T, a *App, args ...string) string {
	t.Helper()
	root := &cobra.Command{Use: "kapi"}
	AddCommandGroups(a, root)
	output.AddPersistentFlags(root.PersistentFlags())
	root.AddCommand(NewContextCmd(a))
	root.SetArgs(append([]string{"context"}, args...))

	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	require.NoError(t, root.Execute())
	return out.String()
}

// TestContextPath_PrintsTheHostRender is the CLI leg of the parity contract:
// the verb prints the answer's own FormatText, which is the body the
// `context://` resource serves. Two renderings of one answer is how the
// surfaces came to disagree before.
func TestContextPath_PrintsTheHostRender(t *testing.T) {
	writeGovernedProject(t)
	a := &App{}

	got := runContext(t, a, "docs/guide.md")

	cmd := host.NewEnvCommand(t.Context(), "context")
	req := host.ContextPointRequest{Path: "docs/guide.md"}
	src, cleanup := a.ContextSourcesAt(cmd, req)
	defer cleanup()
	answer, err := host.ResolveContextAt(t.Context(), src, req)
	require.NoError(t, err)

	var want bytes.Buffer
	require.NoError(t, answer.FormatText(&want))
	assert.Equal(t, want.String(), got)
}

// TestContextPath_AnswersWhatAppliesHere: the point, the voice in force and the
// vocabulary bound there, in one document — the question a writer actually has.
func TestContextPath_AnswersWhatAppliesHere(t *testing.T) {
	writeGovernedProject(t)

	got := runContext(t, &App{}, "docs/guide.md")

	assert.Contains(t, got, "# Context at docs/guide.md")
	assert.Contains(t, got, "Point `acme/docs`: profile `acme`, channel `docs`, collection `acme-docs`.")
	assert.Contains(t, got, "## Voice Guide: Acme")
	assert.Contains(t, got, "~~translation memory~~ → say **content memory**")
	assert.Contains(t, got, "Answered from this project alone")
}

// TestContextPath_JSONIsTheSameDocument: --json is the structured rendering of
// the answer the markdown renders, so a program and a model read one thing.
func TestContextPath_JSONIsTheSameDocument(t *testing.T) {
	writeGovernedProject(t)

	raw := runContext(t, &App{}, "docs/guide.md", "--json")

	var got host.ContextAnswer
	require.NoError(t, json.Unmarshal([]byte(raw), &got))
	assert.Equal(t, "acme/docs", got.Point.Ref)
	assert.Equal(t, host.ScopeProject, got.Scope)
	require.NotNil(t, got.Voice)
	assert.Equal(t, "Acme", got.Voice.Name)
	assert.NotEmpty(t, got.Voice.Guide, "the guidance travels in the structured shape too")
}

// TestContextPath_UnclaimedPathSaysSo: a location no collection claims is
// governed by the project default, which is an answer — and a different one
// from "no governance exists".
func TestContextPath_UnclaimedPathSaysSo(t *testing.T) {
	writeGovernedProject(t)

	got := runContext(t, &App{}, "README.md")

	assert.Contains(t, got, "This location sits at the project's default point: no profile claims it.")
}

// TestContextPath_NoAddressShowsHelp: `kapi context` on its own is not a
// question, so it explains the two that are rather than answering neither.
func TestContextPath_NoAddressShowsHelp(t *testing.T) {
	writeGovernedProject(t)

	got := runContext(t, &App{})

	assert.Contains(t, got, "kapi context <path>")
	assert.Contains(t, got, "kapi context search <query>")
}
