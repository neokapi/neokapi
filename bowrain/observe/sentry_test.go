package observe

import (
	"errors"
	"testing"
)

func TestInitSentry_NoDSNIsNoOp(t *testing.T) {
	sentryEnabled = false
	if InitSentry(SentryConfig{DSN: ""}) {
		t.Fatal("InitSentry with empty DSN should return false")
	}
	if SentryEnabled() {
		t.Fatal("SentryEnabled should be false without a DSN")
	}
}

func TestCaptureError_DisabledReturnsEmpty(t *testing.T) {
	sentryEnabled = false
	if id := CaptureError(errors.New("boom"), "req-1", map[string]string{"route": "/x"}); id != "" {
		t.Fatalf("CaptureError should return empty when disabled, got %q", id)
	}
}

func TestCaptureError_NilErrorReturnsEmpty(t *testing.T) {
	sentryEnabled = true
	defer func() { sentryEnabled = false }()
	if id := CaptureError(nil, "req-1", nil); id != "" {
		t.Fatalf("CaptureError(nil) should return empty, got %q", id)
	}
}
