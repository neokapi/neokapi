//go:build integration

package archive

import (
	"os"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/plugin/bridge/filters/bridgetest"
	"github.com/stretchr/testify/require"
)

const filterClass = "net.sf.okapi.filters.archive.ArchiveFilter"
const mimeType = "application/zip"

// readArchiveFile reads an archive file from testdata/okapi/filters/archive/src/test/resources/ and returns the extracted parts.
func readArchiveFile(t *testing.T, relPath string, filterParams map[string]any) []*model.Part {
	t.Helper()
	pool, cfg := bridgetest.SharedBridge(t)
	path := bridgetest.TestdataFile(t, "okapi/filters/archive/src/test/resources/"+relPath)
	return bridgetest.ReadFile(t, pool, cfg, filterClass, path, mimeType, filterParams)
}

// roundtripArchive performs a roundtrip on an archive file and returns the result.
func roundtripArchive(t *testing.T, relPath string, filterParams map[string]any) bridgetest.RoundTripResult {
	t.Helper()
	pool, cfg := bridgetest.SharedBridge(t)
	path := bridgetest.TestdataFile(t, "okapi/filters/archive/src/test/resources/"+relPath)
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	return bridgetest.RoundTrip(t, pool, cfg, filterClass, content, path, mimeType, filterParams)
}

// allBlocks returns all blocks (translatable and non-translatable) from parts.
func allBlocks(parts []*model.Part) []*model.Block {
	return bridgetest.FilterBlocks(parts)
}

// findBlockContaining finds a block whose source text contains the given substring.
func findBlockContaining(blocks []*model.Block, substr string) *model.Block {
	for _, b := range blocks {
		if strings.Contains(b.SourceText(), substr) {
			return b
		}
	}
	return nil
}

// countPartsByType counts parts of a given type.
func countPartsByType(parts []*model.Part, pt model.PartType) int {
	n := 0
	for _, p := range parts {
		if p.Type == pt {
			n++
		}
	}
	return n
}

// xliffOnlyParams returns filter params that extract only XLIFF files from the archive.
// okapi: ArchiveFilterTest — fileNames="*.xlf", configIds="okf_xliff"
func xliffOnlyParams() map[string]any {
	return map[string]any{
		"fileNames": "*.xlf",
		"configIds": "okf_xliff",
	}
}

// tmxOnlyParams returns filter params that extract only TMX files from the archive.
// okapi: ArchiveFilterTest — fileNames="*.tmx", configIds="okf_tmx"
func tmxOnlyParams() map[string]any {
	return map[string]any{
		"fileNames": "*.tmx",
		"configIds": "okf_tmx",
	}
}

// xliffAndTMXParams returns filter params that extract both XLIFF and TMX files.
// okapi: ArchiveFilterTest — combined fileNames/configIds
func xliffAndTMXParams() map[string]any {
	return map[string]any{
		"fileNames": "*.xlf,*.tmx",
		"configIds": "okf_xliff,okf_tmx",
	}
}
