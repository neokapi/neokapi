//go:build !js

package host

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A translation identical to its source fails `kapi check SOURCE --target
// TARGET` and check_file as it fails the ship gate, so a one-off check of an
// untranslated file does not pass it.
func TestCheckTarget_AnIdenticalTargetFailsAsTheShipGateDoes(t *testing.T) {
	isolateCheckExecution(t)
	dir := t.TempDir()
	src := filepath.Join(dir, "en.json")
	tgt := filepath.Join(dir, "nb.json")
	require.NoError(t, os.WriteFile(src, []byte(`{"title":"Book a video appointment","help":"Read the guide"}`), 0o644))
	require.NoError(t, os.WriteFile(tgt, []byte(`{"title":"Book a video appointment","help":"Les veiledningen"}`), 0o644))

	cmd := executionCommand(t)
	cmd.Flags().String("target", tgt, "")
	cmd.Flags().String("target-lang", "nb", "")
	report, err := (&App{SourceLang: "en"}).ComputeCheck(cmd, []string{src})
	require.NoError(t, err)
	_, mcp, err := (&App{SourceLang: "en"}).checkFileMCP(t.Context(), checkFileInput{File: src, Target: tgt, TargetLang: "nb"})
	require.NoError(t, err)
	for name, r := range map[string][]string{"cli": rulesOf(&report), "mcp": rulesOf(&mcp)} {
		assert.Equal(t, []string{"qa.target-same-as-source"}, r, "%s: the untranslated block, and only it", name)
	}
	assert.False(t, report.Pass)
	assert.Equal(t, 1, report.Summary.Failing)
	assert.Equal(t, "title", report.Findings[0].Location.Block)
}
