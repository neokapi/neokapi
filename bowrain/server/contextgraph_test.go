package server

import (
	"testing"

	"github.com/stretchr/testify/assert"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
)

// The stream a push landed on is part of a node's identity, so a rebuild that
// guessed differently from the push would write a second subgraph beside the
// first rather than replacing it.
func TestPushedStream(t *testing.T) {
	tests := map[string]struct {
		fromEvent string
		project   *platstore.Project
		want      string
	}{
		"the event names it":      {"release", &platstore.Project{DefaultStream: "main"}, "release"},
		"the project's default":   {"", &platstore.Project{DefaultStream: "trunk"}, "trunk"},
		"neither, so the default": {"", &platstore.Project{}, "main"},
		"no project at all":       {"", nil, "main"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tt.want, pushedStream(tt.fromEvent, tt.project))
		})
	}
}

// A connector ingest publishes no workspace, and a graph written without the
// workspace's vocabulary is the wrong graph rather than a smaller one — so the
// slug is resolved from the project instead.
func TestWorkspaceSlugFallsBackToTheProjectsWorkspace(t *testing.T) {
	srv, _ := newTestServer(t)
	ctx := t.Context()

	assert.Equal(t, "carried", srv.workspaceSlug(ctx, "carried", "test-ws"))
	assert.Equal(t, "test", srv.workspaceSlug(ctx, "", "test-ws"))
	assert.Empty(t, srv.workspaceSlug(ctx, "", ""))
	assert.Empty(t, srv.workspaceSlug(ctx, "", "no-such-workspace"))
}
