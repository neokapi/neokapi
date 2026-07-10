package server

import (
	"context"
	"testing"

	"github.com/neokapi/neokapi/bowrain/billing"
	"github.com/stretchr/testify/assert"
)

// creditCheckStore embeds the package mockBillingStore and overrides CheckCredits
// so tests can vary the reported balance and error.
type creditCheckStore struct {
	mockBillingStore
	remaining int64
	err       error
}

func (s *creditCheckStore) CheckCredits(context.Context, string) (int64, error) {
	return s.remaining, s.err
}

// TestInsufficientPlatformCredits exercises the enqueue credit pre-check gate
// (Epic 004, task 6): platform-key jobs on a zero-credit workspace are refused,
// while BYO jobs and unbilled/degraded cases are always allowed.
func TestInsufficientPlatformCredits(t *testing.T) {
	tests := []struct {
		name             string
		noStore          bool
		remaining        int64
		err              error
		workspaceID      string
		providerConfigID string
		want             bool
	}{
		{name: "nil billing store allows", noStore: true, workspaceID: "ws-1", providerConfigID: "platform", want: false},
		{name: "empty workspace allows", remaining: 0, workspaceID: "", providerConfigID: "platform", want: false},
		{name: "BYO config skips check even at zero", remaining: 0, workspaceID: "ws-1", providerConfigID: "cfg-1", want: false},
		{name: "platform with credits allows", remaining: 5000, workspaceID: "ws-1", providerConfigID: "platform", want: false},
		{name: "platform empty id with credits allows", remaining: 5000, workspaceID: "ws-1", providerConfigID: "", want: false},
		{name: "platform zero credits blocks", remaining: 0, workspaceID: "ws-1", providerConfigID: "platform", want: true},
		{name: "platform empty id zero credits blocks", remaining: 0, workspaceID: "ws-1", providerConfigID: "", want: true},
		{name: "platform negative credits blocks", remaining: -100, workspaceID: "ws-1", providerConfigID: "platform", want: true},
		{name: "check error degrades to allow", remaining: 0, err: assert.AnError, workspaceID: "ws-1", providerConfigID: "platform", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Server{}
			if !tt.noStore {
				var store billing.BillingStore = &creditCheckStore{remaining: tt.remaining, err: tt.err}
				s.BillingStore = store
			}
			got := s.insufficientPlatformCredits(context.Background(), tt.workspaceID, tt.providerConfigID)
			assert.Equal(t, tt.want, got)
		})
	}
}
