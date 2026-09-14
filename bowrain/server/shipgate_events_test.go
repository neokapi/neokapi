package server

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	platev "github.com/neokapi/neokapi/bowrain/core/event"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/event"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/core/id"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The quality gate events come from the ship gate. Each read of the dashboard
// derives every language's ship state, and a gate whose result changed since
// the last announcement is announced once: a fail when a gate that withholds
// the language becomes unmet, a pass when a failure announced earlier clears.

// gateSentinel marks a point in the bus, so a test can read every gate event
// published before it.
const gateSentinel platev.EventType = "test.gate.sentinel"

type gateCapture struct {
	mu     sync.Mutex
	events []platev.Event
	seen   map[string]bool
}

func captureGateEvents(t *testing.T, srv *Server) *gateCapture {
	t.Helper()
	c := &gateCapture{seen: map[string]bool{}}
	sub := srv.EventBus.SubscribeAll(func(ev platev.Event) {
		c.mu.Lock()
		defer c.mu.Unlock()
		switch ev.Type {
		case gateSentinel:
			c.seen[ev.ID] = true
		case platev.EventQualityGateFail, platev.EventQualityGatePass:
			c.events = append(c.events, ev)
		}
	})
	t.Cleanup(func() { srv.EventBus.Unsubscribe(sub) })
	return c
}

// drain returns the gate events published since the previous drain, once the
// bus has delivered everything published before this call.
func (c *gateCapture) drain(t *testing.T, srv *Server) []platev.Event {
	t.Helper()
	marker := id.New()
	srv.EventBus.Publish(platev.Event{ID: marker, Type: gateSentinel})
	require.Eventually(t, func() bool {
		c.mu.Lock()
		defer c.mu.Unlock()
		return c.seen[marker]
	}, 5*time.Second, 10*time.Millisecond)
	c.mu.Lock()
	defer c.mu.Unlock()
	out := c.events
	c.events = nil
	return out
}

// gateLine is the part of a gate event a test compares.
func gateLine(ev platev.Event) string {
	return fmt.Sprintf("%s %s %s actual=%s required=%s not_checked=%s",
		ev.Type, ev.Data["gate_name"], ev.Data["locale"], ev.Data["actual"], ev.Data["required"], ev.Data["not_checked"])
}

func gateLines(events []platev.Event) []string {
	out := make([]string, 0, len(events))
	for _, ev := range events {
		out = append(out, gateLine(ev))
	}
	return out
}

// seedGateProject stores a project translating English into locale in the test
// workspace, with one item whose blocks carry the given targets. An empty
// target leaves its block untranslated.
func seedGateProject(t *testing.T, srv *Server, locale model.LocaleID, targets ...string) string {
	t.Helper()
	proj := &platstore.Project{
		Name:                  "gate-proj",
		DefaultSourceLanguage: "en",
		TargetLanguages:       []model.LocaleID{locale},
		WorkspaceID:           "test-ws",
		Properties:            map[string]string{},
	}
	require.NoError(t, srv.ContentStore.CreateProject(t.Context(), proj))
	require.NoError(t, srv.ContentStore.StoreItem(t.Context(), proj.ID, "main", &platstore.Item{
		Name: "en.json", Format: "json", ItemType: "file",
	}))
	storeGateBlocks(t, srv, proj.ID, locale, targets...)
	return proj.ID
}

// storeGateBlocks rewrites the item's blocks with the given targets.
func storeGateBlocks(t *testing.T, srv *Server, projectID string, locale model.LocaleID, targets ...string) {
	t.Helper()
	blocks := make([]*model.Block, 0, len(targets))
	for i, target := range targets {
		b := &model.Block{ID: fmt.Sprintf("b%d", i+1), Translatable: true}
		b.SetSourceText(fmt.Sprintf("Sentence %d", i+1))
		if target != "" {
			b.SetTargetText(locale, target)
		}
		blocks = append(blocks, b)
	}
	require.NoError(t, srv.ContentStore.StoreBlocksForItem(t.Context(), projectID, "main", "en.json", blocks))
	srv.invalidateDashboardCache("test-ws", projectID)
}

func TestShipGateEvents_WithheldLocalePublishesFail(t *testing.T) {
	srv, token := newTestServer(t)
	events := captureGateEvents(t, srv)
	pid := seedGateProject(t, srv, "nb", "Setning 1", "")

	getDashboard(t, srv, token, pid, "")

	got := events.drain(t, srv)
	require.Equal(t, []string{"quality.gate.fail translated nb actual=1 required=2 not_checked=false"}, gateLines(got),
		"only the coverage gate is evaluated below full coverage, and it is announced once")
	ev := got[0]
	assert.Equal(t, pid, ev.ProjectID)
	assert.Equal(t, "main", ev.Data["stream"])
	assert.Equal(t, "test-ws", ev.Data["workspace_id"])
	assert.Equal(t, "test", ev.Data["workspace_slug"])
	assert.Equal(t, string(platstore.ShipStatePending), ev.Data["ship_state"])
	assert.NotEmpty(t, ev.ID)
}

func TestShipGateEvents_RecoveryPublishesPass(t *testing.T) {
	srv, token := newTestServer(t)
	events := captureGateEvents(t, srv)
	pid := seedGateProject(t, srv, "nb", "Setning 1", "")
	getDashboard(t, srv, token, pid, "")
	require.Len(t, events.drain(t, srv), 1)

	storeGateBlocks(t, srv, pid, "nb", "Setning 1", "Setning 2")
	getDashboard(t, srv, token, pid, "")

	assert.Equal(t, []string{"quality.gate.pass translated nb actual=2 required=2 not_checked=false"}, gateLines(events.drain(t, srv)),
		"the announced failure clears with a pass for the same gate and language")
}

