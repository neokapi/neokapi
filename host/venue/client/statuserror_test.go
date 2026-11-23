package client

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewStatusError(t *testing.T) {
	tests := []struct {
		name string
		op   string
		code int
		body string
		want string
	}{
		{
			name: "full envelope prints the human sentence and the reference",
			op:   "pull",
			code: http.StatusForbidden,
			body: `{"current":1,"error":"project_limit_reached","limit":1,` +
				`"message":"This workspace's plan allows 1 project(s) and it already has 1.",` +
				`"reference":"req-abc-123"}`,
			want: "pull failed (HTTP 403): This workspace's plan allows 1 project(s) and it already has 1. (ref: req-abc-123)",
		},
		{
			name: "envelope without message falls back to the stable code",
			op:   "push init",
			code: http.StatusServiceUnavailable,
			body: `{"error":"content store not configured"}`,
			want: "push init failed (HTTP 503): content store not configured",
		},
		{
			name: "reference_id spelling is tolerated",
			op:   "claim",
			code: http.StatusBadRequest,
			body: `{"error":"invalid or expired token","reference_id":"req-9"}`,
			want: "claim failed (HTTP 400): invalid or expired token (ref: req-9)",
		},
		{
			name: "non-envelope body keeps the historical output",
			op:   "download",
			code: http.StatusBadGateway,
			body: "<html>bad gateway</html>",
			want: "download failed (HTTP 502): <html>bad gateway</html>",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := NewStatusError(tt.op, tt.code, []byte(tt.body))
			assert.Equal(t, tt.want, err.Error())
			assert.Equal(t, tt.code, err.StatusCode)
			assert.Equal(t, tt.body, err.Body, "raw body is preserved for callers")
		})
	}
}

func TestNewStatusError_StructuredFields(t *testing.T) {
	err := NewStatusError("pull", http.StatusForbidden,
		[]byte(`{"error":"project_limit_reached","message":"m.","reference":"req-1","limit":1}`))
	assert.Equal(t, "project_limit_reached", err.Code)
	assert.Equal(t, "m.", err.Message)
	assert.Equal(t, "req-1", err.Reference)
	assert.True(t, err.Permanent())
}

// TestRefusalJSONCarriesTheServersAnswer pins the wire form a desktop frontend
// reads: flow-save-refusals.test.ts in bowrain/packages/ui feeds the same
// shape to the function that renders the web's refusals.
func TestRefusalJSONCarriesTheServersAnswer(t *testing.T) {
	assert.JSONEq(t, `{"status":403,"body":{"error":"insufficient permissions"}}`,
		string(RefusalJSON(http.StatusForbidden, []byte(`{"error":"insufficient permissions"}`))))
	assert.JSONEq(t, `{"status":502,"body":"<html>Bad gateway</html>"}`,
		string(RefusalJSON(http.StatusBadGateway, []byte(`<html>Bad gateway</html>`))))
	assert.JSONEq(t, `{"status":500}`, string(RefusalJSON(http.StatusInternalServerError, nil)))

	// A StatusError returned from a desktop binding marshals the same way.
	var err error = NewStatusError("push", http.StatusConflict, []byte(`{"error":"stale","message":"Someone changed it"}`))
	wire, merr := json.Marshal(&err)
	require.NoError(t, merr)
	assert.JSONEq(t, `{"status":409,"body":{"error":"stale","message":"Someone changed it"}}`, string(wire))

	assert.JSONEq(t, `{"network":true}`, string(UnreachableJSON()))
}
