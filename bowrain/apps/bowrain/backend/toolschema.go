package backend

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/neokapi/neokapi/core/registry"
)

// GetToolSchema returns a tool's option schema as the connected server's
// registry holds it (GET /api/v1/tools/{name}/schema), so a flow step edited
// on the desktop offers the options the server will run the flow with. A tool
// without a schema is nil. Offline, the desktop's own registry answers, the
// way ListFlowDefinitions falls back to the built-in flows.
func (a *App) GetToolSchema(name string) (map[string]any, error) {
	if !a.isConnected() {
		return localToolSchema(a.toolReg, name)
	}
	a.mu.RLock()
	serverURL := a.serverURL
	var token string
	if a.authInfo != nil {
		token = a.authInfo.AccessToken
	}
	a.mu.RUnlock()

	target := serverURL + "/api/v1/tools/" + url.PathEscape(name) + "/schema"
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil //nolint:nilnil // a tool without a schema is a nil document, as the kapi desktop reports it
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("tool schema %s: %d %s", name, resp.StatusCode, b)
	}
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}

// localToolSchema reads a schema from the desktop's own registry, preferring
// the registry's JSON so extension fields pass through unchanged.
func localToolSchema(reg *registry.ToolRegistry, name string) (map[string]any, error) {
	if reg == nil {
		return nil, nil //nolint:nilnil // no registry, no schema
	}
	s := reg.Schema(registry.ToolID(name))
	if s == nil {
		return nil, nil //nolint:nilnil // a tool without a schema is a nil document
	}
	data := s.RawJSON
	if len(data) == 0 {
		var err error
		if data, err = json.Marshal(s); err != nil {
			return nil, err
		}
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return out, nil
}
