package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platev "github.com/neokapi/neokapi/bowrain/core/event"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/event"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/id"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newRunFlowTestServer wires a server the way production wires it for
// automation: the rule, run and flow stores on the test database, and the
// engine subscribed to the test's own bus through the run manager, so a
// stored rule is loaded, fires on a published event, and records its run.
func newRunFlowTestServer(t *testing.T) *Server {
	t.Helper()
	cfg := DefaultConfig()
	srv := shutdownOnCleanup(t, NewServer(cfg))
	initTestStores(t, srv)

	pg := srv.ContentStore.(*bstore.PostgresStore)
	srv.FlowDefStore = bstore.NewFlowDefStore(pg.SQLDB())
	srv.AutomationRunStore = bstore.NewAutomationRunStore(pg.SQLDB())
	srv.AutomationRuleStore = event.NewRuleStore(pg.SQLDB())

	bus := event.NewChannelEventBus()
	t.Cleanup(func() { bus.Close() })
	srv.EventBus = bus

	if srv.AutomationEngine != nil {
		srv.AutomationEngine.Close()
	}
	srv.runManager = event.NewAutomationRunManager(srv.AutomationRunStore, srv.executeAutomationAction)
	engine := event.NewAutomationEngine(bus, srv.runManager.Execute)
	t.Cleanup(engine.Close)
	srv.AutomationEngine = engine
	return srv
}

// seedRunFlowProject creates a project with one target language and one item
// holding one translatable block, and returns the project id.
func seedRunFlowProject(t *testing.T, srv *Server, name string) string {
	t.Helper()
	ctx := context.Background()
	proj := &platstore.Project{
		Name:                  name,
		DefaultSourceLanguage: "en",
		TargetLanguages:       []model.LocaleID{"fr-FR"},
	}
	require.NoError(t, srv.ContentStore.CreateProject(ctx, proj))
	b := model.NewRunsBlock("blk-1", []model.Run{{Text: &model.TextRun{Text: "Hello there."}}})
	b.Translatable = true
	require.NoError(t, srv.ContentStore.StoreBlocksForItem(ctx, proj.ID, "main", "en.json", []*model.Block{b}))
	return proj.ID
}

func storeRunFlowRule(t *testing.T, srv *Server, projectID, name, flowID string) {
	t.Helper()
	require.NoError(t, srv.AutomationRuleStore.CreateRule(context.Background(), &event.StoredRule{
		ID:        id.New(),
		ProjectID: projectID,
		Name:      name,
		Trigger:   platev.EventPullCompleted,
		Actions: []event.AutomationAction{
			{Type: "run_flow", Config: map[string]string{"flow": flowID}},
		},
		Enabled: true,
	}))
	srv.reloadAutomationRules()
}

func publishPull(srv *Server, projectID string) {
	srv.EventBus.Publish(platev.Event{
		ID:        id.New(),
		Type:      platev.EventPullCompleted,
		Source:    "test",
		ProjectID: projectID,
		Data:      map[string]string{"items": "en.json", "stream": "main"},
		Timestamp: time.Now().UTC(),
	})
}

func projectBlock(t *testing.T, srv *Server, projectID string) *model.Block {
	t.Helper()
	blocks, err := srv.ContentStore.GetBlocks(context.Background(), platstore.BlockQuery{ProjectID: projectID, Stream: "main"})
	require.NoError(t, err)
	require.Len(t, blocks, 1)
	return blocks[0].Block
}

// runFlowStep waits for the project's automation run to hold a run_flow step
// in a terminal state and returns the run and that step.
func runFlowStep(t *testing.T, srv *Server, projectID string) (*bstore.AutomationRun, *bstore.AutomationStep) {
	t.Helper()
	ctx := context.Background()
	var run *bstore.AutomationRun
	var step *bstore.AutomationStep
	require.Eventually(t, func() bool {
		runs, err := srv.AutomationRunStore.ListRuns(ctx, projectID, "", 10, 0)
		if err != nil || len(runs) != 1 {
			return false
		}
		steps, err := srv.AutomationRunStore.ListSteps(ctx, runs[0].ID)
		if err != nil {
			return false
		}
		for _, s := range steps {
			if s.ActionType == "run_flow" && (s.Status == bstore.StepStatusCompleted || s.Status == bstore.StepStatusFailed) {
				run, step = runs[0], s
				return true
			}
		}
		return false
	}, 10*time.Second, 50*time.Millisecond, "the run_flow step never reached a terminal state")
	// The step closes before the run's own status is recomputed; wait for that too.
	require.Eventually(t, func() bool {
		r, err := srv.AutomationRunStore.GetRun(ctx, run.ID)
		if err != nil {
			return false
		}
		run = r
		return r.Status != bstore.RunStatusRunning
	}, 5*time.Second, 50*time.Millisecond)
	return run, step
}

