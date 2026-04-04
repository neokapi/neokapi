// Package bridgetest provides shared test helpers for Java bridge filter
// integration tests. It manages a shared bridge process, testdata loading,
// and common extraction utilities used across all filter test packages.
package bridgetest

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/plugin/bridge"
	"github.com/stretchr/testify/require"
)

// sharedBridge holds the singleton bridge registry used across all filter tests
// within a single test binary invocation.
var (
	initShared     func()
	sharedRegistry *bridge.BridgeRegistry
	sharedCfg      bridge.BridgeConfig
	sharedErr      error
	doCleanup      func()
)

func init() {
	doCleanup = sync.OnceFunc(func() {
		if sharedRegistry != nil {
			sharedRegistry.Shutdown()
		}
		bridge.KillTrackedProcesses()
	})

	initShared = sync.OnceFunc(func() {
		// External bridge mode: connect to pre-started JVM(s).
		if addrs := os.Getenv("NEOKAPI_BRIDGE_ADDRS"); addrs != "" {
			addrList := strings.Split(addrs, ",")
			var trimmed []string
			for _, a := range addrList {
				a = strings.TrimSpace(a)
				if a != "" {
					trimmed = append(trimmed, a)
				}
			}
			if len(trimmed) == 0 {
				sharedErr = errFatal("NEOKAPI_BRIDGE_ADDRS is set but contains no valid addresses")
				return
			}

			// For external bridges, create a registry with enough capacity.
			sharedRegistry = bridge.NewBridgeRegistry(len(trimmed), len(trimmed), log.Default())
			// Pre-warm each external address.
			for _, addr := range trimmed {
				cfg := bridge.BridgeConfig{Address: addr, PoolGroup: "external-bridges"}
				if err := sharedRegistry.Warmup(cfg); err != nil {
					sharedErr = errFatalf("connecting to external bridge at %s: %v", addr, err)
					return
				}
			}
			sharedCfg = bridge.BridgeConfig{PoolGroup: "external-bridges"}
			return
		}

		// Subprocess mode: start a JVM per binary.
		jar := os.Getenv("NEOKAPI_BRIDGE_JAR")
		if jar == "" {
			sharedErr = errFatal("NEOKAPI_BRIDGE_JAR not set — run scripts/fetch-okapi-bridge.sh")
			return
		}
		if _, err := os.Stat(jar); os.IsNotExist(err) {
			sharedErr = errFatalf("JAR not found at %s", jar)
			return
		}

		javaCmd := javaCommand()

		sharedCfg = bridge.BridgeConfig{
			Command: javaCmd,
			Args:    []string{"-jar", jar},
		}

		// Create registry and warmup the bridge.
		sharedRegistry = bridge.NewBridgeRegistry(8, 8, log.Default())
		if err := sharedRegistry.Warmup(sharedCfg); err != nil {
			sharedErr = errFatalf("failed to start bridge: %v", err)
			return
		}
	})
}

// Run runs all tests and then cleans up the shared bridge registry. Use this from
// TestMain in any package that calls SharedBridge:
//
//	func TestMain(m *testing.M) { os.Exit(bridgetest.Run(m)) }
func Run(m *testing.M) int {
	code := m.Run()
	Cleanup()
	return code
}

// Cleanup shuts down the shared bridge registry and kills any tracked bridge
// subprocesses. Safe to call multiple times (idempotent via sync.OnceFunc).
func Cleanup() {
	doCleanup()
}

// SharedBridge returns a shared BridgeRegistry and BridgeConfig for integration tests.
// It starts a single JVM process and reuses it across all tests in the binary.
// If Java or the bridge JAR is unavailable, it fails the test.
func SharedBridge(t *testing.T) (*bridge.BridgeRegistry, bridge.BridgeConfig) {
	t.Helper()

	initShared()

	if sharedErr != nil {
		t.Fatal(sharedErr.Error())
	}

	return sharedRegistry, sharedCfg
}

// RequireFilter verifies that a bridge is available for the given config.
// Since ListFilters has been removed, this acquires a bridge slot and
// immediately releases it to confirm the bridge is healthy. The actual
// filter class validation happens when Open is called.
func RequireFilter(t *testing.T, registry *bridge.BridgeRegistry, cfg bridge.BridgeConfig, filterClass string) {
	t.Helper()
	b, release, err := registry.Acquire(cfg)
	if err != nil {
		t.Fatalf("bridge not available for filter %s: %v", filterClass, err)
	}
	_ = b
	release()
}

// javaCommand returns the java binary path, respecting JAVA_HOME.
func javaCommand() string {
	if home := os.Getenv("JAVA_HOME"); home != "" {
		return filepath.Join(home, "bin", "java")
	}
	return "java"
}

// ReadString extracts parts from a string input using the specified filter.
func ReadString(t *testing.T, registry *bridge.BridgeRegistry, cfg bridge.BridgeConfig, filterClass, content, uri, mimeType string, filterParams map[string]any) []*model.Part {
	t.Helper()
	return ReadBytes(t, registry, cfg, filterClass, []byte(content), uri, mimeType, filterParams)
}

// ReadBytes extracts parts from byte content using the specified filter.
func ReadBytes(t *testing.T, registry *bridge.BridgeRegistry, cfg bridge.BridgeConfig, filterClass string, content []byte, uri, mimeType string, filterParams map[string]any) []*model.Part {
	t.Helper()
	return ReadBytesWithLocales(t, registry, cfg, filterClass, content, uri, mimeType, filterParams, "en", "fr")
}

