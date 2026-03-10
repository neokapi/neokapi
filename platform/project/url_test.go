package project

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseProjectURL(t *testing.T) {
	tests := []struct {
		name     string
		rawURL   string
		expected ProjectURLInfo
	}{
		{
			name:   "workspace project",
			rawURL: "https://bowrain.example.com/my-team/abc123",
			expected: ProjectURLInfo{
				ServerURL: "https://bowrain.example.com",
				Workspace: "my-team",
				ProjectID: "abc123",
			},
		},
		{
			name:   "claim URL",
			rawURL: "https://bowrain.example.com/claim/clm_xyz",
			expected: ProjectURLInfo{
				ServerURL:  "https://bowrain.example.com",
				ClaimToken: "clm_xyz",
			},
		},
		{
			name:   "direct project",
			rawURL: "https://bowrain.example.com/projects/abc123",
			expected: ProjectURLInfo{
				ServerURL: "https://bowrain.example.com",
				ProjectID: "abc123",
			},
		},
		{
			name:   "single segment project ID",
			rawURL: "https://bowrain.example.com/abc123",
			expected: ProjectURLInfo{
				ServerURL: "https://bowrain.example.com",
				ProjectID: "abc123",
			},
		},
		{
			name:   "server only (no path)",
			rawURL: "https://bowrain.example.com",
			expected: ProjectURLInfo{
				ServerURL: "https://bowrain.example.com",
			},
		},
		{
			name:     "empty URL",
			rawURL:   "",
			expected: ProjectURLInfo{},
		},
		{
			name:   "trailing slash",
			rawURL: "https://bowrain.example.com/ws/proj/",
			expected: ProjectURLInfo{
				ServerURL: "https://bowrain.example.com",
				Workspace: "ws",
				ProjectID: "proj",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseProjectURL(tt.rawURL)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFormatProjectURL(t *testing.T) {
	tests := []struct {
		name       string
		serverURL  string
		workspace  string
		projectID  string
		claimToken string
		expected   string
	}{
		{
			name:      "workspace project",
			serverURL: "https://bowrain.example.com",
			workspace: "my-team",
			projectID: "abc123",
			expected:  "https://bowrain.example.com/my-team/abc123",
		},
		{
			name:       "claim URL",
			serverURL:  "https://bowrain.example.com",
			claimToken: "clm_xyz",
			expected:   "https://bowrain.example.com/claim/clm_xyz",
		},
		{
			name:      "direct project (no workspace)",
			serverURL: "https://bowrain.example.com",
			projectID: "abc123",
			expected:  "https://bowrain.example.com/projects/abc123",
		},
		{
			name:      "server only",
			serverURL: "https://bowrain.example.com",
			expected:  "https://bowrain.example.com",
		},
		{
			name:     "empty server",
			expected: "",
		},
		{
			name:      "trailing slash on server URL",
			serverURL: "https://bowrain.example.com/",
			workspace: "ws",
			projectID: "proj",
			expected:  "https://bowrain.example.com/ws/proj",
		},
		{
			name:       "claim takes priority over workspace/project",
			serverURL:  "https://bowrain.example.com",
			workspace:  "ws",
			projectID:  "proj",
			claimToken: "clm_xyz",
			expected:   "https://bowrain.example.com/claim/clm_xyz",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatProjectURL(tt.serverURL, tt.workspace, tt.projectID, tt.claimToken)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParseFormatRoundTrip(t *testing.T) {
	tests := []struct {
		name       string
		serverURL  string
		workspace  string
		projectID  string
		claimToken string
	}{
		{"workspace project", "https://example.com", "team", "proj-1", ""},
		{"claim URL", "https://example.com", "", "", "clm_abc"},
		{"direct project", "https://example.com", "", "proj-1", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := FormatProjectURL(tt.serverURL, tt.workspace, tt.projectID, tt.claimToken)
			info := ParseProjectURL(url)
			assert.Equal(t, tt.serverURL, info.ServerURL)
			if tt.claimToken != "" {
				assert.Equal(t, tt.claimToken, info.ClaimToken)
			} else {
				assert.Equal(t, tt.workspace, info.Workspace)
				assert.Equal(t, tt.projectID, info.ProjectID)
			}
		})
	}
}
