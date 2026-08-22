package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// A coordinate is minted by the recipe and by nothing else, so an axis approved
// on the server is not real until it reaches kapi.yaml here and a push carries
// content at it. These two calls are the client half of that hand-off: read what
// is waiting, and say when the working tree has taken it.

// PendingRecipeChange is one recipe field an approval wants set, in the shape a
// `kapi apply` recipe entry already uses. The caller hands Path and Value to the
// same allowlisted setter the local fix loop uses, so there is one writer.
type PendingRecipeChange struct {
	ID          string          `json:"id"`
	WorkspaceID string          `json:"workspace_id"`
	ProjectID   string          `json:"project_id"`
	Path        string          `json:"path"`
	Value       json.RawMessage `json:"value"`
	Status      string          `json:"status"`
	CreatedBy   string          `json:"created_by,omitempty"`
	CreatedAt   time.Time       `json:"created_at"`
}

// PendingRecipeChanges lists what this project's working tree has not yet taken,
// oldest first so they apply in the order they were decided.
func (c *BowrainClient) PendingRecipeChanges(ctx context.Context) ([]PendingRecipeChange, error) {
	if c.workspace == "" || c.projectID == "" {
		return nil, errors.New("recipe changes are project-scoped (use NewWorkspaceBowrainClient)")
	}

	path := "/projects/" + url.PathEscape(c.projectID) + "/recipe-changes"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.wsPrefix()+path, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	resp, err := c.Do(req)
	if err != nil {
		return nil, fmt.Errorf("list pending recipe changes: %w", err)
	}
	defer resp.Body.Close()

	// A server that does not know this endpoint is one that predates the
	// hand-off, not a failure: an older venue simply has nothing pending, and a
	// pull against it must still pull.
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("list pending recipe changes: unexpected status %d", resp.StatusCode)
	}

	var out []PendingRecipeChange
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode pending recipe changes: %w", err)
	}
	return out, nil
}

// MarkRecipeChangeApplied settles one change once the working tree holds it.
// Marking twice is a no-op on the server: a retried pull must converge.
func (c *BowrainClient) MarkRecipeChangeApplied(ctx context.Context, changeID string) error {
	if c.workspace == "" || c.projectID == "" {
		return errors.New("recipe changes are project-scoped (use NewWorkspaceBowrainClient)")
	}

	path := "/projects/" + url.PathEscape(c.projectID) +
		"/recipe-changes/" + url.PathEscape(changeID) + "/applied"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.wsPrefix()+path, bytes.NewReader(nil))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	resp, err := c.Do(req)
	if err != nil {
		return fmt.Errorf("mark recipe change applied: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("mark recipe change applied: unexpected status %d", resp.StatusCode)
	}
	return nil
}
