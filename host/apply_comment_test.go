package host

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/comment"
	"github.com/neokapi/neokapi/core/comment/golang"
	fmtpkg "github.com/neokapi/neokapi/core/format"
)

// repairGo holds a doubled word in the doc comment of Parse, on lines 5-7, and
// an untouched comment on line 10.
const repairGo = "package demo\n\nimport \"io\"\n\n// Parse reads the the input from an [io.Reader].\n//\n// It stops at the end.\nfunc Parse(r io.Reader) {}\n\n// Other is untouched.\nfunc Other() {}\n"

const repairedParse = "Parse reads the input from an [io.Reader].\n\nIt stops at the end."

// commentRun is what kapi apply's comment branch did with a change set: the
// result it reports, and the comment path's own outcome in each file, which
// says why an edit did not land in more detail than the result's codes.
type commentRun struct {
	Result   *change.Result
	Comments []commentFileResult
}

// runApplyChangeSet runs kapi apply's comment branch over entries, as a
// person at a command line whose change set is in a file. With diff set the
// change set is a dry run. See runCommentEntries.
func runApplyChangeSet(t *testing.T, cmd *EnvCommand, diff bool, entries ...map[string]any) (commentRun, string, error) {
	t.Helper()
	return runCommentEntries(t, &App{SourceLang: "en"}, cmd, diff, entries...)
}

// runCommentEntries runs the code kapi apply runs for a change set of code
// comments once it has read one: each entry becomes the set_content that
// rewrites its comment, guarded by the revision its comment_sha256 names
// ("*" when it names none), the change set is decoded as kapi apply decodes
// one and routed as kapi apply routes it, the comment branch applies it, and
// the error is the exit kapi apply returns. The account kapi apply prints,
// and each document's diff for a dry run, come back as the output.
func runCommentEntries(t *testing.T, a *App, cmd *EnvCommand, diff bool, entries ...map[string]any) (commentRun, string, error) {
	t.Helper()
	var ops []map[string]any
	for _, e := range entries {
		rev := change.AnyRevision
		if sum, _ := e["comment_sha256"].(string); sum != "" {
			rev = commentRevision(sum)
		}
		ops = append(ops, map[string]any{"op": "set_content", "at": map[string]any{"doc": e["file"], "block": e["id"]}, "if_match": rev, "text": e["text"]})
	}
	envelope := map[string]any{"ops": ops}
	if diff {
		envelope["mode"] = "preview"
	}
	body, err := json.Marshal(envelope)
	require.NoError(t, err)
	set, err := change.Decode(bytes.NewReader(body))
	require.NoError(t, err)
	wd, err := os.Getwd()
	require.NoError(t, err)
	a.InitRegistries()
	comments, err := a.commentSet(set, wd, a.newCommentDocs(""))
	require.NoError(t, err)
	require.True(t, comments, "kapi apply routes the change set to its comment branch")

	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	res, outcomes, err := a.applyCommentChange(cmd, set, wd, "", false)
	require.NoError(t, err)
	for _, d := range res.Docs {
		fmt.Fprint(&stdout, d.Diff)
	}
	printChangeResult(&stderr, res)
	return commentRun{Result: res, Comments: outcomes}, stdout.String() + stderr.String(), commentSetExit(set, res)
}

// refusalOf returns the refusal of the one operation run refused.
func refusalOf(t *testing.T, run commentRun) *change.Error {
	t.Helper()
	for _, op := range run.Result.Ops {
		if op.Status == change.OpRefused {
			require.NotNil(t, op.Error)
			return op.Error
		}
	}
	require.Failf(t, "no refusal", "%+v", run.Result.Ops)
	return nil
}

// staleFingerprint is a fingerprint no comment has.
var staleFingerprint = strings.Repeat("0", 64)