func TestShipGateEvents_UnchangedVerdictPublishesNothing(t *testing.T) {
	srv, token := newTestServer(t)
	events := captureGateEvents(t, srv)
	pid := seedGateProject(t, srv, "nb", "Setning 1", "")
	getDashboard(t, srv, token, pid, "")
	require.Len(t, events.drain(t, srv), 1)

	srv.invalidateDashboardCache("test-ws", pid)
	getDashboard(t, srv, token, pid, "")
	assert.Empty(t, events.drain(t, srv), "a withheld language that stays withheld is not announced again")

	storeGateBlocks(t, srv, pid, "nb", "Setning 1", "Setning 2")
	getDashboard(t, srv, token, pid, "")
	require.Len(t, events.drain(t, srv), 1)
	srv.invalidateDashboardCache("test-ws", pid)
	getDashboard(t, srv, token, pid, "")
	assert.Empty(t, events.drain(t, srv), "a recovered language that stays shippable is not announced again")
}

func TestShipGateEvents_FirstShippableVerdictPublishesNothing(t *testing.T) {
	srv, token := newTestServer(t)
	events := captureGateEvents(t, srv)
	pid := seedGateProject(t, srv, "nb", "Setning 1", "Setning 2")

	getDashboard(t, srv, token, pid, "")
	assert.Empty(t, events.drain(t, srv), "a language whose first verdict is shippable has no failure to clear")
}

// A block whose target is only an inline code has no terminology result where
// terms govern the language. That withholds the language, and the fail says the
// gate was not checked. When coverage then drops, terminology is not evaluated
// at all, and no pass is announced for it.
func TestShipGateEvents_NotCheckedTermsPublishFailAndNeverPass(t *testing.T) {
	srv, token := newTestServer(t)
	seedCheckedTerminology(t, srv, "test")
	events := captureGateEvents(t, srv)

	proj := &platstore.Project{
		Name: "terms-proj", DefaultSourceLanguage: "en", TargetLanguages: []model.LocaleID{"fr"},
		WorkspaceID: "test-ws", Properties: map[string]string{},
	}
	require.NoError(t, srv.ContentStore.CreateProject(t.Context(), proj))
	require.NoError(t, srv.ContentStore.StoreItem(t.Context(), proj.ID, "main", &platstore.Item{
		Name: "en.json", Format: "json", ItemType: "file",
	}))
	codeOnly := func() *model.Block {
		b := &model.Block{ID: "code", Translatable: true, Source: []model.Run{textRun("Hello "), phRun()}}
		b.SetTargetRuns("fr", []model.Run{phRun()})
		return b
	}
	require.NoError(t, srv.ContentStore.StoreBlocksForItem(t.Context(), proj.ID, "main", "en.json", []*model.Block{codeOnly()}))

	getDashboard(t, srv, token, proj.ID, "")
	assert.Equal(t, []string{"quality.gate.fail terms fr actual=1 required=0 not_checked=true"}, gateLines(events.drain(t, srv)))

	untranslated := &model.Block{ID: "later", Translatable: true}
	untranslated.SetSourceText("Later")
	require.NoError(t, srv.ContentStore.StoreBlocksForItem(t.Context(), proj.ID, "main", "en.json", []*model.Block{codeOnly(), untranslated}))
	srv.invalidateDashboardCache("test-ws", proj.ID)

	getDashboard(t, srv, token, proj.ID, "")
	got := events.drain(t, srv)
	assert.Equal(t, []string{"quality.gate.fail translated fr actual=1 required=2 not_checked=false"}, gateLines(got),
		"terminology below full coverage is not evaluated, so it announces no pass")
}

// A stored automation rule on the quality.gate.fail trigger runs when the ship
// gate announces a failure for its project.
func TestShipGateEvents_QualityGateFailTriggerRunsItsRule(t *testing.T) {
	srv, token := newTestServer(t)
	pg := srv.ContentStore.(*bstore.PostgresStore)
	srv.AutomationRunStore = bstore.NewAutomationRunStore(pg.SQLDB())
	srv.AutomationRuleStore = event.NewRuleStore(pg.SQLDB())
	bus := event.NewChannelEventBus()
	t.Cleanup(bus.Close)
	srv.EventBus = bus
	if srv.AutomationEngine != nil {
		srv.AutomationEngine.Close()
	}
	srv.runManager = srv.newRunManager()
	engine := event.NewAutomationEngine(bus, srv.runManager.Execute)
	t.Cleanup(engine.Close)
	srv.AutomationEngine = engine

	pid := seedGateProject(t, srv, "nb", "Setning 1", "")
	require.NoError(t, srv.AutomationRuleStore.CreateRule(context.Background(), &event.StoredRule{
		ID:        id.New(),
		ProjectID: pid,
		Name:      "on-gate-fail",
		Trigger:   platev.EventQualityGateFail,
		Actions:   []event.AutomationAction{{Type: "notify", Config: map[string]string{"user_id": "test-user", "title": "A gate failed"}}},
		Enabled:   true,
	}))
	srv.reloadAutomationRules()

	getDashboard(t, srv, token, pid, "")

	require.Eventually(t, func() bool {
		runs, err := srv.AutomationRunStore.ListRuns(context.Background(), pid, "", 10, 0)
		return err == nil && len(runs) == 1
	}, 5*time.Second, 20*time.Millisecond, "the rule on quality.gate.fail must run")
}
