package editorclient

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/neokapi/neokapi/host/venue/client"
)

// This file adds the real-time editor surface — block review, presence
// reporting, and the project change-event subscription — that the Bowrain
// desktop used to reach over the bespoke gRPC EditorService (WatchProject +
// UpdatePresence + ReviewBlock). They travel over the same REST/SSE routes the
// web app already uses.

// EditorChangeEvent mirrors event.ChangeEvent relayed by GET /api/v1/:ws/events.
// It is a flattened projection of a platform event: the discriminating type
// plus the identifying fields a client needs to decide what to refresh.
// Presence events ("editor.presence.*") additionally carry the acting user's
// identity.
type EditorChangeEvent struct {
	Type       string `json:"type"`
	ProjectID  string `json:"projectId,omitempty"`
	Stream     string `json:"stream,omitempty"`
	ItemName   string `json:"itemName,omitempty"`
	BlockID    string `json:"blockId,omitempty"`
	ChangedBy  string `json:"changedBy,omitempty"`
	ChangeType string `json:"changeType,omitempty"`
	Actor      string `json:"actor,omitempty"`

	UserID    string `json:"userId,omitempty"`
	UserName  string `json:"userName,omitempty"`
	AvatarURL string `json:"avatarUrl,omitempty"`
}

// ReviewBlock sets or clears the reviewed status on the block's target for ONE
// locale — the per-locale model.Target.Status ladder rung, distinct from the
// governance status lifecycle. The server rejects reviewing a locale that has
// no non-empty translation (422).
//
// status optionally picks the rung, and each direction has its own two. With
// reviewed=true it is "" for an approval (landing on reviewed) or "signed-off"
// for a sign-off. With reviewed=false it is "" or "translated" for a plain
// un-review, "draft" for a reviewer rejection (the unit re-enters the work
// queue). Any other pairing is a 400.
//
// PUT /api/v1/:ws/:id/blocks/main/:bid/review
func (c *EditorClient) ReviewBlock(ctx context.Context, ws, projectID, itemName, blockID, targetLocale string, reviewed bool, status string) error {
	body := struct {
		TargetLocale string `json:"target_locale"`
		ItemName     string `json:"item_name,omitempty"`
		Reviewed     bool   `json:"reviewed"`
		Status       string `json:"status,omitempty"`
	}{TargetLocale: targetLocale, ItemName: itemName, Reviewed: reviewed, Status: status}
	return c.DoJSON(ctx, http.MethodPut, blockPath(ws, projectID, blockID, "/review"), nil, body, nil)
}

// ReportPresence records the caller's editing focus in a project. The server
// publishes an "editor.presence.moved" event that watchers receive over the
// /:ws/events SSE stream. Best-effort: a server without an event bus returns 204.
//
// POST /api/v1/:ws/:id/presence
func (c *EditorClient) ReportPresence(ctx context.Context, ws, projectID, itemName, blockID string) error {
	path := fmt.Sprintf("/api/v1/%s/%s/presence", url.PathEscape(ws), url.PathEscape(projectID))
	body := struct {
		ItemName string `json:"item_name,omitempty"`
		BlockID  string `json:"block_id,omitempty"`
	}{ItemName: itemName, BlockID: blockID}
	return c.DoJSON(ctx, http.MethodPost, path, nil, body, nil)
}

// StreamProjectEvents opens one SSE connection to the project's change-event
// stream and invokes onEvent for each frame until the connection ends or ctx is
// cancelled. It reads a single connection to exhaustion — callers that want
// resilience wrap it in a reconnect loop (the desktop watcher does). It returns
// a non-nil error only for a hard failure (bad HTTP status, request build);
// ctx cancellation returns ctx.Err(); a clean/transport EOF returns nil so the
// caller can reconnect.
//
// GET /api/v1/:ws/events?project=:id
func (c *EditorClient) StreamProjectEvents(ctx context.Context, ws, projectID string, onEvent func(EditorChangeEvent)) error {
	u := fmt.Sprintf("%s/api/v1/%s/events?project=%s", c.BaseURL(), url.PathEscape(ws), url.QueryEscape(projectID))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "text/event-stream")
	c.ApplyAuth(req)

	streamClient := &http.Client{} // no timeout: SSE is long-lived, ctx cancels it
	resp, err := streamClient.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return nil // transport error → caller reconnects
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return client.NewStatusError("subscribe project events", resp.StatusCode, respBody)
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for scanner.Scan() {
		data, ok := strings.CutPrefix(scanner.Text(), "data:")
		if !ok {
			continue // SSE comments, event: lines, id: lines, blank separators
		}
		data = strings.TrimSpace(data)
		if data == "" {
			continue
		}
		var ev EditorChangeEvent
		if err := json.Unmarshal([]byte(data), &ev); err != nil {
			continue // tolerate a malformed frame rather than abort the stream
		}
		if onEvent != nil {
			onEvent(ev)
		}
	}
	if err := scanner.Err(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return nil // read error mid-stream → caller reconnects
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return nil // clean EOF → caller reconnects
}
