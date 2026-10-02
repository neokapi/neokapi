package host

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
)

// runApply runs kapi apply as cmd over body, written to a change-set file,
// and returns what it wrote to standard output and standard error.
func runApply(t *testing.T, a *App, cmd *EnvCommand, body string, opts ApplyOptions) (string, string, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "change.json")
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	err := a.RunApply(cmd, path, opts)
	return stdout.String(), stderr.String(), err
}

// applyJSON runs kapi apply --json over body and decodes the result.
func applyJSON(t *testing.T, a *App, cmd *EnvCommand, body string, opts ApplyOptions) (change.Result, error) {
	t.Helper()
	opts.JSON = true
	stdout, stderr, err := runApply(t, a, cmd, body, opts)
	var res change.Result
	require.NoError(t, json.Unmarshal([]byte(stdout), &res), "stdout: %s\nstderr: %s", stdout, stderr)
	return res, err
}

// noProject keeps a test's kapi from finding a recipe above the working
// directory.
func noProject(t *testing.T) {
	t.Helper()
	t.Setenv("KAPI_NO_PROJECT", "1")
}

// changeSetOf is a change set holding ops, as JSON.
func changeSetOf(t *testing.T, ops ...map[string]any) string {
	t.Helper()
	b, err := json.Marshal(map[string]any{"ops": ops})
	require.NoError(t, err)
	return string(b)
}

// readRecord is one record kapi inspect printed.
type readRecord struct {
	Ref  change.Ref `json:"ref"`
	Rev  string     `json:"rev"`
	Text string     `json:"text"`
}

// inspectJSONL runs kapi inspect --jsonl over files and decodes each record.
func inspectJSONL(t *testing.T, a *App, files ...string) []readRecord {
	t.Helper()
	cmd := NewEnvCommand(t.Context(), "inspect")
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	require.NoError(t, a.RunInspect(t.Context(), cmd, files, "jsonl", nil), stderr.String())
	var out []readRecord
	for line := range strings.SplitSeq(strings.TrimSpace(stdout.String()), "\n") {
		if line == "" {
			continue
		}
		var r readRecord
		require.NoError(t, json.Unmarshal([]byte(line), &r), line)
		out = append(out, r)
	}
	return out
}

// recordOf returns the record of block in recs.
func recordOf(t *testing.T, recs []readRecord, block string) readRecord {
	t.Helper()
	for _, r := range recs {
		if r.Ref.Block == block {
			return r
		}
	}
	require.Failf(t, "no record", "no block %q among %+v", block, recs)
	return readRecord{}
}

// kapi apply reads one change set in each of its input forms, and the edit an
// inspect read addresses lands with every byte around it kept.
func TestApply_ReadsEveryInputForm(t *testing.T) {
	noProject(t)
	const page = `<p>Read the <a href="https://old.example/guide">shop guide</a> before you <b>order</b>.</p>` + "\n"
	const edited = `<p>Read the <a href="https://old.example/guide">store guide</a> before you <b>order</b>.</p>` + "\n"
	for name, form := range map[string]func(op string) string{
		"object": func(op string) string { return `{"note":"Rename the shop","ops":[` + op + `]}` },
		"jsonl":  func(op string) string { return `{"note":"Rename the shop"}` + "\n" + op + "\n" },
		"array":  func(op string) string { return "[" + op + "]" },
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			t.Chdir(dir)
			require.NoError(t, os.WriteFile("guide.html", []byte(page), 0o644))
			app := newToolboxApp(t)
			p := recordOf(t, inspectJSONL(t, app, "guide.html"), "p")
			op := `{"op":"replace_text","at":{"doc":"guide.html","block":"p"},"if_match":"` + p.Rev + `","edits":[{"find":"shop","text":"store"}]}`
			res, err := applyJSON(t, app, NewEnvCommand(t.Context(), "apply"), form(op), ApplyOptions{})
			require.NoError(t, err)
			assert.Equal(t, change.SetApplied, res.Status)
			got, _ := os.ReadFile("guide.html")
			assert.Equal(t, edited, string(got))
		})
	}
}

