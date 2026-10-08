package agentrules

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const section = StartLine + "\n## Writing rules\n\n- Space, not \"Workspace\"\n" + End + "\n"

func TestUpsert(t *testing.T) {
	tests := []struct {
		name string
		doc  string
		want string
	}{
		{name: "empty file", doc: "", want: section},
		{
			name: "appended after the person's text",
			doc:  "# Project\n\nOur notes.",
			want: "# Project\n\nOur notes.\n\n" + section,
		},
		{
			name: "replaced in place",
			doc:  "# Project\n\n" + StartLine + "\nold\n" + End + "\n\nMore notes.\n",
			want: "# Project\n\n" + section + "\nMore notes.\n",
		},
		{
			name: "takes the place of the voice pointer",
			doc: "# Project\n\n<!-- kapi:voice (managed by kapi; refreshed by 'kapi voice pointer') -->\n## Voice\n\nold\n" +
				"<!-- /kapi:voice -->\n\nAfter.\n",
			want: "# Project\n\n" + section + "\nAfter.\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Upsert([]byte(tt.doc), section)
			require.NoError(t, err)
			assert.Equal(t, tt.want, string(got))
		})
	}
}

func TestUpsertRefusesAnUnterminatedSection(t *testing.T) {
	_, err := Upsert([]byte("# P\n\n"+StartLine+"\nno end\n"), section)
	require.ErrorIs(t, err, ErrUnterminated)
}

func TestRemove(t *testing.T) {
	tests := []struct {
		name    string
		doc     string
		want    string
		removed bool
	}{
		{name: "nothing to remove", doc: "# P\n", want: "# P\n"},
		{name: "section alone", doc: section, want: "", removed: true},
		{name: "keeps the person's text", doc: "# P\n\nNotes.\n\n" + section, want: "# P\n\nNotes.\n", removed: true},
		{
			name:    "voice pointer",
			doc:     "# P\n\n<!-- kapi:voice -->\n## Voice\n<!-- /kapi:voice -->\n",
			want:    "# P\n",
			removed: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, removed, err := Remove([]byte(tt.doc))
			require.NoError(t, err)
			assert.Equal(t, tt.want, string(got))
			assert.Equal(t, tt.removed, removed)
		})
	}
}

func TestImportsAgents(t *testing.T) {
	assert.True(t, ImportsAgents([]byte("# P\n\n@AGENTS.md\n")))
	assert.True(t, ImportsAgents([]byte("  @AGENTS.md  ")))
	assert.False(t, ImportsAgents([]byte("See @AGENTS.md for more.\n")))
}
