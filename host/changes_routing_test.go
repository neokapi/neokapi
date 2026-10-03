package host

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/model"
)

// qtCatalog is a Qt Linguist catalog: a .ts file, the extension TypeScript
// uses too, found by its content.
const qtCatalog = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE TS>
<TS version="2.1" language="fr" sourcelanguage="en">
<context>
    <name>MainWindow</name>
    <message>
        <source>Hello</source>
        <translation>Bonjour</translation>
    </message>
</context>
</TS>
`

// A Qt Linguist catalog shares TypeScript's .ts extension. kapi inspect and
// kapi apply read it as the catalog its content says it is, never as a
// TypeScript source file's comments, so the change set ksed prints for it is
// one kapi apply applies.
func TestQtLinguistCatalogIsADocumentNotTypeScript(t *testing.T) {
	noProject(t)
	t.Chdir(t.TempDir())
	require.NoError(t, os.WriteFile("app_fr.ts", []byte(qtCatalog), 0o644))
	require.NoError(t, os.WriteFile("code.ts", []byte("// Greets the user.\nexport const x = 1;\n"), 0o644))
	app := newToolboxApp(t)

	comments := app.newCommentDocs("")
	assert.False(t, comments.is("app_fr.ts"), "a Qt Linguist catalog is a document")
	assert.True(t, comments.is("code.ts"), "a TypeScript file is read for its comments")

	recs := inspectJSONL(t, app, "app_fr.ts")
	rec := recordOf(t, recs, "MainWindow/Hello")
	assert.Equal(t, "Hello", rec.Text)

	prog, err := ParseSedProgram([]string{"s/Bonjour/Salut/"})
	require.NoError(t, err)
	printed, err := captureStdout(t, func() error {
		return app.RunSed(context.Background(), sedCommand(), []string{"app_fr.ts"}, prog, SedOptions{PrintOps: true, Target: "fr"})
	})
	require.NoError(t, err)
	res, err := applyJSON(t, app, NewEnvCommand(t.Context(), "apply"), printed, ApplyOptions{})
	require.NoError(t, err)
	assert.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
	assert.Contains(t, fileText(t, "app_fr.ts"), "<translation>Salut</translation>")
}

// A file is read for its comments only when no format reads it: not when
// --format names one, and not when the project binds the file to one.
func TestCommentDocsYieldToAFormat(t *testing.T) {
	root := t.TempDir()
	recipe := filepath.Join(root, "kapi.yaml")
	require.NoError(t, os.WriteFile(recipe, []byte(`version: v1
name: scripts
defaults:
  source_language: en
collections:
  - name: scripts
    source_only: true
    content:
      - path: "bin/*.sh"
        format: plaintext
  - name: code
    source_only: true
    content:
      - path: "src/*.go"
        comments: true
`), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "bin"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "src"), 0o755))
	script := filepath.Join(root, "bin", "run.sh")
	require.NoError(t, os.WriteFile(script, []byte("#!/bin/sh\n# Runs the tool.\necho hi\n"), 0o644))
	source := filepath.Join(root, "src", "parse.go")
	require.NoError(t, os.WriteFile(source, []byte(repairGo), 0o644))

	app := newToolboxApp(t)
	assert.True(t, app.newCommentDocs("").is(script), "outside the project, a shell script is read for its comments")
	in := app.newCommentDocs(recipe)
	assert.False(t, in.is(script), "the project binds the script to plain text")
	assert.True(t, in.is(source), "a file declared for its comments alone is read for them")

	app.FormatFlag = "plaintext"
	assert.False(t, app.newCommentDocs("").is(source), "--format names the format that reads the file")
	set, err := change.Decode(strings.NewReader(changeSetOf(t, map[string]any{"op": "set_content", "at": map[string]any{"doc": source, "block": "func/Parse"}, "if_match": "*", "text": "x"})))
	require.NoError(t, err)
	comments, err := app.commentSet(set, root, app.newCommentDocs(""))
	require.NoError(t, err)
	assert.False(t, comments, "kapi apply -f routes the change set to the format")
}

// A change set that rewrites code comments holds only their set_content
// operations: one that also writes a term is refused by naming the term, with
// nothing written.
func TestApplyRefusesCommentsBesideAStoreOperation(t *testing.T) {
	isolateCheckExecution(t)
	noProject(t)
	t.Chdir(t.TempDir())
	require.NoError(t, os.WriteFile("parse.go", []byte(repairGo), 0o644))
	body := changeSetOf(t,
		map[string]any{"op": "set_content", "at": map[string]any{"doc": "parse.go", "block": "func/Parse"}, "if_match": "*", "text": repairedParse},
		map[string]any{"op": "term", "action": "upsert", "term": "reader", "status": "preferred"})
	_, _, err := runApply(t, newToolboxApp(t), NewEnvCommand(t.Context(), "apply"), body, ApplyOptions{})
	require.Error(t, err)
	assert.Equal(t, ExitUsage, ExitCode(nil, err))
	assert.Contains(t, err.Error(), "term operation")
	assert.NotContains(t, err.Error(), "documents", "a term is not a document")
	assertUnchanged(t, "parse.go", repairGo)
}

// The check of a written code comment holds kapi apply to it: a check that
// read nothing exits 3 under the enforcing gate, and --gate report lands the
// comment and exits 0 with the finding reported.
func TestApplyHoldsAWrittenCommentToItsCheck(t *testing.T) {
	for name, tc := range map[string]struct {
		opts ApplyOptions
		exit int
	}{
		"enforce": {ApplyOptions{}, ExitGate},
		"report":  {ApplyOptions{Gate: change.GateReport}, 0},
	} {
		t.Run(name, func(t *testing.T) {
			root := commentProjectFixture(t)
			file := writeCheckInput(t, root, "undeclared.go", repairGo)
			cmd := NewEnvCommand(t.Context(), "apply")
			AddProjectFlag(cmd)
			require.NoError(t, cmd.Flags().Set(projectFlagName, filepath.Join(root, "kapi.yaml")))
			body := changeSetOf(t, map[string]any{"op": "set_content", "at": map[string]any{"doc": "undeclared.go", "block": "func/Parse"}, "if_match": "*", "text": repairedParse})
			_, stderr, err := runApply(t, &App{SourceLang: "en"}, cmd, body, tc.opts)
			assert.Equal(t, tc.exit, ExitCode(nil, err), stderr)
			assert.Contains(t, stderr, "fails check.did-not-run", "the finding is reported")
			assert.Contains(t, fileText(t, file), "// Parse reads the input from an [io.Reader].", "the comment was written")
		})
	}
}

// Inside a project, kapi inspect names a file outside the project by its
// absolute path, and kapi apply in the project edits it through that path. A
// change set that edits such a file and a document of the project is refused.
func TestInspectAndApplyAgreeOnAFileOutsideTheProject(t *testing.T) {
	root := sedProject(t)
	outside := filepath.Join(t.TempDir(), "o.json")
	require.NoError(t, os.WriteFile(outside, []byte(`{"a": "Hello"}`), 0o644))
	app := newToolboxApp(t)

	rec := recordOf(t, inspectJSONL(t, app, outside), "a")
	assert.Equal(t, filepath.ToSlash(outside), rec.Ref.Doc)
	res, err := applyJSON(t, app, NewEnvCommand(t.Context(), "apply"),
		changeSetOf(t, map[string]any{"op": "set_content", "at": rec.Ref, "if_match": rec.Rev, "text": "Hi"}), ApplyOptions{})
	require.NoError(t, err)
	assert.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
	assert.JSONEq(t, `{"a": "Hi"}`, fileText(t, outside))

	guide := recordOf(t, inspectJSONL(t, app, "docs/sub/guide.md"), "guide/p")
	rec = recordOf(t, inspectJSONL(t, app, outside), "a")
	_, _, err = runApply(t, app, NewEnvCommand(t.Context(), "apply"), changeSetOf(t,
		map[string]any{"op": "set_content", "at": rec.Ref, "if_match": rec.Rev, "text": "Hey"},
		map[string]any{"op": "set_content", "at": guide.Ref, "if_match": guide.Rev, "text": "Visit us."}), ApplyOptions{})
	require.Error(t, err)
	assert.Equal(t, ExitUsage, ExitCode(nil, err))
	assert.Contains(t, err.Error(), "outside the project")
	res, err = applyJSON(t, app, NewEnvCommand(t.Context(), "apply"), changeSetOf(t,
		map[string]any{"op": "set_content", "at": rec.Ref, "if_match": rec.Rev, "text": "Hey"},
		map[string]any{"op": "set_content", "at": guide.Ref, "if_match": guide.Rev, "text": "Visit us."}), ApplyOptions{})
	assert.Equal(t, ExitUsage, ExitCode(nil, err))
	assert.Equal(t, change.SetRefused, res.Status, "--json answers with the refused result")
	require.NotNil(t, res.Error)
	assert.Contains(t, res.Error.Message, "outside the project")
	assert.JSONEq(t, `{"a": "Hi"}`, fileText(t, outside))
	assert.Equal(t, "# Guide\n\nVisit the shop today.\n", fileText(t, filepath.Join(root, "docs", "sub", "guide.md")))

	// With the project named by KAPI_PROJECT from a directory outside it, a
	// file in that directory is named by its absolute path too: a path from
	// the working directory would read as a document of the project.
	elsewhere := t.TempDir()
	t.Setenv("KAPI_PROJECT", filepath.Join(root, "kapi.yaml"))
	t.Chdir(elsewhere)
	require.NoError(t, os.WriteFile("guide.json", []byte(`{"a": "Hello"}`), 0o644))
	rec = recordOf(t, inspectJSONL(t, app, "guide.json"), "a")
	assert.Equal(t, filepath.ToSlash(filepath.Join(elsewhere, "guide.json")), rec.Ref.Doc)
	res, err = applyJSON(t, app, NewEnvCommand(t.Context(), "apply"),
		changeSetOf(t, map[string]any{"op": "set_content", "at": rec.Ref, "if_match": rec.Rev, "text": "Hi"}), ApplyOptions{})
	require.NoError(t, err)
	assert.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
	assert.JSONEq(t, `{"a": "Hi"}`, fileText(t, "guide.json"))
}

// kapi inspect -p reads a file as a document of the project it names, even
// with discovery off: by its project-relative path and with the format the
// recipe binds, as kapi apply -p reads it.
func TestInspectReadsTheProjectItNames(t *testing.T) {
	root := sedProject(t)
	noProject(t)
	t.Chdir(t.TempDir())
	app := newToolboxApp(t)
	cmd := NewEnvCommand(t.Context(), "inspect")
	AddProjectFlag(cmd)
	require.NoError(t, cmd.Flags().Set(projectFlagName, filepath.Join(root, "kapi.yaml")))
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	require.NoError(t, app.RunInspect(t.Context(), cmd, []string{filepath.Join(root, "notes", "a.txt")}, "jsonl", nil), stderr.String())
	var refs []change.Ref
	for line := range strings.SplitSeq(strings.TrimSpace(stdout.String()), "\n") {
		var rec readRecord
		require.NoError(t, json.Unmarshal([]byte(line), &rec), line)
		refs = append(refs, rec.Ref)
	}
	assert.Contains(t, refs, change.Ref{Doc: "notes/a.txt", Block: "note/p", Edition: model.EditionKey{Locale: "en"}},
		"the Markdown paragraph the recipe's format reads, in the recipe's source language")
}

// The hooks that resolve a project from a command resolve none for a service
// outside a project, even where discovery from the working directory would
// find one; a project's service gets the command that names it.
func TestChangeHookCommandResolvesNoProjectOutsideOne(t *testing.T) {
	root := sedProject(t)
	recipe, err := ResolveProjectPath(NewEnvCommand(t.Context(), "probe"))
	require.NoError(t, err)
	require.Equal(t, filepath.Join(root, "kapi.yaml"), recipe, "discovery finds the project here")

	got, err := ResolveProjectPath(changeHookCommand(t.Context(), NewEnvCommand(t.Context(), "ksed"), ""))
	require.NoError(t, err)
	assert.Empty(t, got, "an edit outside a project is held to no project's hooks")

	t.Setenv("KAPI_PROJECT", recipe)
	got, err = ResolveProjectPath(changeHookCommand(t.Context(), nil, ""))
	require.NoError(t, err)
	assert.Empty(t, got, "nor to the one the environment names")

	cmd := NewEnvCommand(t.Context(), "apply")
	assert.Same(t, cmd, changeHookCommand(t.Context(), cmd, recipe).(*EnvCommand))
	got, err = ResolveProjectPath(changeHookCommand(t.Context(), nil, recipe))
	require.NoError(t, err)
	assert.Equal(t, recipe, got)
}

// kapi apply refuses a file no format reads, where the toolbox would read it
// as plain text.
func TestApplyRefusesAFileNoFormatReads(t *testing.T) {
	noProject(t)
	t.Chdir(t.TempDir())
	require.NoError(t, os.WriteFile("NOTES", []byte("The colour.\n"), 0o644))
	res, err := applyJSON(t, newToolboxApp(t), NewEnvCommand(t.Context(), "apply"),
		changeSetOf(t, map[string]any{"op": "replace_text", "at": map[string]any{"doc": "NOTES", "block": "tu1"}, "if_match": "*", "edits": []map[string]any{{"find": "colour", "text": "color"}}}), ApplyOptions{})
	assert.Equal(t, ExitGate, ExitCode(nil, err))
	require.NotNil(t, res.Ops[0].Error)
	assert.Equal(t, change.CodeUnsupported, res.Ops[0].Error.Code)
	assert.Contains(t, res.Ops[0].Error.Message, "no format reads NOTES")
	assert.Equal(t, "The colour.\n", fileText(t, "NOTES"))
}
