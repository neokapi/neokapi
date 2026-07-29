package event

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	platev "github.com/neokapi/neokapi/bowrain/core/event"
)

// WebhookConfig configures a webhook delivery endpoint.
type WebhookConfig struct {
	URL    string
	Secret string // HMAC-SHA256 signing secret
}

// WebhookDelivery delivers events to HTTP webhook endpoints
// with HMAC-SHA256 signing and retry with exponential backoff.
type WebhookDelivery struct {
	config  WebhookConfig
	client  *http.Client
	retries int
}

// NewWebhookDelivery creates a new webhook delivery system.
func NewWebhookDelivery(config WebhookConfig) *WebhookDelivery {
	return &WebhookDelivery{
		config:  config,
		client:  &http.Client{Timeout: 10 * time.Second},
		retries: 3,
	}
}

// Deliver sends an event to the webhook endpoint.
//
// ctx bounds the whole delivery, backoff included. Three attempts spend around
// seven seconds waiting between them, so a delivery that ignored cancellation
// would hold a shutting-down process open sleeping against an endpoint nobody
// is waiting for any more.
func (w *WebhookDelivery) Deliver(ctx context.Context, event platev.Event) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}

	signature := w.sign(payload)

	var lastErr error
	for attempt := range w.retries {
		if attempt > 0 {
			// Exponential backoff: 1s, 2s, 4s...
			backoff := time.NewTimer(time.Duration(1<<uint(attempt-1)) * time.Second)
			select {
			case <-ctx.Done():
				backoff.Stop()
				if lastErr != nil {
					return fmt.Errorf("webhook delivery abandoned after %d attempts: %w (last error: %w)", attempt, ctx.Err(), lastErr)
				}
				return fmt.Errorf("webhook delivery abandoned after %d attempts: %w", attempt, ctx.Err())
			case <-backoff.C:
			}
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.config.URL, bytes.NewReader(payload))
		if err != nil {
			return fmt.Errorf("create request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Neokapi-Signature", signature)
		req.Header.Set("X-Neokapi-Event", string(event.Type))

		resp, err := w.client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		resp.Body.Close()

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return nil
		}
		lastErr = fmt.Errorf("webhook returned HTTP %d", resp.StatusCode)
	}

	return fmt.Errorf("webhook delivery failed after %d attempts: %w", w.retries, lastErr)
}

// sign computes the HMAC-SHA256 signature of the payload.
func (w *WebhookDelivery) sign(payload []byte) string {
	mac := hmac.New(sha256.New, []byte(w.config.Secret))
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

// VerifySignature checks an HMAC-SHA256 signature against a payload and secret.
func VerifySignature(payload []byte, secret, signature string) bool {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(signature))
}
