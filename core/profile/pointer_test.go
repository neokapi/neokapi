package profile

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderVoicePointer(t *testing.T) {
	tests := []struct {
		name    string
		pointer VoicePointer
		want    []string
		absent  []string
	}{
		{
			name:    "named voice",
			pointer: VoicePointer{Name: "Professional B2B"},
			want: []string{
				VoicePointerStartLine + "\n",
				"## Voice\n",
				"This project's voice, Professional B2B, is held by kapi and applies to any prose written here.",
				"Retrieve what is in force before writing, with `kapi voice guide`.",
				VoicePointerEnd + "\n",
			},
			absent: []string{"<path>"},
		},
		{
			name:    "unnamed binding",
			pointer: VoicePointer{},
			want: []string{
				"This project's voice is held by kapi and applies to any prose written here.",
				"with `kapi voice guide`.",
			},
			absent: []string{"voice, ,"},
		},
		{
			name:    "per-file voices",
			pointer: VoicePointer{Name: "House", PerFile: true},
			want: []string{
				"This project's voice, House, is held by kapi",
				"Some collections carry a voice of their own",
				"`kapi voice guide <path>`",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RenderVoicePointer(tt.pointer)
			for _, w := range tt.want {
				assert.Contains(t, got, w)
			}
			for _, a := range tt.absent {
				assert.NotContains(t, got, a)
			}
			assert.True(t, strings.HasSuffix(got, "\n"), "the section ends in a newline")
			assert.NotContains(t, got, "—", "no em dashes in a file a person reads")
			// Nothing about how to write: the pointer names the command and stops.
			assert.NotContains(t, strings.ToLower(got), "tone")
		})
	}
}

func TestUpsertVoicePointer(t *testing.T) {
	section := RenderVoicePointer(VoicePointer{Name: "One"})
	updated := RenderVoicePointer(VoicePointer{Name: "Two"})

	tests := []struct {
		name string
		doc  string
		sec  string
		want string
	}{
		{
			name: "empty document becomes the section",
			doc:  "",
			sec:  section,
			want: section,
		},
		{
			name: "appended after a blank line",
			doc:  "# Project\n\nHand-written.\n",
			sec:  section,
			want: "# Project\n\nHand-written.\n\n" + section,
		},
		{
			name: "a document without a trailing newline gets one",
			doc:  "# Project\n\nHand-written.",
			sec:  section,
			want: "# Project\n\nHand-written.\n\n" + section,
		},
		{
			name: "a document already ending in a blank line is not padded again",
			doc:  "# Project\n\n",
			sec:  section,
			want: "# Project\n\n" + section,
		},
		{
			name: "an existing section is replaced in place",
			doc:  "# Project\n\n" + section + "\n## After\n\nKept.\n",
			sec:  updated,
			want: "# Project\n\n" + updated + "\n## After\n\nKept.\n",
		},
		{
			name: "a section written by an earlier version with a different hint is still found",
			doc:  "Before.\n\n<!-- kapi:voice -->\nold body\n" + VoicePointerEnd + "\nAfter.\n",
			sec:  section,
			want: "Before.\n\n" + section + "After.\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := UpsertVoicePointer([]byte(tt.doc), tt.sec)
			require.NoError(t, err)
			assert.Equal(t, tt.want, string(got))

			// Idempotent: a second upsert of the same section changes nothing.
			again, err := UpsertVoicePointer(got, tt.sec)
			require.NoError(t, err)
			assert.Equal(t, string(got), string(again))
		})
	}
}

func TestUpsertVoicePointer_RefusesAnUnterminatedSection(t *testing.T) {
	doc := "# Project\n\n" + VoicePointerStartLine + "\nbody without an end\n"
	_, err := UpsertVoicePointer([]byte(doc), RenderVoicePointer(VoicePointer{}))
	require.ErrorIs(t, err, ErrVoicePointerUnterminated)
}

func TestRemoveVoicePointer(t *testing.T) {
	section := RenderVoicePointer(VoicePointer{Name: "One"})

	tests := []struct {
		name    string
		doc     string
		want    string
		removed bool
	}{
		{
			name:    "no section leaves the document alone",
			doc:     "# Project\n",
			want:    "# Project\n",
			removed: false,
		},
		{
			name:    "an appended section goes with its separating blank line",
			doc:     "# Project\n\nHand-written.\n\n" + section,
			want:    "# Project\n\nHand-written.\n",
			removed: true,
		},
		{
			name:    "a section in the middle keeps what follows",
			doc:     "# Project\n\n" + section + "\n## After\n",
			want:    "# Project\n\n## After\n",
			removed: true,
		},
		{
			name:    "a document that was only the section becomes empty",
			doc:     section,
			want:    "",
			removed: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, removed, err := RemoveVoicePointer([]byte(tt.doc))
			require.NoError(t, err)
			assert.Equal(t, tt.removed, removed)
			assert.Equal(t, tt.want, string(got))
		})
	}
}
