package editorclient

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/host/venue/client"
)

const changesPath = "/api/v1/acme/projects/p1/streams/main/changes"

// A change set travels as sent, to the stream's changes route, and every answer
// that carries a change result comes back as one, a refusal included.
func TestApplyChanges(t *testing.T) {
	set := []byte(`{"ops":[{"op":"set_content","at":{"doc":"a.json","block":"b1","edition":"fr"},"if_match":"r:00000000000000aa","text":"Bonjour"}]}`)
	cases := []struct {
		name   string
		status int
		body   string
		want   change.SetStatus
		code   change.Code
	}{
		{"an applied change set", http.StatusOK,
			`{"schema":"kapi.change-result/v1","status":"applied","record":"c1","docs":[],"ops":[{"i":0,"op":"set_content","status":"applied","before":"r:00000000000000aa","after":"r:00000000000000bb"}]}`,
			change.SetApplied, ""},
		{"a stale refusal", http.StatusConflict,
			`{"schema":"kapi.change-result/v1","status":"refused","record":null,"docs":[],"ops":[{"i":0,"op":"set_content","status":"refused","error":{"code":"stale","field":"if_match","message":"edition fr is at r:00000000000000cc, not r:00000000000000aa"},"current":{"rev":"r:00000000000000cc","text":"Salut"}}]}`,
			change.SetRefused, change.CodeStale},
		{"a change set refused whole", http.StatusBadRequest,
			`{"schema":"kapi.change-result/v1","status":"refused","record":null,"docs":[],"ops":[],"error":{"code":"invalid","pointer":"/ops/0/wat","message":"unknown field"}}`,
			change.SetRefused, change.CodeInvalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, got := editorServer(t, map[string]route{"POST " + changesPath: {tc.status, tc.body}})
			res, err := c.ApplyChanges(context.Background(), "acme", "p1", "main", set)
			require.NoError(t, err)
			assert.Equal(t, http.MethodPost, got.method)
			assert.Equal(t, changesPath, got.path)
			assert.Equal(t, "Bearer tok", got.auth)
			assert.JSONEq(t, string(set), string(got.body), "the change set travels as sent")
			assert.Equal(t, tc.want, res.Status)
			switch {
			case tc.code == "":
			case res.Error != nil:
				assert.Equal(t, tc.code, res.Error.Code)
			default:
				require.NotNil(t, res.Ops[0].Error)
				assert.Equal(t, tc.code, res.Ops[0].Error.Code)
			}
		})
	}

	t.Run("a stale refusal carries the translation as it stands", func(t *testing.T) {
		c, _ := editorServer(t, map[string]route{"POST " + changesPath: {cases[1].status, cases[1].body}})
		res, err := c.ApplyChanges(context.Background(), "acme", "p1", "", set)
		require.NoError(t, err, "an empty stream is the main stream")
		require.NotNil(t, res.Ops[0].Current)
		assert.Equal(t, "Salut", res.Ops[0].Current.Text)
		assert.Equal(t, "r:00000000000000cc", res.Ops[0].Current.Rev)
	})

	t.Run("an answer that is no change result is a status error", func(t *testing.T) {
		c, _ := editorServer(t, map[string]route{"POST " + changesPath: {http.StatusForbidden, `{"error":"forbidden","message":"no access to the project"}`}})
		_, err := c.ApplyChanges(context.Background(), "acme", "p1", "main", set)
		var statusErr *client.StatusError
		require.ErrorAs(t, err, &statusErr)
		assert.Equal(t, http.StatusForbidden, statusErr.StatusCode)
		assert.True(t, statusErr.Permanent())
		assert.Contains(t, err.Error(), "no access to the project")
	})
}
