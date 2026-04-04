//go:build integration

package agenticmcp_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/neokapi/neokapi/bowrain/agentic-testing/agenticmcp"
	"github.com/neokapi/neokapi/bowrain/storage"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func openTestPostgres(t *testing.T) *storage.PgDB {
	t.Helper()
	connStr := os.Getenv("BOWRAIN_TEST_POSTGRES_URL")
	if connStr == "" {
		connStr = "postgres://bowrain:bowrain@localhost:5432/bowrain_test?sslmode=disable"
	}
	db, err := storage.OpenPostgres(connStr)
	if err != nil {
		t.Skipf("PostgreSQL not available: %v", err)
	}
	t.Cleanup(func() {
		db.Exec("DELETE FROM agentic_events")
		db.Exec("DELETE FROM agentic_executions")
		db.Close()
	})
	return db
}

func openTestRedis(t *testing.T) *redis.Client {
	t.Helper()
	redisURL := os.Getenv("BOWRAIN_TEST_REDIS_URL")
	if redisURL == "" {
		redisURL = "redis://localhost:6379"
	}
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		t.Skipf("invalid Redis URL: %v", err)
	}
	client := redis.NewClient(opts)
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		t.Skipf("Redis not available: %v", err)
	}
	t.Cleanup(func() { client.Close() })
	return client
}

// TestIntegration_ExecutionStoreCRUD verifies the PostgreSQL store can insert
// and query executions and events.
func TestIntegration_ExecutionStoreCRUD(t *testing.T) {
	pgDB := openTestPostgres(t)
	ctx := t.Context()

	store, err := agenticmcp.NewPostgresExecutionStore(pgDB)
	require.NoError(t, err)

	execID := fmt.Sprintf("exec_test_%d", time.Now().UnixNano())

	// Insert exec.started event.
	started := &agenticmcp.AgenticEvent{
		Type:        agenticmcp.EventExecStarted,
		ExecutionID: execID,
		Workspace:   "integration-ws",
		Agent:       "test-agent",
		Role:        "translator",
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
		Data:        map[string]any{"task": "Translate 10 blocks", "locale": "fr-FR"},
	}
	require.NoError(t, store.InsertEvent(ctx, started))
	require.NoError(t, store.UpsertExecution(ctx, started))

	// Verify execution is running.
	execs, err := store.ListExecutions(ctx, agenticmcp.ExecutionFilter{
		WorkspaceSlug: "integration-ws",
		Limit:         10,
	})
	require.NoError(t, err)
	require.Len(t, execs, 1)
	assert.Equal(t, execID, execs[0].ID)
	assert.Equal(t, "running", execs[0].Status)
	assert.Equal(t, "Translate 10 blocks", execs[0].Task)
	assert.Equal(t, "fr-FR", execs[0].Locale)

	// Insert a progress event.
	progress := &agenticmcp.AgenticEvent{
		Type:        agenticmcp.EventExecProgress,
		ExecutionID: execID,
		Workspace:   "integration-ws",
		Agent:       "test-agent",
		Role:        "translator",
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
		Data:        map[string]any{"message": "5 of 10 blocks done", "blocks_done": float64(5), "blocks_total": float64(10)},
	}
	require.NoError(t, store.InsertEvent(ctx, progress))

	// Complete the execution.
	completed := &agenticmcp.AgenticEvent{
		Type:        agenticmcp.EventExecCompleted,
		ExecutionID: execID,
		Workspace:   "integration-ws",
		Agent:       "test-agent",
		Role:        "translator",
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
		Data:        map[string]any{"summary": "All blocks translated", "tokens_used": float64(4500), "duration_sec": float64(120)},
	}
	require.NoError(t, store.InsertEvent(ctx, completed))
	require.NoError(t, store.UpsertExecution(ctx, completed))

	// Verify execution is completed with token count.
	execs, err = store.ListExecutions(ctx, agenticmcp.ExecutionFilter{
		WorkspaceSlug: "integration-ws",
		Limit:         10,
	})
	require.NoError(t, err)
	require.Len(t, execs, 1)
	assert.Equal(t, "completed", execs[0].Status)
	assert.Equal(t, "All blocks translated", execs[0].Summary)
	assert.Equal(t, 4500, execs[0].TokensUsed)
	assert.NotEmpty(t, execs[0].CompletedAt)

	// Verify all 3 events are in the log.
	events, err := store.ListEvents(ctx, agenticmcp.EventFilter{
		ExecutionID: execID,
		Limit:       10,
	})
	require.NoError(t, err)
	assert.Len(t, events, 3)

	// Verify event type filtering.
	progressEvents, err := store.ListEvents(ctx, agenticmcp.EventFilter{
		ExecutionID: execID,
		EventType:   "exec.progress",
		Limit:       10,
	})
	require.NoError(t, err)
	assert.Len(t, progressEvents, 1)
	assert.Equal(t, float64(5), progressEvents[0].Data["blocks_done"])
}

