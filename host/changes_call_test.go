package host

import (
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/projector"
)

// callPage is an HTML page whose paragraph holds a link, read as one block
// with inline codes.
const callPage = `<!DOCTYPE html>
<html><head><meta charset="utf-8"><title>Guide</title></head>
<body>
<p>Read the <a href="https://old.example/guide">shop guide</a> before you <b>order</b>.</p>
</body></html>
`

// callProject is a project holding docs/page.html and docs/app.json.
func callProject(t *testing.T) (*App, string) {
	t.Helper()
	a, recipe := changeProject(t, project.ContentItem{Path: "docs/*"}, map[string]string{
		"docs/page.html": callPage,
		"docs/app.json":  `{"greeting": "Hello there", "farewell": "Goodbye now"}` + "\n",
	})
	t.Cleanup(a.Shutdown)
	return a, recipe
}

// callOptions is the options JSON naming the project at recipe, with extra
// fields merged in.
func callOptions(t *testing.T, recipe string, extra map[string]any) []byte {
	t.Helper()
	opts := map[string]any{"project": filepath.Dir(recipe)}
	maps.Copy(opts, extra)
	raw, err := json.Marshal(opts)
	require.NoError(t, err)
	return raw
}

func asJSON(t *testing.T, v any) []byte {
	t.Helper()
	raw, err := json.Marshal(v)
	require.NoError(t, err)
	return raw
}

func readPageJSON(t *testing.T, a *App, recipe string, req map[string]any) (*change.Page, string) {
	t.Helper()
	out, err := a.ReadChangesJSON(t.Context(), "browser", asJSON(t, req), callOptions(t, recipe, nil))
	require.NoError(t, err)
	var page change.Page
	require.NoError(t, json.Unmarshal(out, &page), string(out))
	return &page, string(out)
}

func resultJSON(t *testing.T, out []byte) change.Result {
	t.Helper()
	var res change.Result
	require.NoError(t, json.Unmarshal(out, &res), string(out))
	assert.Equal(t, change.ResultSchemaID, res.Schema)
	return res
}

// TestChangesJSON_ReadsAppliesAndDescribes drives the contract the browser
// engine's kapiRead, kapiApply and kapiDescribe carry: a read answers the page
// with each block's reference and revision, an apply of an operation built
// from them lands through the project's service and is recorded in the
// workspace's log as one content.edit operation, a replay is refused as stale
// with the current text, and a description names what the format supports.
func TestChangesJSON_ReadsAppliesAndDescribes(t *testing.T) {
	a, recipe := callProject(t)
	ctx := t.Context()
	const doc = "docs/page.html"

	page, raw := readPageJSON(t, a, recipe, map[string]any{"doc": doc})
	assert.Equal(t, doc, page.Doc)
	assert.Equal(t, "html", page.Format)
	para := blockWith(t, page, `Read the <x id="1"/>shop guide<x id="/1"/> before you <x id="2"/>order<x id="/2"/>.`)
	assert.Contains(t, raw, `<x id=`, "the answer keeps placeholders as written")
	assert.NotContains(t, raw, `\u003c`, "the answer does not escape markup")
	assert.Contains(t, para.Ops, change.KindReplaceText)

	set := map[string]any{"note": "Rename the guide", "ops": []any{map[string]any{
		"op": "replace_text", "at": para.Ref, "if_match": para.Rev,
		"edits": []any{map[string]any{"find": "shop guide", "text": "handbook"}},
	}}}
	out, err := a.ApplyChangesJSON(ctx, "browser", asJSON(t, set), callOptions(t, recipe, nil))
	require.NoError(t, err)
	res := resultJSON(t, out)
	require.Equal(t, change.SetApplied, res.Status, "%s", out)
	require.NotNil(t, res.Record, "an edit in a project is recorded")
	assert.Equal(t, strings.Replace(callPage, "shop guide", "handbook", 1), readFile(t, recipe, doc))

	ops := editOps(t, a, filepath.Dir(recipe))
	require.Len(t, ops, 1)
	assert.Equal(t, projector.KindEdit, ops[0].Kind)
	assert.Equal(t, *res.Record, ops[0].ID)
	var e projector.Edit
	require.NoError(t, json.Unmarshal(ops[0].Payload, &e))
	assert.Equal(t, change.ActorPerson, e.Actor.Kind, "a call that names no actor is a person's, as kapi apply records one")
	assert.Equal(t, "browser", e.Origin.By)
	assert.Equal(t, "Rename the guide", e.Note)

	out, err = a.ApplyChangesJSON(ctx, "browser", asJSON(t, set), callOptions(t, recipe, nil))
	require.NoError(t, err)
	res = resultJSON(t, out)
	require.Equal(t, change.SetRefused, res.Status, "%s", out)
	require.NotNil(t, res.Ops[0].Error)
	assert.Equal(t, change.CodeStale, res.Ops[0].Error.Code)
	require.NotNil(t, res.Ops[0].Current)
	assert.Contains(t, res.Ops[0].Current.Text, "handbook", "a stale refusal carries the text as it stands")
	assert.Len(t, editOps(t, a, filepath.Dir(recipe)), 1, "a refusal records nothing")

	for _, req := range []map[string]any{{"format": "html"}, {"doc": doc}} {
		out, err := a.DescribeChangesJSON(ctx, "browser", asJSON(t, req), callOptions(t, recipe, nil))
		require.NoError(t, err)
		var d change.Description
		require.NoError(t, json.Unmarshal(out, &d), string(out))
		assert.Equal(t, "html", d.Format, "%v", req)
		assert.NotNil(t, d.Ops.SetContent, "%v", req)
		assert.NotNil(t, d.Ops.ReplaceText, "%v", req)
	}
}

