package main

import (
	"net/http"
	"strconv"

	agentictesting "github.com/neokapi/neokapi/bowrain/agentic-testing"
)

func registerBowrainHandlers(mux *http.ServeMux, cfg config) {
	client := &agentictesting.BowrainClient{
		BaseURL: cfg.BowrainAPIURL,
		Token:   cfg.BowrainAPIToken,
	}

	mux.HandleFunc("GET /api/v1/workspaces", handleWorkspaces(client))
	mux.HandleFunc("GET /api/v1/workspaces/{ws}/projects", handleProjects(client))
	mux.HandleFunc("GET /api/v1/workspaces/{ws}/members", handleMembers(client))
	mux.HandleFunc("GET /api/v1/workspaces/{ws}/audit-log", handleAuditLog(client))
	mux.HandleFunc("GET /api/v1/workspaces/{ws}/projects/{id}/sync/blocks", handleBlocks(client))
}

func handleWorkspaces(c *agentictesting.BowrainClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		workspaces, err := c.ListWorkspaces(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		writeJSON(w, map[string]any{"workspaces": workspaces})
	}
}

func handleProjects(c *agentictesting.BowrainClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		projects, err := c.ListProjects(r.Context(), r.PathValue("ws"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		writeJSON(w, map[string]any{"projects": projects})
	}
}

func handleMembers(c *agentictesting.BowrainClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		members, err := c.ListMembers(r.Context(), r.PathValue("ws"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		writeJSON(w, map[string]any{"members": members})
	}
}

func handleAuditLog(c *agentictesting.BowrainClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		if limit <= 0 {
			limit = 50
		}
		entries, err := c.ListAuditLog(r.Context(), r.PathValue("ws"), limit)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		writeJSON(w, map[string]any{"entries": entries})
	}
}

func handleBlocks(c *agentictesting.BowrainClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		blocks, err := c.ListBlocks(r.Context(), r.PathValue("ws"), r.PathValue("id"), agentictesting.BlockListOptions{
			Locale: r.URL.Query().Get("locale"),
			Status: r.URL.Query().Get("status"),
			Limit:  limit,
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		writeJSON(w, map[string]any{"blocks": blocks})
	}
}
