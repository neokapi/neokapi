package pluginhost_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/neokapi/neokapi/cli/pluginhost"
	"github.com/neokapi/neokapi/core/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// schemaExtMu serializes schema-extension test cases that all touch the
// process-global core/project extension registry.
var schemaExtMu sync.Mutex

// validServerSchema describes a `server:` block that requires `url` (a URL
// string) and `team` (a non-empty string).
const validServerSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "type": "object",
  "required": ["url", "team"],
  "additionalProperties": false,
  "properties": {
    "url": {"type": "string", "format": "uri"},
    "team": {"type": "string", "minLength": 1},
    "stream": {"type": "boolean"}
  }
}`

// brokenSchema is malformed JSON. RegisterSchemaExtensions should fall
// back to a structural decoder and emit a warning at register time.
const brokenSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "type": "object",
  "properties": {`

// makeFakePlugin lays out a manifest.json + JSON Schema file under tmp
// and returns the discovered Host wired with the resulting plugin. The
// caller passes a schema body (valid or invalid) that gets written to
// `schemas/server.json`. When schemaBody is empty, no schema file is
// written so the read step fails — RegisterSchemaExtensions should
// fall back gracefully.
func makeFakePlugin(t *testing.T, schemaBody string) (*pluginhost.Host, *capturedWarn) {
	t.Helper()
	tmp := t.TempDir()
	pluginDir := filepath.Join(tmp, "demo")
	require.NoError(t, os.MkdirAll(filepath.Join(pluginDir, "schemas"), 0o755))

	// Always write the manifest.
	manifestBody := `{
  "manifest_version": "1",
  "plugin": "demo",
  "version": "0.1.0",
  "binary": "kapi-demo",
  "capabilities": {
    "schema_extensions": [
      {
        "name": "server",
        "scope": "project",
        "group": "demo",
        "json_schema": "schemas/server.json"
      }
    ]
  }
}`
	require.NoError(t, os.WriteFile(filepath.Join(pluginDir, "manifest.json"), []byte(manifestBody), 0o644))

	if schemaBody != "" {
		require.NoError(t, os.WriteFile(filepath.Join(pluginDir, "schemas", "server.json"), []byte(schemaBody), 0o644))
	}

	plugins := pluginhost.Discover(pluginhost.DiscoverOptions{
		EnvPluginsDir: tmp,
		HomeDir:       "/nonexistent",
		SystemDirs:    []string{},
	})
	require.Len(t, plugins, 1)

	cw := &capturedWarn{}
	host := pluginhost.NewHost(plugins, cw.add)
	require.NotNil(t, host)
	require.Equal(t, "demo", plugins[0].Name())
	require.Equal(t, "schemas/server.json", plugins[0].Manifest.Capabilities.SchemaExtensions[0].JSONSchema)
	return host, cw
}

type capturedWarn struct {
	mu   sync.Mutex
	msgs []string
}

func (c *capturedWarn) add(s string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.msgs = append(c.msgs, s)
}

func (c *capturedWarn) joined() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return strings.Join(c.msgs, "\n")
}

// TestSchemaExt_ValidPayloadPasses verifies that a recipe which conforms
// to the plugin's JSON Schema validates without errors.
func TestSchemaExt_ValidPayloadPasses(t *testing.T) {
	schemaExtMu.Lock()
	defer schemaExtMu.Unlock()

	project.ResetExtensionsForTest()
	defer project.ResetExtensionsForTest()

	host, cw := makeFakePlugin(t, validServerSchema)
	pluginhost.RegisterSchemaExtensions(host, cw.add)

	p := &project.KapiProject{}
	require.NoError(t, yaml.Unmarshal([]byte(`
version: v1
server:
  url: https://example.com/team/proj
  team: acme
  stream: true
`), p))

	require.NoError(t, p.Validate(), "warnings: %s", cw.joined())
}

// TestSchemaExt_MissingRequiredFieldFails verifies that a recipe missing
// a required schema field is rejected with a clear error pointing at
// the recipe location.
func TestSchemaExt_MissingRequiredFieldFails(t *testing.T) {
	schemaExtMu.Lock()
	defer schemaExtMu.Unlock()

	project.ResetExtensionsForTest()
	defer project.ResetExtensionsForTest()

	host, cw := makeFakePlugin(t, validServerSchema)
	pluginhost.RegisterSchemaExtensions(host, cw.add)

	p := &project.KapiProject{}
	require.NoError(t, yaml.Unmarshal([]byte(`
version: v1
server:
  url: https://example.com/team/proj
`), p))

	err := p.Validate()
	require.Error(t, err)
	// The error must surface the recipe key (project-scope prefix is empty).
	assert.Contains(t, err.Error(), "server:")
	assert.Contains(t, err.Error(), "demo.server")
	// The missing-field cause should be present in the flattened message.
	assert.Contains(t, strings.ToLower(err.Error()), "team")
}

