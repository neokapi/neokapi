package server

import (
	"context"
	"testing"
	"time"

	platev "github.com/neokapi/neokapi/bowrain/core/event"
	"github.com/neokapi/neokapi/bowrain/event"
	"github.com/neokapi/neokapi/core/id"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// storeRuleReturningID stores an enabled rule triggered by an event type and
// returns the row id the execution record must name.
func storeRuleReturningID(t *testing.T, srv *Server, projectID, name, flowID string) string {
	t.Helper()
	ruleID := id.New()
	require.NoError(t, srv.AutomationRuleStore.CreateRule(context.Background(), &event.StoredRule{
		ID:        ruleID,
		ProjectID: projectID,
		Name:      name,
		Trigger:   platev.EventPullCompleted,
		Actions: []event.AutomationAction{
			{Type: "run_flow", Config: map[string]string{"flow": flowID}},
		},
		Enabled: true,
	}))
	srv.reloadAutomationRules()
	return ruleID
}

// historyForRule waits for the project's execution history to hold an entry
// written by the named rule and returns it.
func historyForRule(t *testing.T, srv *Server, projectID, ruleID string) event.HistoryEntry {
	t.Helper()
	var found event.HistoryEntry
	require.Eventually(t, func() bool {
		page, err := srv.AutomationRuleStore.ListHistory(context.Background(), event.HistoryQuery{
			ProjectID: projectID, Limit: 50,
		})
		if err != nil {
			return false
		}
		for _, e := range page.Entries {
			if e.RuleID == ruleID {
				found = e
				return true
			}
		}
		return false
	}, 10*time.Second, 50*time.Millisecond, "no history entry names rule %q", ruleID)
	return found
}

// TestAutomationHistoryNamesTheStoredRule proves an event-triggered rule's
// execution record carries the rule's id, so a history row traces back to the
// rule that fired rather than only to the event.
func TestAutomationHistoryNamesTheStoredRule(t *testing.T) {
	srv := newRunFlowTestServer(t)
	projID := seedRunFlowProject(t, srv, "Rule identity")
	ruleID := storeRuleReturningID(t, srv, projID, "pseudo-on-pull", "pseudo-translate")

	publishPull(srv, projID)

	entry := historyForRule(t, srv, projID, ruleID)
	assert.Equal(t, "success", entry.Status)
	assert.Equal(t, projID, entry.ProjectID)

	// The run's step names the same rule, by id and by the name it carried.
	_, step := runFlowStep(t, srv, projID)
	assert.Equal(t, ruleID, step.RuleID)
	assert.Equal(t, "pseudo-on-pull", step.RuleName)
}

// TestAutomationHistoryNamesABuiltInRule proves a platform rule is named too.
// It has no stored row, so its id carries its name under the built-in prefix.
func TestAutomationHistoryNamesABuiltInRule(t *testing.T) {
	srv := newRunFlowTestServer(t)
	projID := seedRunFlowProject(t, srv, "Built-in identity")
	srv.reloadAutomationRules()

	// auto-extract-on-push matches this event. With no items and no push id
	// the action has nothing to extract and returns at once, which is enough:
	// what is under test is the record the executor writes either way.
	srv.EventBus.Publish(platev.Event{
		ID:        id.New(),
		Type:      platev.EventPushCompleted,
		Source:    "test",
		ProjectID: projID,
		Timestamp: time.Now().UTC(),
	})

	entry := historyForRule(t, srv, projID, event.BuiltInRuleID("auto-extract-on-push"))
	assert.Equal(t, "success", entry.Status)
}