func latestHistory(t *testing.T, srv *Server, projectID string) event.HistoryEntry {
	t.Helper()
	var entry event.HistoryEntry
	require.Eventually(t, func() bool {
		page, err := srv.AutomationRuleStore.ListHistory(context.Background(), event.HistoryQuery{ProjectID: projectID, Limit: 10})
		if err != nil || len(page.Entries) == 0 {
			return false
		}
		entry = page.Entries[0]
		return true
	}, 5*time.Second, 50*time.Millisecond)
	return entry
}

// TestAutomation_RunFlow_StartsTheFlow proves an event-triggered rule with a
// run_flow action runs the named flow over the event's items for the
// project's target languages, and that the run history records it.
func TestAutomation_RunFlow_StartsTheFlow(t *testing.T) {
	srv := newRunFlowTestServer(t)
	projID := seedRunFlowProject(t, srv, "Run flow")
	storeRunFlowRule(t, srv, projID, "pseudo-on-pull", "pseudo-translate")

	publishPull(srv, projID)

	require.Eventually(t, func() bool {
		return strings.Contains(projectBlock(t, srv, projID).TargetText("fr-FR"), "▒")
	}, 10*time.Second, 50*time.Millisecond, "the flow never wrote a target")

	run, step := runFlowStep(t, srv, projID)
	assert.Equal(t, bstore.StepStatusCompleted, step.Status)
	assert.Empty(t, step.Error)
	assert.Equal(t, "pseudo-on-pull", step.RuleName)
	assert.Equal(t, bstore.RunStatusCompleted, run.Status)

	logs, err := srv.AutomationRunStore.ListLogs(context.Background(), step.ID, 20)
	require.NoError(t, err)
	var wrote bool
	for _, l := range logs {
		if strings.Contains(l.Message, `"pseudo-translate" wrote 1 blocks`) {
			wrote = true
		}
	}
	assert.True(t, wrote, "the step log names what the flow wrote: %+v", logs)

	assert.Equal(t, "success", latestHistory(t, srv, projID).Status)
}

// TestAutomation_RunFlow_UnknownFlowFailsLoudly proves a rule naming a flow
// the project cannot see fails its step with the reason, in the run history
// and in the execution history, rather than doing nothing.
func TestAutomation_RunFlow_UnknownFlowFailsLoudly(t *testing.T) {
	srv := newRunFlowTestServer(t)
	projID := seedRunFlowProject(t, srv, "Unknown flow")
	storeRunFlowRule(t, srv, projID, "ghost", "no-such-flow")

	publishPull(srv, projID)

	run, step := runFlowStep(t, srv, projID)
	assert.Equal(t, bstore.StepStatusFailed, step.Status)
	assert.Contains(t, step.Error, "flow not found")
	assert.Contains(t, step.Error, "no-such-flow")
	assert.Equal(t, bstore.RunStatusPartial, run.Status)

	entry := latestHistory(t, srv, projID)
	assert.Equal(t, "failed", entry.Status)
	assert.Contains(t, entry.Error, "no-such-flow")

	assert.Empty(t, projectBlock(t, srv, projID).TargetText("fr-FR"))
}

// TestAutomation_RunFlow_ProjectScope proves two things about scope: a flow
// one project authored is out of reach of another project's rule, and a
// rule authored on one project never fires on another project's events.
func TestAutomation_RunFlow_ProjectScope(t *testing.T) {
	srv := newRunFlowTestServer(t)
	ctx := context.Background()
	projA := seedRunFlowProject(t, srv, "Project A")
	projB := seedRunFlowProject(t, srv, "Project B")

	require.NoError(t, srv.FlowDefStore.Upsert(ctx, projA, &flow.FlowDefinition{
		ID:   "a-only",
		Name: "A only",
		Nodes: []flow.FlowNode{
			{ID: "pseudo", Type: flow.NodeTool, Name: "pseudo-translate"},
		},
	}))
	storeRunFlowRule(t, srv, projA, "a-rule", "pseudo-translate")
	storeRunFlowRule(t, srv, projB, "b-borrows-a", "a-only")

	publishPull(srv, projB)

	run, step := runFlowStep(t, srv, projB)
	assert.Equal(t, bstore.StepStatusFailed, step.Status)
	assert.Contains(t, step.Error, "a-only")
	assert.Equal(t, "b-borrows-a", step.RuleName)
	assert.Equal(t, bstore.RunStatusPartial, run.Status)
	assert.Empty(t, projectBlock(t, srv, projB).TargetText("fr-FR"), "A's rule must not run on B's event")

	runsA, err := srv.AutomationRunStore.ListRuns(ctx, projA, "", 10, 0)
	require.NoError(t, err)
	assert.Empty(t, runsA, "A's rule is scoped to A and B's event is none of its business")

	// A's own event runs A's own rule, and A's stored flow resolves for A.
	storeRunFlowRule(t, srv, projA, "a-own-flow", "a-only")
	publishPull(srv, projA)
	require.Eventually(t, func() bool {
		return strings.Contains(projectBlock(t, srv, projA).TargetText("fr-FR"), "▒")
	}, 10*time.Second, 50*time.Millisecond)
}