// TestChangesJSON_AnswersARefusalAsAResult pins that every request the
// contract refuses as a whole, and every one that does not decode, is answered
// as a kapi.change-result/v1 naming the error, with nothing written.
func TestChangesJSON_AnswersARefusalAsAResult(t *testing.T) {
	a, recipe := callProject(t)
	ctx := t.Context()
	read := func(req, opts string) ([]byte, error) {
		return a.ReadChangesJSON(ctx, "browser", []byte(req), []byte(opts))
	}
	apply := func(set, opts string) ([]byte, error) {
		return a.ApplyChangesJSON(ctx, "browser", []byte(set), []byte(opts))
	}
	describe := func(req, opts string) ([]byte, error) {
		return a.DescribeChangesJSON(ctx, "browser", []byte(req), []byte(opts))
	}
	project := string(callOptions(t, recipe, nil))
	set := `{"ops":[{"op":"set_content","at":{"doc":"docs/app.json","block":"greeting"},"if_match":"*","text":"Hi"}]}`
	cases := []struct {
		name    string
		call    func(req, opts string) ([]byte, error)
		req     string
		opts    string
		code    change.Code
		pointer string
		field   string
	}{
		{name: "an empty read request", call: read, opts: project, code: change.CodeInvalid},
		{name: "a read request with a field it does not take", call: read, req: `{"doc":"docs/app.json","rows":3}`, opts: project, code: change.CodeInvalid},
		{name: "a read request followed by more", call: read, req: `{"doc":"docs/app.json"} {}`, opts: project, code: change.CodeInvalid},
		{name: "a read naming no document", call: read, req: `{"limit":5}`, opts: project, code: change.CodeInvalid, field: "doc"},
		{name: "a read naming an edition that is no language", call: read, req: `{"doc":"docs/app.json","editions":["not a language"]}`, opts: project, code: change.CodeInvalid, field: "editions"},
		{name: "a read naming an actor", call: read, req: `{"doc":"docs/app.json"}`, opts: string(callOptions(t, recipe, map[string]any{"actor": map[string]any{"kind": "person"}})), code: change.CodeInvalid, pointer: "/actor"},
		{name: "a read of a document the project does not hold", call: read, req: `{"doc":"docs/missing.json"}`, opts: project, code: change.CodeNotFound},
		{name: "options with a field they do not take", call: apply, req: set, opts: string(callOptions(t, recipe, map[string]any{"projects": "x"})), code: change.CodeInvalid},
		{name: "a change set that does not decode", call: apply, req: `{"ops":[{"op":"set_content","at":{"doc":"docs/app.json","block":"greeting"}}]}`, opts: project, code: change.CodeInvalid, pointer: "/ops/0"},
		{name: "a change set that lists no operations", call: apply, req: `{"note":"nothing"}`, opts: project, code: change.CodeInvalid, pointer: "/ops"},
		{name: "a tool as the actor", call: apply, req: set, opts: string(callOptions(t, recipe, map[string]any{"actor": map[string]any{"kind": "tool", "name": "x"}})), code: change.CodeInvalid, pointer: "/actor/kind"},
		{name: "an empty describe request", call: describe, req: `{}`, opts: project, code: change.CodeInvalid},
	}
	before := readFile(t, recipe, "docs/app.json")
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := tc.call(tc.req, tc.opts)
			require.NoError(t, err)
			res := resultJSON(t, out)
			assert.Equal(t, change.SetRefused, res.Status, "%s", out)
			require.NotNil(t, res.Error, "%s", out)
			assert.Equal(t, tc.code, res.Error.Code, "%s", out)
			if tc.pointer != "" {
				assert.True(t, strings.HasPrefix(res.Error.Pointer, tc.pointer), "pointer %q, want %q", res.Error.Pointer, tc.pointer)
			}
			if tc.field != "" {
				assert.Equal(t, tc.field, res.Error.Field)
			}
			assert.Empty(t, res.Docs)
			assert.Empty(t, res.Ops)
			assert.Contains(t, string(out), `"docs":[]`, "a refusal lists no documents rather than none at all")
		})
	}
	assert.Equal(t, before, readFile(t, recipe, "docs/app.json"), "no refusal writes")

	// A change set with no operation applies and writes nothing.
	out, err := apply(`{"ops":[]}`, project)
	require.NoError(t, err)
	res := resultJSON(t, out)
	assert.Equal(t, change.SetApplied, res.Status, "%s", out)
	assert.Nil(t, res.Error)
	assert.Empty(t, res.Ops)
	assert.Equal(t, before, readFile(t, recipe, "docs/app.json"))
}

