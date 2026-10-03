//go:build !js

package host

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A file declared for its comments alone is still content when a caller names
// it. A check of the named file reads its values at the point they resolve to,
// past the item that claims only the comments, and its comments at their own
// point. A check of the project reads only the comments.
func TestNamedCheckReadsTheContentOfAFileDeclaredForItsCommentsAlone(t *testing.T) {
	t.Run("kapi check", func(t *testing.T) {
		root := namedContentProject(t)
		cmd := executionCommand(t)
		cmd.Flags().String(projectFlagName, filepath.Join(root, "kapi.yaml"), "")
		report, err := (&App{SourceLang: "en"}).ComputeCheck(cmd, []string{filepath.Join(root, "config", "app.yaml"), filepath.Join(root, "code", "parse.go")})
		require.NoError(t, err)
		assert.Equal(t, 6, report.Target.Blocks, "the YAML file's two values and two comments, and the Go file's two comments")
		assert.ElementsMatch(t, namedContentFindings, governedFindings(report.Findings))
		assertNamedPoints(t, report.Findings)
	})

	t.Run("MCP check_file", func(t *testing.T) {
		report := checkFileOverMCP(t, namedContentProject(t), "config/app.yaml")
		assert.Equal(t, 4, report.Target.Blocks, "the file's two values and two comments")
		assert.ElementsMatch(t, inFile("app.yaml", namedContentFindings), governedFindings(report.Findings))
		assertNamedPoints(t, report.Findings)
	})

	t.Run("a check of the project reads only the comments", func(t *testing.T) {
		report := checkProject(t, namedContentProject(t))
		assert.Equal(t, 4, report.Target.Blocks, "two YAML comments and two Go comments")
		assert.ElementsMatch(t, commentsOnlyApart, governedFindings(report.Findings))
	})
}
