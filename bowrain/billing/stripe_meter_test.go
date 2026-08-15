package billing

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestMeterEventIdentifier_Deterministic verifies the Stripe meter idempotency
// key is derived deterministically from the reference id (Epic 004) — it does
// not depend on wall-clock time, so a retried report reuses the exact key.
func TestMeterEventIdentifier_Deterministic(t *testing.T) {
	t.Parallel()

	a := meterEventIdentifier("ws-1", "ai_token_usage", "ai_translation", "job-42:0")
	b := meterEventIdentifier("ws-1", "ai_token_usage", "ai_translation", "job-42:0")
	assert.Equal(t, a, b, "same inputs must produce the same key on every call")
	assert.NotEmpty(t, a)
}

// TestMeterEventIdentifier_Distinct verifies distinct deductions never collide:
// two chunks of the same job (different reference id) and two operations get
// different keys, so neither is dropped as a false duplicate.
func TestMeterEventIdentifier_Distinct(t *testing.T) {
	t.Parallel()

	base := meterEventIdentifier("ws-1", "ai_token_usage", "ai_translation", "job-42:0")

	tests := []struct {
		name                 string
		ws, event, op, refID string
	}{
		{"different chunk offset", "ws-1", "ai_token_usage", "ai_translation", "job-42:50"},
		{"different job", "ws-1", "ai_token_usage", "ai_translation", "job-99:0"},
		{"different workspace", "ws-2", "ai_token_usage", "ai_translation", "job-42:0"},
		{"different operation", "ws-1", "ai_token_usage", "bravo_container", "job-42:0"},
		{"different event name", "ws-1", "container_time_usage", "ai_translation", "job-42:0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := meterEventIdentifier(tt.ws, tt.event, tt.op, tt.refID)
			assert.NotEqual(t, base, got, "distinct dimensions must not collide")
		})
	}
}

// TestMeterEventIdentifier_CallerRefIDs pins the contract for the three callers
// whose reference id must be per-deduction unique rather than constant
// (BLOCKER-3): the bravo message path, the editor AI-translate path, and the
// container-time path. A unique id keeps successive deductions from collapsing
// into one Stripe meter event, while a retried report of the SAME deduction
// reuses its key.
func TestMeterEventIdentifier_CallerRefIDs(t *testing.T) {
	t.Parallel()

	const ws = "ws-1"

	// Bravo message: two messages in the same conversation → distinct message ids
	// → distinct meter keys, not one key per conversation.
	msgA := meterEventIdentifier(ws, "ai_token_usage", "bravo_message", "msg-aaa")
	msgB := meterEventIdentifier(ws, "ai_token_usage", "bravo_message", "msg-bbb")
	assert.NotEqual(t, msgA, msgB, "two bravo messages must not collapse into one meter event")
	assert.Equal(t, msgA, meterEventIdentifier(ws, "ai_token_usage", "bravo_message", "msg-aaa"),
		"a retried report of the same message reuses its key")

	// Editor AI-translate: same project, different item/locale → distinct keys,
	// not one key per project.
	edA := meterEventIdentifier(ws, "ai_token_usage", "ai_translation", "proj-1:en.json:fr:nonce-1")
	edB := meterEventIdentifier(ws, "ai_token_usage", "ai_translation", "proj-1:de.json:fr:nonce-2")
	edC := meterEventIdentifier(ws, "ai_token_usage", "ai_translation", "proj-1:en.json:fr:nonce-3")
	assert.NotEqual(t, edA, edB, "different items in one project must not collapse")
	assert.NotEqual(t, edA, edC, "re-translating the same item (new nonce) must not collapse")

	// Container time: two container sessions → distinct keys, not one key per
	// conversation.
	ctA := meterEventIdentifier(ws, "container_time_usage", "bravo_container", "container-aaa")
	ctB := meterEventIdentifier(ws, "container_time_usage", "bravo_container", "container-bbb")
	assert.NotEqual(t, ctA, ctB, "two container sessions must not collapse into one meter event")
}