// A change set whose decoding fails, or that names its own actor, is refused
// with exit 2 before anything is read or written.
func TestApply_RefusesAChangeSetThatDoesNotDecode(t *testing.T) {
	noProject(t)
	t.Chdir(t.TempDir())
	require.NoError(t, os.WriteFile("en.json", []byte(`{"a":"Hello"}`), 0o644))
	for name, tc := range map[string]struct{ body, want string }{
		"unknown operation": {`{"ops":[{"op":"rewrite","at":{"doc":"en.json","block":"a"}}]}`, "/ops/0/op"},
		"no if_match":       {`{"ops":[{"op":"set_content","at":{"doc":"en.json","block":"a"},"text":"Hi"}]}`, "if_match"},
		"an actor":          {`{"actor":{"kind":"person"},"ops":[{"op":"set_content","at":{"doc":"en.json","block":"a"},"if_match":"*","text":"Hi"}]}`, "actor"},
		"not JSON":          {`{"ops":`, "not JSON"},
		"no operations":     {`{"ops":[]}`, "at least one operation"},
		"a term's field":    {`{"ops":[{"op":"set_content","at":{"doc":"en.json","block":"a"},"if_match":"*","replacement":"Hi"}]}`, "/ops/0/replacement"},
	} {
		t.Run(name, func(t *testing.T) {
			_, _, err := runApply(t, newToolboxApp(t), NewEnvCommand(t.Context(), "apply"), tc.body, ApplyOptions{})
			require.Error(t, err)
			assert.Equal(t, ExitUsage, ExitCode(nil, err))
			assert.Contains(t, err.Error(), tc.want)
			got, _ := os.ReadFile("en.json")
			assert.JSONEq(t, `{"a":"Hello"}`, string(got))
		})
	}
}

// The entry shape kapi apply read before kapi.change/v1 is refused by name,
// with the operation that takes each kind's place, and exits 2.
func TestApply_RefusesTheRetiredEntryShape(t *testing.T) {
	noProject(t)
	t.Chdir(t.TempDir())
	for name, tc := range map[string]struct{ body, want string }{
		"content":       {`{"kind":"content","file":"en.json","id":"tu1","content_hash":"abc","text":"Hi"}`, `"op": "set_content"`},
		"content array": {`[{"kind":"content","file":"en.json","id":"tu1","content_hash":"abc","text":"Hi"}]`, `"op": "set_content"`},
		"term":          {`{"kind":"term","op":"upsert","term":"leverage","status":"forbidden"}`, `"op": "term", "action": "upsert"`},
		"review":        {`{"kind":"review","file":"en.json","id":"greeting","locale":"nb"}`, `"op": "decide"`},
		"comment":       {`{"kind":"comment","file":"p.go","id":"func/Parse","text":"x"}`, `"block": "func/Parse"`},
		"voice":         {`{"kind":"voice","term":"leverage"}`, "word rules are terms"},
	} {
		t.Run(name, func(t *testing.T) {
			_, _, err := runApply(t, newToolboxApp(t), NewEnvCommand(t.Context(), "apply"), tc.body, ApplyOptions{})
			require.Error(t, err)
			assert.Equal(t, ExitUsage, ExitCode(nil, err))
			assert.Contains(t, err.Error(), tc.want)
			if name != "voice" {
				assert.Contains(t, err.Error(), "retired entry shape")
			}
		})
	}
}

// --print-ops echoes the change set as decoded, defaults filled in, and
// applies nothing; --schema prints the contract's schema.
func TestApply_PrintOpsAndSchema(t *testing.T) {
	noProject(t)
	t.Chdir(t.TempDir())
	require.NoError(t, os.WriteFile("en.json", []byte(`{"a":"Hello"}`), 0o644))
	body := `[{"op":"set_content","at":{"doc":"en.json","block":"a","edition":"FR"},"if_match":"absent","text":"<b>Bonjour</b>"}]`
	stdout, _, err := runApply(t, newToolboxApp(t), NewEnvCommand(t.Context(), "apply"), body, ApplyOptions{PrintOps: true, DryRun: true})
	require.NoError(t, err)
	var set map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &set), stdout)
	assert.Equal(t, change.SchemaID, set["schema"])
	assert.Equal(t, "preview", set["mode"], "--dry-run shows in the echo")
	assert.Equal(t, "enforce", set["gate"])
	assert.Contains(t, stdout, `"edition": "fr"`, "the edition key is canonical")
	assert.Contains(t, stdout, `<b>Bonjour</b>`, "markup is not escaped")
	got, _ := os.ReadFile("en.json")
	assert.JSONEq(t, `{"a":"Hello"}`, string(got))

	var schema bytes.Buffer
	require.NoError(t, RunApplySchema(&schema))
	assert.True(t, json.Valid(schema.Bytes()))
	assert.Contains(t, schema.String(), `"replace_text"`)
}