// TestSchemaExt_WrongTypeFails verifies that a recipe with a value of
// the wrong type for a schema property is rejected.
func TestSchemaExt_WrongTypeFails(t *testing.T) {
	schemaExtMu.Lock()
	defer schemaExtMu.Unlock()

	project.ResetExtensionsForTest()
	defer project.ResetExtensionsForTest()

	host, cw := makeFakePlugin(t, validServerSchema)
	pluginhost.RegisterSchemaExtensions(host, cw.add)

	p := &project.KapiProject{}
	require.NoError(t, yaml.Unmarshal([]byte(`
version: v1
server:
  url: https://example.com/team/proj
  team: acme
  stream: "not-a-boolean"
`), p))

	err := p.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "server:")
	// jsonschema-go reports the type mismatch with the offending value
	// and the expected type. Both "string" (what we got) and "boolean"
	// (what stream wants) should appear.
	assert.Contains(t, err.Error(), "boolean")
}

// TestSchemaExt_AdditionalPropertiesRejected verifies that the schema's
// additionalProperties:false is honored.
func TestSchemaExt_AdditionalPropertiesRejected(t *testing.T) {
	schemaExtMu.Lock()
	defer schemaExtMu.Unlock()

	project.ResetExtensionsForTest()
	defer project.ResetExtensionsForTest()

	host, cw := makeFakePlugin(t, validServerSchema)
	pluginhost.RegisterSchemaExtensions(host, cw.add)

	p := &project.KapiProject{}
	require.NoError(t, yaml.Unmarshal([]byte(`
version: v1
server:
  url: https://example.com/team/proj
  team: acme
  unknown_key: oops
`), p))

	err := p.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "server:")
}

// TestSchemaExt_BadSchemaFallsBack verifies that an unparseable JSON
// Schema causes a warning and falls back to structural-only validation,
// so the rest of recipe loading still works.
func TestSchemaExt_BadSchemaFallsBack(t *testing.T) {
	schemaExtMu.Lock()
	defer schemaExtMu.Unlock()

	project.ResetExtensionsForTest()
	defer project.ResetExtensionsForTest()

	host, cw := makeFakePlugin(t, brokenSchema)
	pluginhost.RegisterSchemaExtensions(host, cw.add)

	// A warning must have been emitted at register time, naming the
	// plugin and explaining that we are falling back.
	joined := cw.joined()
	assert.Contains(t, joined, "demo")
	assert.Contains(t, strings.ToLower(joined), "falling back")
	assert.Contains(t, strings.ToLower(joined), "cannot parse json schema")

	// Recipe load should now succeed regardless of payload, because the
	// fallback decoder is structural-only.
	p := &project.KapiProject{}
	require.NoError(t, yaml.Unmarshal([]byte(`
version: v1
server:
  url: https://example.com/team/proj
  team: acme
  totally_unknown: 42
`), p))

	require.NoError(t, p.Validate())
}

// TestSchemaExt_MissingSchemaFileFallsBack verifies the file-not-found
// path also falls back gracefully.
func TestSchemaExt_MissingSchemaFileFallsBack(t *testing.T) {
	schemaExtMu.Lock()
	defer schemaExtMu.Unlock()

	project.ResetExtensionsForTest()
	defer project.ResetExtensionsForTest()

	host, cw := makeFakePlugin(t, "")
	pluginhost.RegisterSchemaExtensions(host, cw.add)

	assert.Contains(t, strings.ToLower(cw.joined()), "cannot read json schema")
	assert.Contains(t, strings.ToLower(cw.joined()), "falling back")

	p := &project.KapiProject{}
	require.NoError(t, yaml.Unmarshal([]byte(`
version: v1
server:
  url: https://example.com/team/proj
  team: acme
`), p))
	require.NoError(t, p.Validate())
}

