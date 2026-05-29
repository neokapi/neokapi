package backend

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/neokapi/neokapi/core/flow"
)

// Flow definitions on the desktop (Bowrain AD-013, #766).
//
// Flows are server-side, project-scoped pipeline graphs. The desktop is a
// working copy of the server — it does NOT author flows to a local
// ~/.bowrain/flows store any more. These Wails methods proxy to the Bowrain
// server's flow-definition REST API (/api/v1/{ws}/{projectId}/flows) over the
// authenticated HTTP base URL, so a flow created on the desktop is the same
// row the web superset editor and the automation engine see.
//
// Built-in flows (flow.BuiltInFlows) are always available, even when offline;
// they are merged in by the server's list endpoint, and surfaced locally as a
// fallback when no connection is available.

// FlowDefinitionInfo is the frontend-facing flow definition type. It mirrors
// the framework's flow.FlowDefinition JSON shape so the shared
// @neokapi/flow-editor defToSpec/specToDef adapter consumes it unchanged.
type FlowDefinitionInfo struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Nodes       []FlowNodeInfo `json:"nodes"`
	Edges       []FlowEdgeInfo `json:"edges"`
	Source      string         `json:"source"`
	CreatedAt   string         `json:"created_at,omitempty"`
	ModifiedAt  string         `json:"modified_at,omitempty"`
}

// FlowNodeInfo is the frontend-facing flow node type.
type FlowNodeInfo struct {
	ID    string `json:"id"`
	Type  string `json:"type"`
	Name  string `json:"name"`
	Label string `json:"label,omitempty"`
	// Stage is the pipeline stage for this node. Empty means the main stage;
	// "source-transform" means the leading source-rewrite stage.
	Stage    string         `json:"stage,omitempty"`
	Config   map[string]any `json:"config,omitempty"`
	Position PositionInfo   `json:"position"`
}

// PositionInfo holds x/y for a node.
type PositionInfo struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// FlowEdgeInfo is the frontend-facing flow edge type.
type FlowEdgeInfo struct {
	ID     string `json:"id"`
	Source string `json:"source"`
	Target string `json:"target"`
}

func flowDefToInfo(def flow.FlowDefinition) FlowDefinitionInfo {
	nodes := make([]FlowNodeInfo, len(def.Nodes))
	for i, n := range def.Nodes {
		nodes[i] = FlowNodeInfo{
			ID:       n.ID,
			Type:     string(n.Type),
			Name:     n.Name,
			Label:    n.Label,
			Stage:    string(n.Stage),
			Config:   n.Config,
			Position: PositionInfo{X: n.Position.X, Y: n.Position.Y},
		}
	}
	edges := make([]FlowEdgeInfo, len(def.Edges))
	for i, e := range def.Edges {
		edges[i] = FlowEdgeInfo{
			ID:     e.ID,
			Source: e.Source,
			Target: e.Target,
		}
	}
	return FlowDefinitionInfo{
		ID:          def.ID,
		Name:        def.Name,
		Description: def.Description,
		Nodes:       nodes,
		Edges:       edges,
		Source:      def.Source,
		CreatedAt:   def.CreatedAt,
		ModifiedAt:  def.ModifiedAt,
	}
}

// flowREST returns the authenticated HTTP base for the active project's flow
// endpoints, e.g. "https://host/api/v1/{ws}/{projectId}/flows", plus the bearer
// token. Returns ok=false when not connected or no workspace is selected.
func (a *App) flowREST(projectID string) (base, token string, ok bool) {
	if !a.isConnected() {
		return "", "", false
	}
	a.mu.RLock()
	serverURL := a.serverURL
	ws := a.activeWS
	if a.authInfo != nil {
		token = a.authInfo.AccessToken
	}
	a.mu.RUnlock()
	if serverURL == "" || ws == "" || projectID == "" {
		return "", "", false
	}
	base = fmt.Sprintf("%s/api/v1/%s/%s/flows", serverURL, url.PathEscape(ws), url.PathEscape(projectID))
	return base, token, true
}

func flowDo(method, urlStr, token string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(buf)
	}
	req, err := http.NewRequest(method, urlStr, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("flow API %s %s: %d %s", method, urlStr, resp.StatusCode, b)
	}
	if out != nil && resp.StatusCode != http.StatusNoContent {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

// builtInFlowInfos returns the built-in flows as frontend infos. Used as the
// offline fallback for ListFlowDefinitions.
func builtInFlowInfos() []FlowDefinitionInfo {
	defs := flow.BuiltInFlows()
	infos := make([]FlowDefinitionInfo, len(defs))
	for i, def := range defs {
		infos[i] = flowDefToInfo(def)
	}
	return infos
}

// ListFlowDefinitions returns the project's flows (built-in + project-stored)
// from the server. When offline, only the built-in flows are returned.
func (a *App) ListFlowDefinitions(projectID string) ([]FlowDefinitionInfo, error) {
	base, token, ok := a.flowREST(projectID)
	if !ok {
		return builtInFlowInfos(), nil
	}
	var out []FlowDefinitionInfo
	if err := flowDo(http.MethodGet, base, token, nil, &out); err != nil {
		// Server unreachable — fall back to built-ins so the editor still works.
		return builtInFlowInfos(), nil
	}
	if out == nil {
		out = []FlowDefinitionInfo{}
	}
	return out, nil
}

// GetFlowDefinition returns a specific flow definition by id from the server.
func (a *App) GetFlowDefinition(projectID, id string) (*FlowDefinitionInfo, error) {
	base, token, ok := a.flowREST(projectID)
	if !ok {
		for _, info := range builtInFlowInfos() {
			if info.ID == id {
				return &info, nil
			}
		}
		return nil, fmt.Errorf("flow %q not found (offline)", id)
	}
	var out FlowDefinitionInfo
	if err := flowDo(http.MethodGet, base+"/"+url.PathEscape(id), token, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// SaveFlowDefinition creates or updates a project flow definition on the server.
// Built-in flows are immutable; the server rejects writes to them.
func (a *App) SaveFlowDefinition(projectID string, info FlowDefinitionInfo) (*FlowDefinitionInfo, error) {
	base, token, ok := a.flowREST(projectID)
	if !ok {
		return nil, fmt.Errorf("not connected: connect to a server to save flows")
	}
	if info.Source == "built-in" {
		return nil, fmt.Errorf("cannot modify built-in flows")
	}
	info.Source = "project"

	var out FlowDefinitionInfo
	if info.ID == "" {
		// Create.
		if err := flowDo(http.MethodPost, base, token, info, &out); err != nil {
			return nil, err
		}
		return &out, nil
	}
	// Update (create-or-replace by id).
	if err := flowDo(http.MethodPut, base+"/"+url.PathEscape(info.ID), token, info, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteFlowDefinition removes a project flow definition on the server.
func (a *App) DeleteFlowDefinition(projectID, id string) error {
	base, token, ok := a.flowREST(projectID)
	if !ok {
		return fmt.Errorf("not connected: connect to a server to delete flows")
	}
	return flowDo(http.MethodDelete, base+"/"+url.PathEscape(id), token, nil, nil)
}
