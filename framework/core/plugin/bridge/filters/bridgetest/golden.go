package bridgetest

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// CompareGolden compares actual output bytes against a golden file.
// If the GOKAPI_UPDATE_GOLDEN environment variable is set, the golden file
// is updated with the actual output instead of compared.
func CompareGolden(t *testing.T, goldenPath string, actual []byte) {
	t.Helper()

	if os.Getenv("GOKAPI_UPDATE_GOLDEN") != "" {
		require.NoError(t, os.MkdirAll(filepath.Dir(goldenPath), 0o755),
			"creating golden file directory")
		require.NoError(t, os.WriteFile(goldenPath, actual, 0o644),
			"updating golden file %s", goldenPath)
		t.Logf("updated golden file: %s", goldenPath)
		return
	}

	expected, err := os.ReadFile(goldenPath)
	require.NoError(t, err, "reading golden file %s", goldenPath)

	assert.Equal(t, string(expected), string(actual),
		"output does not match golden file %s\nRun with GOKAPI_UPDATE_GOLDEN=1 to update", goldenPath)
}
