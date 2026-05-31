package connector

import (
	"testing"

	"github.com/neokapi/neokapi/core/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGitValidateRepoURL(t *testing.T) {
	tests := []struct {
		name    string
		repoURL string
		wantErr bool
	}{
		// Valid forms.
		{"https", "https://github.com/org/repo.git", false},
		{"https no .git", "https://github.com/org/repo", false},
		{"ssh scheme", "ssh://git@github.com/org/repo.git", false},
		{"ssh scheme with port", "ssh://git@github.com:22/org/repo.git", false},
		{"git scheme", "git://github.com/org/repo.git", false},
		{"scp-like github", "git@github.com:org/repo.git", false},
		{"scp-like custom host", "user@host.example.com:path/to/repo", false},
		{"https uppercase scheme", "HTTPS://github.com/org/repo.git", false},

		// Disallowed transports / RCE vectors.
		{"ext transport", "ext::sh -c 'touch /tmp/pwned'", true},
		{"ext transport double colon plain", "ext::git-remote-bad", true},
		{"fd transport", "fd::17", true},
		{"file scheme", "file:///etc/passwd", true},
		{"http insecure", "http://github.com/org/repo.git", true},
		{"unknown scheme", "ftp://example.com/repo", true},
		{"transport helper double colon", "transport::https://x", true},

		// Option injection.
		{"leading dash", "-oProxyCommand=evil", true},
		{"leading dash upload pack", "--upload-pack=evil", true},

		// Malformed / empty / control characters.
		{"empty", "", true},
		{"bare path", "/some/local/path", true},
		{"relative path", "./repo", true},
		{"control char", "https://github.com/org/repo\n.git", true},
		{"nul byte", "https://github.com/org/repo\x00", true},
		{"plain word", "notaurl", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateRepoURL(tt.repoURL)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestGitValidateBranch(t *testing.T) {
	tests := []struct {
		name    string
		branch  string
		wantErr bool
	}{
		// Valid.
		{"main", "main", false},
		{"feature slash", "feature/new-thing", false},
		{"dotted", "release/1.2.3", false},
		{"underscore", "my_branch", false},
		{"digit start", "1-hotfix", false},

		// Invalid.
		{"empty", "", true},
		{"leading dash", "-D", true},
		{"leading dash long opt", "--force", true},
		{"space", "main branch", true},
		{"tab", "main\tbranch", true},
		{"newline", "main\nrm -rf", true},
		{"control char", "main\x01", true},
		{"shell metachar", "main;rm", true},
		{"dollar", "main$(evil)", true},
		{"leading dot", ".hidden", true},
		{"leading slash", "/abs", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateBranch(tt.branch)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestNewGitConnectorRejectsUnsafeConfig(t *testing.T) {
	reg := registry.NewFormatRegistry()

	tests := []struct {
		name   string
		config map[string]string
	}{
		{"ext transport repo", map[string]string{"repo": "ext::sh -c evil"}},
		{"leading dash repo", map[string]string{"repo": "-oProxyCommand=evil"}},
		{"file scheme repo", map[string]string{"repo": "file:///etc/passwd"}},
		{"leading dash branch", map[string]string{"repo": "https://github.com/org/repo.git", "branch": "-D"}},
		{"newline branch", map[string]string{"repo": "https://github.com/org/repo.git", "branch": "main\nevil"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := NewGitConnector(reg, tt.config)
			require.Error(t, err)
			assert.Nil(t, c)
		})
	}
}

func TestNewGitConnectorAcceptsSafeConfig(t *testing.T) {
	reg := registry.NewFormatRegistry()

	c, err := NewGitConnector(reg, map[string]string{
		"repo":   "https://github.com/org/repo.git",
		"branch": "feature/x",
	})
	require.NoError(t, err)
	require.NotNil(t, c)
	assert.Equal(t, "https://github.com/org/repo.git", c.repoURL)
	assert.Equal(t, "feature/x", c.branch)
}

func TestNewGitConnectorDefaultsBranchToMain(t *testing.T) {
	reg := registry.NewFormatRegistry()

	c, err := NewGitConnector(reg, map[string]string{
		"repo": "git@github.com:org/repo.git",
	})
	require.NoError(t, err)
	require.NotNil(t, c)
	assert.Equal(t, "main", c.branch)
}
