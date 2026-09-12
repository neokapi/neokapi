package comment_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/comment"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBlocksNameCommentsByWhatTheySitOn(t *testing.T) {
	f := &comment.File{
		Language: "go",
		Comments: []comment.Comment{
			{Subject: "func/Parse", Doc: true, Style: comment.StyleLine, Span: comment.Span{Start: 10, End: 30}, Lines: comment.LineRange{First: 3, Last: 4}, Runs: []model.Run{model.TextR("Parse parses.")}},
			{Subject: "func/Parse/comment", Style: comment.StyleLine, Span: comment.Span{Start: 60, End: 70}, Lines: comment.LineRange{First: 7, Last: 7}, Runs: []model.Run{model.TextR("first")}},
			{Subject: "func/Parse/comment", Style: comment.StyleBlock, Deprecated: true, Span: comment.Span{Start: 80, End: 90}, Lines: comment.LineRange{First: 9, Last: 9}, Runs: []model.Run{model.TextR("second")}},
		},
	}
	blocks := f.Blocks()
	require.Len(t, blocks, 3)

	assert.Equal(t, []string{"func/Parse", "func/Parse/comment", "func/Parse/comment#2"},
		[]string{blocks[0].ID, blocks[1].ID, blocks[2].ID})
	for _, b := range blocks {
		assert.Equal(t, b.ID, b.Name)
		assert.Equal(t, comment.BlockType, b.Type)
		assert.True(t, b.Translatable)
		require.NotNil(t, b.Identity)
		assert.Equal(t, "go", b.Properties[comment.PropLanguage])
	}

	assert.Equal(t, "true", blocks[0].Properties[comment.PropDoc])
	assert.Equal(t, "10-30", blocks[0].Properties[comment.PropSpan])
	assert.Equal(t, "3-4", blocks[0].Properties[comment.PropLines])
	assert.Equal(t, "block", blocks[2].Properties[comment.PropStyle])
	assert.Equal(t, "true", blocks[2].Properties[comment.PropDeprecated])
	assert.NotContains(t, blocks[1].Properties, comment.PropDoc)
}

// Moving a comment down the file changes where it is and nothing about what it
// is, so its identity does not move.
func TestBlockIdentityIgnoresPosition(t *testing.T) {
	at := func(start int) *model.Block {
		f := &comment.File{Language: "go", Comments: []comment.Comment{{
			Subject: "type/Block", Style: comment.StyleLine,
			Span: comment.Span{Start: start, End: start + 10}, Lines: comment.LineRange{First: start, Last: start},
			Runs: []model.Run{model.TextR("Block is a block.")},
		}}}
		return f.Blocks()[0]
	}
	assert.Equal(t, at(10).Identity.RecordHash(), at(500).Identity.RecordHash())
}
