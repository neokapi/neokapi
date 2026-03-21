package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/coder/websocket"
	"github.com/neokapi/neokapi/bowrain/agentic-testing/agenticmcp"
)

func registerHandlers(mux *http.ServeMux, mcpSrv *agenticmcp.Server) {
	mux.HandleFunc("GET /api/v1/agentic/agents", handleListAgents(mcpSrv))
	mux.HandleFunc("GET /api/v1/agentic/executions", handleListExecutions(mcpSrv))
	mux.HandleFunc("GET /api/v1/agentic/executions/{id}/events", handleGetExecutionEvents(mcpSrv))
	mux.HandleFunc("GET /api/v1/agentic/events", handleListEvents(mcpSrv))
	mux.HandleFunc("GET /api/v1/agentic/events/ws", handleEventsWebSocket(mcpSrv))
	mux.HandleFunc("GET /api/v1/agentic/issues", handleListIssues(mcpSrv))
}

func handleListAgents(s *agenticmcp.Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		store := s.ExecStore()
		if store == nil {
			http.Error(w, "execution store not configured", http.StatusServiceUnavailable)
			return
		}
		agents, err := store.ListAgents(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]any{"agents": agents})
	}
}

func handleListExecutions(s *agenticmcp.Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		store := s.ExecStore()
		if store == nil {
			http.Error(w, "execution store not configured", http.StatusServiceUnavailable)
			return
		}
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		if limit <= 0 {
			limit = 50
		}
		execs, err := store.ListExecutions(r.Context(), agenticmcp.ExecutionFilter{
			WorkspaceSlug: r.URL.Query().Get("workspace"),
			Agent:         r.URL.Query().Get("agent"),
			Since:         r.URL.Query().Get("since"),
			Limit:         limit,
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]any{"executions": execs})
	}
}

func handleGetExecutionEvents(s *agenticmcp.Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		store := s.ExecStore()
		if store == nil {
			http.Error(w, "execution store not configured", http.StatusServiceUnavailable)
			return
		}
		execID := r.PathValue("id")
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		if limit <= 0 {
			limit = 200
		}
		events, err := store.ListEvents(r.Context(), agenticmcp.EventFilter{
			ExecutionID: execID,
			Limit:       limit,
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]any{"execution_id": execID, "events": events})
	}
}

func handleListEvents(s *agenticmcp.Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		store := s.ExecStore()
		if store == nil {
			http.Error(w, "execution store not configured", http.StatusServiceUnavailable)
			return
		}
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		if limit <= 0 {
			limit = 100
		}
		events, err := store.ListEvents(r.Context(), agenticmcp.EventFilter{
			ExecutionID:   r.URL.Query().Get("execution_id"),
			WorkspaceSlug: r.URL.Query().Get("workspace"),
			EventType:     r.URL.Query().Get("event_type"),
			Limit:         limit,
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]any{"events": events})
	}
}

func handleEventsWebSocket(s *agenticmcp.Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		hub := s.EventHub()
		if hub == nil {
			http.Error(w, "event hub not configured", http.StatusServiceUnavailable)
			return
		}

		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			OriginPatterns: []string{"*"},
		})
		if err != nil {
			return
		}
		defer func() { _ = conn.CloseNow() }()

		client := &agenticmcp.EventClient{
			C:             make(chan agenticmcp.AgenticEvent, 64),
			WorkspaceSlug: r.URL.Query().Get("workspace"),
		}
		hub.Subscribe(client)
		defer hub.Unsubscribe(client)

		ctx := r.Context()

		// Read loop to detect disconnection.
		go func() {
			for {
				_, _, err := conn.Read(ctx)
				if err != nil {
					return
				}
			}
		}()

		// Write loop.
		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-client.C:
				if !ok {
					return
				}
				data, err := json.Marshal(ev)
				if err != nil {
					continue
				}
				writeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
				err = conn.Write(writeCtx, websocket.MessageText, data)
				cancel()
				if err != nil {
					return
				}
			}
		}
	}
}

func handleListIssues(s *agenticmcp.Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tracker := s.IssueTracker()
		if tracker == nil {
			http.Error(w, "issue tracker not configured", http.StatusServiceUnavailable)
			return
		}
		state := r.URL.Query().Get("state")
		if state == "" {
			state = "all"
		}
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		if limit <= 0 {
			limit = 20
		}
		issues, err := tracker.ListIssues(r.Context(), state, limit)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]any{"issues": issues})
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
