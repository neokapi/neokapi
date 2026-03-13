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
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/plugin/bridge"
	"github.com/stretchr/testify/require"
)

// sharedBridge holds the singleton bridge pool used across all filter tests
// within a single test binary invocation.
var (
	sharedOnce    sync.Once
	sharedPool    *bridge.BridgePool
	sharedCfg     bridge.BridgeConfig
	sharedErr     error
	availableOnce sync.Once
	availableSet  map[string]bool
	cleanupOnce   sync.Once
)

// Run runs all tests and then cleans up the shared bridge pool. Use this from
// TestMain in any package that calls SharedBridge:
//
//	func TestMain(m *testing.M) { os.Exit(bridgetest.Run(m)) }
func Run(m *testing.M) int {
	code := m.Run()
	Cleanup()
	return code
}

// Cleanup shuts down the shared bridge pool and kills any tracked bridge
// subprocesses. Safe to call multiple times (idempotent via sync.Once).
func Cleanup() {
	cleanupOnce.Do(func() {
		if sharedPool != nil {
			sharedPool.Shutdown()
		}
		bridge.KillTrackedProcesses()
	})
}

// SharedBridge returns a shared BridgePool and BridgeConfig for integration tests.
// It starts a single JVM process and reuses it across all tests in the binary.
// If Java or the bridge JAR is unavailable, it fails the test.
//
// The pool size defaults to 2 but can be overridden with the
// NEOKAPI_BRIDGE_POOL_SIZE environment variable.
func SharedBridge(t *testing.T) (*bridge.BridgePool, bridge.BridgeConfig) {
	t.Helper()

	sharedOnce.Do(func() {
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

			sharedPool = bridge.NewBridgePool(len(trimmed), log.Default())
			for _, addr := range trimmed {
				cfg := bridge.BridgeConfig{Address: addr}
				b := bridge.NewJavaBridge(cfg, log.Default())
				if err := b.Start(); err != nil {
					sharedErr = errFatalf("connecting to external bridge at %s: %v", addr, err)
					return
				}
				sharedPool.Seed(b)
			}
			// Use first address as the shared config (for pool.Acquire fallback).
			sharedCfg = bridge.BridgeConfig{Address: trimmed[0]}
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

		b := bridge.NewJavaBridge(sharedCfg, log.Default())
		if err := b.Start(); err != nil {
			sharedErr = errFatalf("failed to start bridge: %v", err)
			return
		}

		poolSize := 2
		if s := os.Getenv("NEOKAPI_BRIDGE_POOL_SIZE"); s != "" {
			if n, err := strconv.Atoi(s); err == nil && n > 0 {
				poolSize = n
			}
		}

		sharedPool = bridge.NewBridgePool(poolSize, log.Default())
		sharedPool.Seed(b)
	})

	if sharedErr != nil {
		t.Fatal(sharedErr.Error())
	}

	return sharedPool, sharedCfg
}

// RequireFilter skips the test if the given filter class is not available in
// the bridge JAR. This allows tests for optional filters (e.g. PlainTextFilter,
// MarkdownFilter, XLIFF2Filter) to gracefully skip when the filter is missing.
func RequireFilter(t *testing.T, pool *bridge.BridgePool, cfg bridge.BridgeConfig, filterClass string) {
	t.Helper()

	availableOnce.Do(func() {
		b, err := pool.Acquire(cfg)
		if err != nil {
			return
		}
		defer pool.Release(b)

		lf, err := b.ListFilters()
		if err != nil {
			return
		}

		availableSet = make(map[string]bool, len(lf.Filters))
		for _, f := range lf.Filters {
			availableSet[f.FilterClass] = true
		}
	})

	if !availableSet[filterClass] {
		t.Skipf("filter %s not available in bridge JAR", filterClass)
	}
}

// javaCommand returns the java binary path, respecting JAVA_HOME.
func javaCommand() string {
	if home := os.Getenv("JAVA_HOME"); home != "" {
		return filepath.Join(home, "bin", "java")
	}
	return "java"
}

