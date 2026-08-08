package host_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/neokapi/neokapi/host"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSearchContextSurfacesBoundedProfiles(t *testing.T) {
	src := host.ContextSearchSources{
		Scope: host.ScopeProject,
		Profiles: []host.ContextProfileHit{
			{Name: "spring-sale", ValidFrom: "2026-01-01", ValidTo: "2026-07-01", State: "expired"},
		},
	}
	res, err := host.SearchContext(context.Background(), src, host.ContextSearchRequest{Query: "x"})
	require.NoError(t, err)
	require.Len(t, res.Profiles, 1)
	assert.Equal(t, "spring-sale", res.Profiles[0].Name)
	assert.Equal(t, "expired", res.Profiles[0].State)
}

func TestFormatTextRendersGoverningProfiles(t *testing.T) {
	res := &host.ContextSearchResult{
		Query: "x",
		Profiles: []host.ContextProfileHit{
			{Name: "spring-sale", ValidFrom: "2026-01-01", ValidTo: "2026-07-01", State: "expired"},
		},
	}
	var buf bytes.Buffer
	require.NoError(t, res.FormatText(&buf))
	out := buf.String()
	assert.Contains(t, out, "Governing profiles")
	assert.Contains(t, out, "spring-sale")
	assert.Contains(t, out, "from 2026-01-01")
	assert.Contains(t, out, "until 2026-07-01")
	assert.Contains(t, out, "expired")
}
