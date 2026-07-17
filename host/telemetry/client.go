//go:build !notelemetry

package telemetry

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"maps"
	"net/http"
	"runtime"
	"sync"
	"time"

	"github.com/neokapi/neokapi/host/config"
)

// compiledOut is false in normal builds; the notelemetry build tag swaps in
// stub.go, which stubs the whole client out at compile time.
const compiledOut = false

// queueSize bounds the in-memory event queue. A CLI process emits at most a
// couple of events per run; anything beyond the buffer is dropped, never
// waited on.
const queueSize = 32

// event is one queued capture.
type event struct {
	name  string
	props map[string]any
	ts    time.Time
}

// Client is a fire-and-forget PostHog capture client: Capture enqueues into
// a bounded in-memory queue drained by a background goroutine that posts
// batches over plain net/http. All methods are nil-safe; failures are
// silent (debug-log only) and never block or fail the running command.
type Client struct {
	endpoint   string
	apiKey     string
	distinctID string
	base       map[string]any

	httpc *http.Client
	queue chan event
	done  chan struct{}

	mu     sync.Mutex
	closed bool
}

// New returns a running Client, or nil when the opt-out lattice disables
// telemetry (Resolve). The nil client is safe to use: Capture and Close are
// no-ops on it, so callers never branch.
func New(cfg *config.AppConfig, opts Options) *Client {
	st := Resolve(cfg)
	if !st.Enabled {
		debugf("disabled: %s", st.Reason)
		return nil
	}
	id := opts.DistinctID
	if id == "" {
		id = MachineID(cfg)
	}
	c := &Client{
		endpoint:   resolvedEndpoint(),
		apiKey:     apiKey,
		distinctID: id,
		base: map[string]any{
			"surface":     opts.Surface,
			"app_version": opts.Version,
			"os":          runtime.GOOS,
			"arch":        runtime.GOARCH,
		},
		httpc: &http.Client{Timeout: 2 * time.Second},
		queue: make(chan event, queueSize),
		done:  make(chan struct{}),
	}
	go c.loop()
	return c
}

// Capture enqueues an event. It never blocks: when the queue is full the
// event is dropped. Safe on a nil or closed client.
func (c *Client) Capture(name string, props map[string]any) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	select {
	case c.queue <- event{name: name, props: props, ts: time.Now().UTC()}:
	default:
		debugf("queue full, dropped %s", name)
	}
}

// Close stops the client and waits at most wait for the background goroutine
// to flush what it can. Anything still in flight when the wait expires is
// abandoned — exiting the process must never be delayed by telemetry.
func (c *Client) Close(wait time.Duration) {
	if c == nil {
		return
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.closed = true
	close(c.queue)
	c.mu.Unlock()

	select {
	case <-c.done:
	case <-time.After(wait):
		debugf("flush wait expired after %s", wait)
	}
}

// loop drains the queue, coalescing whatever is immediately available into
// one /batch POST per wakeup. It exits when the queue is closed and empty.
func (c *Client) loop() {
	defer close(c.done)
	for {
		ev, ok := <-c.queue
		if !ok {
			return
		}
		batch := []event{ev}
	drain:
		for {
			select {
			case next, more := <-c.queue:
				if !more {
					c.send(batch)
					return
				}
				batch = append(batch, next)
			default:
				break drain
			}
		}
		c.send(batch)
	}
}

// send posts one batch to the PostHog batch endpoint. Best-effort: any
// error is swallowed (debug-logged) — telemetry can never fail a command.
func (c *Client) send(batch []event) {
	items := make([]map[string]any, 0, len(batch))
	for _, ev := range batch {
		props := make(map[string]any, len(c.base)+len(ev.props)+1)
		maps.Copy(props, c.base)
		maps.Copy(props, ev.props)
		props["$lib"] = "neokapi-host-telemetry"
		items = append(items, map[string]any{
			"event":       ev.name,
			"distinct_id": c.distinctID,
			"timestamp":   ev.ts.Format(time.RFC3339),
			"properties":  props,
		})
	}
	body, err := json.Marshal(map[string]any{
		"api_key": c.apiKey,
		"batch":   items,
	})
	if err != nil {
		debugf("marshal batch: %v", err)
		return
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost,
		c.endpoint+"/batch/", bytes.NewReader(body))
	if err != nil {
		debugf("build request: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpc.Do(req)
	if err != nil {
		debugf("post batch: %v", err)
		return
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	debugf("posted %d event(s): %s", len(items), resp.Status)
}
