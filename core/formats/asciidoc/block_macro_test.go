package asciidoc

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
					texts = append(texts, model.RunsEditText(b.Source))
				}
			}
			assert.Equal(t, tc.texts, texts)
		})
	}
}

func TestBlockMacro_EditReachesOnlyTheAltText(t *testing.T) {
	tests := []struct {
		name, doc, from, to, want string
	}{
		{
			name: "unquoted alt text",
			doc:  "= T\n\nimage::sunset.png[Sunset over the bay,300]\n",
			from: "Sunset over the bay",
			to:   "Dusk over the bay",
			want: "= T\n\nimage::sunset.png[Dusk over the bay,300]\n",
		},
		{
			name: "quoted alt text",
			doc:  "= T\n\nimage::sunset.png[\"Sunset, bay\"]\n",
			from: "Sunset, bay",
			to:   "Dusk, bay",
			want: "= T\n\nimage::sunset.png[\"Dusk, bay\"]\n",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, editAsciidocSource(t, tc.doc, tc.from, tc.to))
		})
	}
}
