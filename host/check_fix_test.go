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
	"github.com/neokapi/neokapi/core/check"
)

// TestCheck_AFindingWithAReplacementCarriesItsFix pins the loop `kapi check
// --json | jq … | kapi apply -` runs: a term finding whose rule names a
// replacement carries the replace_text that applies it, guarded by the
// revision the check read, and that operation lands through the change
// service as it is, through JSON both ways.
func TestCheck_AFindingWithAReplacementCarriesItsFix(t *testing.T) {
	f := newCommitFixture(t)
	guide := filepath.Join(f.root, "docs", "guide.md")
	require.NoError(t, os.WriteFile(guide, []byte("# Guide\n\nWe utilize the widget every day.\n"), 0o644))

	report, err := f.app.ComputeCheck(f.command(t), []string{guide})
	require.NoError(t, err)
	var found *check.Diagnostic
	for i, d := range report.Findings {
		if d.Rule == "terms.vocabulary" && d.Location.Snippet == "utilize" {
			found = &report.Findings[i]
		}
	}
	require.NotNil(t, found, "the check finds the failing term: %+v", report.Findings)
	require.NotNil(t, found.Fix, "a finding whose rule names a replacement carries its fix")
	assert.Equal(t, change.KindReplaceText, found.Fix.Kind)

	raw, err := json.Marshal(report)
	require.NoError(t, err)
	var wire struct {
		Findings []struct {
			Rule string          `json:"rule"`
			Fix  json.RawMessage `json:"fix"`
		} `json:"findings"`
	}
	require.NoError(t, json.Unmarshal(raw, &wire))
	var fix json.RawMessage
	for _, d := range wire.Findings {
		if d.Rule == "terms.vocabulary" && len(d.Fix) > 0 {
			fix = d.Fix
		}
	}
	require.NotEmpty(t, fix)
	assert.Contains(t, string(fix), `"op":"replace_text"`)
	assert.Contains(t, string(fix), `"text":"use"`)

	set, err := change.Decode(bytes.NewReader([]byte(`{"ops": [` + string(fix) + `]}`)))
	require.NoError(t, err, "the fix decodes as a change set's operation")
	svc, err := f.app.ChangeService(t.Context(), ChangeServiceOptions{Project: f.recipe, Origin: "test"})
	require.NoError(t, err)
	res, err := svc.Apply(t.Context(), set, change.Actor{Kind: change.ActorPerson})
	require.NoError(t, err)
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
	got, err := os.ReadFile(guide)
	require.NoError(t, err)
	assert.Equal(t, "# Guide\n\nWe use the widget every day.\n", string(got))
}

// The loop `kapi check --json | jq '{ops: [.findings[].fix | select(.)]}' |
// kapi apply -` runs over a block with two findings: each fix names its words
// by position in the block the check read, and both land in one change set.
func TestCheck_TwoFixesOfOneBlockLandTogether(t *testing.T) {
	f := newCommitFixture(t)
	guide := filepath.Join(f.root, "docs", "guide.md")
	require.NoError(t, os.WriteFile(guide, []byte("# Guide\n\nWe utilize the widget and utilize it every day.\n"), 0o644))

	report, err := f.app.ComputeCheck(f.command(t), []string{guide})
	require.NoError(t, err)
	raw, err := json.Marshal(report)
	require.NoError(t, err)
	var wire struct {
		Findings []struct {
			Fix json.RawMessage `json:"fix"`
		} `json:"findings"`
	}
	require.NoError(t, json.Unmarshal(raw, &wire))
	var fixes []string
	for _, d := range wire.Findings {
		if len(d.Fix) > 0 {
			fixes = append(fixes, string(d.Fix))
		}
	}
	require.Len(t, fixes, 2, "one fix per finding: %s", raw)

	set, err := change.Decode(strings.NewReader(`{"ops": [` + strings.Join(fixes, ",") + `]}`))
	require.NoError(t, err)
	svc, err := f.app.ChangeService(t.Context(), ChangeServiceOptions{Project: f.recipe, Origin: "test"})
	require.NoError(t, err)
	res, err := svc.Apply(t.Context(), set, change.Actor{Kind: change.ActorPerson})
	require.NoError(t, err)
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
	got, err := os.ReadFile(guide)
	require.NoError(t, err)
	assert.Equal(t, "# Guide\n\nWe use the widget and use it every day.\n", string(got))
}

// fixFor runs `kapi check` on files in the fixture's project and returns the
// failing "utilize" finding.
func fixFor(t *testing.T, f commitFixture, files ...string) check.Diagnostic {
	t.Helper()
	report, err := f.app.ComputeCheck(f.command(t), files)
	require.NoError(t, err)
	for _, d := range report.Findings {
		if d.Rule == "terms.vocabulary" && d.Location.Snippet == "utilize" {
			return d
		}
	}
	require.FailNow(t, "the check finds the failing term", "%+v", report.Findings)
	return check.Diagnostic{}
}

