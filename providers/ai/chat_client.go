package aiprovider

import (
	"net/http"
	"time"

	"github.com/neokapi/neokapi/core/httputil"
)

// chatRequestTimeout bounds one model request, retries included. Reasoning
// models answer a batch of 50 blocks in well over a minute, so the 30-second
// default of [httputil.NewResilientClient] times them out. The caller's
// context still applies.
const chatRequestTimeout = 5 * time.Minute

// newChatClient returns the retrying HTTP client a provider sends model
// requests through.
func newChatClient() *http.Client {
	client := httputil.NewResilientClient()
	client.Timeout = chatRequestTimeout
	return client
}