// TestChangesJSON_SendsAsTheActorTheCallNames pins that the actor an apply's
// options name sends its change set: an agent's edit is recorded as the
// agent's, and the policy refuses an agent what is a person's.
func TestChangesJSON_SendsAsTheActorTheCallNames(t *testing.T) {
	a, recipe := callProject(t)
	ctx := t.Context()
	page, _ := readPageJSON(t, a, recipe, map[string]any{"doc": "docs/app.json"})
	greeting := blockWith(t, page, "Hello there")
	agent := map[string]any{"actor": map[string]any{"kind": "agent", "name": "lab-agent", "session": "s_lab"}}

	set := map[string]any{"ops": []any{map[string]any{"op": "set_content", "at": greeting.Ref, "if_match": greeting.Rev, "text": "Hello again"}}}
	out, err := a.ApplyChangesJSON(ctx, "browser", asJSON(t, set), callOptions(t, recipe, agent))
	require.NoError(t, err)
	res := resultJSON(t, out)
	require.Equal(t, change.SetApplied, res.Status, "%s", out)
	ops := editOps(t, a, filepath.Dir(recipe))
	require.Len(t, ops, 1)
	var e projector.Edit
	require.NoError(t, json.Unmarshal(ops[0].Payload, &e))
	assert.Equal(t, change.Actor{Kind: change.ActorAgent, Name: "lab-agent", Session: "s_lab"}, e.Actor)

	term := map[string]any{"ops": []any{map[string]any{"op": "term", "action": "upsert", "term": "handbook", "locale": "en"}}}
	out, err = a.ApplyChangesJSON(ctx, "browser", asJSON(t, term), callOptions(t, recipe, agent))
	require.NoError(t, err)
	res = resultJSON(t, out)
	require.Equal(t, change.SetRefused, res.Status, "%s", out)
	require.NotNil(t, res.Ops[0].Error)
	assert.Equal(t, change.CodeNotPermitted, res.Ops[0].Error.Code, "writing a term is a person's")
}

