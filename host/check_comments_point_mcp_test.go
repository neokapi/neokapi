//go:build !js

package host

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// MCP check_file holds each block of the file it names to its own point.
func TestMCPCheckFileGovernsCommentsAtTheirOwnPoint(t *testing.T) {
	t.Run("comments placed apart are held to their own point", func(t *testing.T) {
		report := checkFileOverMCP(t, commentPointProject(t, onItem), "config/app.yaml")
		assert.Equal(t, 4, report.Target.Blocks)
		assert.ElementsMatch(t, inFile("app.yaml", commentsApart), governedFindings(report.Findings))
		assertPoints(t, report.Findings, true)

		report = checkFileOverMCP(t, commentPointProject(t, onItem), "code/parse.go")
		assert.Equal(t, 2, report.Target.Blocks)
		assert.ElementsMatch(t, inFile("parse.go", commentsApart), governedFindings(report.Findings))
		assertPoints(t, report.Findings, true)
	})

	t.Run("must fail: comments with no point of their own sit at their file's point", func(t *testing.T) {
		report := checkFileOverMCP(t, commentPointProject(t, atFilePoint), "config/app.yaml")
		assert.Equal(t, 4, report.Target.Blocks)
		assert.ElementsMatch(t, inFile("app.yaml", commentsTogether), governedFindings(report.Findings))
		assertPoints(t, report.Findings, false)
	})
}