// ReadString extracts parts from a string input using the specified filter.
// Returns the collected parts. Fails the test on any error.
func ReadString(t *testing.T, pool *bridge.BridgePool, cfg bridge.BridgeConfig, filterClass, content, uri, mimeType string, filterParams map[string]any) []*model.Part {
	t.Helper()
	return ReadBytes(t, pool, cfg, filterClass, []byte(content), uri, mimeType, filterParams)
}

// ReadBytes extracts parts from byte content using the specified filter.
// Returns the collected parts. Fails the test on any error.
func ReadBytes(t *testing.T, pool *bridge.BridgePool, cfg bridge.BridgeConfig, filterClass string, content []byte, uri, mimeType string, filterParams map[string]any) []*model.Part {
	t.Helper()

	reader := bridge.NewBridgeFormatReader(pool, cfg, filterClass)
	if filterParams != nil {
		reader.SetFilterParams(filterParams)
	}

	doc := &model.RawDocument{
		URI:          uri,
		SourceLocale: "",
		TargetLocale: "",
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

// ReadFile reads testdata from disk and extracts parts using the specified filter.
// The path must be an absolute path (use TestdataFile to resolve relative paths).
// Fails the test on any error.
func ReadFile(t *testing.T, pool *bridge.BridgePool, cfg bridge.BridgeConfig, filterClass, path, mimeType string, filterParams map[string]any) []*model.Part {
	t.Helper()

	content, err := os.ReadFile(path)
	require.NoError(t, err, "reading test file %s", path)

	return ReadBytes(t, pool, cfg, filterClass, content, path, mimeType, filterParams)
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

// TestdataDir returns the path to the versioned okapi-testdata directory at
// the repo root (e.g. okapi-testdata/1.48.0-v2/). It finds the most recent
// version subdirectory automatically. Fails the test if no version has been
// fetched.
func TestdataDir(t *testing.T) string {
	t.Helper()

	// Walk up from the test binary's working directory to find the repo root.
	// The testdata lives at <repo-root>/okapi-testdata/<version>/.
	dir, err := findRepoRoot()
	require.NoError(t, err, "finding repo root")

	baseDir := filepath.Join(dir, "okapi-testdata")
	if _, err := os.Stat(baseDir); os.IsNotExist(err) {
		t.Fatal("okapi-testdata/ not found — run scripts/fetch-okapi-testdata.sh to fetch test data")
	}

	// Find version subdirectories (contain okapi/filters/).
	entries, err := os.ReadDir(baseDir)
	require.NoError(t, err, "reading okapi-testdata/")

	// Pick the last version lexicographically (highest version).
	var latest string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		// Verify it looks like a testdata version (contains okapi/filters/).
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
// It fails the test if the testdata directory or the specific file doesn't exist.
func TestdataFile(t *testing.T, relPath string) string {
	t.Helper()

	dir := TestdataDir(t)
	full := filepath.Join(dir, relPath)
	if _, err := os.Stat(full); os.IsNotExist(err) {
		t.Fatalf("testdata file not found: %s", relPath)
	}
	return full
}

// FilterTestResourceDir returns the path to a filter module's unit test resources
// within the okapi-testdata directory. The filterModule is the Okapi filter module
// name (e.g., "html", "json", "xliff"). The returned path points to:
//
//	okapi-testdata/<version>/okapi/filters/<filterModule>/src/test/resources
//
// Fails the test if the directory doesn't exist.
func FilterTestResourceDir(t *testing.T, filterModule string) string {
	t.Helper()

	dir := filepath.Join(TestdataDir(t), "okapi", "filters", filterModule, "src", "test", "resources")
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Fatalf("filter test resource dir not found: %s (filter module %q)", dir, filterModule)
	}
	return dir
}

// IntegrationTestResourceDir returns the path to the Okapi integration test
// resources within the okapi-testdata directory. The returned path points to:
//
//	okapi-testdata/<version>/integration-tests/okapi/src/test/resources
//
// Fails the test if the directory doesn't exist.
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

// fatalError is used internally to convey fatal reasons through sync.Once.
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
