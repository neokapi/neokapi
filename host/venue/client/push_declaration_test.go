package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func declarationTestBlocks() map[string][]*model.Block {
	return map[string][]*model.Block{
		"src/App.jsx": {
			{
				ID:           "b1",
				Name:         "greeting",
				Translatable: true,
				Source:       []model.Run{{Text: &model.TextRun{Text: "Hello"}}},
				Properties:   map[string]string{"hash": "k1_abc"},
			},
		},
	}
}

// commitCapture answers a push the short way — one item, one block needed — and
// keeps the manifest for inspection.
func commitCapture(t *testing.T, got *PushCommitRequest) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/push/init"):
			_ = json.NewEncoder(w).Encode(PushInitResponse{
				UploadID: "up1", Status: "diff_computed", NewItems: []string{"src/App.jsx"},
			})
		case strings.HasSuffix(r.URL.Path, "/push/commit"):
			_ = json.NewDecoder(r.Body).Decode(got)
			_ = json.NewEncoder(w).Encode(map[string]string{"push_id": "p1", "status": "queued"})
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
}

// The declaration rides on the commit, because that is where the server decides
// what this push is authoritative about.
func TestPushDeclaresTheKeysItsReadersEmit(t *testing.T) {
	var got PushCommitRequest
	srv := commitCapture(t, &got)
	defer srv.Close()

	c := NewClaimTokenClient(srv.URL, "proj1", "tok")
	_, err := c.Push(context.Background(), declarationTestBlocks(), nil, nil, nil,
		DeclareBlockProperties([]string{"component", "hash"}))
	require.NoError(t, err)

	assert.Equal(t, []string{"component", "hash"}, got.BlockPropertyKeys)
}

// A caller that declares nothing says nothing, and a server reads that as
// knowing nothing rather than as claiming its blocks are bare.
func TestPushWithoutADeclarationSendsNone(t *testing.T) {
	var got PushCommitRequest
	srv := commitCapture(t, &got)
	defer srv.Close()

	c := NewClaimTokenClient(srv.URL, "proj1", "tok")
	_, err := c.Push(context.Background(), declarationTestBlocks(), nil, nil, nil)
	require.NoError(t, err)

	assert.Empty(t, got.BlockPropertyKeys)
}
