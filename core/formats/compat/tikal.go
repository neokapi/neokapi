//go:build integration

package compat

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// tikalRoundTrip performs an identity roundtrip using Okapi's tikal CLI:
// extract to XLIFF (tikal -x) then merge back (tikal -m).
func tikalRoundTrip(t *testing.T, tikalPath string, input []byte, filename string) []byte {
	t.Helper()

	tmpDir := t.TempDir()
	inputPath := filepath.Join(tmpDir, filename)
	require.NoError(t, os.WriteFile(inputPath, input, 0o644))

	// Extract: produces <inputPath>.xlf alongside the input file.
	cmd := exec.Command(tikalPath, "-x", inputPath, "-sl", "en", "-tl", "fr")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "tikal -x failed: %s", string(out))

	xliffPath := inputPath + ".xlf"
	require.FileExists(t, xliffPath, "tikal -x did not produce XLIFF")

	// Merge: reads the XLIFF and writes the output to -od directory.
	outputDir := filepath.Join(tmpDir, "output")
	require.NoError(t, os.MkdirAll(outputDir, 0o755))

	cmd = exec.Command(tikalPath, "-m", xliffPath, "-od", outputDir)
	out, err = cmd.CombinedOutput()
	require.NoError(t, err, "tikal -m failed: %s", string(out))

	outputPath := filepath.Join(outputDir, filename)
	result, err := os.ReadFile(outputPath)
	require.NoError(t, err, "reading tikal output: %s", outputPath)

	return result
}
