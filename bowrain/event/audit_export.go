package event

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	platev "github.com/neokapi/neokapi/bowrain/core/event"
)

// SIEMSink receives serialized audit events for forwarding to an external
// system (SIEM, log aggregator, cold storage). Implementations must be safe for
// use from a single goroutine.
type SIEMSink interface {
	// Export forwards one newline-delimited JSON event. It should return an
	// error on failure so the exporter can log it.
	Export(ctx context.Context, ndjson []byte) error
}

// HTTPSink posts each event as a JSON body to a webhook URL.
type HTTPSink struct {
	URL     string
	Headers map[string]string
	Client  *http.Client
}

// Export posts one event to the webhook.
func (s *HTTPSink) Export(ctx context.Context, ndjson []byte) error {
	client := s.Client
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.URL, bytes.NewReader(ndjson))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-ndjson")
	for k, v := range s.Headers {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("siem sink returned status %d", resp.StatusCode)
	}
	return nil
}

// SIEMExporter subscribes to all events and forwards them to a sink. It buffers
// events on a channel and forwards them from a single worker so it never blocks
// the event bus; on overflow it drops (and logs) rather than stall the system.
type SIEMExporter struct {
	bus    platev.EventBus
	sub    *platev.Subscription
	sink   SIEMSink
	queue  chan platev.Event
	done   chan struct{}
	closed chan struct{}
}

// NewSIEMExporter starts an exporter forwarding all events to sink. A nil sink
// disables export (returns nil).
func NewSIEMExporter(bus platev.EventBus, sink SIEMSink) *SIEMExporter {
	if bus == nil || sink == nil {
		return nil
	}
	e := &SIEMExporter{
		bus:    bus,
		sink:   sink,
		queue:  make(chan platev.Event, 1024),
		done:   make(chan struct{}),
		closed: make(chan struct{}),
	}
	e.sub = bus.SubscribeGroup("siem-exporter", e.enqueue)
	go e.loop()
	return e
}

func (e *SIEMExporter) enqueue(ev platev.Event) {
	select {
	case e.queue <- ev:
	default:
		slog.Warn("siem-exporter: queue full, dropping event", "event_id", ev.ID, "event_type", ev.Type)
	}
}

func (e *SIEMExporter) loop() {
	defer close(e.closed)
	for {
		select {
		case <-e.done:
			return
		case ev := <-e.queue:
			line, err := json.Marshal(ev)
			if err != nil {
				continue
			}
			line = append(line, '\n')
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			if err := e.sink.Export(ctx, line); err != nil {
				slog.Info("siem-exporter: export failed", "event_type", ev.Type, "error", err)
			}
			cancel()
		}
	}
}

// Close unsubscribes and stops the worker.
func (e *SIEMExporter) Close() {
	if e == nil {
		return
	}
	if e.sub != nil {
		e.bus.Unsubscribe(e.sub)
	}
	close(e.done)
	<-e.closed
}
