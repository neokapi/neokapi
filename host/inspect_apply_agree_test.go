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

	"github.com/neokapi/neokapi/core/structrec"
)

// inspectRecords runs `kapi inspect --jsonl` on path and returns the raw
// output and the decoded records.
func inspectRecords(t *testing.T, app *App, path string) (string, []structrec.Record) {
	t.Helper()
	cmd := NewEnvCommand(t.Context(), "inspect")
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	require.NoError(t, app.RunInspect(t.Context(), cmd, []string{path}, "jsonl", nil), stderr.String())
	var recs []structrec.Record
	dec := json.NewDecoder(bytes.NewReader(stdout.Bytes()))
	for dec.More() {
		var r structrec.Record
		require.NoError(t, dec.Decode(&r))
		recs = append(recs, r)
	}
	return stdout.String(), recs
}

// applyDiffJSON runs `kapi apply --diff --json` and returns the report and the
// diff a person reads on stderr.
func applyDiffJSON(t *testing.T, app *App, entries []map[string]any) (applyOutput, string) {
	t.Helper()
	var lines bytes.Buffer
	for _, e := range entries {
		b, err := json.Marshal(e)
		require.NoError(t, err)
		lines.Write(b)
		lines.WriteByte('\n')
	}
	changeset := filepath.Join(t.TempDir(), "edits.jsonl")
	require.NoError(t, os.WriteFile(changeset, lines.Bytes(), 0o600))
	cmd := NewEnvCommand(t.Context(), "apply")
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	_ = app.RunApply(cmd, changeset, true, "", true)
	var out applyOutput
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &out), "stdout: %s", stdout.String())
	return out, stderr.String()
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

// `kapi inspect` is the read an edit is addressed from, so every id and
// content_hash it prints must be one `kapi apply` resolves, and `--diff` must
// preview the block the write then changes. An inline element's translatable
// attribute (an image's alt) and a character reference are where the two
// reads used to part: the write numbered the attribute after its paragraph and
// kept the reference as an inline code.
func TestInspectAddressesTheBlocksApplyWrites(t *testing.T) {
	tests := []struct {
		name     string
		src      string
		readText string // the inspect record the edit addresses, by its text
		edit     func(text string) string
		want     string // a substring of the written file
		keep     string // a substring the write must leave alone
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
			readText: `Fish <x id="1/"/> chips <x id="2/"/>3`,
			edit:     func(s string) string { return strings.Replace(s, "chips", "fries", 1) },
			want:     `<p>Fish &amp; fries &lt;3</p>`,
			keep:     `<html><body>`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			app := newToolboxApp(t)
			path := filepath.Join(t.TempDir(), "page.html")
			require.NoError(t, os.WriteFile(path, []byte(tc.src), 0o600))

			_, recs := inspectRecords(t, app, path)
			var rec *structrec.Record
			for i := range recs {
				if recs[i].Text == tc.readText {
					rec = &recs[i]
				}
			}
			require.NotNil(t, rec, "inspect shows a block reading %q: %+v", tc.readText, recs)
			entries := []map[string]any{{
				"kind": "content", "file": path, "id": rec.ID,
				"content_hash": rec.ContentHash, "text": tc.edit(rec.Text),
			}}

			preview, diff := applyDiffJSON(t, app, entries)
			assert.Equal(t, []string{rec.ID}, preview.Content.Applied, "--diff previews the inspected block")
			assert.Contains(t, diff, "page.html:"+rec.ID+" (before)", "a hunk is labelled with the block id")
			unchanged, err := os.ReadFile(path)
			require.NoError(t, err)
			assert.Equal(t, tc.src, string(unchanged), "--diff writes nothing")

			written := applyViaCLI(t, app, entries)
			assert.Equal(t, []string{rec.ID}, written.Content.Applied, "the write changes the block --diff previewed")
			assert.Empty(t, written.Content.Stale)
			assert.Empty(t, written.Content.GuardFailed)
			got, err := os.ReadFile(path)
			require.NoError(t, err)
			assert.Contains(t, string(got), tc.want)
			assert.Contains(t, string(got), tc.keep)
		})
	}
}
