package model

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func txt(s string) Run { return Run{Text: &TextRun{Text: s}} }

func entityRun(id, data string) Run {
	return Run{Ph: &PlaceholderRun{ID: id, Type: "code:entity", SubType: "html:entity", Data: data}}
}

func TestCharacterReference(t *testing.T) {
	tests := []struct {
		name   string
		ph     *PlaceholderRun
		want   string
		wantOK bool
	}{
		{name: "named", ph: &PlaceholderRun{Data: "&amp;"}, want: "&", wantOK: true},
		{name: "named beyond ASCII", ph: &PlaceholderRun{Data: "&rsquo;"}, want: "’", wantOK: true},
		{name: "decimal", ph: &PlaceholderRun{Data: "&#39;"}, want: "'", wantOK: true},
		{name: "hexadecimal", ph: &PlaceholderRun{Data: "&#x3C;"}, want: "<", wantOK: true},
		{name: "an unknown name", ph: &PlaceholderRun{Data: "&nosuchname;"}},
		{name: "a reference inside other markup", ph: &PlaceholderRun{Data: "<b>&amp;</b>"}},
		{name: "a tag", ph: &PlaceholderRun{Data: "<br/>"}},
		{name: "nil", ph: nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := CharacterReference(tc.ph)
			assert.Equal(t, tc.wantOK, ok)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestRunsEditText(t *testing.T) {
	tests := []struct {
		name string
		runs []Run
		want string
	}{
		{
			name: "a reference reads as its character",
			runs: []Run{txt("Fish "), entityRun("1", "&amp;"), txt(" chips "), entityRun("2", "&lt;"), txt("3")},
			want: "Fish & chips <3",
		},
		{
			name: "other codes stay tokens",
			runs: []Run{
				txt("Read the "),
				{PcOpen: &PcOpenRun{ID: "1", Data: `<a href="t">`}},
				txt("terms"),
				{PcClose: &PcCloseRun{ID: "1", Data: "</a>"}},
				entityRun("2", "&nbsp;"),
				txt("now"),
			},
			want: "Read the <x id=\"1\"/>terms<x id=\"/1\"/> now",
		},
		{
			name: "inside a plural branch",
			runs: []Run{{Plural: &PluralRun{Forms: map[PluralForm][]Run{
				PluralOther: {txt("a "), entityRun("1", "&mdash;"), txt(" b")},
			}}}},
			want: "a — b",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, RunsEditText(tc.runs))
		})
	}
}

func TestParseRunsEditText(t *testing.T) {
	src := []Run{
		txt("Don"), entityRun("1", "&rsquo;"), txt("t pay"), entityRun("2", "&nbsp;"),
		txt("more "), entityRun("3", "&mdash;"), txt(" read the "),
		{PcOpen: &PcOpenRun{ID: "4", Data: `<a href="t">`}},
		txt("terms"),
		{PcClose: &PcCloseRun{ID: "4", Data: "</a>"}},
		txt(" & go"),
	}
	tests := []struct {
		name string
		edit string
		// want is the edit's runs rendered with each code's data, which shows
		// which characters came back as the reference that stood for them.
		want string
	}{
		{
			name: "the unchanged text gives back every reference",
			edit: RunsEditText(src),
			want: RenderRunsWithData(src),
		},
		{
			name: "a word changed elsewhere keeps the references around it",
			edit: "Don’t pay less — read the <x id=\"4\"/>terms<x id=\"/4\"/> & go",
			want: `Don&rsquo;t pay&nbsp;less &mdash; read the <a href="t">terms</a> & go`,
		},
		{
			name: "a rewrite drops the references it no longer holds",
			edit: "Pay less <x id=\"4\"/>today<x id=\"/4\"/>.",
			want: `Pay less <a href="t">today</a>.`,
		},
		{
			name: "a character the edit adds is text",
			edit: "Don’t pay — <x id=\"4\"/>x<x id=\"/4\"/> ’",
			want: "Don&rsquo;t pay &mdash; <a href=\"t\">x</a> ’",
		},
		{
			name: "a token is never taken for a kept character",
			edit: "<x id=\"4\"/>terms<x id=\"/4\"/>",
			want: `<a href="t">terms</a>`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseRunsEditText(tc.edit, src)
			assert.Equal(t, tc.want, RenderRunsWithData(got))
			assert.Equal(t, tc.edit, RunsEditText(got), "the parsed runs read back as the edit")
		})
	}
}

func TestParseRunsEditText_NoReferencesIsPlaceholderParse(t *testing.T) {
	src := []Run{txt("a "), {Ph: &PlaceholderRun{ID: "1", Data: "<br/>"}}, txt(" b")}
	edit := "c <x id=\"1/\"/> d"
	assert.Equal(t, ParseRunsPlaceholderText(edit, src), ParseRunsEditText(edit, src))
}

func TestEditSourceKeepsTheSourceAsRead(t *testing.T) {
	read := []Run{txt("Hello "), entityRun("1", "&amp;"), txt(" bye")}
	b := &Block{ID: "b1"}
	b.SetSourceRuns(read)

	runs, edited := b.SourceAsRead()
	assert.False(t, edited, "a block no edit touched")
	assert.Equal(t, read, runs)

	b.EditSourceText("first")
	b.EditSourceRuns([]Run{txt("second")})
	runs, edited = b.SourceAsRead()
	assert.True(t, edited)
	assert.Equal(t, read, runs, "the first edit keeps the source as read")

	b.EditSourceRuns(read)
	_, edited = b.SourceAsRead()
	assert.False(t, edited, "an edit back to the bytes as read is no edit")
}

func TestRenderRunsWith(t *testing.T) {
	runs := []Run{
		txt("a<b"),
		{Ph: &PlaceholderRun{ID: "1", Type: "fmt:code", Data: "<br/>"}},
		{Plural: &PluralRun{Forms: map[PluralForm][]Run{PluralOther: {txt("c&d")}}}},
	}
	var codes []string
	var b strings.Builder
	RenderRunsWith(&b, runs, &RunRenderer{
		Text: func(b *strings.Builder, s string) {
			b.WriteString(strings.NewReplacer("<", "&lt;", "&", "&amp;").Replace(s))
		},
		Code: func(r Run) { codes = append(codes, string(r.Kind())) },
	})
	assert.Equal(t, "a&lt;b<br/>c&amp;d", b.String())
	assert.Equal(t, []string{"ph"}, codes)

	var plain strings.Builder
	RenderRunsWith(&plain, runs, nil)
	require.Equal(t, RenderRunsWithData(runs), plain.String())
}
