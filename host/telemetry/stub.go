//go:build notelemetry

package telemetry

import (
	"time"

	"github.com/neokapi/neokapi/host/config"
)

// compiledOut marks this binary as built with the notelemetry tag: the
// whole capture client is compiled out, Resolve always reports disabled,
// and New never constructs a client.
const compiledOut = true

// Client is the compile-time stub: no queue, no goroutine, no net/http.
type Client struct{}

// New always returns nil in notelemetry builds.
func New(*config.AppConfig, Options) *Client { return nil }

// Capture is a no-op in notelemetry builds.
func (c *Client) Capture(string, map[string]any) {}

// Close is a no-op in notelemetry builds.
func (c *Client) Close(time.Duration) {}