// commentEntry builds a comment entry guarded by the fingerprint the comment id
// has in the file now. A file that holds no such comment gets staleFingerprint,
// which no rewrite reads before the id resolves.
func commentEntry(file, id string, lines *fmtpkg.LineRange, text string) map[string]any {
	e := map[string]any{"kind": "comment", "file": file, "id": id, "text": text, "comment_sha256": staleFingerprint}
	if lines != nil {
		e["lines"] = lines
	}
	src, err := os.ReadFile(file)
	if err != nil {
		return e
	}
	located, err := golang.Provider{}.Locate(file, src)
	if err != nil {
		return e
	}
	for i, b := range located.Blocks() {
		if b.ID == id {
			e["comment_sha256"] = comment.Fingerprint(src, located.Comments[i])
		}
	}
	return e
}

// findingOf returns the one finding a report holds under rule.
func findingOf(t *testing.T, report check.Report, rule string) check.Diagnostic {
	t.Helper()
	var found []check.Diagnostic
	for _, d := range report.Findings {
		if d.Rule == rule {
			found = append(found, d)
		}
	}
	require.Len(t, found, 1, "%+v", report.Findings)
	return found[0]
}

func rulesOf(report *check.Report) []string {
	var rules []string
	for _, d := range report.Findings {
		rules = append(rules, d.Rule)
	}
	return rules
}