// A refused operation sets the exit code: stale and not_found are 3, and
// nothing in the change set is written. --dry-run writes nothing and prints a
// diff per document.
func TestApply_ExitCodesAndDryRun(t *testing.T) {
	noProject(t)
	t.Chdir(t.TempDir())
	const doc = `{"a":"Hello","b":"World"}`
	require.NoError(t, os.WriteFile("en.json", []byte(doc), 0o644))
	app := newToolboxApp(t)
	recs := inspectJSONL(t, app, "en.json")
	a, b := recordOf(t, recs, "a"), recordOf(t, recs, "b")
	set := func(bRev string) string {
		return changeSetOf(t,
			map[string]any{"op": "set_content", "at": map[string]any{"doc": "en.json", "block": "a"}, "if_match": a.Rev, "text": "Hi"},
			map[string]any{"op": "set_content", "at": map[string]any{"doc": "en.json", "block": "b"}, "if_match": bRev, "text": "Earth"})
	}

	t.Run("stale refuses the whole change set", func(t *testing.T) {
		res, err := applyJSON(t, app, NewEnvCommand(t.Context(), "apply"), set("r:0000000000000000"), ApplyOptions{})
		assert.Equal(t, 3, ExitCode(nil, err))
		assert.Equal(t, change.SetRefused, res.Status)
		assert.Equal(t, change.OpNotApplied, res.Ops[0].Status)
		require.NotNil(t, res.Ops[1].Error)
		assert.Equal(t, change.CodeStale, res.Ops[1].Error.Code)
		require.NotNil(t, res.Ops[1].Current)
		assert.Equal(t, b.Rev, res.Ops[1].Current.Rev)
		assert.Equal(t, "World", res.Ops[1].Current.Text)
		got, _ := os.ReadFile("en.json")
		assert.Equal(t, doc, string(got))
	})

	t.Run("an unknown block is not_found and exits 3", func(t *testing.T) {
		body := changeSetOf(t, map[string]any{"op": "set_content", "at": map[string]any{"doc": "en.json", "block": "zz"}, "if_match": "*", "text": "x"})
		_, stderr, err := runApply(t, app, NewEnvCommand(t.Context(), "apply"), body, ApplyOptions{})
		assert.Equal(t, 3, ExitCode(nil, err))
		assert.Contains(t, stderr, "not_found")
		assert.Contains(t, stderr, "change set refused")
	})

	t.Run("--dry-run prints the diff and writes nothing", func(t *testing.T) {
		stdout, stderr, err := runApply(t, app, NewEnvCommand(t.Context(), "apply"), set(b.Rev), ApplyOptions{DryRun: true})
		require.NoError(t, err)
		assert.Contains(t, stdout, `+{"a":"Hi","b":"Earth"}`)
		assert.Contains(t, stderr, "change set previewed: 2 previewed")
		got, _ := os.ReadFile("en.json")
		assert.Equal(t, doc, string(got))
	})

	t.Run("--dry-run --json carries the diff in the result", func(t *testing.T) {
		res, err := applyJSON(t, app, NewEnvCommand(t.Context(), "apply"), set(b.Rev), ApplyOptions{DryRun: true})
		require.NoError(t, err)
		assert.Equal(t, change.SetPreviewed, res.Status)
		require.Len(t, res.Docs, 1)
		assert.Contains(t, res.Docs[0].Diff, `+{"a":"Hi","b":"Earth"}`)
	})

	t.Run("the change lands, with a backup beside the file", func(t *testing.T) {
		res, err := applyJSON(t, app, NewEnvCommand(t.Context(), "apply"), set(b.Rev), ApplyOptions{BackupSuffix: ".bak"})
		require.NoError(t, err)
		assert.Equal(t, change.SetApplied, res.Status)
		got, _ := os.ReadFile("en.json")
		assert.JSONEq(t, `{"a":"Hi","b":"Earth"}`, string(got))
		backup, err := os.ReadFile("en.json.bak")
		require.NoError(t, err)
		assert.Equal(t, doc, string(backup))
	})
}

// The exit code follows the result: 0 when the change set applied or
// previewed, the refusal's own code when it was refused (2 invalid, 5
// unreachable, 3 otherwise), and 3 when it landed in part.
func TestChangeResultExit(t *testing.T) {
	refused := func(code change.Code) *change.Result {
		blocked := 1
		return &change.Result{Status: change.SetRefused, Ops: []change.OpResult{
			{I: 0, Status: change.OpNotApplied, BlockedBy: &blocked},
			{I: 1, Status: change.OpRefused, Error: &change.Error{Code: code}},
		}}
	}
	for name, tc := range map[string]struct {
		res  *change.Result
		want int
	}{
		"applied":     {&change.Result{Status: change.SetApplied}, 0},
		"previewed":   {&change.Result{Status: change.SetPreviewed}, 0},
		"invalid":     {refused(change.CodeInvalid), 2},
		"stale":       {refused(change.CodeStale), 3},
		"gate failed": {refused(change.CodeGateFailed), 3},
		"unreachable": {refused(change.CodeUnreachable), 5},
		"partial":     {&change.Result{Status: change.SetPartial}, 3},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, ExitCode(nil, changeResultExit(tc.res)))
		})
	}
}

