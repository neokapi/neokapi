package analytics

import (
	"github.com/posthog/posthog-go"

	"github.com/neokapi/neokapi/core/version"
)

// DefaultHost is the default PostHog ingestion host. All bowrain analytics
// go to the PostHog EU project (EU data residency, Bowrain AD-018 / epic 018).
const DefaultHost = "https://eu.i.posthog.com"

// PostHogClient wraps the PostHog Go SDK for product analytics.
type PostHogClient struct {
	client posthog.Client
}

// NewPostHogClient creates a PostHogClient. If apiKey is empty, all
// operations are no-ops.
func NewPostHogClient(apiKey, host string) (*PostHogClient, error) {
	if apiKey == "" {
		return &PostHogClient{}, nil
	}

	if host == "" {
		host = DefaultHost
	}

	client, err := posthog.NewWithConfig(apiKey, posthog.Config{
		Endpoint: host,
	})
	if err != nil {
		return nil, err
	}

	return &PostHogClient{client: client}, nil
}

// CaptureEvent tracks a product event. Every event automatically carries the
// mandatory taxonomy properties (surface: "server", app_version). When the
// properties include a non-empty workspace_id string, the event is also
// associated with the PostHog "workspace" group ($groups) for group analytics.
func (c *PostHogClient) CaptureEvent(distinctID, event string, properties map[string]any) {
	if c == nil || c.client == nil {
		return
	}

	props := posthog.NewProperties()
	props.Set(PropSurface, SurfaceServer)
	props.Set(PropAppVersion, version.Version)
	for k, v := range properties {
		props.Set(k, v)
	}

	var groups posthog.Groups
	if ws, ok := properties[PropWorkspaceID].(string); ok && ws != "" {
		groups = posthog.NewGroups().Set(GroupWorkspace, ws)
	}

	_ = c.client.Enqueue(posthog.Capture{
		DistinctId: distinctID,
		Event:      event,
		Properties: props,
		Groups:     groups,
	})
}

// Identify associates properties with a user.
func (c *PostHogClient) Identify(distinctID string, properties map[string]any) {
	if c == nil || c.client == nil {
		return
	}

	props := posthog.NewProperties()
	for k, v := range properties {
		props.Set(k, v)
	}

	_ = c.client.Enqueue(posthog.Identify{
		DistinctId: distinctID,
		Properties: props,
	})
}

// Close flushes pending events and closes the client.
func (c *PostHogClient) Close() error {
	if c == nil || c.client == nil {
		return nil
	}
	return c.client.Close()
}
