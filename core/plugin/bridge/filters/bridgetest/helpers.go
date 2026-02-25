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
	sharedOnce sync.Once
	sharedPool *bridge.BridgePool
	sharedCfg  bridge.BridgeConfig
	sharedErr  error
)

// SharedBridge returns a shared BridgePool and BridgeConfig for integration tests.
// It starts a single JVM process and reuses it across all tests in the binary.
// If Java or the bridge JAR is unavailable, it calls t.Skip.
func SharedBridge(t *testing.T) (*bridge.BridgePool, bridge.BridgeConfig) {
	t.Helper()

	sharedOnce.Do(func() {
		jar := os.Getenv("GOKAPI_BRIDGE_JAR")
		if jar == "" {
			sharedErr = errSkip("GOKAPI_BRIDGE_JAR not set")
			return
		}
		if _, err := os.Stat(jar); os.IsNotExist(err) {
			sharedErr = errSkipf("JAR not found at %s", jar)
			return
		}

		javaCmd := javaCommand()

		sharedCfg = bridge.BridgeConfig{
			Command: javaCmd,
			Args:    []string{"-jar", jar},
		}

		b := bridge.NewJavaBridge(sharedCfg, log.Default())
		if err := b.Start(); err != nil {
			sharedErr = errSkipf("failed to start bridge: %v", err)
			return
		}

		sharedPool = bridge.NewBridgePool(2, log.Default())
		sharedPool.Seed(b)
	})

	if sharedErr != nil {
		t.Skip(sharedErr.Error())
	}

	return sharedPool, sharedCfg
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
// The path is relative to the testdata root (see TestdataDir).
// Fails the test on any error.
func ReadFile(t *testing.T, pool *bridge.BridgePool, cfg bridge.BridgeConfig, filterClass, path, mimeType string, filterParams map[string]any) []*model.Part {
	t.Helper()

	content, err := os.ReadFile(path)
	require.NoError(t, err, "reading test file %s", path)

	uri := filepath.Base(path)
	return ReadBytes(t, pool, cfg, filterClass, content, uri, mimeType, filterParams)
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

// TestdataDir returns the path to the okapi-testdata directory at the repo root.
// It skips the test if the directory doesn't exist (hasn't been fetched).
func TestdataDir(t *testing.T) string {
	t.Helper()

	// Walk up from the test binary's working directory to find the repo root.
	// The testdata lives at <repo-root>/okapi-testdata/.
	dir, err := findRepoRoot()
	require.NoError(t, err, "finding repo root")

	tdDir := filepath.Join(dir, "okapi-testdata")
	if _, err := os.Stat(tdDir); os.IsNotExist(err) {
		t.Skip("okapi-testdata/ not found — run scripts/fetch-okapi-testdata.sh")
	}
	return tdDir
}

// TestdataFile returns the full path to a file within the okapi-testdata directory.
// It skips the test if the testdata directory or the specific file doesn't exist.
func TestdataFile(t *testing.T, relPath string) string {
	t.Helper()

	dir := TestdataDir(t)
	full := filepath.Join(dir, relPath)
	if _, err := os.Stat(full); os.IsNotExist(err) {
		t.Skipf("testdata file not found: %s", relPath)
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

// skipError is used internally to convey skip reasons through sync.Once.
type skipError struct {
	msg string
}

func (e *skipError) Error() string { return e.msg }

func errSkip(msg string) error {
	return &skipError{msg: msg}
}

func errSkipf(format string, args ...any) error {
	return &skipError{msg: fmt.Sprintf(format, args...)}
}
