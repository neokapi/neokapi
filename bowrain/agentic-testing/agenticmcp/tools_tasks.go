package agenticmcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// registerTaskTools registers Bowrain task API tools for persona agents (AD-034).
func (s *Server) registerTaskTools() {
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "list_my_tasks",
		Description: "List open tasks assigned to the current agent. Returns tasks with type, locale, items, and mode. Use this to find work assigned by the workflow system.",
	}, s.handleListMyTasks)

	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "claim_task",
		Description: "Claim (assign) a task to start working on it. Sets the task status to in_progress.",
	}, s.handleClaimTask)

	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "complete_task",
		Description: "Mark a task as completed after finishing the translation or review work.",
	}, s.handleCompleteTask)
}

// ── list_my_tasks ────────────────────────────────────────────────────────

type listMyTasksInput struct {
	WorkspaceSlug string `json:"workspace_slug" jsonschema:"workspace slug to query tasks for"`
	Status        string `json:"status,omitempty" jsonschema:"filter by status (default: open)"`
}

type taskInfo struct {
	ID          string            `json:"id"`
	Type        string            `json:"type"`
	Status      string            `json:"status"`
	Priority    string            `json:"priority"`
	Title       string            `json:"title"`
	Description string            `json:"description,omitempty"`
	ProjectID   string            `json:"project_id"`
	Data        map[string]string `json:"data,omitempty"`
	CreatedAt   string            `json:"created_at"`
}

type listMyTasksOutput struct {
	Tasks []taskInfo `json:"tasks"`
}

func (s *Server) handleListMyTasks(ctx context.Context, req *mcp.CallToolRequest, input listMyTasksInput) (*mcp.CallToolResult, listMyTasksOutput, error) {
	if s.bowrainURL == "" {
		return nil, listMyTasksOutput{}, errors.New("bowrain API not configured")
	}

	status := input.Status
	if status == "" {
		status = "open"
	}

	path := fmt.Sprintf("/api/v1/workspaces/%s/my/tasks?status=%s", input.WorkspaceSlug, status)
	body, err := s.bowrainGet(ctx, path)
	if err != nil {
		return nil, listMyTasksOutput{}, fmt.Errorf("list tasks: %w", err)
	}

	var resp struct {
		Tasks []taskInfo `json:"tasks"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, listMyTasksOutput{}, fmt.Errorf("parse tasks: %w", err)
	}

	return nil, listMyTasksOutput{Tasks: resp.Tasks}, nil
}

// ── claim_task ──────────────────────────────────────────────────────────

type claimTaskInput struct {
	WorkspaceSlug string `json:"workspace_slug" jsonschema:"workspace slug"`
	TaskID        string `json:"task_id" jsonschema:"ID of the task to claim"`
}

type claimTaskOutput struct {
	OK bool `json:"ok"`
}

func (s *Server) handleClaimTask(ctx context.Context, req *mcp.CallToolRequest, input claimTaskInput) (*mcp.CallToolResult, claimTaskOutput, error) {
	if s.bowrainURL == "" {
		return nil, claimTaskOutput{}, errors.New("bowrain API not configured")
	}

	path := fmt.Sprintf("/api/v1/workspaces/%s/tasks/%s/assign", input.WorkspaceSlug, input.TaskID)
	_, err := s.bowrainPost(ctx, path, nil)
	if err != nil {
		return nil, claimTaskOutput{}, fmt.Errorf("claim task: %w", err)
	}

	return nil, claimTaskOutput{OK: true}, nil
}

// ── complete_task ───────────────────────────────────────────────────────

type completeTaskInput struct {
	WorkspaceSlug string `json:"workspace_slug" jsonschema:"workspace slug"`
	TaskID        string `json:"task_id" jsonschema:"ID of the task to complete"`
}

type completeTaskOutput struct {
	OK bool `json:"ok"`
}

func (s *Server) handleCompleteTask(ctx context.Context, req *mcp.CallToolRequest, input completeTaskInput) (*mcp.CallToolResult, completeTaskOutput, error) {
	if s.bowrainURL == "" {
		return nil, completeTaskOutput{}, errors.New("bowrain API not configured")
	}

	path := fmt.Sprintf("/api/v1/workspaces/%s/tasks/%s/complete", input.WorkspaceSlug, input.TaskID)
	_, err := s.bowrainPost(ctx, path, nil)
	if err != nil {
		return nil, completeTaskOutput{}, fmt.Errorf("complete task: %w", err)
	}

	return nil, completeTaskOutput{OK: true}, nil
}

// ── HTTP helpers ────────────────────────────────────────────────────────

func (s *Server) bowrainGet(ctx context.Context, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.bowrainURL+path, nil)
	if err != nil {
		return nil, err
	}
	if s.bowrainToken != "" {
		req.Header.Set("Authorization", "Bearer "+s.bowrainToken)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}
	return body, nil
}

func (s *Server) bowrainPost(ctx context.Context, path string, payload any) ([]byte, error) {
	var bodyReader io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		bodyReader = strings.NewReader(string(data))
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.bowrainURL+path, bodyReader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if s.bowrainToken != "" {
		req.Header.Set("Authorization", "Bearer "+s.bowrainToken)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}
	return body, nil
}