// TestIntegration_SubscriberPipeline verifies the full Redis → Store → EventHub
// pipeline: publish an event to Redis, verify it's persisted in PostgreSQL and
// broadcast to EventHub subscribers.
func TestIntegration_SubscriberPipeline(t *testing.T) {
	pgDB := openTestPostgres(t)
	redisClient := openTestRedis(t)
	ctx := t.Context()

	store, err := agenticmcp.NewPostgresExecutionStore(pgDB)
	require.NoError(t, err)

	redisURL := os.Getenv("BOWRAIN_TEST_REDIS_URL")
	if redisURL == "" {
		redisURL = "redis://localhost:6379"
	}

	sub, err := agenticmcp.NewExecutionSubscriber(redisURL, "", store)
	require.NoError(t, err)

	hub := agenticmcp.NewEventHub()
	sub.SetEventHub(hub)

	subCtx, subCancel := context.WithCancel(ctx)
	sub.Start(subCtx)
	t.Cleanup(func() {
		subCancel()
		sub.Close()
	})

	// Subscribe a client to the hub.
	client := &agenticmcp.EventClient{C: make(chan agenticmcp.AgenticEvent, 16)}
	hub.Subscribe(client)
	defer hub.Unsubscribe(client)

	// Give the subscriber time to connect to Redis.
	time.Sleep(200 * time.Millisecond)

	execID := fmt.Sprintf("exec_pipe_%d", time.Now().UnixNano())

	// Publish exec.started via Redis.
	startedEv := agenticmcp.AgenticEvent{
		Type:        agenticmcp.EventExecStarted,
		ExecutionID: execID,
		Workspace:   "pipeline-ws",
		Agent:       "pipe-agent",
		Role:        "translator",
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
		Data:        map[string]any{"task": "Pipeline test"},
	}
	payload, err := json.Marshal(startedEv)
	require.NoError(t, err)
	err = redisClient.Publish(ctx, "agentic:events", string(payload)).Err()
	require.NoError(t, err)

	// Wait for the event to flow through the pipeline.
	select {
	case ev := <-client.C:
		assert.Equal(t, agenticmcp.EventExecStarted, ev.Type)
		assert.Equal(t, execID, ev.ExecutionID)
		assert.Equal(t, "pipeline-ws", ev.Workspace)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for event on EventHub client")
	}

	// Verify event was persisted in PostgreSQL.
	// Small delay to let the async write complete.
	time.Sleep(200 * time.Millisecond)

	events, err := store.ListEvents(ctx, agenticmcp.EventFilter{
		ExecutionID: execID,
		Limit:       10,
	})
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, "exec.started", string(events[0].Type))

	// Verify execution row was created.
	execs, err := store.ListExecutions(ctx, agenticmcp.ExecutionFilter{
		WorkspaceSlug: "pipeline-ws",
		Limit:         10,
	})
	require.NoError(t, err)
	require.Len(t, execs, 1)
	assert.Equal(t, execID, execs[0].ID)
	assert.Equal(t, "running", execs[0].Status)

	// Now publish exec.completed and verify the full lifecycle.
	completedEv := agenticmcp.AgenticEvent{
		Type:        agenticmcp.EventExecCompleted,
		ExecutionID: execID,
		Workspace:   "pipeline-ws",
		Agent:       "pipe-agent",
		Role:        "translator",
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
		Data:        map[string]any{"summary": "Done", "tokens_used": float64(2000), "duration_sec": float64(30)},
	}
	payload, err = json.Marshal(completedEv)
	require.NoError(t, err)
	err = redisClient.Publish(ctx, "agentic:events", string(payload)).Err()
	require.NoError(t, err)

	// Wait for completed event on hub.
	select {
	case ev := <-client.C:
		assert.Equal(t, agenticmcp.EventExecCompleted, ev.Type)
		assert.Equal(t, "Done", ev.Data["summary"])
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for completed event")
	}

	time.Sleep(200 * time.Millisecond)

	// Verify execution row was updated to completed.
	execs, err = store.ListExecutions(ctx, agenticmcp.ExecutionFilter{
		WorkspaceSlug: "pipeline-ws",
		Limit:         10,
	})
	require.NoError(t, err)
	require.Len(t, execs, 1)
	assert.Equal(t, "completed", execs[0].Status)
	assert.Equal(t, 2000, execs[0].TokensUsed)
	assert.Equal(t, "Done", execs[0].Summary)
}