// ReadBytesWithLocales is like ReadBytes but allows specifying source and target locales.
func ReadBytesWithLocales(t *testing.T, registry *bridge.BridgeRegistry, cfg bridge.BridgeConfig, filterClass string, content []byte, uri, mimeType string, filterParams map[string]any, srcLocale, tgtLocale string) []*model.Part {
	t.Helper()

	reader := bridge.NewBridgeFormatReader(registry, cfg, filterClass, format.FormatSignature{})
	if filterParams != nil {
		reader.SetFilterParams(filterParams)
	}

	doc := &model.RawDocument{
		URI:          uri,
		SourceLocale: model.LocaleID(srcLocale),
		TargetLocale: model.LocaleID(tgtLocale),
		Encoding:     "UTF-8",
		MimeType:     mimeType,
		Reader:       io.NopCloser(bytes.NewReader(content)),
	}

	ctx := context.Background()
	require.NoError(t, reader.Open(ctx, doc))

	var parts []*model.Part
	for pr := range reader.Read(ctx) {
		require.NoError(t, pr.Error, "reading part from bridge")
		parts = append(parts, pr.Part)
	}

	require.NoError(t, reader.Close())
	return parts
}

// ReadFileWithLocales reads testdata from disk with explicit locales.
func ReadFileWithLocales(t *testing.T, registry *bridge.BridgeRegistry, cfg bridge.BridgeConfig, filterClass, path, mimeType string, filterParams map[string]any, srcLocale, tgtLocale string) []*model.Part {
	t.Helper()

	content, err := os.ReadFile(path)
	require.NoError(t, err, "reading test file %s", path)

	return ReadBytesWithLocales(t, registry, cfg, filterClass, content, path, mimeType, filterParams, srcLocale, tgtLocale)
}

// ReadFile reads testdata from disk and extracts parts using the specified filter.
func ReadFile(t *testing.T, registry *bridge.BridgeRegistry, cfg bridge.BridgeConfig, filterClass, path, mimeType string, filterParams map[string]any) []*model.Part {
	t.Helper()

	content, err := os.ReadFile(path)
	require.NoError(t, err, "reading test file %s", path)

	return ReadBytes(t, registry, cfg, filterClass, content, path, mimeType, filterParams)
}

// FilterBlocks returns only Block parts from a list of parts.
func FilterBlocks(parts []*model.Part) []*model.Block {
	var blocks []*model.Block
	for _, p := range parts {
		if p.Type == model.PartBlock {
			if block, ok := p.Resource.(*model.Block); ok {
				blocks = append(blocks, block)
			}
		}
	}
	return blocks
}

// BlockTexts returns the source text of each block.
func BlockTexts(blocks []*model.Block) []string {
	texts := make([]string, len(blocks))
	for i, b := range blocks {
		texts[i] = b.SourceText()
	}
	return texts
}

// TranslatableBlocks returns only translatable blocks from a part list.
func TranslatableBlocks(parts []*model.Part) []*model.Block {
	var blocks []*model.Block
	for _, p := range parts {
		if p.Type == model.PartBlock {
			if block, ok := p.Resource.(*model.Block); ok && block.Translatable {
				blocks = append(blocks, block)
			}
		}
	}
	return blocks
}

// DataParts returns all Data parts from a part list.
func DataParts(parts []*model.Part) []*model.Part {
	var result []*model.Part
	for _, p := range parts {
		if p.Type == model.PartData {
			result = append(result, p)
		}
	}
	return result
}

// TestdataDir returns the path to the versioned okapi-testdata directory.
func TestdataDir(t *testing.T) string {
	t.Helper()

	dir, err := findRepoRoot()
	require.NoError(t, err, "finding repo root")

	baseDir := filepath.Join(dir, "okapi-testdata")
	if _, err := os.Stat(baseDir); os.IsNotExist(err) {
		t.Fatal("okapi-testdata/ not found — run scripts/fetch-okapi-testdata.sh to fetch test data")
	}

	entries, err := os.ReadDir(baseDir)
	require.NoError(t, err, "reading okapi-testdata/")

	var latest string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if _, serr := os.Stat(filepath.Join(baseDir, e.Name(), "okapi", "filters")); serr == nil {
			latest = e.Name()
		}
	}
	if latest == "" {
		t.Fatal("okapi-testdata/ has no version subdirectories — run scripts/fetch-okapi-testdata.sh to fetch test data")
	}

	return filepath.Join(baseDir, latest)
}

// TestdataFile returns the full path to a file within the okapi-testdata directory.
func TestdataFile(t *testing.T, relPath string) string {
	t.Helper()

	dir := TestdataDir(t)
	full := filepath.Join(dir, relPath)
	if _, err := os.Stat(full); os.IsNotExist(err) {
		t.Fatalf("testdata file not found: %s", relPath)
	}
	return full
}

// FilterTestResourceDir returns the path to a filter module's unit test resources.
func FilterTestResourceDir(t *testing.T, filterModule string) string {
	t.Helper()

	dir := filepath.Join(TestdataDir(t), "okapi", "filters", filterModule, "src", "test", "resources")
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Fatalf("filter test resource dir not found: %s (filter module %q)", dir, filterModule)
	}
	return dir
}

// IntegrationTestResourceDir returns the path to the Okapi integration test resources.
func IntegrationTestResourceDir(t *testing.T) string {
	t.Helper()

	dir := filepath.Join(TestdataDir(t), "integration-tests", "okapi", "src", "test", "resources")
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Fatalf("integration test resource dir not found: %s", dir)
	}
	return dir
}

// findRepoRoot walks up from the current directory looking for go.work.
func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.work")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", os.ErrNotExist
		}
		dir = parent
	}
}

type fatalError struct {
	msg string
}

func (e *fatalError) Error() string { return e.msg }

func errFatal(msg string) error {
	return &fatalError{msg: msg}
}

func errFatalf(format string, args ...any) error {
	return &fatalError{msg: fmt.Sprintf(format, args...)}
}