// TestProseP3_go is the P3 rung for Go comments through the product. A finding
// kapi check reports is repaired through `kapi apply`, the bytes on disk
// outside the comment stay identical, and gofmt agrees. The check
// that comes back with the edit clears the finding, and every refusal writes
// nothing. A write canary runs before any comment is written.
//
// The subtests named "must fail" break the write path on purpose and assert
// that the run notices. Nothing in here skips.
func TestProseP3_go(t *testing.T) {
	t.Run("a finding is repaired through kapi apply and the check of the change clears it", func(t *testing.T) {
		isolateCheckExecution(t)
		file := writeCheckInput(t, t.TempDir(), "parse.go", repairGo)
		before, err := (&App{SourceLang: "en"}).ComputeCheck(executionCommand(t), []string{file})
		require.NoError(t, err)
		finding := findingOf(t, before, "hygiene.doubled-word")
		require.Equal(t, "func/Parse", finding.Location.Block)
		require.Equal(t, &fmtpkg.LineRange{First: 5, Last: 7}, finding.Location.Lines)

		out, _, err := runApplyChangeSet(t, NewEnvCommand(t.Context(), "apply"), false,
			commentEntry(file, finding.Location.Block, finding.Location.Lines, repairedParse))
		require.NoError(t, err)
		require.Len(t, out.Comments, 1)
		edit := out.Comments[0].Edits[0]
		assert.Equal(t, commentWritten, edit.Status, edit.Detail)
		assert.Equal(t, &fmtpkg.LineRange{First: 5, Last: 7}, edit.Lines)

		result := out.Comments[0].Check
		require.NotNil(t, result, out.Comments[0].CheckError)
		assert.Equal(t, check.VerdictPassed, result.Verdict, result.DidNotRun)
		assert.NotContains(t, rulesOf(result), "hygiene.doubled-word", "the repair cleared the finding")
		require.NotNil(t, result.Scope)
		require.Len(t, result.Scope.Files, 1)
		assert.Equal(t, check.ScopeChecked, result.Scope.Files[0].Status)
		assert.Equal(t, []check.ScopeBlock{{Block: "func/Parse", Lines: fmtpkg.LineRange{First: 5, Last: 7}}}, result.Scope.Files[0].Blocks,
			"the check reads the comment the edit wrote and nothing else")

		after, err := os.ReadFile(file)
		require.NoError(t, err)
		located, err := golang.Provider{}.Locate(file, []byte(repairGo))
		require.NoError(t, err)
		c := located.Comments[0]
		end := c.End + len(after) - len(repairGo)
		assert.Equal(t, repairGo[:c.Start], string(after[:c.Start]), "the bytes before the comment")
		assert.Equal(t, repairGo[c.End:], string(after[end:]), "the bytes after the comment")
		assert.Equal(t, "// Parse reads the input from an [io.Reader].\n//\n// It stops at the end.", string(after[c.Start:end]))
		formatted, err := format.Source(after)
		require.NoError(t, err)
		assert.Equal(t, string(after), string(formatted), "gofmt agrees with the written file")
	})

	t.Run("a voice finding at the project's point is repaired, and the check of the change is held to it", func(t *testing.T) {
		root := commentProjectFixture(t)
		recipe := filepath.Join(root, "kapi.yaml")
		file := filepath.Join(root, "src", "parse.go")
		checkCmd := executionCommand(t)
		checkCmd.Flags().String(projectFlagName, recipe, "")
		before, err := (&App{SourceLang: "en"}).ComputeCheck(checkCmd, []string{file})
		require.NoError(t, err)
		voice := findingsOf(before, "voice")
		require.Len(t, voice, 1, "%+v", before.Findings)
		finding := voice[0]

		cmd := NewEnvCommand(t.Context(), "apply")
		cmd.Flags().String(projectFlagName, recipe, "")
		out, _, err := runApplyChangeSet(t, cmd, false, commentEntry(file, finding.Location.Block, finding.Location.Lines, "Parse helps you use the input."))
		require.NoError(t, err)
		require.NotNil(t, out.Comments[0].Check, out.Comments[0].CheckError)
		assert.Equal(t, check.VerdictPassed, out.Comments[0].Check.Verdict, out.Comments[0].Check.DidNotRun)
		assert.Empty(t, findingsOf(*out.Comments[0].Check, "voice"), "the repair cleared the voice finding")
		after, err := os.ReadFile(file)
		require.NoError(t, err)
		assert.Equal(t, "package demo\n\n// Parse helps you use the input.\nfunc Parse() string { return \"utilize\" }\n", string(after),
			"the string literal is code and stays as it is")
	})

	t.Run("a refused edit writes nothing and exits on the gate code", func(t *testing.T) {
		isolateCheckExecution(t)
		file := writeCheckInput(t, t.TempDir(), "parse.go", repairGo)
		out, _, err := runApplyChangeSet(t, NewEnvCommand(t.Context(), "apply"), false,
			commentEntry(file, "func/Parse", nil, "Parse reads the input.\n\nIt stops at the end."))
		require.Error(t, err)
		assert.Equal(t, ExitGate, ExitCode(nil, err))
		edit := out.Comments[0].Edits[0]
		assert.Equal(t, commentRefused, edit.Status)
		assert.Equal(t, string(comment.RefusedStructure), edit.Reason, edit.Detail)
		assert.Empty(t, out.Comments[0].Diff)
		assert.Nil(t, out.Comments[0].Check)
		assertUnchanged(t, file, repairGo)
	})

	t.Run("set-aside comments, changed comments and text a comment cannot hold are refused and write nothing", func(t *testing.T) {
		for _, tc := range []struct {
			name, file, src, id, text string
			// reason is why the comment path refused the edit, empty when the
			// change set was refused before the path ran.
			reason string
			code   change.Code
			stale  bool
		}{
			// A comment its provider sets aside is no block kapi edits, so a
			// reference to it names nothing.
			{"a directive", "d.go", "package demo\n\n//go:noinline\nfunc Parse() {}\n", "func/Parse", "Parse parses.", string(comment.RefusedUnknown), change.CodeNotFound, false},
			{"a generated file", "g.go", "// Code generated by gen. DO NOT EDIT.\n\npackage demo\n\n// Parse parses.\nfunc Parse() {}\n", "func/Parse", "Parse reads.", string(comment.RefusedGenerated), change.CodeUnsupported, false},
			{"the cgo preamble", "c.go", "package demo\n\n// #include <stdio.h>\nimport \"C\"\n", "import/C", "Nothing.", string(comment.RefusedUnknown), change.CodeNotFound, false},
			{"an example's output", "x_test.go", "package demo\n\nimport \"fmt\"\n\nfunc ExampleParse() {\n\tfmt.Println(1)\n\t// Output: 1\n}\n", "func/ExampleParse/comment", "Output: 2", string(comment.RefusedUnknown), change.CodeNotFound, false},
			{"a line comment and a delimited comment in one group", "b.go", "package demo\n\n// Parse parses.\n/* More. */\nfunc Parse() {}\n", "func/Parse", "Parse reads.", string(comment.RefusedLayout), change.CodeGuard, false},
			{"text holding the closer of a delimited comment", "t.go", "package demo\n\n/* Parse parses. */\nfunc Parse() {}\n", "func/Parse", "Parse ends */ here.", string(comment.RefusedTerminator), change.CodeGuard, false},
			{"a comment that changed since it was checked", "s.go", repairGo, "func/Parse", repairedParse, "", change.CodeStale, true},
			{"text that forms a directive", "n.go", repairGo, "func/Other", "nolint", string(comment.RefusedDirective), change.CodeUnsupported, false},
		} {
			t.Run(tc.name, func(t *testing.T) {
				isolateCheckExecution(t)
				file := writeCheckInput(t, t.TempDir(), tc.file, tc.src)
				entry := commentEntry(file, tc.id, nil, tc.text)
				if tc.stale {
					entry["comment_sha256"] = staleFingerprint
				}
				out, _, err := runApplyChangeSet(t, NewEnvCommand(t.Context(), "apply"), false, entry)
				assert.Equal(t, ExitGate, ExitCode(nil, err))
				assert.Equal(t, change.SetRefused, out.Result.Status)
				assert.Equal(t, tc.code, refusalOf(t, out).Code)
				if tc.reason != "" {
					edit := out.Comments[0].Edits[0]
					assert.Equal(t, commentRefused, edit.Status)
					assert.Equal(t, tc.reason, edit.Reason, edit.Detail)
				} else {
					assert.Empty(t, out.Comments, "the comment path never ran")
				}
				assertUnchanged(t, file, tc.src)
			})
		}
	})

	t.Run("two edits read from one check land from the last comment up", func(t *testing.T) {
		isolateCheckExecution(t)
		file := writeCheckInput(t, t.TempDir(), "parse.go", repairGo)
		out, _, err := runApplyChangeSet(t, NewEnvCommand(t.Context(), "apply"), false,
			commentEntry(file, "func/Parse", &fmtpkg.LineRange{First: 5, Last: 7}, repairedParse+"\n\nIt reports what it read."),
			commentEntry(file, "func/Other", &fmtpkg.LineRange{First: 10, Last: 10}, "Other is left alone."))
		require.NoError(t, err)
		edits := out.Comments[0].Edits
		assert.Equal(t, commentWritten, edits[0].Status, edits[0].Detail)
		assert.Equal(t, commentWritten, edits[1].Status, edits[1].Detail)
		assert.Equal(t, &fmtpkg.LineRange{First: 5, Last: 9}, edits[0].Lines)
		assert.Equal(t, &fmtpkg.LineRange{First: 12, Last: 12}, edits[1].Lines)
		after, err := os.ReadFile(file)
		require.NoError(t, err)
		assert.Contains(t, string(after), "// It reports what it read.\nfunc Parse(r io.Reader) {}\n\n// Other is left alone.\nfunc Other() {}\n")
	})

	t.Run("a second operation on one comment refuses the change set and writes nothing", func(t *testing.T) {
		isolateCheckExecution(t)
		file := writeCheckInput(t, t.TempDir(), "parse.go", repairGo)
		out, _, err := runApplyChangeSet(t, NewEnvCommand(t.Context(), "apply"), false,
			commentEntry(file, "func/Parse", nil, repairedParse),
			commentEntry(file, "func/Parse", nil, "Parse reads something else from an [io.Reader]."))
		assert.Equal(t, ExitUsage, ExitCode(nil, err), "a change set that contradicts itself")
		assert.Equal(t, change.CodeInvalid, refusalOf(t, out).Code)
		assertUnchanged(t, file, repairGo)
	})

	t.Run("--dry-run shows the edit and writes nothing", func(t *testing.T) {
		isolateCheckExecution(t)
		file := writeCheckInput(t, t.TempDir(), "parse.go", repairGo)
		_, output, err := runApplyChangeSet(t, NewEnvCommand(t.Context(), "apply"), true, commentEntry(file, "func/Parse", nil, repairedParse))
		require.NoError(t, err)
		assert.Contains(t, output, "-// Parse reads the the input from an [io.Reader].\n+// Parse reads the input from an [io.Reader].\n")
		assert.Contains(t, output, "change set previewed: 1 previewed")
		assertUnchanged(t, file, repairGo)
	})

	t.Run("a file another writer changes meanwhile is left as that writer left it", func(t *testing.T) {
		isolateCheckExecution(t)
		file := writeCheckInput(t, t.TempDir(), "parse.go", repairGo)
		const theirs = "package demo\n\n// Parse is someone else's edit.\nfunc Parse() {}\n"
		swapRewriteComment(t, func(p comment.Provider, name string, src []byte, declared comment.Directives, target comment.Target, text string, opts comment.RenderOptions) (*comment.Rewritten, error) {
			r, err := comment.Rewrite(p, name, src, declared, target, text, opts)
			if name == file {
				require.NoError(t, os.WriteFile(file, []byte(theirs), 0o644))
			}
			return r, err
		})
		out, _, err := runApplyChangeSet(t, NewEnvCommand(t.Context(), "apply"), false, commentEntry(file, "func/Parse", nil, repairedParse))
		assert.Equal(t, ExitGate, ExitCode(nil, err))
		assert.Equal(t, change.CodeStale, refusalOf(t, out).Code)
		// The dry run that checks the change set first finds the comment it
		// guards; the write that follows reads the other writer's file and
		// finds it changed.
		edit := out.Comments[0].Edits[0]
		assert.Equal(t, commentRefused, edit.Status)
		assert.Equal(t, string(comment.RefusedChanged), edit.Reason, edit.Detail)
		assertUnchanged(t, file, theirs)
	})

	t.Run("must fail: a rewrite that skips containment invalidates the run and writes nothing", func(t *testing.T) {
		isolateCheckExecution(t)
		file := writeCheckInput(t, t.TempDir(), "parse.go", repairGo)
		swapRewriteComment(t, uncontainedCommentRewrite)
		out, _, err := runApplyChangeSet(t, NewEnvCommand(t.Context(), "apply"), false, commentEntry(file, "func/Parse", nil, repairedParse))
		assert.Equal(t, ExitGate, ExitCode(nil, err))
		edit := out.Comments[0].Edits[0]
		assert.Equal(t, commentNotRun, edit.Status)
		assert.Equal(t, reasonCanary, edit.Reason)
		assert.Contains(t, edit.Detail, "must be refused")
		assertUnchanged(t, file, repairGo)
	})

	t.Run("must fail: a provider whose prose loses a byte invalidates the run", func(t *testing.T) {
		isolateCheckExecution(t)
		file := writeCheckInput(t, t.TempDir(), "parse.go", repairGo)
		swapCommentProviders(t, lossyGoProse{})
		out, _, err := runApplyChangeSet(t, NewEnvCommand(t.Context(), "apply"), false, commentEntry(file, "func/Parse", nil, repairedParse))
		assert.Equal(t, ExitGate, ExitCode(nil, err))
		assert.Equal(t, reasonCanary, out.Comments[0].Edits[0].Reason)
		assertUnchanged(t, file, repairGo)
	})

	t.Run("must fail: a written edit whose check did not run fails apply", func(t *testing.T) {
		root := commentProjectFixture(t)
		file := writeCheckInput(t, root, "undeclared.go", repairGo)
		cmd := NewEnvCommand(t.Context(), "apply")
		cmd.Flags().String(projectFlagName, filepath.Join(root, "kapi.yaml"), "")
		out, _, err := runApplyChangeSet(t, cmd, false, commentEntry(file, "func/Parse", nil, repairedParse))
		assert.Equal(t, ExitGate, ExitCode(nil, err), "a check that read nothing is never a pass")
		assert.Equal(t, commentWritten, out.Comments[0].Edits[0].Status)
		require.NotNil(t, out.Comments[0].Check)
		assert.Equal(t, check.VerdictDidNotRun, out.Comments[0].Check.Verdict)
	})
}

