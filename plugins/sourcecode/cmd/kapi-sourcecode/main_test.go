package main

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pb "github.com/neokapi/neokapi/core/plugin/proto/v2"
	"github.com/neokapi/neokapi/core/plugin/protoconvert"
	"github.com/neokapi/neokapi/plugins/sourcecode/internal/comments"
)

// The RPC answers with what the comments package locates, carried so the host
// reads back the same file.
func TestLocateCommentsAnswersWithWhatThePackageLocates(t *testing.T) {
	src := []byte("// eslint-disable-next-line\n/** Parses the input. */\nexport function parse(): void {}\n")
	resp, err := (&server{}).LocateComments(context.Background(), &pb.LocateCommentsRequest{Language: "typescript", Name: "parse.ts", Source: src})
	require.NoError(t, err)
	require.Empty(t, resp.GetError())
	assert.False(t, resp.GetUnlocated())

	want, err := comments.Locate("typescript", "parse.ts", src)
	require.NoError(t, err)
	got, err := protoconvert.ProtoToCommentFile("typescript", len(src), resp)
	require.NoError(t, err)
	assert.Equal(t, want, got)
	require.Len(t, got.Comments, 1)
	assert.Equal(t, "func/parse", got.Comments[0].Subject)
}

func TestLocateCommentsReportsWhatItCannotLocate(t *testing.T) {
	t.Run("a file that does not parse is unlocated", func(t *testing.T) {
		resp, err := (&server{}).LocateComments(context.Background(), &pb.LocateCommentsRequest{Language: "typescript", Name: "broken.ts", Source: []byte("export function (\n")})
		require.NoError(t, err)
		assert.True(t, resp.GetUnlocated())
		assert.Contains(t, resp.GetError(), "does not parse")
		assert.Empty(t, resp.GetComments())
	})
	t.Run("a language the plugin has no grammar for is an error", func(t *testing.T) {
		resp, err := (&server{}).LocateComments(context.Background(), &pb.LocateCommentsRequest{Language: "cobol", Source: []byte("x")})
		require.NoError(t, err)
		assert.False(t, resp.GetUnlocated())
		assert.Contains(t, resp.GetError(), `no comment grammar for language "cobol"`)
	})
}

// doctor locates each comment language's canary, so a build whose grammars no
// longer yield the canary's comment fails `kapi plugins doctor`.
func TestDoctorPasses(t *testing.T) {
	assert.Equal(t, 0, runDoctor())
}
