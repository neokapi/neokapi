package service

import (
	"context"
	"sync"
	"testing"

	"github.com/neokapi/neokapi/bowrain/analytics"
	"github.com/neokapi/neokapi/bowrain/core/connector"
	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recordingTracker is a test EventTracker that records captured events.
type recordingTracker struct {
	mu     sync.Mutex
	events []capturedEvent
}

type capturedEvent struct {
	distinctID string
	event      string
	props      map[string]any
}

func (r *recordingTracker) CaptureEvent(distinctID, event string, properties map[string]any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, capturedEvent{distinctID, event, properties})
}

func (r *recordingTracker) captured() []capturedEvent {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]capturedEvent(nil), r.events...)
}

func (r *recordingTracker) find(event string) *capturedEvent {
	for _, e := range r.captured() {
		if e.event == event {
			ev := e
			return &ev
		}
	}
	return nil
}

// TestFlowServiceExecuteFlow_TracksFlowRunCompleted proves ExecuteFlow fires a
// flow_run_completed event with the taxonomy properties after a successful run,
// and that a nil tracker is a safe no-op.
func TestFlowServiceExecuteFlow_TracksFlowRunCompleted(t *testing.T) {
	f, err := flow.NewFlow("test-flow").
		AddTool(&tool.BaseTool{ToolName: "pass"}).
		Build()
	require.NoError(t, err)

	items := []*flow.Item{{OutputBlocks: []*model.Block{model.NewBlock("b1", "hello")}}}

	// Nil tracker: must not panic.
	fs := NewFlowService(nil, nil, nil)
	require.NoError(t, fs.ExecuteFlow(context.Background(), f, items, ""))

	rec := &recordingTracker{}
	fs.tracker = rec
	require.NoError(t, fs.ExecuteFlow(context.Background(), f, items, ""))

	ev := rec.find(analytics.EventFlowRunCompleted)
	require.NotNil(t, ev, "flow_run_completed not captured")
	assert.Equal(t, "server", ev.distinctID, "no project scope falls back to server")
	assert.Equal(t, "test-flow", ev.props["flow"])
	assert.Equal(t, "completed", ev.props["outcome"])
	assert.Equal(t, 1, ev.props["part_count"])
	assert.NotEmpty(t, ev.props["duration_bucket"])
	assert.NotContains(t, ev.props, analytics.PropProjectID,
		"empty project id must be omitted")
}

// TestConnectorServicePublish_TracksConnectorPublished proves Publish fires a
// connector_published event carrying connector type, outcome, and bucketed
// counts — and no content.
func TestConnectorServicePublish_TracksConnectorPublished(t *testing.T) {
	s := newTestStore(t)
	reg := connector.NewRegistry()
	reg.Register("test", connector.CategoryFile, func(config map[string]string) (connector.IntegrationConnector, error) {
		return &testConnector{id: "conn-1"}, nil
	})

	svc := NewConnectorService(s, reg)
	rec := &recordingTracker{}
	svc.tracker = rec

	_, err := svc.AddConnector("ws-pub", "test", nil)
	require.NoError(t, err)

	require.NoError(t, svc.Publish(context.Background(), "ws-pub", "conn-1", "proj-pub", connector.PublishOptions{}))

	ev := rec.find(analytics.EventConnectorPublished)
	require.NotNil(t, ev, "connector_published not captured")
	assert.Equal(t, "ws-pub", ev.distinctID)
	assert.Equal(t, "ws-pub", ev.props[analytics.PropWorkspaceID])
	assert.Equal(t, "proj-pub", ev.props[analytics.PropProjectID])
	assert.Equal(t, "test", ev.props["connector_type"])
	assert.Equal(t, "completed", ev.props["outcome"])
	assert.Equal(t, "0", ev.props["block_count_bucket"])
}
