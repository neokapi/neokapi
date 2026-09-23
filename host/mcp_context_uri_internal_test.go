package host

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The `context://` address space (AD-037): two address forms, one primitive,
// and a rendering carried as a query rather than as a second entry point.

func TestParseContextURI(t *testing.T) {
	tests := []struct {
		name        string
		uri         string
		wantPath    string
		wantProfile string
		wantJSON    bool
		wantProject string
		wantErr     string
	}{
		{
			name:     "a project-relative location",
			uri:      "context://docs/guide.md",
			wantPath: "docs/guide.md",
		},
		{
			// url.Parse would read `Docs` as an authority and lowercase it,
			// renaming the location on a case-sensitive filesystem.
			name:     "case is preserved through the first segment",
			uri:      "context://Docs/Guide.md",
			wantPath: "Docs/Guide.md",
		},
		{
			name:     "a percent-escaped location",
			uri:      "context://docs/a%20guide.md",
			wantPath: "docs/a guide.md",
		},
		{
			name:        "a profile by name",
			uri:         "context://profile/marketing",
			wantProfile: "marketing",
		},
		{
			name:     "the json rendering",
			uri:      "context://docs/guide.md?format=json",
			wantPath: "docs/guide.md",
			wantJSON: true,
		},
		{
			name:        "the json rendering of a profile",
			uri:         "context://profile/marketing?format=json",
			wantProfile: "marketing",
			wantJSON:    true,
		},
		{
			name:     "an explicit markdown rendering",
			uri:      "context://docs/guide.md?format=markdown",
			wantPath: "docs/guide.md",
		},
		{
			name:    "an unknown rendering is refused, not silently prose",
			uri:     "context://docs/guide.md?format=xml",
			wantErr: "unknown context rendering",
		},
		{
			name:        "a location in a named project",
			uri:         "context://docs/guide.md?project=/srv/acme",
			wantPath:    "docs/guide.md",
			wantProject: "/srv/acme",
		},
		{
			name:        "a named project beside the json rendering",
			uri:         "context://docs/guide.md?format=json&project=/srv/acme/kapi.yaml",
			wantPath:    "docs/guide.md",
			wantJSON:    true,
			wantProject: "/srv/acme/kapi.yaml",
		},
		{
			name:        "a profile in a named project",
			uri:         "context://profile/marketing?project=/srv/acme",
			wantProfile: "marketing",
			wantProject: "/srv/acme",
		},
		{
			name:    "another scheme is not this resource",
			uri:     "file:///docs/guide.md",
			wantErr: "Resource not found",
		},
		{
			name:    "an empty address names nothing",
			uri:     "context://",
			wantErr: "name a location or `profile/<name>`",
		},
		{
			name:    "a bare profile prefix names no profile",
			uri:     "context://profile/",
			wantErr: "name a profile after `profile/`",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, asJSON, namedProject, err := parseContextURI(tt.uri)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantPath, req.Path)
			assert.Equal(t, tt.wantProfile, req.Profile)
			assert.Equal(t, tt.wantJSON, asJSON)
			assert.Equal(t, tt.wantProject, namedProject)
		})
	}
}