// TestIntegration_EventHubWorkspaceFilter verifies that workspace-filtered hub
// clients only receive events for their workspace through the full pipeline.
func TestIntegration_EventHubWorkspaceFilter(t *testing.T) {
	pgDB := openTestPostgres(t)
	redisClient := openTestRedis(t)
	ctx := t.Context()

	store, err := agenticmcp.NewPostgresExecutionStore(pgDB)
	require.NoError(t, err)

	redisURL := os.Getenv("BOWRAIN_TEST_REDIS_URL")
	if redisURL == "" {
		redisURL = "redis://localhost:6379"
	}

	sub, err := agenticmcp.NewExecutionSubscriber(redisURL, "", store)
	require.NoError(t, err)

	hub := agenticmcp.NewEventHub()
	sub.SetEventHub(hub)

	subCtx, subCancel := context.WithCancel(ctx)
	sub.Start(subCtx)
	t.Cleanup(func() {
		subCancel()
		sub.Close()
	})

	// Two clients: one for ws-alpha, one for all.
	alphaClient := &agenticmcp.EventClient{C: make(chan agenticmcp.AgenticEvent, 16), WorkspaceSlug: "ws-alpha"}
	allClient := &agenticmcp.EventClient{C: make(chan agenticmcp.AgenticEvent, 16)}
	hub.Subscribe(alphaClient)
	hub.Subscribe(allClient)
	defer hub.Unsubscribe(alphaClient)
	defer hub.Unsubscribe(allClient)

	time.Sleep(200 * time.Millisecond)

	// Publish event for ws-beta (should NOT reach alphaClient).
	betaEv := agenticmcp.AgenticEvent{
		Type:        agenticmcp.EventExecStarted,
		ExecutionID: fmt.Sprintf("exec_beta_%d", time.Now().UnixNano()),
		Workspace:   "ws-beta",
		Agent:       "beta-agent",
		Role:        "translator",
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
		Data:        map[string]any{"task": "Beta task"},
	}
	payload, _ := json.Marshal(betaEv)
	require.NoError(t, redisClient.Publish(ctx, "agentic:events", string(payload)).Err())

	// Publish event for ws-alpha (should reach both clients).
	alphaEv := agenticmcp.AgenticEvent{
		Type:        agenticmcp.EventExecStarted,
		ExecutionID: fmt.Sprintf("exec_alpha_%d", time.Now().UnixNano()),
		Workspace:   "ws-alpha",
		Agent:       "alpha-agent",
		Role:        "translator",
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
		Data:        map[string]any{"task": "Alpha task"},
	}
	payload, _ = json.Marshal(alphaEv)
	require.NoError(t, redisClient.Publish(ctx, "agentic:events", string(payload)).Err())

	// Wait for allClient to receive both events.
	received := 0
	timeout := time.After(5 * time.Second)
	for received < 2 {
		select {
		case <-allClient.C:
			received++
		case <-timeout:
			t.Fatalf("allClient received %d/2 events before timeout", received)
		}
	}

	// alphaClient should have exactly 1 event (ws-alpha only).
	select {
	case ev := <-alphaClient.C:
		assert.Equal(t, "ws-alpha", ev.Workspace)
	case <-time.After(1 * time.Second):
		t.Fatal("alphaClient did not receive ws-alpha event")
	}

	// Verify no extra events for alphaClient.
	select {
	case ev := <-alphaClient.C:
		t.Fatalf("alphaClient received unexpected event: %+v", ev)
	case <-time.After(500 * time.Millisecond):
		// Expected — no more events.
	}
}