// --gate takes the place of the change set's own gate, as the decoded set
// echoes it.
func TestApply_GateOverridesTheChangeSet(t *testing.T) {
	noProject(t)
	t.Chdir(t.TempDir())
	body := `{"gate":"enforce","ops":[{"op":"set_content","at":{"doc":"en.json","block":"a"},"if_match":"*","text":"Hi"}]}`
	stdout, _, err := runApply(t, newToolboxApp(t), NewEnvCommand(t.Context(), "apply"), body, ApplyOptions{PrintOps: true, Gate: change.GateReport})
	require.NoError(t, err)
	assert.Contains(t, stdout, `"gate": "report"`)
}

// A PO catalog's msgstr reads as a translation only in the language its reader
// is told. kapi inspect --target-lang lists it as an edition, and kapi apply
// reads the catalog in the one language the change set's operations name, so
// an edit of that edition lands in the msgstr.
func TestApply_EditsTheTranslationAPOCatalogHolds(t *testing.T) {
	noProject(t)
	t.Chdir(t.TempDir())
	const po = "msgid \"\"\nmsgstr \"\"\n\"Language: fr\\n\"\n\"Content-Type: text/plain; charset=UTF-8\\n\"\n\nmsgid \"Hello world\"\nmsgstr \"Bonjour le monde\"\n"
	require.NoError(t, os.WriteFile("fr.po", []byte(po), 0o644))
	app := newToolboxApp(t)

	app.TargetLang = "fr"
	cmd := NewEnvCommand(t.Context(), "inspect")
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	require.NoError(t, app.RunInspect(t.Context(), cmd, []string{"fr.po"}, "jsonl", nil), stderr.String())
	var rec struct {
		Ref      change.Ref                    `json:"ref"`
		Editions map[string]change.EditionRead `json:"editions"`
	}
	require.NoError(t, json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &rec), stdout.String())
	fr, ok := rec.Editions["fr"]
	require.True(t, ok, "the msgstr is listed as the fr edition: %s", stdout.String())
	assert.Equal(t, "Bonjour le monde", fr.Text)

	app.TargetLang = ""
	body := changeSetOf(t, map[string]any{"op": "replace_text", "at": map[string]any{"doc": "fr.po", "block": rec.Ref.Block, "edition": "fr"},
		"if_match": fr.Rev, "edits": []map[string]any{{"find": "Bonjour", "text": "Salut"}}})
	res, err := applyJSON(t, app, NewEnvCommand(t.Context(), "apply"), body, ApplyOptions{})
	require.NoError(t, err)
	require.Equal(t, change.OpApplied, res.Ops[0].Status, "%+v", res.Ops[0].Error)
	got, _ := os.ReadFile("fr.po")
	assert.Contains(t, string(got), `msgstr "Salut le monde"`)
	assert.Contains(t, string(got), `msgid "Hello world"`)
}

// Outside a project a reference is a path from the working directory, and it
// may lead out of it, as the files a command line names do.
func TestApply_ResolvesAPathOutsideTheWorkingDirectory(t *testing.T) {
	noProject(t)
	other := t.TempDir()
	file := filepath.Join(other, "en.json")
	require.NoError(t, os.WriteFile(file, []byte(`{"a":"Hello"}`), 0o644))
	t.Chdir(t.TempDir())
	app := newToolboxApp(t)
	rec := recordOf(t, inspectJSONL(t, app, file), "a")
	assert.Equal(t, filepath.ToSlash(file), rec.Ref.Doc, "a file outside the working directory keeps its own path")
	body := changeSetOf(t, map[string]any{"op": "set_content", "at": map[string]any{"doc": rec.Ref.Doc, "block": "a"}, "if_match": rec.Rev, "text": "Hi"})
	res, err := applyJSON(t, app, NewEnvCommand(t.Context(), "apply"), body, ApplyOptions{})
	require.NoError(t, err)
	assert.Equal(t, change.SetApplied, res.Status)
	got, _ := os.ReadFile(file)
	assert.JSONEq(t, `{"a":"Hi"}`, string(got))
}
