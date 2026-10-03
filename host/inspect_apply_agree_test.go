package host

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/model"
)

// inspectOutput runs `kapi inspect --jsonl` on path and returns the raw output.
func inspectOutput(t *testing.T, app *App, path string, project ...string) string {
	t.Helper()
	cmd := NewEnvCommand(t.Context(), "inspect")
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	require.NoError(t, app.RunInspect(t.Context(), cmd, []string{path}, "jsonl", project), stderr.String())
	return stdout.String()
}

const pricingPage = `<!DOCTYPE html>
<html lang="en">
<head><meta charset="utf-8"><title>Pricing</title></head>
<body>
<h1>Pricing</h1>
<p>Our plans start at ten dollars a month. Read the <a href="https://old.example.com/terms">terms of service</a> first.</p>
<p><img src="chart.png" alt="chart"> Prices rose last year.</p>
</body>
</html>
`

// `kapi inspect` is the read an edit is addressed from, so every reference and
// revision it prints is one `kapi apply` resolves, and --dry-run previews the
// change the write then makes. An inline element's translatable attribute (an
// image's alt) and a character reference are where reads can part: the write
// numbers the attribute after its paragraph and keeps the reference as an
// inline code, which inspect shows as its character.
func TestInspectAddressesTheBlocksApplyWrites(t *testing.T) {
	noProject(t)
	tests := []struct {
		name     string
		src      string
		readText string // the inspect record the edit addresses, by its text
		edit     func(text string) string
		want     string // a substring of the written file
		keep     string // a substring the write must leave alone
		guard    bool   // the edit is refused as a guard and writes nothing
	}{
		{
			name:     "an image's alt text",
			src:      pricingPage,
			readText: "chart",
			edit:     func(string) string { return "Bar chart of prices" },
			want:     `<img src="chart.png" alt="Bar chart of prices"> Prices rose last year.`,
			keep:     `Read the <a href="https://old.example.com/terms">terms of service</a> first.`,
		},
		{
			name:     "the paragraph that holds the image",
			src:      pricingPage,
			readText: `<x id="1/"/> Prices rose last year.`,
			edit:     func(s string) string { return strings.Replace(s, "rose", "fell", 1) },
			want:     `<img src="chart.png" alt="chart"> Prices fell last year.`,
			keep:     `<title>Pricing</title>`,
		},
		{
			name:     "a paragraph with character references",
			src:      "<html><body><p>Fish &amp; chips &lt;3</p></body></html>\n",
			readText: `Fish & chips <3`,
			edit:     func(s string) string { return strings.Replace(s, "chips", "fries", 1) },
			want:     `<p>Fish &amp; fries &lt;3</p>`,
			keep:     `<html><body>`,
		},
		{
			// The HTML reader keeps each character reference as an inline
			// code it does not mark deletable, so a rewrite that drops them
			// is refused as a guard and writes nothing.
			name:     "a rewrite that drops the character references",
			src:      "<html><body><p>Don&rsquo;t pay&nbsp;more &mdash; it&#39;s &copy; 2026</p></body></html>\n",
			readText: "Don’t pay more — it's © 2026",
			edit:     func(string) string { return "Pay less today." },
			guard:    true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			app := newToolboxApp(t)
			path := filepath.Join(t.TempDir(), "page.html")
			require.NoError(t, os.WriteFile(path, []byte(tc.src), 0o600))

			var rec *readRecord
			recs := inspectJSONL(t, app, path)
			for i := range recs {
				if recs[i].Text == tc.readText {
					rec = &recs[i]
				}
			}
			require.NotNil(t, rec, "inspect shows a block reading %q: %+v", tc.readText, recs)
			body := changeSetOf(t, map[string]any{"op": "set_content", "at": rec.Ref, "if_match": rec.Rev, "text": tc.edit(rec.Text)})

			preview, err := applyJSON(t, app, NewEnvCommand(t.Context(), "apply"), body, ApplyOptions{DryRun: true})
			if tc.guard {
				assert.Equal(t, ExitGate, ExitCode(nil, err))
				require.NotNil(t, preview.Ops[0].Error)
				assert.Equal(t, change.CodeGuard, preview.Ops[0].Error.Code)
				assert.Equal(t, change.SubcodeCodesChanged, preview.Ops[0].Error.Subcode)
				_, err = applyJSON(t, app, NewEnvCommand(t.Context(), "apply"), body, ApplyOptions{})
				assert.Equal(t, ExitGate, ExitCode(nil, err))
				got, rerr := os.ReadFile(path)
				require.NoError(t, rerr)
				assert.Equal(t, tc.src, string(got))
				return
			}
			require.NoError(t, err)
			assert.Equal(t, change.OpPreviewed, preview.Ops[0].Status, "--dry-run previews the inspected block: %+v", preview.Ops[0].Error)
			require.Len(t, preview.Docs, 1)
			assert.Contains(t, preview.Docs[0].Diff, tc.want, "the preview shows the change the write makes")
			unchanged, err := os.ReadFile(path)
			require.NoError(t, err)
			assert.Equal(t, tc.src, string(unchanged), "--dry-run writes nothing")

			written, err := applyJSON(t, app, NewEnvCommand(t.Context(), "apply"), body, ApplyOptions{})
			require.NoError(t, err)
			assert.Equal(t, change.OpApplied, written.Ops[0].Status, "the write changes the block --dry-run previewed")
			got, err := os.ReadFile(path)
			require.NoError(t, err)
			assert.Contains(t, string(got), tc.want)
			assert.Contains(t, string(got), tc.keep)
		})
	}
}

