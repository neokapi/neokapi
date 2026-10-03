package host

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/comment"
	"github.com/neokapi/neokapi/core/comment/golang"
)

// A code comment is a block of its source file. kapi inspect lists it with the
// revision a set_content sends as if_match, which is "r:" and the first
// sixteen hex digits of the comment_sha256 kapi check reports; kapi apply
// rewrites it through the comment layer, every byte outside the comment kept.
func TestApplyRewritesACommentByItsRevision(t *testing.T) {
	isolateCheckExecution(t)
	noProject(t)
	t.Chdir(t.TempDir())
	require.NoError(t, os.WriteFile("parse.go", []byte(repairGo), 0o644))
	app := newToolboxApp(t)

	rec := recordOf(t, inspectJSONL(t, app, "parse.go"), "func/Parse")
	assert.Equal(t, change.Ref{Doc: "parse.go", Block: "func/Parse"}, rec.Ref)
	assert.Equal(t, "Parse reads the the input from an [io.Reader].\n\nIt stops at the end.", rec.Text)
	src := []byte(repairGo)
	located, err := golang.Provider{}.Locate("parse.go", src)
	require.NoError(t, err)
	assert.Equal(t, "r:"+comment.Fingerprint(src, located.Comments[0])[:16], rec.Rev)

	set := func(rev string) string {
		return changeSetOf(t, map[string]any{"op": "set_content", "at": rec.Ref, "if_match": rev, "text": repairedParse})
	}

	t.Run("--dry-run prints the diff and writes nothing", func(t *testing.T) {
		stdout, stderr, err := runApply(t, app, NewEnvCommand(t.Context(), "apply"), set(rec.Rev), ApplyOptions{DryRun: true})
		require.NoError(t, err, stderr)
		assert.Contains(t, stdout, "-// Parse reads the the input from an [io.Reader].\n+// Parse reads the input from an [io.Reader].\n")
		assertUnchanged(t, "parse.go", repairGo)
	})

	t.Run("a stale revision is refused with the comment as it stands", func(t *testing.T) {
		res, err := applyJSON(t, app, NewEnvCommand(t.Context(), "apply"), set("r:0000000000000000"), ApplyOptions{})
		assert.Equal(t, ExitGate, ExitCode(nil, err))
		require.NotNil(t, res.Ops[0].Error)
		assert.Equal(t, change.CodeStale, res.Ops[0].Error.Code)
		require.NotNil(t, res.Ops[0].Current)
		assert.Equal(t, rec.Rev, res.Ops[0].Current.Rev)
		assert.Equal(t, rec.Text, res.Ops[0].Current.Text)
		assertUnchanged(t, "parse.go", repairGo)
	})

	t.Run("the comment is rewritten and the result names its new revision", func(t *testing.T) {
		res, err := applyJSON(t, app, NewEnvCommand(t.Context(), "apply"), set(rec.Rev), ApplyOptions{})
		require.NoError(t, err)
		assert.Equal(t, change.SetApplied, res.Status)
		assert.Equal(t, change.OpApplied, res.Ops[0].Status)
		assert.Equal(t, rec.Rev, res.Ops[0].Before)
		after := recordOf(t, inspectJSONL(t, app, "parse.go"), "func/Parse")
		assert.Equal(t, after.Rev, res.Ops[0].After)
		require.Len(t, res.Docs, 1)
		assert.True(t, res.Docs[0].Written)
		got, err := os.ReadFile("parse.go")
		require.NoError(t, err)
		assert.Equal(t, strings.Replace(repairGo, "the the input", "the input", 1), string(got))

		res, err = applyJSON(t, app, NewEnvCommand(t.Context(), "apply"), set(rec.Rev), ApplyOptions{})
		assert.Equal(t, ExitGate, ExitCode(nil, err), "the revision it was read at no longer holds")
		assert.Equal(t, change.CodeStale, res.Ops[0].Error.Code)
	})
}

// A comment the comment layer will not rewrite is refused with the code its
// reason maps to, and nothing is written; a change set that edits a comment
// and a document is refused before anything is read.
func TestApplyRefusesACommentEditTheCommentLayerRefuses(t *testing.T) {
	isolateCheckExecution(t)
	noProject(t)
	t.Chdir(t.TempDir())
	require.NoError(t, os.WriteFile("parse.go", []byte(repairGo), 0o644))
	require.NoError(t, os.WriteFile("en.json", []byte(`{"a":"Hello"}`), 0o644))
	app := newToolboxApp(t)

	t.Run("a comment no reader names is not_found", func(t *testing.T) {
		body := changeSetOf(t, map[string]any{"op": "set_content", "at": map[string]any{"doc": "parse.go", "block": "func/Nope"}, "if_match": "*", "text": "x"})
		res, err := applyJSON(t, app, NewEnvCommand(t.Context(), "apply"), body, ApplyOptions{})
		assert.Equal(t, ExitGate, ExitCode(nil, err))
		require.NotNil(t, res.Ops[0].Error)
		assert.Equal(t, change.CodeNotFound, res.Ops[0].Error.Code)
		assertUnchanged(t, "parse.go", repairGo)
	})

	t.Run("a comment is rewritten whole, never by replace_text", func(t *testing.T) {
		rec := recordOf(t, inspectJSONL(t, app, "parse.go"), "func/Parse")
		body := changeSetOf(t, map[string]any{"op": "replace_text", "at": rec.Ref, "if_match": rec.Rev, "edits": []map[string]any{{"find": "the the", "text": "the"}}})
		res, err := applyJSON(t, app, NewEnvCommand(t.Context(), "apply"), body, ApplyOptions{})
		assert.Equal(t, ExitGate, ExitCode(nil, err))
		require.NotNil(t, res.Ops[0].Error)
		assert.Equal(t, change.CodeUnsupported, res.Ops[0].Error.Code)
		assertUnchanged(t, "parse.go", repairGo)
	})

	t.Run("comments and documents in one change set", func(t *testing.T) {
		body := changeSetOf(t,
			map[string]any{"op": "set_content", "at": map[string]any{"doc": "parse.go", "block": "func/Parse"}, "if_match": "*", "text": repairedParse},
			map[string]any{"op": "set_content", "at": map[string]any{"doc": "en.json", "block": "a"}, "if_match": "*", "text": "Hi"})
		_, _, err := runApply(t, app, NewEnvCommand(t.Context(), "apply"), body, ApplyOptions{})
		assert.Equal(t, ExitUsage, ExitCode(nil, err))
		assert.Contains(t, err.Error(), "code comments and documents")
		assertUnchanged(t, "parse.go", repairGo)

		// --json answers with the refused result on standard output.
		res, err := applyJSON(t, app, NewEnvCommand(t.Context(), "apply"), body, ApplyOptions{})
		assert.Equal(t, ExitUsage, ExitCode(nil, err))
		assert.Equal(t, change.SetRefused, res.Status)
		require.NotNil(t, res.Error)
		assert.Equal(t, change.CodeInvalid, res.Error.Code)
		assert.Contains(t, res.Error.Message, "code comments and documents")
		assertUnchanged(t, "parse.go", repairGo)
	})
}