func TestApplyCommentEntryValidation(t *testing.T) {
	for name, tc := range map[string]struct {
		entry changeEntry
		want  string
	}{
		"no file":      {changeEntry{Kind: kindComment, ID: "func/Parse", Text: "x"}, `no "file"`},
		"no id":        {changeEntry{Kind: kindComment, File: "p.go", Text: "x"}, `no "id"`},
		"replacement":  {changeEntry{Kind: kindComment, File: "p.go", ID: "func/Parse", Replacement: "x"}, `"text"`},
		"content hash": {changeEntry{Kind: kindComment, File: "p.go", ID: "func/Parse", Text: "x", ContentHash: "h"}, `"comment_sha256"`},
		"no guard":     {changeEntry{Kind: kindComment, File: "p.go", ID: "func/Parse", Text: "x"}, `"current_text"`},
	} {
		err := validateChangeSet([]changeEntry{tc.entry})
		require.Error(t, err, name)
		assert.Contains(t, err.Error(), tc.want, name)
	}
	require.NoError(t, validateChangeSet([]changeEntry{{Kind: kindComment, File: "p.go", ID: "func/Parse", Text: "x", CommentSHA256: staleFingerprint}}))
}

func assertUnchanged(t *testing.T, file, want string) {
	t.Helper()
	got, err := os.ReadFile(file)
	require.NoError(t, err)
	assert.Equal(t, want, string(got), "the file is unchanged")
}

