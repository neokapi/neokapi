package sample_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/contextop"
	"github.com/neokapi/neokapi/host"
	"github.com/neokapi/neokapi/kapi-desktop/backend/sample"
)

// TestSeedSessions_TheDigestOnFirstOpen scaffolds the sample, records its
// sessions, and reads the digest a person sees on first open: one
// disagreement, three names and spellings, one word, one note on how the
// project writes. Every quote is a sentence in the file it names. A second
// seed records nothing.
func TestSeedSessions_TheDigestOnFirstOpen(t *testing.T) {
	t.Setenv(host.EnvDataDir, t.TempDir())
	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
	dir := t.TempDir()
	require.NoError(t, sample.Scaffold("kapimart", dir))
	recipe := filepath.Join(dir, "kapi.yaml")

	app := &host.App{}
	app.InitRegistries()
	t.Cleanup(app.Shutdown)
	ctx := t.Context()

	seeded, err := sample.SeedSessions(ctx, app, "kapimart", recipe)
	require.NoError(t, err)
	assert.True(t, seeded)

	d, err := app.ContextDigest(ctx, host.ContextDigestRequest{Project: recipe})
	require.NoError(t, err)

	require.Len(t, d.Conflicts, 1, "Needs you")
	var sides []string
	for _, side := range d.Conflicts[0].Sides {
		sides = append(sides, side.NoticedBy.Name+": "+side.Sentence)
	}
	assert.ElementsMatch(t, []string{
		"claude-code: Write sign in, not log in.",
		"codex: Write log in, not sign in.",
	}, sides)

	themes := map[string]int{}
	for _, theme := range d.Suggested {
		for _, g := range theme.Groups {
			themes[theme.Theme] += len(g.Items)
			for _, it := range g.Items {
				require.NotNil(t, it.Quote, it.Sentence)
				body, rerr := os.ReadFile(filepath.Join(dir, filepath.FromSlash(it.Quote.Path)))
				require.NoError(t, rerr, it.Quote.Path)
				assert.Contains(t, string(body), it.Quote.Quote, "the quote is in %s", it.Quote.Path)
			}
		}
	}
	assert.Equal(t, map[string]int{
		host.DigestThemeNames:   3,
		host.DigestThemeWords:   1,
		host.DigestThemeWriting: 1,
	}, themes)
	for _, side := range d.Conflicts[0].Sides {
		require.NotNil(t, side.Quote)
		body, rerr := os.ReadFile(filepath.Join(dir, filepath.FromSlash(side.Quote.Path)))
		require.NoError(t, rerr)
		assert.Contains(t, string(body), side.Quote.Quote)
	}

	log, err := app.ContextOperations(ctx, host.ContextLogRequest{Project: recipe})
	require.NoError(t, err)
	sessions := map[string]int{}
	for _, op := range log.Operations {
		if op.Actor.Kind == contextop.ActorAgent {
			sessions[op.Actor.Name+"/"+op.Actor.Session]++
		}
	}
	assert.Equal(t, map[string]int{
		"claude-code/" + sample.ClaudeCodeSession: 5,
		"codex/" + sample.CodexSession:            2,
	}, sessions)

	again, err := sample.SeedSessions(ctx, app, "kapimart", recipe)
	require.NoError(t, err)
	assert.False(t, again, "a log that holds an agent session is left as it is")
	after, err := app.ContextOperations(ctx, host.ContextLogRequest{Project: recipe})
	require.NoError(t, err)
	assert.Len(t, after.Operations, len(log.Operations))
}

// TestSeedSessions_OtherSamplesRecordNothing: only the sample that ships a
// session records one.
func TestSeedSessions_OtherSamplesRecordNothing(t *testing.T) {
	seeded, err := sample.SeedSessions(t.Context(), &host.App{}, "other", filepath.Join(t.TempDir(), "kapi.yaml"))
	require.NoError(t, err)
	assert.False(t, seeded)
	assert.False(t, strings.Contains(sample.ClaudeCodeSession, " "), "a session id is one token")
}
