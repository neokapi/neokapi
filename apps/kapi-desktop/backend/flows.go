package backend

import (
	"cmp"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/registry"
	"gopkg.in/yaml.v3"
)

// UserFlowInfo is the frontend-facing flow summary.
type UserFlowInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Source      string `json:"source"` // registry.SourceBuiltIn or "user"
	StepCount   int    `json:"step_count"`
	Modified    string `json:"modified,omitempty"`
}

// UserFlowDetail is the full flow data for editing.
type UserFlowDetail struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Source      string          `json:"source"`
	Steps       []flow.FlowStep `json:"steps"`
}

// SaveUserFlowRequest is the request to save a user flow.
type SaveUserFlowRequest struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Steps       []flow.FlowStep `json:"steps"`
}

// userFlowFile is the on-disk format for user flows.
type userFlowFile struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Steps       []flow.FlowStep `json:"steps"`
	ModifiedAt  string          `json:"modified_at"`
}

func userFlowsDir() string {
	return namedResourceDir("flows")
}

// ListUserFlows returns built-in + user-saved flows.
func (a *App) ListUserFlows() []UserFlowInfo {
	var result []UserFlowInfo

	// Built-in flows.
	for _, def := range flow.BuiltInFlows() {
		stepCount := 0
		for _, n := range def.Nodes {
			if n.Type == flow.NodeTool {
				stepCount++
			}
		}
		result = append(result, UserFlowInfo{
			ID:          def.ID,
			Name:        def.Name,
			Description: def.Description,
			Source:      registry.SourceBuiltIn,
			StepCount:   stepCount,
		})
	}

	// User flows from ~/.config/kapi/flows/.
	dir := userFlowsDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return result
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		uf, err := loadUserFlow(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		result = append(result, UserFlowInfo{
			ID:          uf.ID,
			Name:        uf.Name,
			Description: uf.Description,
			Source:      "user",
			StepCount:   len(uf.Steps),
			Modified:    uf.ModifiedAt,
		})
	}

	slices.SortFunc(result, func(a, b UserFlowInfo) int {
		// Built-in first, then user by modified time.
		if a.Source != b.Source {
			if a.Source == registry.SourceBuiltIn {
				return -1
			}
			return 1
		}
		return cmp.Compare(b.Modified, a.Modified)
	})

	return result
}

// GetUserFlow returns a flow by ID (built-in or user).
func (a *App) GetUserFlow(id string) *UserFlowDetail {
	// Check built-in flows — convert graph to steps manually.
	for _, def := range flow.BuiltInFlows() {
		if def.ID == id {
			steps := graphToSteps(&def)
			return &UserFlowDetail{
				ID:          def.ID,
				Name:        def.Name,
				Description: def.Description,
				Source:      registry.SourceBuiltIn,
				Steps:       steps,
			}
		}
	}

	// Check user flows.
	path := filepath.Join(userFlowsDir(), id+".json")
	uf, err := loadUserFlow(path)
	if err != nil {
		return nil
	}
	return &UserFlowDetail{
		ID:          uf.ID,
		Name:        uf.Name,
		Description: uf.Description,
		Source:      "user",
		Steps:       uf.Steps,
	}
}

// SaveUserFlow persists a flow to ~/.config/kapi/flows/.
func (a *App) SaveUserFlow(req SaveUserFlowRequest) error {
	dir := userFlowsDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create flows directory: %w", err)
	}

	uf := userFlowFile{
		ID:          req.ID,
		Name:        req.Name,
		Description: req.Description,
		Steps:       req.Steps,
		ModifiedAt:  time.Now().Format(time.RFC3339),
	}

	data, err := json.MarshalIndent(uf, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal flow: %w", err)
	}

	path := filepath.Join(dir, req.ID+".json")
	return os.WriteFile(path, data, 0o644)
}

// DeleteUserFlow deletes a user flow. Returns error for built-in flows.
func (a *App) DeleteUserFlow(id string) error {
	for _, def := range flow.BuiltInFlows() {
		if def.ID == id {
			return fmt.Errorf("cannot delete built-in flow %q", id)
		}
	}

	path := filepath.Join(userFlowsDir(), id+".json")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete flow: %w", err)
	}
	return nil
}

