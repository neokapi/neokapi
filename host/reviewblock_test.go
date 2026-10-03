package host

import (
	"testing"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
)

// A review queue row addresses its block as the edit contract does: the
// source document, the block key, and the translation's language. A source
// row names no edition, and a row with no source path falls back to the file
// it lists.
func TestReviewQueueRef(t *testing.T) {
	tests := []struct {
		name string
		item ReviewQueueItem
		want change.Ref
	}{
		{
			name: "a translation",
			item: ReviewQueueItem{Locale: "nb", File: "nb.json", Relative: "en.json", Key: "a"},
			want: change.Ref{Doc: "en.json", Block: "a", Edition: model.EditionKey{Locale: "nb"}},
		},
		{
			name: "a region is canonical",
			item: ReviewQueueItem{Locale: "pt-br", File: "pt-BR/guide.md", Relative: "docs/guide.md", Key: "intro/p"},
			want: change.Ref{Doc: "docs/guide.md", Block: "intro/p", Edition: model.EditionKey{Locale: "pt-BR"}},
		},
		{
			name: "a source unit",
			item: ReviewQueueItem{Locale: "en", File: "en.json", Relative: "en.json", Key: "a", IsSource: true},
			want: change.Ref{Doc: "en.json", Block: "a"},
		},
		{
			name: "no source path",
			item: ReviewQueueItem{Locale: "nb", File: "nb.json", Key: "a"},
			want: change.Ref{Doc: "nb.json", Block: "a", Edition: model.EditionKey{Locale: "nb"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ReviewQueueRef(tt.item))
		})
	}
}

// A pre-review is recorded under the name the review queue shows beside its
// score: agent/<client> for an agent, the bare agent when the client gave no
// name, and anyone else's own name.
func TestReviewerName(t *testing.T) {
	tests := []struct {
		actor change.Actor
		want  string
	}{
		{change.Actor{Kind: change.ActorAgent, Name: "claude-code", Session: "s1"}, "agent/claude-code"},
		{change.Actor{Kind: change.ActorAgent}, "agent"},
		{change.Actor{Kind: change.ActorPerson, Name: "asgeir"}, "asgeir"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, reviewerName(tt.actor))
	}
}

// The plain edition of the source language is the source itself; a tone or a
// channel of that language is an edition of its own.
func TestIsLanguageEdition(t *testing.T) {
	assert.True(t, isLanguageEdition(model.EditionKey{Locale: "en"}, "en"))
	assert.True(t, isLanguageEdition(model.EditionKey{Locale: "en-US"}, "en-us"))
	assert.False(t, isLanguageEdition(model.EditionKey{Locale: "en", Channel: "short"}, "en"))
	assert.False(t, isLanguageEdition(model.EditionKey{Locale: "nb"}, "en"))
	assert.False(t, isLanguageEdition(model.EditionKey{}, "en"))
}
