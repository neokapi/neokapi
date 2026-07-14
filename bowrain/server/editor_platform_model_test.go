package server

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/neokapi/neokapi/bowrain/auth"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/neokapi/neokapi/bowrain/platformconfig"
)

// wsModelAuthStore is a minimal auth.AuthStore that resolves a single workspace
// by id, for testing workspace-aware platform model resolution.
type wsModelAuthStore struct {
	auth.AuthStore
	ws *platauth.Workspace
}

func (m *wsModelAuthStore) GetWorkspace(_ context.Context, id string) (*platauth.Workspace, error) {
	if m.ws != nil && m.ws.ID == id {
		return m.ws, nil
	}
	return nil, assert.AnError
}

func TestPlatformProviderConfigForWorkspace(t *testing.T) {
	const (
		defaultModel = "eu.anthropic.claude-sonnet-4-6"
		altModel     = "eu.anthropic.claude-opus-4-6-v1"
	)

	choiceOn := platformconfig.NewInMemoryService(
		platformconfig.Defaults{AIProvider: "bedrock", AIDefaultModel: defaultModel},
		map[string]any{
			platformconfig.KeyAICustomerChoice: true,
			platformconfig.KeyAIEnabledModels:  []string{defaultModel, altModel},
		},
	)
	choiceOff := platformconfig.NewInMemoryService(
		platformconfig.Defaults{AIProvider: "bedrock", AIDefaultModel: defaultModel},
		nil,
	)

	tests := []struct {
		name      string
		cfg       *platformconfig.Service
		wsID      string
		preferred string
		wantModel string
	}{
		{"choice on, enabled preferred model wins", choiceOn, "ws1", altModel, altModel},
		{"choice on, empty preference falls back to default", choiceOn, "ws1", "", defaultModel},
		{"choice on, disallowed preference falls back to default", choiceOn, "ws1", "not.enabled", defaultModel},
		{"choice off ignores the preference", choiceOff, "ws1", altModel, defaultModel},
		{"empty workspace id uses default", choiceOn, "", altModel, defaultModel},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := &Server{
				PlatformConfig: tc.cfg,
				AuthStore:      &wsModelAuthStore{ws: &platauth.Workspace{ID: "ws1", PreferredModel: tc.preferred}},
			}
			got := s.platformProviderConfigForWorkspace(context.Background(), tc.wsID)
			assert.Equal(t, "bedrock", got.Provider, "provider stays platform-owned")
			assert.Equal(t, tc.wantModel, got.Model)
		})
	}
}

// TestPlatformProviderConfigForWorkspace_NoAuthStore verifies the resolver is
// safe when no auth store is wired (it must fall back to the platform default).
func TestPlatformProviderConfigForWorkspace_NoAuthStore(t *testing.T) {
	const defaultModel = "eu.anthropic.claude-sonnet-4-6"
	choiceOn := platformconfig.NewInMemoryService(
		platformconfig.Defaults{AIProvider: "bedrock", AIDefaultModel: defaultModel},
		map[string]any{platformconfig.KeyAICustomerChoice: true},
	)
	s := &Server{PlatformConfig: choiceOn} // AuthStore == nil
	got := s.platformProviderConfigForWorkspace(context.Background(), "ws1")
	assert.Equal(t, defaultModel, got.Model)
}
