package change_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
)

// Each code maps once to an exit code and an HTTP status, and every code the
// contract names is in the closed set.
func TestCodes_MapToExitCodesAndHTTPStatuses(t *testing.T) {
	tests := []struct {
		code change.Code
		exit int
		http int
	}{
		{change.CodeInvalid, 2, 400},
		{change.CodeNotFound, 3, 404},
		{change.CodeAmbiguous, 3, 409},
		{change.CodeStale, 3, 409},
		{change.CodeDocChanged, 3, 409},
		{change.CodeGuard, 3, 422},
		{change.CodeGateFailed, 3, 422},
		{change.CodeUnsupported, 3, 422},
		{change.CodeNotPermitted, 3, 403},
		{change.CodeBudgetExceeded, 3, 413},
		{change.CodeUnreachable, 5, 503},
	}
	var codes []change.Code
	for _, tc := range tests {
		t.Run(string(tc.code), func(t *testing.T) {
			assert.Equal(t, tc.exit, tc.code.ExitCode())
			assert.Equal(t, tc.http, tc.code.HTTPStatus())
		})
		codes = append(codes, tc.code)
	}
	assert.Equal(t, codes, change.Codes())
}

func TestError_Message(t *testing.T) {
	assert.Equal(t, "invalid at /ops/0/op: unknown operation", (&change.Error{Code: change.CodeInvalid, Pointer: "/ops/0/op", Message: "unknown operation"}).Error())
	assert.Equal(t, "guard (overlap): edit 1 overlaps edit 0", (&change.Error{Code: change.CodeGuard, Subcode: change.SubcodeOverlap, Message: "edit 1 overlaps edit 0"}).Error())
	assert.Equal(t, "stale: moved", (&change.Error{Code: change.CodeStale, Message: "moved"}).Error())
}

// A change set refused as a whole is answered in the shape of every other
// result: status refused, no record, empty docs and ops, and the error.
func TestErrorResult_IsAResult(t *testing.T) {
	got, err := json.Marshal(change.ErrorResult(&change.Error{Code: change.CodeInvalid, Pointer: "/ops/0/op", Message: "unknown operation"}))
	require.NoError(t, err)
	assert.JSONEq(t, `{"schema":"kapi.change-result/v1","status":"refused","record":null,"docs":[],"ops":[],
		"error":{"code":"invalid","pointer":"/ops/0/op","message":"unknown operation"}}`, string(got))

	applied, err := json.Marshal(change.Result{Schema: change.ResultSchemaID, Status: change.SetApplied, Docs: []change.DocResult{}, Ops: []change.OpResult{}})
	require.NoError(t, err)
	assert.NotContains(t, string(applied), `"error"`, "a result whose operations were considered carries no set-level error")
}