// TestChangesJSON_ResolvesTheProjectTheCallNames pins where a call acts: in
// the project its options name, outside any project under the working
// directory when discovery finds none, and nowhere when the project it names
// does not exist.
func TestChangesJSON_ResolvesTheProjectTheCallNames(t *testing.T) {
	ctx := t.Context()

	t.Run("a project that names nothing is an error, not a refusal", func(t *testing.T) {
		a := &App{}
		a.InitRegistries()
		missing := filepath.Join(t.TempDir(), "nowhere")
		_, err := a.ReadChangesJSON(ctx, "browser", []byte(`{"doc":"a.json"}`), asJSON(t, map[string]any{"project": missing}))
		var perr *MCPProjectError
		require.ErrorAs(t, err, &perr)
		assert.Equal(t, missing, perr.Path)
	})

	t.Run("outside a project a document is a path under the working directory, and nothing is recorded", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "app.json"), []byte(`{"title": "Welcome"}`+"\n"), 0o644))
		t.Setenv(project.NoProjectEnvVar, "1")
		t.Chdir(dir)
		a := &App{}
		a.InitRegistries()
		t.Cleanup(a.Shutdown)

		out, err := a.ReadChangesJSON(ctx, "browser", []byte(`{"doc":"app.json"}`), nil)
		require.NoError(t, err)
		var page change.Page
		require.NoError(t, json.Unmarshal(out, &page), string(out))
		title := blockWith(t, &page, "Welcome")

		set := asJSON(t, map[string]any{"ops": []any{map[string]any{"op": "set_content", "at": title.Ref, "if_match": title.Rev, "text": "Welcome back"}}})
		out, err = a.ApplyChangesJSON(ctx, "browser", set, nil)
		require.NoError(t, err)
		res := resultJSON(t, out)
		require.Equal(t, change.SetApplied, res.Status, "%s", out)
		assert.Nil(t, res.Record, "outside a project there is no log to record in")
		b, err := os.ReadFile(filepath.Join(dir, "app.json"))
		require.NoError(t, err)
		assert.Equal(t, `{"title": "Welcome back"}`+"\n", string(b))
	})
}

// outsideAProjectIn makes dir, holding files, the working directory of a call
// that resolves no project, and returns an App to answer it.
func outsideAProjectIn(t *testing.T, files map[string][]byte) (*App, string) {
	t.Helper()
	dir := t.TempDir()
	for name, data := range files {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), data, 0o644))
	}
	t.Setenv(project.NoProjectEnvVar, "1")
	t.Chdir(dir)
	a := &App{}
	a.InitRegistries()
	t.Cleanup(a.Shutdown)
	return a, dir
}

// A PO catalog keeps its translation beside the source, and its reader reads
// the msgstr only in the language it is told. A read names that language among
// its editions and an apply's operations name it, as kapi apply reads a
// catalog, so a call creates a translation the catalog holds and reads it back.
func TestChangesJSON_TranslatesABilingualCatalogInPlace(t *testing.T) {
	const po = "msgid \"\"\nmsgstr \"\"\n\"Content-Type: text/plain; charset=UTF-8\\n\"\n\nmsgid \"Hello\"\nmsgstr \"\"\n"
	a, dir := outsideAProjectIn(t, map[string][]byte{"messages.po": []byte(po)})
	ctx := t.Context()

	out, err := a.ReadChangesJSON(ctx, "browser", []byte(`{"doc":"messages.po","editions":["fr"]}`), nil)
	require.NoError(t, err)
	var page change.Page
	require.NoError(t, json.Unmarshal(out, &page), string(out))
	hello := blockWith(t, &page, "Hello")
	assert.NotContains(t, hello.Editions, "fr", "the catalog holds no French yet")

	set := asJSON(t, map[string]any{"ops": []any{map[string]any{"op": "set_content",
		"at": map[string]any{"doc": "messages.po", "block": hello.Ref.Block, "edition": "fr"}, "if_match": model.AbsentRevision, "text": "Bonjour"}}})
	out, err = a.ApplyChangesJSON(ctx, "browser", set, nil)
	require.NoError(t, err)
	res := resultJSON(t, out)
	require.Equal(t, change.SetApplied, res.Status, "%s", out)
	b, err := os.ReadFile(filepath.Join(dir, "messages.po"))
	require.NoError(t, err)
	assert.Contains(t, string(b), "msgid \"Hello\"\nmsgstr \"Bonjour\"\n")

	out, err = a.ReadChangesJSON(ctx, "browser", []byte(`{"doc":"messages.po","editions":["fr"]}`), nil)
	require.NoError(t, err)
	page = change.Page{}
	require.NoError(t, json.Unmarshal(out, &page), string(out))
	fr, ok := blockWith(t, &page, "Hello").Editions["fr"]
	require.True(t, ok, "%s", out)
	assert.Equal(t, "Bonjour", fr.Text)
}

