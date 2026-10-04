package asciidoc

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/format/spec"
	"github.com/neokapi/neokapi/core/model"
)

// A block macro's target is markup. The reader used to read an
// `image::target[alt]` line as a paragraph, so an edit to the alt text could
// rewrite the image path, and the writer escaped the macro's `[` as an edit
// that added markup, which left a broken macro in the file.
func TestBlockMacro_ImageAltTextIsTheOnlyContent(t *testing.T) {
	tests := []struct {
		name string
		doc  string
		// texts is the edit text of every translatable block read.
		texts []string
	}{
		{
			name:  "an image's alt text is a block of its own",
			doc:   "= T\n\nimage::sunset.png[Sunset over the bay,300,200]\n",
			texts: []string{"T", "Sunset over the bay"},
		},
		{
			name:  "quoted alt text keeps its commas",
			doc:   "= T\n\nimage::sunset.png[\"Sunset, bay\",300]\n",
			texts: []string{"T", "Sunset, bay"},
		},
		{
			name:  "an image with only named attributes has no alt text",
			doc:   "= T\n\nimage::sunset.png[width=300]\n",
			texts: []string{"T"},
		},
		{
			name:  "an include is markup",
			doc:   "= T\n\ninclude::chapter.adoc[]\n",
			texts: []string{"T"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			parts, err := spec.ReadParts(NewReader(), []byte(tc.doc))
			require.NoError(t, err)
			var texts []string
			for _, p := range parts {
				if b, ok := p.Resource.(*model.Block); ok && b.Translatable {
					texts = append(texts, model.RunsEditText(b.SourceRuns()))
				}
			}
			assert.Equal(t, tc.texts, texts)
		})
	}
}

// writeAsciidoc reads doc through the skeleton round trip `kapi apply` uses,
// hands the block whose source edit text is from to edit, and writes the
// document for locale (empty for none). It returns what was written and the
// writer's error.
func writeAsciidoc(t *testing.T, doc, from string, locale model.LocaleID, edit func(*model.Block)) (string, error) {
	t.Helper()
	reader, writer := NewReader(), NewWriter()
	store, err := format.NewWiredSkeleton(reader, writer)
	require.NoError(t, err)
	require.NotNil(t, store)
	t.Cleanup(func() { _ = store.Close() })

	parts, err := spec.ReadParts(reader, []byte(doc))
	require.NoError(t, err)
	edited := 0
	for _, p := range parts {
		if b, ok := p.Resource.(*model.Block); ok && b.Translatable && model.RunsEditText(b.SourceRuns()) == from {
			edit(b)
			edited++
		}
	}
	require.Equal(t, 1, edited, "exactly one block reads %q", from)
	writer.SetLocale(locale)
	out, err := spec.WriteParts(writer, parts, []byte(doc))
	return string(out), err
}

// readBackTexts returns the edit text of every translatable block of doc.
func readBackTexts(t *testing.T, doc string) []string {
	t.Helper()
	parts, err := spec.ReadParts(NewReader(), []byte(doc))
	require.NoError(t, err)
	var texts []string
	for _, p := range parts {
		if b, ok := p.Resource.(*model.Block); ok && b.Translatable {
			texts = append(texts, model.RunsEditText(b.SourceRuns()))
		}
	}
	return texts
}

// imageAltCase is one edit or translation of an image's alternative text: the
// document, the alt text's edit text as read, what replaces it, the document
// written, and the alt text read back from it.
type imageAltCase struct {
	name, doc, from, to, want string
	// back is the edit text of every translatable block read back from want.
	back []string
	// translate writes to as the fr translation rather than as a source edit.
	translate bool
	refused   error
}

func runImageAltCases(t *testing.T, tests []imageAltCase) {
	t.Helper()
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var locale model.LocaleID
			edit := func(b *model.Block) { b.EditSourceRuns(model.ParseRunsEditText(tc.to, b.SourceRuns())) }
			if tc.translate {
				locale = "fr"
				edit = func(b *model.Block) { b.SetTargetRuns("fr", model.ParseRunsEditText(tc.to, b.SourceRuns())) }
			}
			out, err := writeAsciidoc(t, tc.doc, tc.from, locale, edit)
			if tc.refused != nil {
				require.ErrorIs(t, err, tc.refused)
				assert.Empty(t, out, "a refused write writes nothing")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, out)
			assert.Equal(t, tc.back, readBackTexts(t, out), "the written document reads back with the new alt text")
		})
	}
}

