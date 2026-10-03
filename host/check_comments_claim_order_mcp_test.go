//go:build !js

package host

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/check"
)

// A file one item claims for its values and a comments-only item claims for its
// comments is read for both, whichever item the recipe lists first. The values
// are held to the value item's point and the comments to the comments-only
// item's point.
func TestChecksReadTheValuesAndTheCommentsOfAFileClaimedApart(t *testing.T) {
	want := inFile("app.yaml", commentsApart)
	const patch = "--- a/config/app.yaml\n+++ b/config/app.yaml\n@@ -1,3 +1,3 @@\n-# Callers use this value.\n-greeting: Hello\n-# Confirm the farewell.\n" +
		"+# Callers utilize this value, and ScopedName is fine in a comment.\n+greeting: We utilize ScopedName and LegacyName. FIXME later\n+# FIXME: LegacyName is retired here.\n"

	for name, commentsFirst := range map[string]bool{"the comments-only item first": true, "the value item first": false} {
		t.Run(name, func(t *testing.T) {
			t.Run("a project check", func(t *testing.T) {
				report := checkProject(t, claimOrderProject(t, commentsFirst))
				assert.NotEqual(t, check.VerdictDidNotRun, report.Verdict, report.DidNotRun)
				assert.Equal(t, 4, report.Target.Blocks, "two values and two comments")
				assert.ElementsMatch(t, want, governedFindings(report.Findings))
				assertPoints(t, report.Findings, true)
				require.NotNil(t, report.Execution)
				assert.Equal(t, map[string][]string{"app.yaml": {"site/web", "source/comments"}}, ruleRuns(report.Execution.Analyzers))
			})

			for check, declared := range map[string]bool{"a named check": false, "a check of declared files": true} {
				t.Run(check, func(t *testing.T) {
					root := claimOrderProject(t, commentsFirst)
					cmd := executionCommand(t)
					cmd.Flags().String(projectFlagName, filepath.Join(root, "kapi.yaml"), "")
					compute := (&App{SourceLang: "en"}).ComputeCheck
					if declared {
						compute = (&App{SourceLang: "en"}).ComputeDeclaredCheck
					}
					report, err := compute(cmd, []string{filepath.Join(root, "config", "app.yaml")})
					require.NoError(t, err)
					assert.Equal(t, 4, report.Target.Blocks)
					assert.ElementsMatch(t, want, governedFindings(report.Findings))
					assertPoints(t, report.Findings, true)
				})
			}

			t.Run("MCP check_file", func(t *testing.T) {
				report := checkFileOverMCP(t, claimOrderProject(t, commentsFirst), "config/app.yaml")
				assert.Equal(t, 4, report.Target.Blocks)
				assert.ElementsMatch(t, want, governedFindings(report.Findings))
				assertPoints(t, report.Findings, true)
			})

			t.Run("a diff-scoped check", func(t *testing.T) {
				report := diffCheckProject(t, claimOrderProject(t, commentsFirst), patch)
				assert.Equal(t, 3, report.Target.Blocks, "the two comments and the value the change touches")
				assert.ElementsMatch(t, want, governedFindings(report.Findings))
				assertPoints(t, report.Findings, true)
			})

			for gates, named := range map[string]bool{"the ship gates": false, "the ship gates over the named file": true} {
				t.Run(gates, func(t *testing.T) {
					root := claimOrderProject(t, commentsFirst)
					var args []string
					if named {
						args = []string{filepath.Join(root, "config", "app.yaml")}
					}
					out, err := (&App{}).computeVerify(sourceShipCommand(t, root), args)
					require.NoError(t, err)
					for name, wantGate := range map[string][]governedFinding{
						gateTerms:  caughtOnly(want, "ScopedName", "LegacyName"),
						gateVoice:  want,
						gateChecks: want,
					} {
						gate, ok := gateByName(out, name)
						require.True(t, ok, "the %s gate ran", name)
						require.NotNil(t, gate.Coverage, name)
						var got []governedFinding
						for _, f := range gate.Findings {
							if f.Block == "" {
								continue
							}
							got = append(got, governedFinding{file: filepath.Base(f.File), block: blockKind(f.Block), caught: caughtWord(f.Message)})
							if assert.NotNil(t, f.Point, "%s %s %s", name, f.File, f.Block) {
								assert.Equal(t, wantPoint(f.Block, true), *f.Point, "%s %s %s", name, f.File, f.Block)
							}
						}
						assert.ElementsMatch(t, wantGate, got, name)
					}
				})
			}
		})
	}
}
