package commentproto_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/comment"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/plugin/commentproto"
	bridgepb "github.com/neokapi/neokapi/core/plugin/proto/v2"
)

func TestCommentFileRoundTrip(t *testing.T) {
	src := "/** Parses the input. */\nexport function parse() {} // done\n// eslint-disable-next-line\n"
	want := &comment.File{
		Language: "typescript",
		Comments: []comment.Comment{
			{
				Start: 0, End: 24, Lines: format.LineRange{First: 1, Last: 1}, Style: comment.StyleBlock,
				Subject: "func/parse", Doc: true, Deprecated: true,
				Runs: []model.Run{
					model.TextR("Parses the "),
					model.PhR(model.PlaceholderRun{ID: "c1", Type: "code", SubType: "jsdoc:code", Data: "`input`"}),
					model.TextR("."),
				},
			},
			{Start: 53, End: 60, Lines: format.LineRange{First: 2, Last: 2}, Style: comment.StyleLine, Subject: "comment", Runs: []model.Run{model.TextR("done")}},
		},
		Excluded: []comment.Excluded{
			{Start: 61, End: 88, Lines: format.LineRange{First: 3, Last: 3}, Reason: comment.ReasonDirective, Form: "eslint-disable-next-line"},
		},
	}

	got, err := commentproto.FromProto("typescript", len(src), commentproto.ToProto(want))
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestFromProtoRefusesAnImpossibleSpan(t *testing.T) {
	for name, span := range map[string]*bridgepb.CommentSpan{
		"past the end":        {Start: 0, End: 11, FirstLine: 1, LastLine: 1},
		"empty":               {Start: 4, End: 4, FirstLine: 1, LastLine: 1},
		"negative start":      {Start: -1, End: 3, FirstLine: 1, LastLine: 1},
		"no first line":       {Start: 0, End: 3, FirstLine: 0, LastLine: 1},
		"lines run backwards": {Start: 0, End: 3, FirstLine: 2, LastLine: 1},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := commentproto.FromProto("typescript", 10, &bridgepb.LocateCommentsResponse{Comments: []*bridgepb.CommentSpan{span}})
			require.Error(t, err)
		})
	}
	_, err := commentproto.FromProto("typescript", 10, &bridgepb.LocateCommentsResponse{
		Excluded: []*bridgepb.CommentExclusion{{Start: 8, End: 12, FirstLine: 1, LastLine: 1}},
	})
	require.Error(t, err, "an exclusion is held to the same bounds")
}