// TestAutomation_UnknownActionTypeFailsTheStep proves an action the executor
// does not run fails its step instead of being skipped in silence.
func TestAutomation_UnknownActionTypeFailsTheStep(t *testing.T) {
	srv := newRunFlowTestServer(t)
	projID := seedRunFlowProject(t, srv, "Unknown action")

	err := srv.executeAutomationAction(event.AutomationAction{Type: "webhook", Name: "hook"}, platev.Event{
		ID: id.New(), Type: platev.EventPullCompleted, ProjectID: projID, Timestamp: time.Now().UTC(),
	}, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `unsupported action type "webhook"`)
	assert.Equal(t, "failed", latestHistory(t, srv, projID).Status)
}

// automationRuleRequest invokes the create handler directly with full
// permissions, the way the flow handler tests do.
func automationRuleRequest(t *testing.T, srv *Server, projectID, body string) *httptest.ResponseRecorder {
	t.Helper()
	e := srv.GetEcho()
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(r, rec)
	c.Set("project_permissions", platauth.PermAll)
	c.SetParamNames("ws", "id")
	c.SetParamValues("demo", projectID)
	require.NoError(t, srv.HandleCreateAutomationRule(c))
	return rec
}

// TestCreateAutomationRule_ValidatesRunFlow proves a rule is rejected when it
// could not run as written, and that a rule that saves is live in the engine
// without a restart.
func TestCreateAutomationRule_ValidatesRunFlow(t *testing.T) {
	srv := newRunFlowTestServer(t)
	projID := seedRunFlowProject(t, srv, "Validate")
	ctx := context.Background()
	require.NoError(t, srv.FlowDefStore.Upsert(ctx, projID, &flow.FlowDefinition{
		ID:    "mine",
		Name:  "Mine",
		Nodes: []flow.FlowNode{{ID: "qa", Type: flow.NodeTool, Name: "qa"}},
	}))

	cases := []struct {
		name    string
		actions string
		code    int
		message string
	}{
		{"no flow named", `[{"type":"run_flow","config":{}}]`, http.StatusBadRequest, "run_flow names no flow"},
		{"unknown flow", `[{"type":"run_flow","config":{"flow":"nope"}}]`, http.StatusBadRequest, "flow not found"},
		{"unknown action type", `[{"type":"webhook","config":{"url":"https://example.com"}}]`, http.StatusBadRequest, `unsupported action type "webhook"`},
		{"no actions", `[]`, http.StatusBadRequest, "at least one action"},
		{"built-in flow", `[{"type":"run_flow","config":{"flow":"qa"}}]`, http.StatusCreated, ""},
		{"project flow", `[{"type":"run_flow","config":{"flow":"mine"}}]`, http.StatusCreated, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := `{"name":"` + tc.name + `","trigger":"connector.pull.completed","conditions":[],"actions":` + tc.actions + `,"enabled":true}`
			rec := automationRuleRequest(t, srv, projID, body)
			require.Equal(t, tc.code, rec.Code, rec.Body.String())
			if tc.message != "" {
				var resp map[string]string
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
				assert.Contains(t, resp["error"], tc.message)
			}
		})
	}

	// Nameless and triggerless rules are rejected too.
	rec := automationRuleRequest(t, srv, projID, `{"name":"","trigger":"connector.pull.completed","actions":[{"type":"notify","config":{}}],"enabled":true}`)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	rec = automationRuleRequest(t, srv, projID, `{"name":"x","trigger":"","actions":[{"type":"notify","config":{}}],"enabled":true}`)
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var live []string
	for _, r := range srv.AutomationEngine.Rules() {
		if r.ProjectID == projID {
			live = append(live, r.Name)
		}
	}
	assert.ElementsMatch(t, []string{"built-in flow", "project flow"}, live, "saved rules are live in the engine, scoped to their project")
}