// An inline image macro reads the same way: its target is markup and its alt
// text is content, so an edit to the paragraph reaches the alt text and leaves
// the image path as written. The alt text sits in the macro's attribute list,
// which a comma, an `=` or a `]` would end or reshape, so an edit or a
// translation that holds one is written quoted, with `]` escaped. The alt text
// reads back as the document spells it, escapes included.
func TestInlineImageMacro(t *testing.T) {
	runImageAltCases(t, []imageAltCase{
		{
			name: "the alt text is content",
			doc:  "= T\n\nSee image:sunset.png[Sunset photo,40] here.\n",
			from: `See <x id="1"/>Sunset photo<x id="/1"/> here.`,
			to:   `See <x id="1"/>Dusk photo<x id="/1"/> now.`,
			want: "= T\n\nSee image:sunset.png[Dusk photo,40] now.\n",
			back: []string{"T", `See <x id="1"/>Dusk photo<x id="/1"/> now.`},
		},
		{
			name: "an image with no alt text is one code",
			doc:  "= T\n\nSee image:sunset.png[] here.\n",
			from: `See <x id="1/"/> here.`,
			to:   `See <x id="1/"/> now.`,
			want: "= T\n\nSee image:sunset.png[] now.\n",
			back: []string{"T", `See <x id="1/"/> now.`},
		},
		{
			name: "a word ending in image: is text",
			doc:  "= T\n\nAn myimage:x.png[y] word.\n",
			from: "An myimage:x.png[y] word.",
			to:   "An myimage:x.png[y] words.",
			want: "= T\n\nAn myimage:x.png[y] words.\n",
			back: []string{"T", "An myimage:x.png[y] words."},
		},
		{
			name: "a comma and a bracket in the alt text",
			doc:  "= T\n\nSee image:sunset.png[Sunset photo,40] here.\n",
			from: `See <x id="1"/>Sunset photo<x id="/1"/> here.`,
			to:   `See <x id="1"/>Sunset, photo]<x id="/1"/> here.`,
			want: "= T\n\nSee image:sunset.png[\"Sunset, photo\\]\",40] here.\n",
			back: []string{"T", `See <x id="1"/>Sunset, photo\]<x id="/1"/> here.`},
		},
		{
			name: "a bracket alone is escaped",
			doc:  "= T\n\nSee image:sunset.png[Sunset photo,40] here.\n",
			from: `See <x id="1"/>Sunset photo<x id="/1"/> here.`,
			to:   `See <x id="1"/>Sunset ]photo<x id="/1"/> here.`,
			want: "= T\n\nSee image:sunset.png[Sunset \\]photo,40] here.\n",
			back: []string{"T", `See <x id="1"/>Sunset \]photo<x id="/1"/> here.`},
		},
		{
			name:      "a translation with a comma",
			doc:       "= T\n\nSee image:sunset.png[Sunset photo,40] here.\n",
			from:      `See <x id="1"/>Sunset photo<x id="/1"/> here.`,
			to:        `Voir <x id="1"/>Coucher, photo<x id="/1"/> ici.`,
			translate: true,
			want:      "= T\n\nVoir image:sunset.png[\"Coucher, photo\",40] ici.\n",
			back:      []string{"T", `Voir <x id="1"/>Coucher, photo<x id="/1"/> ici.`},
		},
	})
}

// A block macro's alt text is a block of its own, written into the macro's
// attribute list. A comma or an `=` there would split the alt text into
// attributes, so an edit or a translation that holds one is written quoted,
// and a quote inside quotes is escaped. A block macro's list runs to the last
// `]` on its line, so a bracket needs nothing. A line break would end the
// macro, and is refused.
func TestBlockMacro_EditReachesOnlyTheAltText(t *testing.T) {
	runImageAltCases(t, []imageAltCase{
		{
			name: "unquoted alt text",
			doc:  "= T\n\nimage::sunset.png[Sunset over the bay,300]\n",
			from: "Sunset over the bay",
			to:   "Dusk over the bay",
			want: "= T\n\nimage::sunset.png[Dusk over the bay,300]\n",
			back: []string{"T", "Dusk over the bay"},
		},
		{
			name: "quoted alt text",
			doc:  "= T\n\nimage::sunset.png[\"Sunset, bay\"]\n",
			from: "Sunset, bay",
			to:   "Dusk, bay",
			want: "= T\n\nimage::sunset.png[\"Dusk, bay\"]\n",
			back: []string{"T", "Dusk, bay"},
		},
		{
			name: "a comma in unquoted alt text",
			doc:  "= T\n\nimage::sunset.png[Sunset over the bay,300]\n",
			from: "Sunset over the bay",
			to:   "Sunset, over the bay",
			want: "= T\n\nimage::sunset.png[\"Sunset, over the bay\",300]\n",
			back: []string{"T", "Sunset, over the bay"},
		},
		{
			name: "an equals sign in unquoted alt text",
			doc:  "= T\n\nimage::sunset.png[Sunset over the bay,300]\n",
			from: "Sunset over the bay",
			to:   "width=wide",
			want: "= T\n\nimage::sunset.png[\"width=wide\",300]\n",
			back: []string{"T", "width=wide"},
		},
		{
			name: "brackets in unquoted alt text",
			doc:  "= T\n\nimage::sunset.png[Sunset over the bay,300]\n",
			from: "Sunset over the bay",
			to:   "Sunset [over] the bay",
			want: "= T\n\nimage::sunset.png[Sunset [over] the bay,300]\n",
			back: []string{"T", "Sunset [over] the bay"},
		},
		{
			name: "a quote in quoted alt text",
			doc:  "= T\n\nimage::sunset.png[\"Sunset, bay\"]\n",
			from: "Sunset, bay",
			to:   `Sunset "bay"`,
			want: "= T\n\nimage::sunset.png[\"Sunset \\\"bay\\\"\"]\n",
			back: []string{"T", `Sunset \"bay\"`},
		},
		{
			name:      "a translation with a comma",
			doc:       "= T\n\nimage::sunset.png[Sunset over the bay,300]\n",
			from:      "Sunset over the bay",
			to:        "Coucher, sur la baie",
			translate: true,
			want:      "= T\n\nimage::sunset.png[\"Coucher, sur la baie\",300]\n",
			back:      []string{"T", "Coucher, sur la baie"},
		},
		{
			name:    "a line break",
			doc:     "= T\n\nimage::sunset.png[Sunset over the bay,300]\n",
			from:    "Sunset over the bay",
			to:      "Sunset\nover the bay",
			refused: ErrAltUnwritable,
		},
	})
}
