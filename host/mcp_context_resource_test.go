package host_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/neokapi/neokapi/host"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The `context://` resources (AD-037). They are the by-location primitive over
// MCP, and they wrap the same host functions `kapi context <path>` calls — so
// these tests assert that the resource body IS the CLI's render, byte for byte,
// rather than a second rendering that agrees today.

// contextFixture writes a small governed project and points the process at it.
// Returns the project root.
func contextFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	write := func(rel, body string) {
		path := filepath.Join(root, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
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
style:
  active_voice: true
vocabulary:
  forbidden_terms:
    - term: utilize
      replacement: use
`)
	write(".kapi/terms.json", `{
  "schemaVersion": "1.0",
  "kind": "kapi-terms",
  "concepts": [
    {
      "id": "c-memory",
      "definition": "The store of source/target pairs the loop reuses.",
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

	t.Setenv("KAPI_PROJECT", filepath.Join(root, "kapi.yaml"))
	t.Chdir(root)
	return root
}

// contextClient starts a real server over an in-memory transport and returns a
// connected client session — the same call sequence an assistant makes.
func contextClient(t *testing.T, app *host.App) *mcp.ClientSession {
	t.Helper()
	server := mcp.NewServer(&mcp.Implementation{Name: "kapi", Version: "test"}, nil)
	host.ApplyMCPToolFactories(server, app)

	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = serverSession.Close() })

	client := mcp.NewClient(&mcp.Implementation{Name: "resource-test", Version: "test"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close() })
	return session
}

// hostAnswer is the by-location answer the CLI verb produces for the same
// request, through the same assembly.
func hostAnswer(t *testing.T, app *host.App, req host.ContextPointRequest) *host.ContextAnswer {
	t.Helper()
	cmd := host.NewEnvCommand(t.Context(), "context")
	src, cleanup := app.ContextSourcesAt(cmd, req)
	defer cleanup()
	res, err := host.ResolveContextAt(t.Context(), src, req)
	require.NoError(t, err)
	return res
}

// TestContextResourcesAreServed: the two addresses AD-037 documents are the two
// the server actually offers, as markdown. Nothing served them before this, and
// a client that cannot list them cannot reach the primitive at all.
func TestContextResourcesAreServed(t *testing.T) {
	contextFixture(t)
	session := contextClient(t, &host.App{})

	res, err := session.ListResourceTemplates(t.Context(), &mcp.ListResourceTemplatesParams{})
	require.NoError(t, err)

	served := map[string]string{}
	for _, tmpl := range res.ResourceTemplates {
		served[tmpl.URITemplate] = tmpl.MIMEType
	}
	assert.Equal(t, "text/markdown", served["context://{+path}{?format}"],
		"the by-location address, rendered as prose for a model")
	assert.Equal(t, "text/markdown", served["context://profile/{name}{?format}"],
		"the by-name address, for a caller with no file in hand")
}

// TestContextResourceMatchesTheCLIRender is the parity contract: the resource
// body is the CLI's markdown, produced by the same host call. The surfaces
// drifted before — six CLI retrieval verbs against three MCP tools — and a test
// is the only thing that stops it recurring.
func TestContextResourceMatchesTheCLIRender(t *testing.T) {
	contextFixture(t)
	app := &host.App{}
	session := contextClient(t, app)

	tests := []struct {
		name string
		uri  string
		req  host.ContextPointRequest
	}{
		{
			name: "a governed location",
			uri:  "context://docs/guide.md",
			req:  host.ContextPointRequest{Path: "docs/guide.md"},
		},
		{
			name: "a location no collection claims",
			uri:  "context://README.md",
			req:  host.ContextPointRequest{Path: "README.md"},
		},
		{
			name: "a profile by name",
			uri:  "context://profile/acme",
			req:  host.ContextPointRequest{Profile: "acme"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			read, err := session.ReadResource(t.Context(), &mcp.ReadResourceParams{URI: tt.uri})
			require.NoError(t, err)
			require.Len(t, read.Contents, 1)
			assert.Equal(t, tt.uri, read.Contents[0].URI)
			assert.Equal(t, "text/markdown", read.Contents[0].MIMEType)

			assert.Equal(t, renderAnswer(t, hostAnswer(t, app, tt.req)), read.Contents[0].Text)
		})
	}
}

// TestContextResourceRenderingIsAProperty: the same address answers as prose or
// as structure, and the mime type says which. That is what dissolved the
// voice-guide tool — rendering is a property of the read, not a second tool.
func TestContextResourceRenderingIsAProperty(t *testing.T) {
	contextFixture(t)
	app := &host.App{}
	session := contextClient(t, app)

	read, err := session.ReadResource(t.Context(),
		&mcp.ReadResourceParams{URI: "context://docs/guide.md?format=json"})
	require.NoError(t, err)
	require.Len(t, read.Contents, 1)
	assert.Equal(t, "application/json", read.Contents[0].MIMEType)

	var got host.ContextAnswer
	require.NoError(t, json.Unmarshal([]byte(read.Contents[0].Text), &got))

	want, err := json.Marshal(hostAnswer(t, app, host.ContextPointRequest{Path: "docs/guide.md"}))
	require.NoError(t, err)
	gotRaw, err := json.Marshal(&got)
	require.NoError(t, err)
	assert.JSONEq(t, string(want), string(gotRaw),
		"the JSON rendering is the same document the CLI's --json emits")
}

// TestContextResourceRejectsAnUnknownRendering: a caller that asked for a shape
// it can parse must not be handed prose it cannot.
func TestContextResourceRejectsAnUnknownRendering(t *testing.T) {
	contextFixture(t)
	session := contextClient(t, &host.App{})

	_, err := session.ReadResource(t.Context(),
		&mcp.ReadResourceParams{URI: "context://docs/guide.md?format=xml"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown context rendering")
}

// TestContextResourceStatesItsScope: a local answer says what it answered from,
// so "this project holds no answer" stays distinguishable from "this scope
// cannot hold one".
func TestContextResourceStatesItsScope(t *testing.T) {
	contextFixture(t)
	session := contextClient(t, &host.App{})

	read, err := session.ReadResource(t.Context(),
		&mcp.ReadResourceParams{URI: "context://docs/guide.md"})
	require.NoError(t, err)
	require.Len(t, read.Contents, 1)
	assert.Contains(t, read.Contents[0].Text, "Answered from this project alone")
	assert.Contains(t, read.Contents[0].Text,
		"project scope: concept relations, revisions and market scoping live in a connected workspace")
}