// A fix names its document by its path from the project root, which is what
// kapi apply resolves, from whichever directory the check ran in.
func TestCheck_AFixNamesItsDocumentFromTheProjectRoot(t *testing.T) {
	f := newCommitFixture(t)
	guide := filepath.Join(f.root, "docs", "guide.md")
	require.NoError(t, os.WriteFile(guide, []byte("# Guide\n\nWe utilize the widget every day.\n"), 0o644))
	t.Chdir(filepath.Join(f.root, "docs"))

	d := fixFor(t, f, "guide.md")
	require.NotNil(t, d.Fix)
	assert.Equal(t, "docs/guide.md", d.Fix.At.Doc)

	svc, err := f.app.ChangeService(t.Context(), ChangeServiceOptions{Project: f.recipe, Origin: "test"})
	require.NoError(t, err)
	res, err := svc.Apply(t.Context(), change.Set{Ops: []change.Op{*d.Fix}}, change.Actor{Kind: change.ActorPerson})
	require.NoError(t, err)
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
	got, err := os.ReadFile(guide)
	require.NoError(t, err)
	assert.Equal(t, "# Guide\n\nWe use the widget every day.\n", string(got))
}

// The file of a translation is read by the change service as that
// translation, and the check held its words to the source language's rules:
// the finding is reported with no fix.
func TestCheck_ATranslationFileGetsNoFix(t *testing.T) {
	f := newCommitFixture(t)
	fr := filepath.Join(f.root, "docs", "fr", "guide.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(fr), 0o755))
	require.NoError(t, os.WriteFile(fr, []byte("# Guide\n\nNous utilize le widget.\n"), 0o644))

	d := fixFor(t, f, fr)
	assert.Nil(t, d.Fix, "a translation's file gets no fix")
}

// A replacement is plain text: words with a code among them get no fix, which
// would delete the code with them.
func TestCheck_WordsThatSpanFormattingGetNoFix(t *testing.T) {
	f := newCommitFixture(t)
	guide := filepath.Join(f.root, "docs", "guide.md")
	require.NoError(t, os.WriteFile(guide, []byte("# Guide\n\nWe util**ize** the widget every day.\n"), 0o644))

	d := fixFor(t, f, guide)
	assert.Nil(t, d.Fix, "the bold inside the words would go with them")
}

// The check reads an HTML page the way the change service does, so its fix
// names the revision and run positions an apply resolves. A paragraph with a
// run of spaces and character references is where the two readings parted: the
// fix was refused as stale although nothing had changed the block.
func TestCheck_AnHTMLFixLandsWhereTheCheckReadIt(t *testing.T) {
	f := newCommitFixture(t)
	page := filepath.Join(f.root, "docs", "page.html")
	const src = "<!DOCTYPE html>\n<html><head><title>Guide</title></head>\n<body>\n<p>We utilize   the <a href=\"x\">widget</a>&nbsp;and &eacute;.</p>\n<p>Keep   this    spacing.</p>\n</body></html>\n"
	require.NoError(t, os.WriteFile(page, []byte(src), 0o644))

	d := fixFor(t, f, page)
	require.NotNil(t, d.Fix)

	svc, err := f.app.ChangeService(t.Context(), ChangeServiceOptions{Project: f.recipe, Origin: "test"})
	require.NoError(t, err)
	read, err := svc.Read(t.Context(), change.ReadRequest{Doc: d.Fix.At.Doc})
	require.NoError(t, err)
	var rev string
	for _, b := range read.Blocks {
		if b.Ref.Block == d.Fix.At.Block {
			rev = b.Rev
		}
	}
	assert.Equal(t, rev, d.Fix.IfMatch, "the check and the change service compute one revision for the block")

	res, err := svc.Apply(t.Context(), change.Set{Ops: []change.Op{*d.Fix}}, change.Actor{Kind: change.ActorPerson})
	require.NoError(t, err)
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
	got, err := os.ReadFile(page)
	require.NoError(t, err)
	assert.Equal(t, strings.Replace(src, "We utilize   the", "We use   the", 1), string(got))
}

// A term written against a character reference is a whole word, as the page
// shows it: the check finds it, and its fix replaces the word and keeps the
// reference as the document spells it.
func TestCheck_ATermBesideACharacterReferenceIsFoundAndFixed(t *testing.T) {
	f := newCommitFixture(t)
	page := filepath.Join(f.root, "docs", "page.html")
	const src = "<html><head><title>Guide</title></head>\n<body>\n<p>We <b>use</b> the widget&nbsp;and   utilize&amp;more &eacute;.</p>\n<p>Say&nbsp;utilize now.</p>\n</body></html>\n"
	require.NoError(t, os.WriteFile(page, []byte(src), 0o644))

	report, err := f.app.ComputeCheck(f.command(t), []string{page})
	require.NoError(t, err)
	var ops []change.Op
	for _, d := range report.Findings {
		if d.Rule == "terms.vocabulary" && d.Location.Snippet == "utilize" {
			require.NotNil(t, d.Fix, "the word beside the reference carries its fix: %+v", d)
			ops = append(ops, *d.Fix)
		}
	}
	require.Len(t, ops, 2, "both uses are found: %+v", report.Findings)

	svc, err := f.app.ChangeService(t.Context(), ChangeServiceOptions{Project: f.recipe, Origin: "test"})
	require.NoError(t, err)
	res, err := svc.Apply(t.Context(), change.Set{Ops: ops}, change.Actor{Kind: change.ActorPerson})
	require.NoError(t, err)
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
	got, err := os.ReadFile(page)
	require.NoError(t, err)
	want := strings.Replace(strings.Replace(src, "and   utilize&amp;", "and   use&amp;", 1), "Say&nbsp;utilize", "Say&nbsp;use", 1)
	assert.Equal(t, want, string(got))
}
