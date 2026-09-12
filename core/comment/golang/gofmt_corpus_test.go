package golang

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Every tracked Go file in the repository is gofmt-clean (scripts/check-gofmt.sh
// gates it), so the formatter comparison must find nothing over the tracked
// set, and must line up every file it reads. A comparison that located nothing
// there because it could not pair the groups would report the same empty list,
// which is why ErrUnlocated is counted rather than tolerated.
func TestGofmtCorpusAgreesWithTrackedFiles(t *testing.T) {
	root := repoRoot(t)
	cmd := exec.CommandContext(t.Context(), "git", "ls-files", "-z", "*.go")
	cmd.Dir = root
	listing, err := cmd.Output()
	require.NoError(t, err)

	checked, withComments := 0, 0
	for _, rel := range strings.Split(strings.TrimRight(string(listing), "\x00"), "\x00") {
		if rel == "" || strings.HasPrefix(rel, ".claude/") {
			continue
		}
		path := filepath.Join(root, rel)
		src, err := os.ReadFile(path)
		require.NoError(t, err)
		if _, perr := groupsOf(path, src); perr != nil {
			continue
		}
		got, err := Provider{}.Locate(path, src)
		require.NoError(t, err, rel)
		ds, err := Provider{}.Disagreements(path, src, got)
		if errors.Is(err, ErrUnlocated) {
			t.Errorf("%s: %v", rel, err)
			continue
		}
		require.NoError(t, err, rel)
		assert.Empty(t, ds, "%s is gofmt-clean, so no comment in it disagrees", rel)
		checked++
		if len(got.Comments) > 0 {
			withComments++
		}
	}
	t.Logf("gofmt-corpus files=%d with-comments=%d", checked, withComments)
	assert.GreaterOrEqual(t, checked, corpusMinFiles, "the formatter comparison read too few files")
}