// A file in another encoding reaches the content model as bytes that are not
// UTF-8, which a read shows as U+FFFD. An edit that would write U+FFFD over
// them is refused, a replace_text or a set_content of the read's text, and
// the block lists no replace_text; an edit of another block lands and keeps
// them.
func TestChangesJSON_KeepsBytesThatAreNotUTF8(t *testing.T) {
	const notes = "Caf\xe9 au lait\n\nSecond line here\n"
	a, dir := outsideAProjectIn(t, map[string][]byte{"notes.txt": []byte(notes)})
	ctx := t.Context()

	out, err := a.ReadChangesJSON(ctx, "browser", []byte(`{"doc":"notes.txt"}`), nil)
	require.NoError(t, err)
	var page change.Page
	require.NoError(t, json.Unmarshal(out, &page), string(out))
	cafe := blockWith(t, &page, "Caf\uFFFD au lait")
	second := blockWith(t, &page, "Second line here")
	assert.NotContains(t, cafe.Ops, change.KindReplaceText)
	assert.Contains(t, second.Ops, change.KindReplaceText)

	replace := func(b change.BlockRead, find, text string) []byte {
		return asJSON(t, map[string]any{"ops": []any{map[string]any{"op": "replace_text", "at": b.Ref, "if_match": b.Rev,
			"edits": []any{map[string]any{"find": find, "text": text}}}}})
	}
	out, err = a.ApplyChangesJSON(ctx, "browser", replace(cafe, "lait", "chaud"), nil)
	require.NoError(t, err)
	res := resultJSON(t, out)
	require.Equal(t, change.SetRefused, res.Status, "%s", out)
	require.NotNil(t, res.Ops[0].Error)
	assert.Equal(t, change.CodeUnsupported, res.Ops[0].Error.Code)
	assert.Equal(t, "encoding", res.Ops[0].Error.Capability)
	got, err := os.ReadFile(filepath.Join(dir, "notes.txt"))
	require.NoError(t, err)
	assert.Equal(t, notes, string(got), "a refusal writes nothing")

	// The text the read answered, edited and sent back whole, still carries
	// U+FFFD where the byte stands.
	out, err = a.ApplyChangesJSON(ctx, "browser", asJSON(t, map[string]any{"ops": []any{map[string]any{"op": "set_content",
		"at": cafe.Ref, "if_match": cafe.Rev, "text": strings.Replace(cafe.Text, "lait", "chaud", 1)}}}), nil)
	require.NoError(t, err)
	res = resultJSON(t, out)
	require.Equal(t, change.SetRefused, res.Status, "%s", out)
	assert.Equal(t, "encoding", res.Ops[0].Error.Capability)

	out, err = a.ApplyChangesJSON(ctx, "browser", replace(second, "here", "there"), nil)
	require.NoError(t, err)
	res = resultJSON(t, out)
	require.Equal(t, change.SetApplied, res.Status, "%s", out)
	got, err = os.ReadFile(filepath.Join(dir, "notes.txt"))
	require.NoError(t, err)
	assert.Equal(t, "Caf\xe9 au lait\n\nSecond line there\n", string(got), "the other block's bytes stay as they were")
}