// TestSchemaExt_ItemScopeValidation verifies validation works at the
// item scope, not just project scope. We register a separate plugin
// with an item-scope extension and exercise the nested recipe path.
func TestSchemaExt_ItemScopeValidation(t *testing.T) {
	schemaExtMu.Lock()
	defer schemaExtMu.Unlock()

	project.ResetExtensionsForTest()
	defer project.ResetExtensionsForTest()

	tmp := t.TempDir()
	pluginDir := filepath.Join(tmp, "sizecheck")
	require.NoError(t, os.MkdirAll(filepath.Join(pluginDir, "schemas"), 0o755))
	manifestBody := `{
  "manifest_version": "1",
  "plugin": "sizecheck",
  "version": "0.1.0",
  "binary": "kapi-sizecheck",
  "capabilities": {
    "schema_extensions": [
      {
        "name": "max_size",
        "scope": "item",
        "group": "sizecheck",
        "json_schema": "schemas/max_size.json"
      }
    ]
  }
}`
	require.NoError(t, os.WriteFile(filepath.Join(pluginDir, "manifest.json"), []byte(manifestBody), 0o644))
	schemaBody := `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "type": "string",
  "pattern": "^[0-9]+(B|KB|MB|GB)$"
}`
	require.NoError(t, os.WriteFile(filepath.Join(pluginDir, "schemas", "max_size.json"), []byte(schemaBody), 0o644))

	plugins := pluginhost.Discover(pluginhost.DiscoverOptions{
		EnvPluginsDir: tmp,
		HomeDir:       "/nonexistent",
		SystemDirs:    []string{},
	})
	require.Len(t, plugins, 1)

	cw := &capturedWarn{}
	host := pluginhost.NewHost(plugins, cw.add)
	pluginhost.RegisterSchemaExtensions(host, cw.add)

	// Valid item-scope value.
	good := &project.KapiProject{}
	require.NoError(t, yaml.Unmarshal([]byte(`
version: v1
content:
  - name: ui
    items:
      - path: src/foo.json
        max_size: "10MB"
`), good))
	require.NoError(t, good.Validate(), "warnings: %s", cw.joined())

	// Invalid item-scope value (does not match the pattern).
	bad := &project.KapiProject{}
	require.NoError(t, yaml.Unmarshal([]byte(`
version: v1
content:
  - name: ui
    items:
      - path: src/foo.json
        max_size: "ten megabytes"
`), bad))
	err := bad.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "content[0].items[0].max_size:")
}

// TestSchemaExt_IdempotentSameGroup verifies that when an extension is
// already registered under the same group — e.g. a binary (kapi-desktop)
// that compiles in a platform's schema via blank import and then
// rediscovers the same plugin through its manifest — RegisterSchemaExtensions
// keeps the existing registration silently (no "already registered" warning)
// and does not replace the compiled-in decoder with the manifest's.
func TestSchemaExt_IdempotentSameGroup(t *testing.T) {
	schemaExtMu.Lock()
	defer schemaExtMu.Unlock()

	project.ResetExtensionsForTest()
	defer project.ResetExtensionsForTest()

	// Simulate the compile-time (blank-import) registration: a typed
	// decoder under group "demo" that rejects everything with a sentinel.
	sentinel := errors.New("compiled-in decoder ran")
	project.RegisterExtension(project.Extension{
		Name:    "server",
		Scope:   project.ScopeProject,
		Group:   "demo",
		Decoder: project.ExtensionDecoderFunc(func(yaml.Node) error { return sentinel }),
	})

	host, cw := makeFakePlugin(t, validServerSchema)
	pluginhost.RegisterSchemaExtensions(host, cw.add)

	// Same group → benign re-discovery, no warning.
	assert.NotContains(t, cw.joined(), "already registered")

	// The compiled-in decoder must be kept: a payload the manifest's JSON
	// Schema would accept is still rejected by the sentinel decoder.
	p := &project.KapiProject{}
	require.NoError(t, yaml.Unmarshal([]byte(`
version: v1
server:
  url: https://example.com/team/proj
  team: acme
`), p))
	err := p.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), sentinel.Error())
}

// TestSchemaExt_DifferentGroupWarns verifies that when a *different* group
// already claims (scope, name), RegisterSchemaExtensions keeps the existing
// entry but warns about the genuine cross-plugin clash (naming the group
// that already owns the key).
func TestSchemaExt_DifferentGroupWarns(t *testing.T) {
	schemaExtMu.Lock()
	defer schemaExtMu.Unlock()

	project.ResetExtensionsForTest()
	defer project.ResetExtensionsForTest()

	project.RegisterExtension(project.Extension{
		Name:    "server",
		Scope:   project.ScopeProject,
		Group:   "other",
		Decoder: project.ExtensionDecoderFunc(func(yaml.Node) error { return nil }),
	})

	host, cw := makeFakePlugin(t, validServerSchema)
	pluginhost.RegisterSchemaExtensions(host, cw.add)

	joined := cw.joined()
	assert.Contains(t, joined, "already registered")
	assert.Contains(t, joined, "other") // names the existing group
	assert.Contains(t, joined, "server")
}