// CopyBuiltInFlow creates a user copy of a built-in flow.
func (a *App) CopyBuiltInFlow(builtInID, newName string) (string, error) {
	detail := a.GetUserFlow(builtInID)
	if detail == nil {
		return "", fmt.Errorf("flow %q not found", builtInID)
	}

	newID := strings.ToLower(strings.ReplaceAll(newName, " ", "-"))
	if err := a.SaveUserFlow(SaveUserFlowRequest{
		ID:          newID,
		Name:        newName,
		Description: detail.Description,
		Steps:       detail.Steps,
	}); err != nil {
		return "", err
	}
	return newID, nil
}

// OpenFlowFileDialog shows a native file dialog to open a flow YAML file.
func (a *App) OpenFlowFileDialog() (*UserFlowDetail, error) {
	if a.app == nil {
		return nil, nil
	}
	path, err := a.app.Dialog.OpenFile().
		AddFilter("Flow Files", "*.yaml;*.yml;*.json").
		AddFilter("All Files", "*").
		PromptForSingleSelection()
	if err != nil {
		return nil, err
	}
	if path == "" {
		return nil, nil
	}
	return a.openFlowFile(path)
}

func (a *App) openFlowFile(path string) (*UserFlowDetail, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read flow file: %w", err)
	}

	// Try JSON first.
	var uf userFlowFile
	if err := json.Unmarshal(data, &uf); err == nil && len(uf.Steps) > 0 {
		return &UserFlowDetail{
			ID:          uf.ID,
			Name:        uf.Name,
			Description: uf.Description,
			Source:      "file",
			Steps:       uf.Steps,
		}, nil
	}

	// Try YAML (StepsSpec format).
	var spec flow.StepsSpec
	if err := yaml.Unmarshal(data, &spec); err == nil && len(spec.Steps) > 0 {
		name := filepath.Base(path)
		name = strings.TrimSuffix(name, filepath.Ext(name))
		return &UserFlowDetail{
			ID:     name,
			Name:   name,
			Source: "file",
			Steps:  spec.Steps,
		}, nil
	}

	return nil, fmt.Errorf("could not parse %q as a flow file", path)
}

// SaveFlowFileDialog shows a native save dialog and writes a flow as YAML.
func (a *App) SaveFlowFileDialog(name string, steps []flow.FlowStep) error {
	if a.app == nil {
		return nil
	}
	path, err := a.app.Dialog.SaveFile().
		AddFilter("YAML Files", "*.yaml").
		AddFilter("JSON Files", "*.json").
		SetFilename(name + ".yaml").
		PromptForSingleSelection()
	if err != nil {
		return err
	}
	if path == "" {
		return nil
	}

	spec := flow.StepsSpec{Steps: steps}

	ext := strings.ToLower(filepath.Ext(path))
	if ext == ".json" {
		data, err := json.MarshalIndent(spec, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal flow: %w", err)
		}
		return os.WriteFile(path, data, 0o644)
	}

	// Default to YAML.
	if ext != ".yaml" && ext != ".yml" {
		path += ".yaml"
	}
	data, err := yaml.Marshal(spec)
	if err != nil {
		return fmt.Errorf("marshal flow: %w", err)
	}
	return os.WriteFile(path, data, 0o644)
}

// graphToSteps extracts tool steps from a FlowDefinition in topological order.
func graphToSteps(def *flow.FlowDefinition) []flow.FlowStep {
	// Sort tool nodes by X position (left to right).
	type toolNode struct {
		name   string
		label  string
		config map[string]any
		x      float64
	}
	var tools []toolNode
	for _, n := range def.Nodes {
		if n.Type == flow.NodeTool {
			tools = append(tools, toolNode{
				name:   n.Name,
				label:  n.Label,
				config: n.Config,
				x:      n.Position.X,
			})
		}
	}
	slices.SortFunc(tools, func(a, b toolNode) int { return cmp.Compare(a.x, b.x) })

	steps := make([]flow.FlowStep, len(tools))
	for i, t := range tools {
		steps[i] = flow.FlowStep{
			Tool:   t.name,
			Label:  t.label,
			Config: t.config,
		}
	}
	return steps
}

func loadUserFlow(path string) (*userFlowFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var uf userFlowFile
	if err := json.Unmarshal(data, &uf); err != nil {
		return nil, err
	}
	return &uf, nil
}
