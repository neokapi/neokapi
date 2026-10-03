package host

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/change/changeschema"
	"github.com/neokapi/neokapi/core/contextop"
	"github.com/neokapi/neokapi/core/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// editSession connects a client named client to a server carrying the edit
// tools, so each call goes through the tool's own decoding and result.
func editSession(t *testing.T, app *App, client string) *mcp.ClientSession {
	t.Helper()
	server := mcp.NewServer(&mcp.Implementation{Name: "kapi", Version: "test"}, nil)
	registerEditMCPTools(server, app)
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(t.Context(), serverTransport, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = serverSession.Close() })
	c := mcp.NewClient(&mcp.Implementation{Name: client, Version: "test"}, nil)
	session, err := c.Connect(t.Context(), clientTransport, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close() })
	return session
}

// callEditTool calls one edit tool and decodes its structured result into
// out. It returns whether the tool reported an error, and the text content.
func callEditTool(t *testing.T, s *mcp.ClientSession, name string, args any, out any) (bool, string) {
	t.Helper()
	res, err := s.CallTool(t.Context(), &mcp.CallToolParams{Name: name, Arguments: args})
	require.NoError(t, err)
	var text strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			text.WriteString(tc.Text)
		}
	}
	if out != nil {
		require.NotNil(t, res.StructuredContent, "%s returns a structured result: %s", name, text.String())
		body, err := json.Marshal(res.StructuredContent)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(body, out), string(body))
	}
	return res.IsError, text.String()
}

// mcpPage is the part of a read_blocks page the tests read.
type mcpPage struct {
	Doc    string `json:"doc"`
	Format string `json:"format"`
	Next   string `json:"next"`
	Blocks []struct {
		Ref      map[string]string `json:"ref"`
		Rev      string            `json:"rev"`
		Text     string            `json:"text"`
		Ops      []string          `json:"ops"`
		Editions map[string]struct {
			Rev  string `json:"rev"`
			Text string `json:"text"`
		} `json:"editions"`
	} `json:"blocks"`
}

// blockWith is the block of page whose text contains s.
func (p mcpPage) blockWith(t *testing.T, s string) (map[string]string, string, string) {
	t.Helper()
	for _, b := range p.Blocks {
		if strings.Contains(b.Text, s) {
			return b.Ref, b.Rev, b.Text
		}
	}
	require.Failf(t, "no block holds the text", "%q in %+v", s, p.Blocks)
	return nil, "", ""
}

// mcpResult is the part of a kapi.change-result/v1 the tests read.
type mcpResult struct {
	Schema string `json:"schema"`
	Status string `json:"status"`
	Ops    []struct {
		Status  string        `json:"status"`
		After   string        `json:"after"`
		Error   *change.Error `json:"error"`
		Current *struct {
			Rev  string `json:"rev"`
			Text string `json:"text"`
		} `json:"current"`
	} `json:"ops"`
	Docs []struct {
		Doc     string `json:"doc"`
		Written bool   `json:"written"`
		Diff    string `json:"diff"`
	} `json:"docs"`
	Error *change.Error `json:"error"`
}

// outsideAProject makes dir the working directory of a server that resolves
// no project, which is where the edit tools find documents outside one.
func outsideAProject(t *testing.T, dir string) {
	t.Helper()
	t.Setenv(project.NoProjectEnvVar, "1")
	t.Chdir(dir)
}

const mcpGuide = `<html><body><p>Read the <a href="https://old.example/guide">shop guide</a> before you <b>order</b>.</p>` +
	"<p>Fish &amp; chips</p>" + `<p><img src="c.png" alt="chart"> Prices rose.</p></body></html>` + "\n"

