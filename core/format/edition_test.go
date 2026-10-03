package format_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
)

func TestAuthoritativeRuns(t *testing.T) {
	tests := []struct {
		name  string
		build func() *model.Block
		want  string
	}{
		{
			name:  "a block with no language",
			build: func() *model.Block { return model.NewBlock("tu1", "Hello") },
			want:  "Hello",
		},
		{
			name: "a block with a translation",
			build: func() *model.Block {
				b := model.NewBlock("tu1", "Hello")
				b.SourceLocale = "en"
				b.SetTargetText("fr", "Bonjour")
				return b
			},
			want: "Hello",
		},
		{
			// A bilingual file from en to en files its target under the
			// source language; the edition the block was read in stays the
			// authoritative one.
			name: "a block with a same-language translation",
			build: func() *model.Block {
				b := model.NewBlock("tu1", "Colour")
				b.SourceLocale = "en"
				b.SetTargetText("en", "Color")
				return b
			},
			want: "Colour",
		},
		{
			name: "an edited block",
			build: func() *model.Block {
				b := model.NewBlock("tu1", "Hello")
				b.EditSourceText("Hi")
				return b
			},
			want: "Hi",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, model.RunsText(format.AuthoritativeRuns(tt.build())))
		})
	}
}

func TestAuthoritativeRuns_EmptySource(t *testing.T) {
	b := &model.Block{ID: "tu1"}
	assert.Empty(t, format.AuthoritativeRuns(b))
}