// The streamed form spells a record the way the array form does: a
// placeholder token keeps its angle brackets rather than encoding/json's
// HTML-safe escapes for '<', '>' and '&'.
func TestInspectJSONLKeepsAngleBracketsReadable(t *testing.T) {
	noProject(t)
	app := newToolboxApp(t)
	path := filepath.Join(t.TempDir(), "page.html")
	require.NoError(t, os.WriteFile(path, []byte(pricingPage), 0o600))

	out := inspectOutput(t, app, path)

	assert.Contains(t, out, `<x id=\"1\"/>terms of service<x id=\"/1\"/>`)
	assert.NotContains(t, out, `\`+"u003c")
	assert.NotContains(t, out, `\`+"u0026")
}

// A read record carries what an edit needs: the reference and revision, the
// text, each inline code with its attributes, and the operations the block
// accepts, with its structural role.
func TestInspectPrintsReadRecords(t *testing.T) {
	noProject(t)
	t.Chdir(t.TempDir())
	app := newToolboxApp(t)
	require.NoError(t, os.WriteFile("page.html", []byte(pricingPage), 0o600))

	var rec struct {
		Ref   change.Ref                 `json:"ref"`
		Rev   string                     `json:"rev"`
		Text  string                     `json:"text"`
		Codes map[string]change.CodeRead `json:"codes"`
		Ops   []change.Kind              `json:"ops"`
		Role  string                     `json:"role"`
		ID    string                     `json:"id"`
		Hash  string                     `json:"content_hash"`
	}
	for line := range strings.SplitSeq(strings.TrimSpace(inspectOutput(t, app, "page.html")), "\n") {
		require.NoError(t, json.Unmarshal([]byte(line), &rec))
		if strings.HasPrefix(rec.Text, "Our plans") {
			break
		}
	}
	assert.Equal(t, change.Ref{Doc: "page.html", Block: "p", Edition: model.EditionKey{Locale: "en"}}, rec.Ref,
		"--source-lang names the document's own edition")
	assert.Regexp(t, `^r:[0-9a-f]{16}$`, rec.Rev)
	require.Contains(t, rec.Codes, "1")
	assert.Equal(t, "link:hyperlink", rec.Codes["1"].Type)
	assert.Equal(t, "https://old.example.com/terms", rec.Codes["1"].Attrs["href"])
	assert.Contains(t, rec.Ops, change.KindReplaceText)
	assert.Equal(t, "paragraph", rec.Role)
	assert.Empty(t, rec.ID, "the reader's own id is not printed")
	assert.Empty(t, rec.Hash, "the content hash is not an edit's precondition")
}

// `kapi inspect --render` renders a block in another format. A character
// reference the HTML reader keeps as an inline code is a character there, and
// each format spells it its own way.
func TestInspectProjectsCharacterReferences(t *testing.T) {
	noProject(t)
	app := newToolboxApp(t)
	path := filepath.Join(t.TempDir(), "amp.html")
	require.NoError(t, os.WriteFile(path, []byte("<p>Fish &amp; chips &lt;3</p>\n"), 0o600))

	var rec inspectRecord
	require.NoError(t, json.Unmarshal([]byte(inspectOutput(t, app, path, "markdown", "html", "asciidoc")), &rec))

	assert.Equal(t, "Fish & chips <3", rec.Text)
	assert.Equal(t, map[string]string{
		"markdown": `Fish & chips \<3`,
		"html":     "<p>Fish &amp; chips &lt;3</p>",
		"asciidoc": "Fish & chips <3",
	}, rec.Projected)
}

// Every block `kapi inspect` lists is one `kapi apply` resolves. Each record
// that accepts set_content, sent back as it was read, is unchanged, so the
// inspect and apply loop settles. The code block of a Markdown file is listed
// with no operations, and an edit of it is refused as unsupported, exits 3 and
// leaves the file as it was.
func TestApplyResolvesEveryBlockInspectLists(t *testing.T) {
	noProject(t)
	const doc = "# Guide\n\nRun the tool.\n\n```go\nfmt.Println(\"hi\")\n```\n"
	app := newToolboxApp(t)
	path := filepath.Join(t.TempDir(), "doc.md")
	require.NoError(t, os.WriteFile(path, []byte(doc), 0o600))

	var recs []inspectRecord
	for line := range strings.SplitSeq(strings.TrimSpace(inspectOutput(t, app, path)), "\n") {
		var r inspectRecord
		require.NoError(t, json.Unmarshal([]byte(line), &r))
		recs = append(recs, r)
	}
	var ops []map[string]any
	var code *inspectRecord
	for i, r := range recs {
		if r.Role == "code" {
			code = &recs[i]
			continue
		}
		require.True(t, slices.Contains(r.Ops, change.KindSetContent), "%+v", r)
		ops = append(ops, map[string]any{"op": "set_content", "at": r.Ref, "if_match": r.Rev, "text": r.Text})
	}
	require.NotNil(t, code, "kapi inspect lists the code block: %+v", recs)
	assert.Empty(t, code.Ops, "a code block takes no edit")

	res, err := applyJSON(t, app, NewEnvCommand(t.Context(), "apply"), changeSetOf(t, ops...), ApplyOptions{})
	require.NoError(t, err, "the loop settles")
	for _, op := range res.Ops {
		assert.Equal(t, change.OpUnchanged, op.Status)
	}

	edit := changeSetOf(t, map[string]any{"op": "set_content", "at": code.Ref, "if_match": code.Rev, "text": `fmt.Println("bye")`})
	res, err = applyJSON(t, app, NewEnvCommand(t.Context(), "apply"), edit, ApplyOptions{})
	assert.Equal(t, ExitGate, ExitCode(nil, err))
	require.NotNil(t, res.Ops[0].Error)
	assert.Equal(t, change.CodeUnsupported, res.Ops[0].Error.Code)
	got, rerr := os.ReadFile(path)
	require.NoError(t, rerr)
	assert.Equal(t, doc, string(got), "the file is as it was")
}
