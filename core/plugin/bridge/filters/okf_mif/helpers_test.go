//go:build integration

package okf_mif

import (
	"os"
	"strings"
	"testing"

	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/core/plugin/bridge/filters/bridgetest"
	"github.com/stretchr/testify/require"
)

const filterClass = "net.sf.okapi.filters.mif.MIFFilter"
const mimeType = "application/vnd.mif"

// readMIF reads a MIF file from testdata and returns parts.
func readMIF(t *testing.T, relPath string, params map[string]any) []*model.Part {
	t.Helper()
	pool, cfg := bridgetest.SharedBridge(t)
	path := bridgetest.TestdataFile(t, "okf_mif/"+relPath)
	return bridgetest.ReadFile(t, pool, cfg, filterClass, path, mimeType, params)
}

// readMIFDefault reads a MIF file from testdata with default (nil) params.
func readMIFDefault(t *testing.T, relPath string) []*model.Part {
	t.Helper()
	return readMIF(t, relPath, nil)
}

// readMIFWithConfig reads a MIF file using a named .fprm config from testdata.
func readMIFWithConfig(t *testing.T, relPath string, configName string) []*model.Part {
	t.Helper()
	tdDir := bridgetest.TestdataDir(t)
	params := map[string]any{
		"configFile": tdDir + "/okf_mif/" + configName,
	}
	return readMIF(t, relPath, params)
}

// readMIFContent reads a MIF file from testdata and returns the raw bytes.
func readMIFContent(t *testing.T, relPath string) []byte {
	t.Helper()
	path := bridgetest.TestdataFile(t, "okf_mif/"+relPath)
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	return content
}

// configParams returns filter params with the given .fprm config file.
func configParams(t *testing.T, configName string) map[string]any {
	t.Helper()
	tdDir := bridgetest.TestdataDir(t)
	return map[string]any{
		"configFile": tdDir + "/okf_mif/" + configName,
	}
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

// mifTestFiles returns all .mif files in the okf_mif testdata directory.
func mifTestFiles(t *testing.T) []string {
	t.Helper()
	tdDir := bridgetest.TestdataDir(t)
	entries, err := os.ReadDir(tdDir + "/okf_mif")
	require.NoError(t, err)
	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".mif") {
			files = append(files, e.Name())
		}
	}
	return files
}