func swapRewriteComment(t *testing.T, f comment.RewriteFunc) {
	t.Helper()
	saved := rewriteComment
	rewriteComment = f
	t.Cleanup(func() { rewriteComment = saved })
}

// uncontainedCommentRewrite renders the text and splices it into the file
// without holding the result to comment.Contain.
func uncontainedCommentRewrite(p comment.Provider, name string, src []byte, declared comment.Directives, target comment.Target, text string, opts comment.RenderOptions) (*comment.Rewritten, error) {
	located, err := comment.Locate(p, name, src, declared)
	if err != nil {
		return nil, err
	}
	for i, b := range located.Blocks() {
		if b.ID != target.ID {
			continue
		}
		c := located.Comments[i]
		span, err := p.(comment.Rewriter).Render(name, src, c, text, opts)
		if err != nil {
			return nil, err
		}
		out := bytes.Join([][]byte{src[:c.Start], span, src[c.End:]}, nil)
		return &comment.Rewritten{Source: out, Index: i, ID: target.ID, Before: c, After: c, Changed: !bytes.Equal(out, src)}, nil
	}
	return nil, &comment.Refusal{Reason: comment.RefusedUnknown, Detail: target.ID}
}

// lossyGoProse reads a Go comment's prose without its last full stop.
type lossyGoProse struct{ golang.Provider }

func (p lossyGoProse) Prose(src []byte, c comment.Comment) (string, error) {
	prose, err := p.Provider.Prose(src, c)
	return strings.TrimSuffix(prose, "."), err
}