// The edit loop an agent runs: read_blocks reports a reference and a
// revision, apply_edits sends them back, and the file changes in the edited
// words alone. Sending the same change set again is refused as stale with the
// text the edition holds now, and writes nothing.
func TestMCPEditTools_ReadThenApplyByReference(t *testing.T) {
	app := newToolboxApp(t)
	dir := t.TempDir()
	outsideAProject(t, dir)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "guide.html"), []byte(mcpGuide), 0o600))
	session := editSession(t, app, "edit-test")

	var page mcpPage
	isErr, text := callEditTool(t, session, "read_blocks", map[string]any{"doc": "guide.html"}, &page)
	require.False(t, isErr, text)
	assert.Equal(t, "guide.html", page.Doc)
	assert.Equal(t, "html", page.Format)
	ref, rev, blockText := page.blockWith(t, "shop guide")
	assert.Equal(t, "guide.html", ref["doc"])
	assert.Regexp(t, `^r:[0-9a-f]{16}$`, rev)
	assert.Contains(t, blockText, `<x id="`, "the read shows inline codes as placeholders")
	assert.Contains(t, text, `<x id=\"`, "the text a client reads carries the placeholders as written")
	assert.NotContains(t, text, `\`+"u003c", "the placeholders are not escaped")

	set := map[string]any{
		"note": "Name the handbook",
		"ops": []any{map[string]any{
			"op": "replace_text", "at": ref, "if_match": rev,
			"edits": []any{map[string]any{"find": "shop guide", "text": "handbook"}},
		}},
	}
	var res mcpResult
	isErr, text = callEditTool(t, session, "apply_edits", set, &res)
	require.False(t, isErr, text)
	assert.Equal(t, change.ResultSchemaID, res.Schema)
	assert.Equal(t, "applied", res.Status)
	require.Len(t, res.Ops, 1)
	assert.Equal(t, "applied", res.Ops[0].Status)
	want := strings.Replace(mcpGuide, "shop guide", "handbook", 1)
	got, err := os.ReadFile(filepath.Join(dir, "guide.html"))
	require.NoError(t, err)
	assert.Equal(t, want, string(got), "the edited words are the only change; the link keeps its href")

	isErr, text = callEditTool(t, session, "apply_edits", set, &res)
	assert.True(t, isErr, "a change set that did not land is an error result")
	assert.Equal(t, "refused", res.Status, text)
	require.Len(t, res.Ops, 1)
	require.NotNil(t, res.Ops[0].Error)
	assert.Equal(t, change.CodeStale, res.Ops[0].Error.Code)
	require.NotNil(t, res.Ops[0].Current)
	assert.Contains(t, res.Ops[0].Current.Text, "handbook", "the refusal carries the edition as it stands")
	assert.Equal(t, res.Ops[0].Current.Rev, mustRev(t, session, "guide.html", "handbook"))
	got, err = os.ReadFile(filepath.Join(dir, "guide.html"))
	require.NoError(t, err)
	assert.Equal(t, want, string(got), "a refused change set writes nothing")
}

// mustRev is the revision read_blocks reports for the block of doc whose text
// contains s.
func mustRev(t *testing.T, s *mcp.ClientSession, doc, text string) string {
	t.Helper()
	var page mcpPage
	isErr, body := callEditTool(t, s, "read_blocks", map[string]any{"doc": doc}, &page)
	require.False(t, isErr, body)
	_, rev, _ := page.blockWith(t, text)
	return rev
}

// Every block read_blocks lists is one apply_edits writes by the reference the
// read reported, with its text as the read showed it: an image's alt text, the
// paragraph after the image, and a paragraph holding a character reference
// read as one block each in both.
func TestMCPReadBlocks_AddressesWhatApplyEditsWrites(t *testing.T) {
	tests := []struct {
		name string
		read string // text of the block the edit addresses
		text string // the block's new text, in the form the read showed
		want string
	}{
		{name: "an image's alt text", read: "chart", text: "Bar chart", want: `alt="Bar chart"`},
		{name: "the paragraph after the image", read: "Prices rose.", text: `<x id="1/"/> Prices fell.`, want: "> Prices fell.</p>"},
		{name: "a paragraph with a character reference", read: "Fish & chips", text: "Fish & fries", want: "<p>Fish &amp; fries</p>"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			app := newToolboxApp(t)
			dir := t.TempDir()
			outsideAProject(t, dir)
			require.NoError(t, os.WriteFile(filepath.Join(dir, "guide.html"), []byte(mcpGuide), 0o600))
			session := editSession(t, app, "edit-test")

			var page mcpPage
			isErr, body := callEditTool(t, session, "read_blocks", map[string]any{"doc": "guide.html"}, &page)
			require.False(t, isErr, body)
			ref, rev, _ := page.blockWith(t, tc.read)

			var res mcpResult
			isErr, body = callEditTool(t, session, "apply_edits", map[string]any{"ops": []any{map[string]any{
				"op": "set_content", "at": ref, "if_match": rev, "text": tc.text,
			}}}, &res)
			require.False(t, isErr, body)
			assert.Equal(t, "applied", res.Status)
			got, err := os.ReadFile(filepath.Join(dir, "guide.html"))
			require.NoError(t, err)
			assert.Contains(t, string(got), tc.want)
		})
	}
}

// The wording of an operation is text. apply_edits writes it through the
// format's writer, which encodes it for the file, so an agent's wording can
// never add markup to the document: inline codes travel as <x id="…"/>
// tokens, and a literal '<' or '&' is a character.
func TestMCPApplyEdits_WritesWordingAsText(t *testing.T) {
	const payload = "Hello <script>alert(1)</script> & goodbye"
	tests := []struct {
		name string
		file string
		src  string
		want string
	}{
		{name: "html", file: "inj.html", src: `<html><body><p>Hello world</p></body></html>`,
			want: `<html><body><p>Hello &lt;script>alert(1)&lt;/script> &amp; goodbye</p></body></html>`},
		{name: "markdown", file: "inj.md", src: "Hello world\n", want: "Hello \\<script>alert(1)\\</script> & goodbye\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			app := newToolboxApp(t)
			dir := t.TempDir()
			outsideAProject(t, dir)
			require.NoError(t, os.WriteFile(filepath.Join(dir, tc.file), []byte(tc.src), 0o600))
			session := editSession(t, app, "edit-test")

			var page mcpPage
			isErr, body := callEditTool(t, session, "read_blocks", map[string]any{"doc": tc.file}, &page)
			require.False(t, isErr, body)
			ref, rev, _ := page.blockWith(t, "Hello world")

			var res mcpResult
			isErr, body = callEditTool(t, session, "apply_edits", map[string]any{"ops": []any{map[string]any{
				"op": "set_content", "at": ref, "if_match": rev, "text": payload,
			}}}, &res)
			require.False(t, isErr, body)
			got, err := os.ReadFile(filepath.Join(dir, tc.file))
			require.NoError(t, err)
			assert.Equal(t, tc.want, string(got))
		})
	}
}

// What apply_edits cannot apply is refused with the contract's code and
// writes nothing: a change set that does not decode names the JSON pointer
// at fault, a block the document does not hold is not_found, and a block the
// format marks as content an edit does not change is unsupported. Each is an
// error result. A change set refused as a whole is a result too: its error
// says why, with no record and with empty docs and ops.
func TestMCPApplyEdits_RefusalsWriteNothing(t *testing.T) {
	const doc = "Run the tool.\n\n```sh\nkapi up\n```\n"
	tests := []struct {
		name    string
		args    func(t *testing.T, s *mcp.ClientSession) map[string]any
		code    change.Code
		pointer string
		// whole says the change set is refused before any operation is
		// considered.
		whole bool
	}{
		{
			name: "an unknown field",
			args: func(*testing.T, *mcp.ClientSession) map[string]any {
				return map[string]any{"ops": []any{map[string]any{"op": "set_content", "at": map[string]any{"doc": "doc.md", "block": "x"},
					"if_match": "*", "text": "x", "wording": "x"}}}
			},
			code: change.CodeInvalid, pointer: "/ops/0/wording", whole: true,
		},
		{
			name: "the old entry shape",
			args: func(*testing.T, *mcp.ClientSession) map[string]any {
				return map[string]any{"changeset": []any{map[string]any{"kind": "content", "file": "doc.md", "text": "x"}}}
			},
			code: change.CodeInvalid, whole: true,
		},
		{
			name: "a project that is not a path",
			args: func(*testing.T, *mcp.ClientSession) map[string]any {
				return map[string]any{"project": 7, "ops": []any{}}
			},
			code: change.CodeInvalid, pointer: "/project", whole: true,
		},
		{
			name: "a block the document does not hold",
			args: func(t *testing.T, s *mcp.ClientSession) map[string]any {
				return map[string]any{"ops": []any{map[string]any{"op": "set_content",
					"at": map[string]any{"doc": "doc.md", "block": "no-such-block"}, "if_match": "r:0000000000000000", "text": "After"}}}
			},
			code: change.CodeNotFound,
		},
		{
			name: "a code block",
			args: func(t *testing.T, s *mcp.ClientSession) map[string]any {
				var page mcpPage
				isErr, body := callEditTool(t, s, "read_blocks", map[string]any{"doc": "doc.md"}, &page)
				require.False(t, isErr, body)
				ref, rev, _ := page.blockWith(t, "kapi up")
				for _, b := range page.Blocks {
					if b.Rev == rev {
						assert.Empty(t, b.Ops, "the read lists no operation for a block an edit does not change")
					}
				}
				return map[string]any{"ops": []any{map[string]any{"op": "set_content", "at": ref, "if_match": rev, "text": "kapi down"}}}
			},
			code: change.CodeUnsupported,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			app := newToolboxApp(t)
			dir := t.TempDir()
			outsideAProject(t, dir)
			file := filepath.Join(dir, "doc.md")
			require.NoError(t, os.WriteFile(file, []byte(doc), 0o600))
			session := editSession(t, app, "edit-test")

			var res mcpResult
			isErr, body := callEditTool(t, session, "apply_edits", tc.args(t, session), &res)
			assert.True(t, isErr, body)
			assert.Equal(t, "refused", res.Status, body)
			e := res.Error
			if e == nil {
				for _, op := range res.Ops {
					if op.Error != nil {
						e = op.Error
					}
				}
			}
			require.NotNil(t, e, body)
			assert.Equal(t, tc.code, e.Code, body)
			if tc.pointer != "" {
				assert.Equal(t, tc.pointer, e.Pointer)
			}
			if tc.whole {
				require.NotNil(t, res.Error, "the change set is refused as a whole: %s", body)
				var raw map[string]json.RawMessage
				require.NoError(t, json.Unmarshal([]byte(body), &raw), body)
				assert.JSONEq(t, `"kapi.change-result/v1"`, string(raw["schema"]), body)
				assert.JSONEq(t, "null", string(raw["record"]), body)
				assert.JSONEq(t, "[]", string(raw["docs"]), body)
				assert.JSONEq(t, "[]", string(raw["ops"]), body)
			}
			got, err := os.ReadFile(file)
			require.NoError(t, err)
			assert.Equal(t, doc, string(got))
		})
	}
}

// apply_edits sends every change set as the calling agent, in the server's
// session. The context policy refuses an agent's term, content-memory and
// recipe operations, so a change set holding one is refused whole: the content
// edit beside it is not written either, the refusal names the agent and its
// session and says what an agent does instead, and nothing is recorded.
func TestMCPApplyEdits_SendsAsTheCallingAgent(t *testing.T) {
	app, _ := contextOpsApp(t)
	app.InitRegistries()
	t.Setenv(project.NoProjectEnvVar, "1")
	root := contextOpsProject(t, "mcp-apply-actor")
	recipeBefore, err := os.ReadFile(recipeOf(root))
	require.NoError(t, err)
	session := editSession(t, app, "actor-test-agent")

	var page mcpPage
	isErr, body := callEditTool(t, session, "read_blocks", map[string]any{"doc": "config/app.yaml", "project": recipeOf(root)}, &page)
	require.False(t, isErr, body)
	ref, rev, _ := page.blockWith(t, "utilise")
	content := map[string]any{"op": "replace_text", "at": ref, "if_match": rev,
		"edits": []any{map[string]any{"find": "utilise", "text": "use"}}}

	for _, asset := range []map[string]any{
		{"op": "term", "action": "upsert", "term": "leverage", "replacement": "use", "locale": "en", "status": "forbidden"},
		{"op": "memory", "action": "add", "from": map[string]any{"edition": "en", "text": "Goodbye"}, "to": map[string]any{"edition": "nb", "text": "Ha det"}},
		{"op": "recipe", "path": "defaults.coordinates.brand", "value": "kapi"},
	} {
		t.Run(asset["op"].(string), func(t *testing.T) {
			var res mcpResult
			isErr, body := callEditTool(t, session, "apply_edits", map[string]any{
				"project": recipeOf(root), "ops": []any{content, asset},
			}, &res)
			assert.True(t, isErr, body)
			assert.Equal(t, "refused", res.Status, body)
			require.Len(t, res.Ops, 2)
			assert.Equal(t, "not_applied", res.Ops[0].Status, "the content edit lands only with the whole change set")
			require.NotNil(t, res.Ops[1].Error)
			assert.Equal(t, change.CodeNotPermitted, res.Ops[1].Error.Code)
			assert.Contains(t, res.Ops[1].Error.Message, "agent actor-test-agent/"+MCPSessionID(),
				"the refusal names the calling agent and its session")
			if asset["op"] == "term" {
				assert.Contains(t, res.Ops[1].Error.Message, "context_observe", "the refusal says what an agent does instead")
			}
		})
	}

	assert.NotContains(t, projectTerms(t, app, root), "leverage")
	log, err := app.ContextOperations(t.Context(), ContextLogRequest{Project: recipeOf(root)})
	require.NoError(t, err)
	for _, op := range log.Operations {
		assert.NotEqual(t, contextop.KindEdit, op.Kind, "nothing was recorded as an edit")
	}
	recipeAfter, err := os.ReadFile(recipeOf(root))
	require.NoError(t, err)
	assert.Equal(t, string(recipeBefore), string(recipeAfter), "the recipe operation wrote nothing")
	got, err := os.ReadFile(filepath.Join(root, "config", "app.yaml"))
	require.NoError(t, err)
	assert.Equal(t, contextOpsYAML, string(got), "the content edit beside a refused operation is not written")

	var res mcpResult
	isErr, body = callEditTool(t, session, "apply_edits", map[string]any{"project": recipeOf(root), "ops": []any{content}}, &res)
	require.False(t, isErr, body)
	assert.Equal(t, "applied", res.Status, "an agent applies content edits")
}

// An agent's applied edit that writes the form a suggestion prefers adds to
// the suggestion's standing, as the agent's own signal, and establishes
// nothing.
func TestMCPApplyEdits_AnAgentsEditAddsStanding(t *testing.T) {
	app, _ := contextOpsApp(t)
	app.InitRegistries()
	t.Setenv(project.NoProjectEnvVar, "1")
	root := contextOpsProject(t, "mcp-apply-standing")
	proposed := proposeUtilise(t, app, root, agentIn("s1"))
	session := editSession(t, app, "standing-agent")

	var page mcpPage
	isErr, body := callEditTool(t, session, "read_blocks", map[string]any{"doc": "config/app.yaml", "project": root}, &page)
	require.False(t, isErr, body)
	ref, rev, _ := page.blockWith(t, "utilise")
	var res mcpResult
	isErr, body = callEditTool(t, session, "apply_edits", map[string]any{"project": root, "ops": []any{map[string]any{
		"op": "replace_text", "at": ref, "if_match": rev, "edits": []any{map[string]any{"find": "utilise", "text": "use"}},
	}}}, &res)
	require.False(t, isErr, body)

	got, err := app.ContextOperations(t.Context(), ContextLogRequest{Project: recipeOf(root), Subjects: true})
	require.NoError(t, err)
	var rule *ContextOperation
	for i, op := range got.Operations {
		if op.ID == proposed.ID {
			rule = &got.Operations[i]
		}
	}
	require.NotNil(t, rule)
	assert.Equal(t, contextop.StatusSuggested, rule.Status, "an agent following a suggestion settles nothing")
	require.NotNil(t, rule.Standing)
	assert.Equal(t, 1, rule.Standing.Applied, "the agent's applied edit is recorded")
}

// A preview computes the change set and the diff it would write, and writes
// nothing.
func TestMCPApplyEdits_PreviewWritesNothing(t *testing.T) {
	app := newToolboxApp(t)
	dir := t.TempDir()
	outsideAProject(t, dir)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "guide.html"), []byte(mcpGuide), 0o600))
	session := editSession(t, app, "edit-test")
	var page mcpPage
	isErr, body := callEditTool(t, session, "read_blocks", map[string]any{"doc": "guide.html"}, &page)
	require.False(t, isErr, body)
	ref, rev, _ := page.blockWith(t, "shop guide")

	var res mcpResult
	isErr, body = callEditTool(t, session, "apply_edits", map[string]any{"mode": "preview", "ops": []any{map[string]any{
		"op": "replace_text", "at": ref, "if_match": rev, "edits": []any{map[string]any{"find": "shop guide", "text": "handbook"}},
	}}}, &res)
	require.False(t, isErr, body)
	assert.Equal(t, "previewed", res.Status)
	require.Len(t, res.Docs, 1)
	assert.False(t, res.Docs[0].Written)
	assert.Contains(t, res.Docs[0].Diff, "+")
	assert.Contains(t, res.Docs[0].Diff, "handbook")
	got, err := os.ReadFile(filepath.Join(dir, "guide.html"))
	require.NoError(t, err)
	assert.Equal(t, mcpGuide, string(got))
}

// read_blocks returns a page at a time, and next continues it until the last
// page. A cursor from a document that changed since is refused as stale.
func TestMCPReadBlocks_Pages(t *testing.T) {
	app := newToolboxApp(t)
	dir := t.TempDir()
	outsideAProject(t, dir)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "en.json"), []byte(`{"a":"One","b":"Two","c":"Three","d":"Four","e":"Five"}`), 0o600))
	session := editSession(t, app, "edit-test")

	var texts []string
	cursor := ""
	pages := 0
	for {
		args := map[string]any{"doc": "en.json", "limit": 2}
		if cursor != "" {
			args["cursor"] = cursor
		}
		var page mcpPage
		isErr, body := callEditTool(t, session, "read_blocks", args, &page)
		require.False(t, isErr, body)
		pages++
		assert.LessOrEqual(t, len(page.Blocks), 2)
		for _, b := range page.Blocks {
			texts = append(texts, b.Text)
		}
		if page.Next == "" {
			break
		}
		cursor = page.Next
		require.Less(t, pages, 10, "the pages end")
	}
	assert.Equal(t, []string{"One", "Two", "Three", "Four", "Five"}, texts)
	assert.Equal(t, 3, pages)

	var first mcpPage
	isErr, body := callEditTool(t, session, "read_blocks", map[string]any{"doc": "en.json", "limit": 2}, &first)
	require.False(t, isErr, body)
	require.NotEmpty(t, first.Next)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "en.json"), []byte(`{"a":"Uno","b":"Two","c":"Three","d":"Four","e":"Five"}`), 0o600))
	var res mcpResult
	isErr, body = callEditTool(t, session, "read_blocks", map[string]any{"doc": "en.json", "cursor": first.Next}, &res)
	assert.True(t, isErr, body)
	require.NotNil(t, res.Error, body)
	assert.Equal(t, change.CodeStale, res.Error.Code)
}

// A call names its project, and the tools read and write that project's
// documents by project-relative reference, whichever project the server
// started in.
func TestMCPEditTools_NamedProject(t *testing.T) {
	app, _ := contextOpsApp(t)
	app.InitRegistries()
	outsideAProject(t, t.TempDir())
	root := contextOpsProject(t, "mcp-named-project")
	session := editSession(t, app, "edit-test")

	var page mcpPage
	isErr, body := callEditTool(t, session, "read_blocks", map[string]any{"doc": "config/app.yaml", "project": filepath.Join(root, "config")}, &page)
	require.False(t, isErr, body)
	assert.Equal(t, "config/app.yaml", page.Doc)
	ref, _, _ := page.blockWith(t, "Goodbye")
	assert.Equal(t, "config/app.yaml", ref["doc"])

	var res mcpResult
	isErr, body = callEditTool(t, session, "read_blocks", map[string]any{"doc": "config/app.yaml"}, &res)
	assert.True(t, isErr, "outside the named project the document is not there: %s", body)
}

// describe_format says what a format supports: by name, or as the format kapi
// reads a document in. A format kapi does not know is not_found.
func TestMCPDescribeFormat(t *testing.T) {
	app := newToolboxApp(t)
	dir := t.TempDir()
	outsideAProject(t, dir)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "fr.po"), []byte("msgid \"Hello\"\nmsgstr \"Bonjour\"\n"), 0o600))
	session := editSession(t, app, "edit-test")

	type description struct {
		Format   string                     `json:"format"`
		Editions string                     `json:"editions"`
		Ops      map[string]json.RawMessage `json:"ops"`
	}
	var html description
	isErr, body := callEditTool(t, session, "describe_format", map[string]any{"format": "html"}, &html)
	require.False(t, isErr, body)
	assert.Equal(t, "html", html.Format)
	assert.Equal(t, "one-per-file", html.Editions)
	assert.NotEqual(t, "null", string(html.Ops["set_content"]))
	assert.Equal(t, "null", string(html.Ops["delete_block"]), "an operation the format refuses is null")

	var po description
	isErr, body = callEditTool(t, session, "describe_format", map[string]any{"doc": "fr.po"}, &po)
	require.False(t, isErr, body)
	assert.Equal(t, "po", po.Format)
	assert.Equal(t, "in-file", po.Editions)

	var res mcpResult
	isErr, body = callEditTool(t, session, "describe_format", map[string]any{"format": "no-such-format"}, &res)
	assert.True(t, isErr, body)
	require.NotNil(t, res.Error)
	assert.Equal(t, change.CodeNotFound, res.Error.Code)
}

// The input schema of apply_edits is the change-set schema, and the per-call
// project is its one addition.
func TestMCPApplyEdits_InputSchemaIsTheChangeSet(t *testing.T) {
	var want map[string]any
	require.NoError(t, json.Unmarshal(changeschema.Schema(), &want))
	body, err := json.Marshal(applyEditsInputSchema())
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal(body, &got))

	props := got["properties"].(map[string]any)
	require.Contains(t, props, "project")
	delete(props, "project")
	assert.Equal(t, want, got)
}

// A call whose project argument names no project is the tool's error result,
// naming the path the call sent, as every project-scoped tool reports it.
func TestMCPApplyEdits_AProjectThatIsNotThereIsAnErrorResult(t *testing.T) {
	app := newToolboxApp(t)
	outsideAProject(t, t.TempDir())
	session := editSession(t, app, "edit-test")
	nowhere := t.TempDir()

	res, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "apply_edits", Arguments: map[string]any{
		"project": nowhere,
		"ops":     []any{map[string]any{"op": "set_content", "at": map[string]any{"doc": "a.md", "block": "p"}, "if_match": "*", "text": "x"}},
	}})
	require.NoError(t, err, "a project that is not there is a tool error, not a protocol error")
	require.True(t, res.IsError)
	var text strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			text.WriteString(tc.Text)
		}
	}
	assert.Contains(t, text.String(), nowhere)
}

// languageProject writes a project of one JSON file per language, the source
// in source and a translation in each of targets, and returns its recipe.
// files holds each file's body by language.
func languageProject(t *testing.T, source string, targets []string, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	recipe := fmt.Sprintf("version: v1\nname: %s-project\ndefaults:\n  source_language: %s\n  target_languages: [%s]\n"+
		"collections:\n  - name: app\n    content:\n      - path: %s.json\n        target: \"{lang}.json\"\n",
		source, source, strings.Join(targets, ", "), source)
	require.NoError(t, os.WriteFile(filepath.Join(root, project.RecipeFileName), []byte(recipe), 0o644))
	for lang, body := range files {
		require.NoError(t, os.WriteFile(filepath.Join(root, lang+".json"), []byte(body), 0o644))
	}
	return filepath.Join(root, project.RecipeFileName)
}

func readFileIn(t *testing.T, root, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, name))
	require.NoError(t, err)
	return string(b)
}

// One server serves several projects, and each call reads and writes the
// project it resolves in that project's source language. An edition in any
// other language is a translation, whichever project the server started in
// and whichever project a review queue read last, so an edit to it lands in
// the translation's file and leaves the source file as it was.
func TestMCPApplyEdits_EachProjectInItsOwnSourceLanguage(t *testing.T) {
	const (
		apple = `{"a":"Apple"}`
		pomme = `{"a":"Pomme"}`
	)
	tests := []struct {
		name string
		// startInA starts the server in project A.
		startInA bool
		// queueB reads project B's review queue before the edit.
		queueB bool
		// call is the project the edit is sent to: a, b or c.
		call string
		doc  string
		// edition is the translation the edit addresses, which lives in
		// edited; source is the source file, which holds sourceBody.
		edition, edited, source, sourceBody string
		wording, before                     string
	}{
		{
			name: "a project translated into the start project's source language", startInA: true,
			call: "b", doc: "fr.json", edition: "en", edited: "en.json", source: "fr.json", sourceBody: pomme,
			wording: "Green apple", before: "Apple",
		},
		{
			name: "the start project after another project's review queue", startInA: true, queueB: true,
			call: "a", doc: "en.json", edition: "fr", edited: "fr.json", source: "en.json", sourceBody: apple,
			wording: "Pomme verte", before: "Pomme",
		},
		{
			name: "a project after another project's review queue, with no start project", queueB: true,
			call: "c", doc: "en.json", edition: "fr", edited: "fr.json", source: "en.json", sourceBody: apple,
			wording: "Pomme verte", before: "Pomme",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(project.NoProjectEnvVar, "1")
			app := &App{Encoding: "UTF-8"}
			app.InitRegistries()
			recipes := map[string]string{
				"a": languageProject(t, "en", []string{"fr"}, map[string]string{"en": apple, "fr": pomme}),
				"b": languageProject(t, "fr", []string{"en"}, map[string]string{"fr": pomme, "en": apple}),
				"c": languageProject(t, "en", []string{"fr"}, map[string]string{"en": apple, "fr": pomme}),
			}
			if tc.startInA {
				require.NoError(t, app.ResolveMCPProject(projectCommand(t.Context(), "mcp", recipes["a"])))
			}
			if tc.queueB {
				_, err := app.ReviewQueue(t.Context(), recipes["b"], "", ReviewQueueOptions{})
				require.NoError(t, err)
			}
			session := editSession(t, app, "language-test")
			named := recipes[tc.call]
			if tc.startInA && tc.call == "a" {
				named = ""
			}
			root := filepath.Dir(recipes[tc.call])

			var page mcpPage
			isErr, body := callEditTool(t, session, "read_blocks", map[string]any{"doc": tc.doc, "editions": []string{tc.edition}, "project": named}, &page)
			require.False(t, isErr, body)
			require.Len(t, page.Blocks, 1, body)
			ed, ok := page.Blocks[0].Editions[tc.edition]
			require.True(t, ok, "the read shows the %s translation: %s", tc.edition, body)
			assert.Equal(t, tc.before, ed.Text)

			var res mcpResult
			isErr, body = callEditTool(t, session, "apply_edits", map[string]any{"project": named, "ops": []any{map[string]any{
				"op": "set_content", "at": map[string]any{"doc": tc.doc, "block": "a", "edition": tc.edition}, "if_match": ed.Rev, "text": tc.wording,
			}}}, &res)
			require.False(t, isErr, body)
			assert.Equal(t, "applied", res.Status, body)
			assert.Equal(t, tc.sourceBody, readFileIn(t, root, tc.source), "the source file is left as it was")
			assert.Contains(t, readFileIn(t, root, tc.edited), tc.wording, "the edit lands in the translation's file")
		})
	}
}

// The change service writes no code comment, so a read or an edit of a
// source file whose comments kapi checks is refused as unsupported under the
// comment capability, and writes nothing.
func TestMCPEditTools_ACodeCommentIsRefusedByName(t *testing.T) {
	app := newToolboxApp(t)
	dir := t.TempDir()
	outsideAProject(t, dir)
	const src = "package demo\n\n// Parse reads the input.\nfunc Parse() {}\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "demo.go"), []byte(src), 0o600))
	session := editSession(t, app, "edit-test")

	var read mcpResult
	isErr, body := callEditTool(t, session, "read_blocks", map[string]any{"doc": "demo.go"}, &read)
	assert.True(t, isErr, body)
	require.NotNil(t, read.Error, body)
	assert.Equal(t, change.CodeUnsupported, read.Error.Code)
	assert.Equal(t, "comment", read.Error.Capability)

	var res mcpResult
	isErr, body = callEditTool(t, session, "apply_edits", map[string]any{"ops": []any{map[string]any{
		"op": "set_content", "at": map[string]any{"doc": "demo.go", "block": "func/Parse"}, "if_match": "r:0000000000000000",
		"text": "Parse reads the whole input.",
	}}}, &res)
	assert.True(t, isErr, body)
	assert.Equal(t, "refused", res.Status, body)
	require.Len(t, res.Ops, 1, body)
	require.NotNil(t, res.Ops[0].Error, body)
	assert.Equal(t, change.CodeUnsupported, res.Ops[0].Error.Code)
	assert.Equal(t, "comment", res.Ops[0].Error.Capability)
	assert.Equal(t, src, readFileIn(t, dir, "demo.go"))
}

// A pre-review carries its score: a decide with outcome advise that gives
// none is refused before anything is written, the content edit beside it
// included.
func TestMCPApplyEdits_APreReviewCarriesItsScore(t *testing.T) {
	t.Setenv(project.NoProjectEnvVar, "1")
	app := &App{Encoding: "UTF-8"}
	app.InitRegistries()
	recipe := languageProject(t, "en", []string{"fr"}, map[string]string{"en": `{"a":"Apple"}`, "fr": `{"a":"Pomme"}`})
	session := editSession(t, app, "review-agent")

	var page mcpPage
	isErr, body := callEditTool(t, session, "read_blocks", map[string]any{"doc": "en.json", "editions": []string{"fr"}, "project": recipe}, &page)
	require.False(t, isErr, body)
	require.Len(t, page.Blocks, 1, body)
	ed := page.Blocks[0].Editions["fr"]
	at := map[string]any{"doc": "en.json", "block": "a", "edition": "fr"}
	var res mcpResult
	isErr, body = callEditTool(t, session, "apply_edits", map[string]any{"project": recipe, "ops": []any{
		map[string]any{"op": "set_content", "at": at, "if_match": ed.Rev, "text": "Pomme verte"},
		map[string]any{"op": "decide", "at": at, "if_match": ed.Rev, "outcome": "advise", "reasons": []string{"reads well"}},
	}}, &res)
	assert.True(t, isErr, body)
	assert.Equal(t, "refused", res.Status, body)
	require.Len(t, res.Ops, 2, body)
	require.NotNil(t, res.Ops[1].Error, body)
	assert.Equal(t, change.CodeInvalid, res.Ops[1].Error.Code)
	assert.Equal(t, "score", res.Ops[1].Error.Field)
	assert.Equal(t, `{"a":"Pomme"}`, readFileIn(t, filepath.Dir(recipe), "fr.json"), "nothing is written")
}

// appliedWording counts the wording an agent wrote into a document's own
// edition, under the document as the result names it, so a document sent as
// ./x and as x counts once, and a translation's wording counts against no
// rule of the source.
func TestAppliedWording_KeysByTheCanonicalDocument(t *testing.T) {
	text := func(s string) *string { return &s }
	set := change.Set{Ops: []change.Op{
		{Kind: change.KindSetContent, At: change.Ref{Doc: "./docs/a.md", Block: "p1"}, Body: &change.SetContent{Text: text("Use the app")}},
		{Kind: change.KindReplaceText, At: change.Ref{Doc: "docs/a.md", Block: "p2"}, Body: &change.ReplaceText{Edits: []change.TextEdit{{Text: "use"}}}},
		{Kind: change.KindSetContent, At: change.Ref{Doc: "docs/fr/a.md", Block: "p1"}, Body: &change.SetContent{Text: text("Utilisez l'app")}},
		{Kind: change.KindSetContent, At: change.Ref{Doc: "docs/a.md", Block: "p3"}, Body: &change.SetContent{Text: text("refused")}},
	}}
	res := &change.Result{Ops: []change.OpResult{
		{Status: change.OpApplied, At: &change.Ref{Doc: "docs/a.md", Block: "p1"}},
		{Status: change.OpApplied, At: &change.Ref{Doc: "docs/a.md", Block: "p2"}},
		{Status: change.OpApplied, At: &change.Ref{Doc: "docs/a.md", Block: "p1", Edition: editionKey(t, "fr")}},
		{Status: change.OpRefused, At: &change.Ref{Doc: "docs/a.md", Block: "p3"}},
	}}
	assert.Equal(t, map[string][]string{"docs/a.md": {"Use the app", "use"}}, appliedWording(set, res))
}
