package pluginhost

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/plugin/manifest"
	"github.com/neokapi/neokapi/core/segment"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const segSchemaJSON = `{
  "title": "Test SaT",
  "type": "object",
  "properties": {
    "satModel": { "type": "string", "title": "SaT Model", "ui:order": 10 },
    "threshold": { "type": "number", "title": "Threshold", "minimum": 0, "maximum": 1, "ui:order": 20 }
  }
}`

// makeSegmenterPlugin assembles a plugin install dir whose manifest declares a
// single segmenter (with a parameter schema) and whose binary is the fakedaemon.
func makeSegmenterPlugin(t *testing.T, daemonBin, pluginName, engineName string) *Plugin {
	t.Helper()
	dir := t.TempDir()
	binDest := filepath.Join(dir, "fakedaemon")
	require.NoError(t, copyFile(daemonBin, binDest))
	require.NoError(t, os.Chmod(binDest, 0o755))

	require.NoError(t, os.MkdirAll(filepath.Join(dir, "segment"), 0o755))
	schemaPath := filepath.Join("segment", "engine.json")
	require.NoError(t, os.WriteFile(filepath.Join(dir, schemaPath), []byte(segSchemaJSON), 0o644))

	m := &manifest.Manifest{
		ManifestVersion: manifest.CurrentVersion,
		Plugin:          pluginName,
		Version:         "0.0.1",
		Binary:          "fakedaemon",
		Capabilities: manifest.Capabilities{
			Segmenters: []manifest.Segmenter{{
				Name:        engineName,
				DisplayName: "Test ML segmenter",
				Description: "deterministic test segmenter",
				Schema:      schemaPath,
			}},
		},
		Daemon: &manifest.DaemonConfig{StartupTimeoutSeconds: 5},
	}
	require.NoError(t, m.Validate())

	enc, err := json.MarshalIndent(m, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "manifest.json"), enc, 0o644))

	return &Plugin{
		Dir:        dir,
		BinaryPath: binDest,
		Manifest:   m,
		Source:     Source{Order: 1, Label: "test", Path: dir},
	}
}

func TestRegisterModeCSegmenters_RegistersEngineWithSchema(t *testing.T) {
	bin := buildFakeDaemon(t)
	const engine = "test-seg-schema"
	plugin := makeSegmenterPlugin(t, bin, "seg-plugin", engine)
	host := NewHost([]*Plugin{plugin}, nil)
	pool := daemonPoolWithBridgeEnv(t)
	t.Cleanup(pool.Shutdown)

	require.True(t, RegisterModeCSegmenters(host, pool), "a new engine was registered")

	desc, ok := segment.Lookup(engine)
	require.True(t, ok, "engine registered into the segment registry")
	assert.Equal(t, "Test ML segmenter", desc.Label)
	require.NotNil(t, desc.Schema, "engine carries the manifest schema")
	assert.Contains(t, desc.Schema.Properties, "satModel")
	assert.Contains(t, desc.Schema.Properties, "threshold")

	// Idempotent: re-registering the same plugin does not add it again.
	assert.False(t, RegisterModeCSegmenters(host, pool), "re-scan adds nothing new")
}

func TestRegisterModeCSegmenters_SegmentOverDaemon(t *testing.T) {
	bin := buildFakeDaemon(t)
	const engine = "test-seg-run"
	plugin := makeSegmenterPlugin(t, bin, "seg-plugin-run", engine)
	host := NewHost([]*Plugin{plugin}, nil)
	pool := daemonPoolWithBridgeEnv(t)
	t.Cleanup(pool.Shutdown)
	require.True(t, RegisterModeCSegmenters(host, pool))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	runs := []model.Run{{Text: &model.TextRun{Text: "Hello world. How are you?"}}}

	// The fake daemon returns an interior boundary after each '.', so this text
	// yields two segments.
	seg, err := segment.Build(engine, segment.BaseConfig{}, map[string]any{"satModel": "sat-3l-sm"})
	require.NoError(t, err)
	spans, err := seg.Segment(ctx, runs, model.LocaleID("en"))
	require.NoError(t, err)
	assert.Len(t, spans, 2, "one interior boundary → two segments")

	// The error path surfaces the daemon's in-band error.
	failSeg, err := segment.Build(engine, segment.BaseConfig{}, map[string]any{"fail": "1"})
	require.NoError(t, err)
	_, err = failSeg.Segment(ctx, runs, model.LocaleID("en"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "forced failure")
}
