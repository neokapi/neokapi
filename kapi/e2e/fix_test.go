//go:build e2e

package e2e

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// failingFindings runs `kapi check --json` on the guide in dir under the
// voice file beside it and counts the failing findings.
func failingFindings(t *testing.T, dir string) int {
	t.Helper()
	cmd := exec.Command(kapiBin, "check", "guide.md", "--profile-file", "voice.yaml", "--json")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), isoEnv...)
	out, err := cmd.Output()
	// A failing finding exits 3; the report is still printed.
	if ee, ok := err.(*exec.ExitError); !ok || ee.ExitCode() != 3 {
		require.NoError(t, err, "kapi check:\n%s", out)
	}
	var report struct {
		Summary struct {
			Failing int `json:"failing"`
		} `json:"summary"`
	}
	require.NoError(t, json.Unmarshal(out, &report), "%s", out)
	return report.Summary.Failing
}

// TestFix_CheckFixCheck drives the loop a person runs: kapi check finds the
// failing findings, kapi fix applies the ones with a replacement, and a
// second kapi check finds fewer. Every byte of the file outside the replaced
// words is as it was.
func TestFix_CheckFixCheck(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "voice.yaml"), []byte(`name: Fix
style:
  prohibited_patterns:
    - regex: '(?i)\bsimply\b'
      description: Say what to do without minimising it
      fails: true
terms:
  - term: utilize
    replacement: use
  - term: e-mail
    replacement: email
`), 0o644))
	guide := filepath.Join(dir, "guide.md")
	require.NoError(t, os.WriteFile(guide, []byte("# Guide\n\n"+
		"We **utilize** the [widget](https://example.com)  and utilize it.\n\n"+
		"* Simply send an e-mail.\n"), 0o644))

	before := failingFindings(t, dir)
	require.Equal(t, 4, before, "three words with a replacement and one pattern without")

	out := kapiIn(t, dir, "fix", "guide.md", "--profile-file", "voice.yaml")
	assert.Contains(t, out, "3 fixed in 1 file\n")
	assert.Contains(t, out, "kapi check: 1 failing finding remains (voice)\n")

	after := failingFindings(t, dir)
	assert.Less(t, after, before)
	assert.Equal(t, 1, after)

	got, err := os.ReadFile(guide)
	require.NoError(t, err)
	assert.Equal(t, "# Guide\n\n"+
		"We **use** the [widget](https://example.com)  and use it.\n\n"+
		"* Simply send an email.\n", string(got))
}
