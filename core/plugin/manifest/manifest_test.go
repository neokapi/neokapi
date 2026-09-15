package manifest_test

import (
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/plugin/manifest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseModeAManifest(t *testing.T) {
	raw := []byte(`{
		"manifest_version": "1",
		"plugin": "bowrain",
		"version": "1.4.0",
		"description": "Sync .kapi projects with Bowrain Server",
		"author": "Neokapi <hello@neokapi.dev>",
		"license": "Apache-2.0",
		"binary": "kapi-bowrain",
		"min_kapi_version": "1.0.0",
		"capabilities": {
			"commands": [
				{
					"name": "push",
					"short": "Upload local changes to the server",
					"args": [{"name": "paths", "variadic": true, "optional": true}],
					"flags": [
						{"name": "force", "type": "bool", "default": false},
						{"name": "dry-run", "type": "bool", "default": false}
					]
				},
				{"name": "auth", "subcommands": ["login", "logout", "status", {"name": "token", "subcommands": ["create", "list", "delete"]}]}
			],
			"mcp_tools": [{"name": "project_status", "description": "Show sync status"}],
			"schema_extensions": [{"name": "server", "scope": "project", "json_schema": "schemas/server.json"}],
			"source_connectors": [{"id": "bowrain"}]
		},
		"daemon": {
			"idle_timeout_seconds": 300,
			"handshake": {"type": "stdio-handshake", "fields": ["socket", "version"]}
		}
	}`)

	m, err := manifest.Parse(raw)
	require.NoError(t, err)
	assert.Equal(t, "1", m.ManifestVersion)
	assert.Equal(t, "bowrain", m.Plugin)
	assert.Equal(t, "1.4.0", m.Version)
	assert.Equal(t, "kapi-bowrain", m.Binary)
	assert.Len(t, m.Capabilities.Commands, 2)
	assert.True(t, m.IsModeA())
	assert.True(t, m.IsModeB())
	assert.True(t, m.IsModeC()) // has source_connectors
	assert.Equal(t, "push", m.Capabilities.Commands[0].Name)
	assert.True(t, m.Capabilities.Commands[0].Args[0].Variadic)
	assert.Equal(t, "auth", m.Capabilities.Commands[1].Name)
	assert.ElementsMatch(t, []string{"login", "logout", "status", "token"}, m.Capabilities.Commands[1].SubcommandNames())
	// The nested "token" subcommand carries its own children.
	var tokenSub *manifest.Subcommand
	for i := range m.Capabilities.Commands[1].Subcommands {
		if m.Capabilities.Commands[1].Subcommands[i].Name == "token" {
			tokenSub = &m.Capabilities.Commands[1].Subcommands[i]
		}
	}
	require.NotNil(t, tokenSub)
	assert.ElementsMatch(t, []string{"create", "list", "delete"}, subcommandNames(tokenSub.Subcommands))
}

// subcommandNames is a small test helper extracting subcommand names.
func subcommandNames(subs []manifest.Subcommand) []string {
	names := make([]string, len(subs))
	for i, s := range subs {
		names[i] = s.Name
	}
	return names
}

func TestParseOkapiBridgeManifest(t *testing.T) {
	raw := []byte(`{
		"manifest_version": "1",
		"plugin": "okapi-bridge",
		"version": "1.47.0",
		"binary": "kapi-okapi-bridge",
		"license": "Apache-2.0",
		"capabilities": {
			"formats": [
				{"name": "okf_idml", "extensions": [".idml"], "capabilities": ["read", "write"]},
				{"name": "okf_html", "extensions": [".html", ".htm"], "capabilities": ["read", "write"]}
			]
		},
		"daemon": {
			"idle_timeout_seconds": 300,
			"startup_timeout_seconds": 30,
			"handshake": {"type": "stdio-handshake", "fields": ["socket", "version"]}
		}
	}`)

	m, err := manifest.Parse(raw)
	require.NoError(t, err)
	assert.False(t, m.IsModeA())
	assert.False(t, m.IsModeB())
	assert.True(t, m.IsModeC())
	require.Len(t, m.Capabilities.Formats, 2)
	assert.True(t, m.Capabilities.Formats[0].HasCapability("read"))
	assert.True(t, m.Capabilities.Formats[0].HasCapability("write"))
}

func TestValidate_RequiredFields(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{"missing manifest_version", `{"plugin": "x", "version": "1", "binary": "x"}`, "manifest_version"},
		{"unsupported version", `{"manifest_version": "9", "plugin": "x", "version": "1", "binary": "x"}`, "unsupported manifest_version"},
		{"missing plugin", `{"manifest_version": "1", "version": "1", "binary": "x"}`, "plugin is required"},
		{"invalid plugin name", `{"manifest_version": "1", "plugin": "Bad_Name", "version": "1", "binary": "x"}`, "invalid plugin name"},
		{"missing version", `{"manifest_version": "1", "plugin": "x", "binary": "x"}`, "version is required"},
		{"missing binary", `{"manifest_version": "1", "plugin": "x", "version": "1"}`, "binary is required"},
		{"binary with backslash", `{"manifest_version": "1", "plugin": "x", "version": "1", "binary": "bin\\x"}`, "backslashes are not allowed"},
		{"binary absolute path", `{"manifest_version": "1", "plugin": "x", "version": "1", "binary": "/usr/local/bin/x"}`, "absolute paths are not allowed"},
		{"binary parent-dir segment", `{"manifest_version": "1", "plugin": "x", "version": "1", "binary": "bin/../x"}`, "parent-dir segments are not allowed"},
		{"daemon required for formats", `{"manifest_version": "1", "plugin": "x", "version": "1", "binary": "x", "capabilities": {"formats": [{"name": "f"}]}}`, "daemon block is required"},
		{"daemon only with mode-c", `{"manifest_version": "1", "plugin": "x", "version": "1", "binary": "x", "daemon": {"idle_timeout_seconds": 1}}`, "daemon block is only valid"},
		{"command needs name", `{"manifest_version": "1", "plugin": "x", "version": "1", "binary": "x", "capabilities": {"commands": [{}]}}`, "name is required"},
		{"schema_extension needs scope", `{"manifest_version": "1", "plugin": "x", "version": "1", "binary": "x", "capabilities": {"schema_extensions": [{"name": "x", "scope": "wrong"}]}}`, "invalid scope"},
		{"daemon required for comments", `{"manifest_version": "1", "plugin": "x", "version": "1", "binary": "x", "capabilities": {"comments": [` + commentLanguage(`"typescript"`, `[".ts"]`, `{"source": "// a", "block": "comment"}`) + `]}}`, "daemon block is required"},
		{"comment language needs a name", `{"manifest_version": "1", "plugin": "x", "version": "1", "binary": "x", "daemon": {}, "capabilities": {"comments": [` + commentLanguage(`""`, `[".ts"]`, `{"source": "// a", "block": "comment"}`) + `]}}`, "language is required"},
		{"comment language name is an identifier", `{"manifest_version": "1", "plugin": "x", "version": "1", "binary": "x", "daemon": {}, "capabilities": {"comments": [` + commentLanguage(`"Type-Script"`, `[".ts"]`, `{"source": "// a", "block": "comment"}`) + `]}}`, "invalid language"},
		{"comment language needs extensions", `{"manifest_version": "1", "plugin": "x", "version": "1", "binary": "x", "daemon": {}, "capabilities": {"comments": [` + commentLanguage(`"typescript"`, `[]`, `{"source": "// a", "block": "comment"}`) + `]}}`, "declares no extensions"},
		{"comment extension has a dot", `{"manifest_version": "1", "plugin": "x", "version": "1", "binary": "x", "daemon": {}, "capabilities": {"comments": [` + commentLanguage(`"typescript"`, `["ts"]`, `{"source": "// a", "block": "comment"}`) + `]}}`, "invalid extension"},
		{"comment language needs markers", `{"manifest_version": "1", "plugin": "x", "version": "1", "binary": "x", "daemon": {}, "capabilities": {"comments": [{"language": "typescript", "extensions": [".ts"], "canary": {"source": "// a", "block": "comment"}}]}}`, "declares no comment markers"},
		{"comment marker is not empty", `{"manifest_version": "1", "plugin": "x", "version": "1", "binary": "x", "daemon": {}, "capabilities": {"comments": [{"language": "typescript", "extensions": [".ts"], "markers": {"block": [{"open": "/*", "close": ""}]}, "canary": {"source": "// a", "block": "comment"}}]}}`, "empty comment marker"},
		{"comment language needs a canary", `{"manifest_version": "1", "plugin": "x", "version": "1", "binary": "x", "daemon": {}, "capabilities": {"comments": [` + commentLanguage(`"typescript"`, `[".ts"]`, `{"source": "// a"}`) + `]}}`, "canary source and block are required"},
		{"rewrite needs a whole canary", commentRewriteManifest(`{"canary": {"name": "c.ts", "source": "// a", "block": "comment"}, "formatters": [`+testFormatter+`], "formatter_canary": "x"}`, false), "canary name, source, block and refused are required"},
		{"rewrite needs a formatter", commentRewriteManifest(`{"canary": `+testRewriteCanary+`, "formatters": [], "formatter_canary": "x"}`, false), "at least one formatter is required"},
		{"rewrite needs a formatter canary", commentRewriteManifest(`{"canary": `+testRewriteCanary+`, "formatters": [`+testFormatter+`]}`, false), "formatter_canary is required"},
		{"rewrite formatter needs a command", commentRewriteManifest(`{"canary": `+testRewriteCanary+`, "formatters": [{"name": "prettier", "detect": [{"file": ".prettierrc"}], "command": []}], "formatter_canary": "x"}`, false), "name and command are required"},
		{"rewrite formatter marker is a file name", commentRewriteManifest(`{"canary": `+testRewriteCanary+`, "formatters": [{"name": "prettier", "detect": [{"file": "config/.prettierrc"}], "command": ["prettier"]}], "formatter_canary": "x"}`, false), "without a directory"},
		{"rewrite of delimited comments needs a terminator case", commentRewriteManifest(`{"canary": `+testRewriteCanary+`, "formatters": [`+testFormatter+`], "formatter_canary": "x"}`, true), "declares delimited and terminator"},
		{"rewrite declares delimited and terminator together", commentRewriteManifest(`{"canary": {"name": "c.ts", "source": "// a", "block": "comment", "refused": "x", "delimited": "func/f"}, "formatters": [`+testFormatter+`], "formatter_canary": "x"}`, true), "declared together"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := manifest.Parse([]byte(tc.raw))
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}

func TestParseAcceptsRelativeBinaryPath(t *testing.T) {
	// jpackage-built plugins (e.g. okapi-bridge v2.42+) declare the
	// launcher under bin/<name> on Linux, Contents/MacOS/<name> on macOS,
	// or <name>.exe on Windows. The validator must accept all three.
	for _, p := range []string{"bin/kapi-okapi-bridge", "Contents/MacOS/kapi-okapi-bridge", "kapi-okapi-bridge.exe"} {
		raw := []byte(`{
			"manifest_version": "1",
			"plugin": "okapi-bridge",
			"version": "2.42.0",
			"binary": "` + p + `"
		}`)
		_, err := manifest.Parse(raw)
		require.NoError(t, err, "expected binary %q to validate", p)
	}
}

func TestSchemaJSONEmbedded(t *testing.T) {
	raw := manifest.SchemaJSON()
	require.NotEmpty(t, raw)
	assert.True(t, strings.Contains(string(raw), "manifest_version"), "schema should mention manifest_version")
}

func TestRoundTrip(t *testing.T) {
	m := &manifest.Manifest{
		ManifestVersion: "1",
		Plugin:          "demo",
		Version:         "0.1.0",
		Binary:          "kapi-demo",
		License:         "MIT",
		Capabilities: manifest.Capabilities{
			Commands: []manifest.Command{{Name: "hello", Short: "Say hello"}},
		},
	}
	raw, err := m.Marshal()
	require.NoError(t, err)
	parsed, err := manifest.Parse(raw)
	require.NoError(t, err)
	assert.Equal(t, m.Plugin, parsed.Plugin)
	assert.Equal(t, m.Capabilities.Commands[0].Name, parsed.Capabilities.Commands[0].Name)
}

// testRewriteCanary and testFormatter are parts of a rewrite declaration that
// the validation table varies around.
const (
	testRewriteCanary = `{"name": "c.ts", "source": "// a", "block": "comment", "refused": "eslint-disable"}`
	testFormatter     = `{"name": "prettier", "detect": [{"file": ".prettierrc"}], "command": ["prettier", "--stdin-filepath", "{file}"]}`
)

// commentRewriteManifest is a manifest declaring one comment language with the
// rewrite declaration rewrite, and delimited comments when delimited is set.
func commentRewriteManifest(rewrite string, delimited bool) string {
	markers := `{"line": ["//"]}`
	if delimited {
		markers = `{"line": ["//"], "block": [{"open": "/*", "close": "*/"}]}`
	}
	return `{"manifest_version": "1", "plugin": "x", "version": "1", "binary": "x", "daemon": {}, "capabilities": {"comments": [{"language": "typescript", "extensions": [".ts"], "markers": ` + markers + `, "canary": {"source": "// a", "block": "comment"}, "rewrite": ` + rewrite + `}]}}`
}

// A whole rewrite declaration parses, and a language without one reads its
// comments and never writes them.
func TestParseCommentRewrite(t *testing.T) {
	raw := commentRewriteManifest(`{"canary": {"name": "c.ts", "source": "// a\n/* b */\n", "block": "comment", "refused": "eslint-disable", "delimited": "comment#2", "terminator": "b */ c"}, "formatters": [`+testFormatter+`], "formatter_canary": "function f() {\n        // x\n}\n"}`, true)
	m, err := manifest.Parse([]byte(raw))
	require.NoError(t, err)
	r := m.Capabilities.Comments[0].Rewrite
	require.NotNil(t, r)
	assert.Equal(t, "comment#2", r.Canary.Delimited)
	require.Len(t, r.Formatters, 1)
	assert.Equal(t, []string{"prettier", "--stdin-filepath", "{file}"}, r.Formatters[0].Command)
	assert.Equal(t, ".prettierrc", r.Formatters[0].Detect[0].File)

	plain, err := manifest.Parse([]byte(`{"manifest_version": "1", "plugin": "x", "version": "1", "binary": "x", "daemon": {}, "capabilities": {"comments": [` + commentLanguage(`"typescript"`, `[".ts"]`, `{"source": "// a", "block": "comment"}`) + `]}}`))
	require.NoError(t, err)
	assert.Nil(t, plain.Capabilities.Comments[0].Rewrite)
}

// commentLanguage is one capabilities.comments entry for the validation table.
func commentLanguage(language, extensions, canary string) string {
	return `{"language": ` + language + `, "extensions": ` + extensions + `, "markers": {"line": ["//"]}, "canary": ` + canary + `}`
}
