package pluginhost

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/plugin/manifest"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// makePluginWithBridge assembles a plugin install dir whose manifest
// declares the given format with read+write capability and whose binary
// is the fakedaemon compiled with FAKE_DAEMON_BRIDGE=1 support.
func makePluginWithBridge(t *testing.T, name, daemonBin, formatName string, exts []string) *Plugin {
	t.Helper()
	dir := t.TempDir()
	binDest := filepath.Join(dir, "fakedaemon")
	require.NoError(t, copyFile(daemonBin, binDest))
	require.NoError(t, os.Chmod(binDest, 0o755))

	m := &manifest.Manifest{
		ManifestVersion: manifest.CurrentVersion,
		Plugin:          name,
		Version:         "0.0.1",
		Binary:          "fakedaemon",
		Capabilities: manifest.Capabilities{
			Formats: []manifest.Format{{
				Name:         formatName,
				DisplayName:  formatName + " (test)",
				Extensions:   exts,
				Capabilities: []string{"read", "write"},
			}},
		},
		Daemon: &manifest.DaemonConfig{StartupTimeoutSeconds: 5},
	}
	require.NoError(t, m.Validate())

	manPath := filepath.Join(dir, "manifest.json")
	enc, err := json.MarshalIndent(m, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(manPath, enc, 0o644))

	return &Plugin{
		Dir:        dir,
		BinaryPath: binDest,
		Manifest:   m,
		Source: Source{
			Order: 1,
			Label: "test",
			Path:  dir,
		},
	}
}

// daemonPoolWithBridgeEnv builds a DaemonPool that spawns the fakedaemon
// with FAKE_DAEMON_BRIDGE=1 set so the BridgeService stub is registered.
// It does this by injecting the env via a wrapper script — the pool's
// spawn() inherits os.Environ(), so we set the env in the test process
// and rely on inheritance.
func daemonPoolWithBridgeEnv(t *testing.T) *DaemonPool {
	t.Helper()
	t.Setenv("FAKE_DAEMON_BRIDGE", "1")
	return NewDaemonPool(DaemonPoolOptions{
		MaxDaemons:     2,
		StartupTimeout: 10 * time.Second,
		ShutdownGrace:  500 * time.Millisecond,
	})
}

func TestRegisterModeCFormats_RegistersReaderAndWriter(t *testing.T) {
	bin := buildFakeDaemon(t)
	plugin := makePluginWithBridge(t, "fmt-plugin", bin, "fakefmt", []string{".fakefmt"})

	host := NewHost([]*Plugin{plugin}, nil)
	pool := daemonPoolWithBridgeEnv(t)
	t.Cleanup(pool.Shutdown)

	reg := registry.NewFormatRegistry()
	RegisterModeCFormats(host, pool, reg)

	require.True(t, reg.HasReader("fakefmt"), "reader factory should be registered")
	require.True(t, reg.HasWriter("fakefmt"), "writer factory should be registered")

	info := reg.FormatInfo("fakefmt")
	require.NotNil(t, info)
	assert.Equal(t, "fmt-plugin", info.Source)
	assert.Equal(t, []string{".fakefmt"}, info.Extensions)

	// Detection by extension goes through the registered signature.
	id, err := reg.DetectByExtension(".fakefmt")
	require.NoError(t, err)
	assert.Equal(t, registry.FormatID("fakefmt"), id)
}

func TestRegisterModeCFormats_ReaderStreamsParts(t *testing.T) {
	bin := buildFakeDaemon(t)
	plugin := makePluginWithBridge(t, "fmt-plugin", bin, "fakefmt", []string{".fakefmt"})

	host := NewHost([]*Plugin{plugin}, nil)
	pool := daemonPoolWithBridgeEnv(t)
	t.Cleanup(pool.Shutdown)

	reg := registry.NewFormatRegistry()
	RegisterModeCFormats(host, pool, reg)

	reader, err := reg.NewReader("fakefmt")
	require.NoError(t, err)
	defer reader.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	doc := &model.RawDocument{
		URI:          "/dev/null", // not used; reader falls back to inline content
		SourceLocale: model.LocaleID("en"),
		TargetLocale: model.LocaleID("fr"),
		Encoding:     "UTF-8",
	}
	require.NoError(t, reader.Open(ctx, doc))

	var blocks []*model.Block
	for result := range reader.Read(ctx) {
		require.NoError(t, result.Error)
		if result.Part.Type == model.PartBlock {
			blocks = append(blocks, result.Part.Resource.(*model.Block))
		}
	}
	require.Len(t, blocks, 1, "fakedaemon emits one block per Process call")
	// The fakedaemon echoes filter_class in the source text.
	require.NotEmpty(t, blocks[0].Source)
	assert.Equal(t, "fakefmt", textOfBlock(blocks[0]))
}

