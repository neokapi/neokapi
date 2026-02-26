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
	"sync"
	"testing"

	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/core/plugin/bridge"
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
)

// SharedBridge returns a shared BridgePool and BridgeConfig for integration tests.
// It starts a single JVM process and reuses it across all tests in the binary.
// If Java or the bridge JAR is unavailable, it fails the test.
func SharedBridge(t *testing.T) (*bridge.BridgePool, bridge.BridgeConfig) {
	t.Helper()

	sharedOnce.Do(func() {
		jar := os.Getenv("GOKAPI_BRIDGE_JAR")
		if jar == "" {
			sharedErr = errFatal("GOKAPI_BRIDGE_JAR not set — run scripts/fetch-okapi-bridge.sh")
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

		sharedPool = bridge.NewBridgePool(2, log.Default())
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
		SourceLocale: "en",
		TargetLocale: "fr",
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

	// Find version subdirectories (contain okf_* filter dirs).
	entries, err := os.ReadDir(baseDir)
	require.NoError(t, err, "reading okapi-testdata/")

	// Pick the last version lexicographically (highest version).
	var latest string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		// Verify it looks like a testdata version (contains filter subdirs).
		if _, serr := os.Stat(filepath.Join(baseDir, e.Name(), "okf_html")); serr == nil {
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
