package analytics

import (
	"github.com/posthog/posthog-go"
)

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
		host = "https://us.i.posthog.com"
	}

	client, err := posthog.NewWithConfig(apiKey, posthog.Config{
		Endpoint: host,
	})
	if err != nil {
		return nil, err
	}

	return &PostHogClient{client: client}, nil
}

// CaptureEvent tracks a product event.
func (c *PostHogClient) CaptureEvent(distinctID, event string, properties map[string]any) {
	if c.client == nil {
		return
	}

	props := posthog.NewProperties()
	for k, v := range properties {
		props.Set(k, v)
	}

	_ = c.client.Enqueue(posthog.Capture{
		DistinctId: distinctID,
		Event:      event,
		Properties: props,
	})
}

// Identify associates properties with a user.
func (c *PostHogClient) Identify(distinctID string, properties map[string]any) {
	if c.client == nil {
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
	if c.client == nil {
		return nil
	}
	return c.client.Close()
}