func TestRegisterModeCFormats_WriterRequiresSource(t *testing.T) {
	bin := buildFakeDaemon(t)
	plugin := makePluginWithBridge(t, "fmt-plugin", bin, "fakefmt", []string{".fakefmt"})

	host := NewHost([]*Plugin{plugin}, nil)
	pool := daemonPoolWithBridgeEnv(t)
	t.Cleanup(pool.Shutdown)

	reg := registry.NewFormatRegistry()
	RegisterModeCFormats(host, pool, reg)

	writer, err := reg.NewWriter("fakefmt")
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	parts := make(chan *model.Part)
	close(parts)

	// No source path / original content set — Write should fail with a
	// clear error rather than hang or panic.
	err = writer.Write(ctx, parts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no source path or original content")
}

func TestRegisterModeCFormats_NilPoolIsNoop(t *testing.T) {
	bin := buildFakeDaemon(t)
	plugin := makePluginWithBridge(t, "fmt-plugin", bin, "fakefmt", []string{".fakefmt"})

	host := NewHost([]*Plugin{plugin}, nil)
	reg := registry.NewFormatRegistry()
	RegisterModeCFormats(host, nil, reg)

	assert.False(t, reg.HasReader("fakefmt"), "no factory should be registered without a pool")
	assert.False(t, reg.HasWriter("fakefmt"))
}

// textOfBlock returns the concatenated text content of a block's source
// segments. Tests use this to assert on the fake daemon's emitted
// payload without traversing the full Run model.
func textOfBlock(b *model.Block) string {
	if b == nil || len(b.Source) == 0 {
		return ""
	}
	var out strings.Builder
	for _, run := range b.Source {
		if run.Text != nil {
			out.WriteString(run.Text.Text)
		}
	}
	return out.String()
}

// daemonReaderClose ensures Close is callable. Compile-time assertion
// would suffice, but a unit test is cheap and serves as documentation.
func TestDaemonReader_CloseIsNoop(t *testing.T) {
	r := newDaemonReader(nil, nil, "x", format.FormatSignature{}, "X")
	assert.NoError(t, r.Close())
}

// TestDaemonWriter_InlineModeOmitsOutputRef verifies that daemonWriter.Write
// does NOT send an OutputRef when only an io.Writer (inline mode) is set
// (issue #636). Previously it sent OutputRef{path:""} which the Java daemon
// tried to open as a file → java.io.FileNotFoundException.
//
// The fakedaemon checks header.Output != nil to decide whether to include
// output in the ProcessComplete response. Without the fix, the daemon would
// see a non-nil OutputRef (with empty path) and try to open "". With the fix
// the OutputRef is absent; the daemon returns inline bytes set via
// FAKE_DAEMON_OUTPUT, which the writer must copy into the provided io.Writer.
func TestDaemonWriter_InlineModeOmitsOutputRef(t *testing.T) {
	bin := buildFakeDaemon(t)
	plugin := makePluginWithBridge(t, "fmt-plugin", bin, "fakefmt", []string{".fakefmt"})

	host := NewHost([]*Plugin{plugin}, nil)
	pool := daemonPoolWithBridgeEnv(t)
	t.Cleanup(pool.Shutdown)

	// Tell the fakedaemon what bytes to return in ProcessComplete.output so
	// we can assert the writer copies them into the io.Writer.
	const fakeOutput = "pseudo-translated-output"
	t.Setenv("FAKE_DAEMON_OUTPUT", fakeOutput)

	reg := registry.NewFormatRegistry()
	RegisterModeCFormats(host, pool, reg)

	writer, err := reg.NewWriter("fakefmt")
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Set inline mode: provide an io.Writer but no output path.
	var buf strings.Builder
	require.NoError(t, writer.SetOutputWriter(&buf))

	// Provide source content so the writer doesn't error on the
	// "no source path or original content" guard.
	if ocs, ok := writer.(format.OriginalContentSetter); ok {
		ocs.SetOriginalContent([]byte("source content"))
	}

	writer.SetLocale(model.LocaleID("qps"))

	// Drive the write with an empty parts channel — no actual blocks to
	// translate, but the round-trip through the daemon must succeed.
	parts := make(chan *model.Part)
	close(parts)

	require.NoError(t, writer.Write(ctx, parts),
		"daemonWriter.Write must not error in inline mode (issue #636: was sending OutputRef{path:\"\"} which caused FileNotFoundException)")

	// The fakedaemon returned fakeOutput bytes; the writer must copy them.
	assert.Equal(t, fakeOutput, buf.String(),
		"inline output bytes from ProcessComplete must be copied into the io.Writer")
}
