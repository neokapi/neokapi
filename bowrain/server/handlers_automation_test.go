package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	platev "github.com/neokapi/neokapi/bowrain/core/event"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestListAutomationEvents_OffersOnlyEmittedTypes guards the trigger picker
// against phantom options: every event type offered as an automation trigger
// must have an emitter somewhere in the platform. EventFlowCompleted /
// EventFlowFailed are defined on the bus but not yet emitted by any
// flow-execution path, so they must not be offered until an emitter exists
// (see AD-013, "Trigger events").
func TestListAutomationEvents_OffersOnlyEmittedTypes(t *testing.T) {
	s := &Server{}
	e := echo.New()
	rec := httptest.NewRecorder()
	c := e.NewContext(httptest.NewRequest(http.MethodGet, "/", nil), rec)

	require.NoError(t, s.HandleListAutomationEvents(c))
	require.Equal(t, http.StatusOK, rec.Code)

	var events []struct {
		Type        string `json:"type"`
		Description string `json:"description"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &events))
	require.NotEmpty(t, events)

	offered := make(map[string]bool, len(events))
	for _, ev := range events {
		offered[ev.Type] = true
	}

	assert.False(t, offered[string(platev.EventFlowCompleted)],
		"flow.completed has no emitter yet and must not be offered as a trigger")
	assert.False(t, offered[string(platev.EventFlowFailed)],
		"flow.failed has no emitter yet and must not be offered as a trigger")

	// Sanity: the emitted core triggers stay offered.
	assert.True(t, offered[string(platev.EventPushCompleted)])
	assert.True(t, offered[string(platev.EventSourceReviewCompleted)])
}
