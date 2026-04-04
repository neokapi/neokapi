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
func (w *WebhookDelivery) Deliver(event platev.Event) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}

	signature := w.sign(payload)

	var lastErr error
	for attempt := 0; attempt < w.retries; attempt++ {
		if attempt > 0 {
			// Exponential backoff: 1s, 2s, 4s...
			time.Sleep(time.Duration(1<<uint(attempt-1)) * time.Second)
		}

		req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, w.config.URL, bytes.NewReader(payload))
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
