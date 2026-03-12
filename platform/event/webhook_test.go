package event

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	platev "github.com/neokapi/neokapi/platform/event"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWebhookDelivery(t *testing.T) {
	var receivedBody []byte
	var receivedSig string
	var receivedType string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedSig = r.Header.Get("X-Neokapi-Signature")
		receivedType = r.Header.Get("X-Neokapi-Event")
		receivedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	wh := NewWebhookDelivery(WebhookConfig{
		URL:    srv.URL,
		Secret: "test-secret",
	})

	event := platev.Event{
		ID:        "evt-1",
		Type:      platev.EventBlockCreated,
		Source:    "test",
		ProjectID: "proj-1",
		Data:      map[string]string{"block_id": "b1"},
	}

	err := wh.Deliver(event)
	require.NoError(t, err)

	assert.Equal(t, "block.created", receivedType)
	assert.NotEmpty(t, receivedSig)

	// Verify HMAC signature.
	assert.True(t, VerifySignature(receivedBody, "test-secret", receivedSig))

	// Verify payload.
	var decoded platev.Event
	require.NoError(t, json.Unmarshal(receivedBody, &decoded))
	assert.Equal(t, "evt-1", decoded.ID)
}

func TestWebhookRetry(t *testing.T) {
	var attempts atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := attempts.Add(1)
		if count < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	wh := NewWebhookDelivery(WebhookConfig{URL: srv.URL, Secret: "s"})

	err := wh.Deliver(platev.Event{Type: platev.EventBlockCreated})
	require.NoError(t, err)
	assert.Equal(t, int32(3), attempts.Load())
}

func TestWebhookDeliveryFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	wh := NewWebhookDelivery(WebhookConfig{URL: srv.URL, Secret: "s"})

	err := wh.Deliver(platev.Event{Type: platev.EventBlockCreated})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed after")
}

func TestVerifySignature(t *testing.T) {
	payload := []byte(`{"type":"test"}`)
	secret := "my-secret"

	wh := &WebhookDelivery{config: WebhookConfig{Secret: secret}}
	sig := wh.sign(payload)

	assert.True(t, VerifySignature(payload, secret, sig))
	assert.False(t, VerifySignature(payload, "wrong-secret", sig))
	assert.False(t, VerifySignature([]byte("tampered"), secret, sig))
}
