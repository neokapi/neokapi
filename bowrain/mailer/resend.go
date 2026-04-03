package mailer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const resendAPIURL = "https://api.resend.com/emails"

// ResendSender implements EmailSenderI using the Resend transactional email
// API (https://resend.com). No external Go SDK is required — the API is a
// simple JSON POST.
type ResendSender struct {
	apiKey string
	from   string
	client *http.Client
}

// NewResendSender creates a ResendSender with the given API key and sender
// address. The from address must be a verified domain in your Resend account.
func NewResendSender(apiKey, from string) *ResendSender {
	return &ResendSender{
		apiKey: apiKey,
		from:   from,
		client: &http.Client{Timeout: 15 * time.Second},
	}
}

type resendRequest struct {
	From    string   `json:"from"`
	To      []string `json:"to"`
	Subject string   `json:"subject"`
	HTML    string   `json:"html"`
}

type resendError struct {
	StatusCode int    `json:"statusCode"`
	Name       string `json:"name"`
	Message    string `json:"message"`
}

// Send sends an HTML email via the Resend API.
func (r *ResendSender) Send(ctx context.Context, to, subject, htmlBody string) error {
	payload := resendRequest{
		From:    r.from,
		To:      []string{to},
		Subject: subject,
		HTML:    htmlBody,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal resend request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, resendAPIURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build resend request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+r.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := r.client.Do(req)
	if err != nil {
		return fmt.Errorf("resend API request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}

	// Parse the error response.
	respBody, _ := io.ReadAll(resp.Body)
	var apiErr resendError
	if jsonErr := json.Unmarshal(respBody, &apiErr); jsonErr == nil && apiErr.Message != "" {
		return fmt.Errorf("resend API error %d (%s): %s", resp.StatusCode, apiErr.Name, apiErr.Message)
	}
	return fmt.Errorf("resend API error %d: %s", resp.StatusCode, string(respBody))
}
