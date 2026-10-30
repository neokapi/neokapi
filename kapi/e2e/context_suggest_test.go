//go:build e2e

package e2e

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// `kapi context suggest` against the built binary: the pass files what it sees
// in the files, and the review queue a person reads in another process lists
// it as noticed by kapi.
func TestContextSuggestFilesWhatReviewShows(t *testing.T) {
	root := t.TempDir()
	write := func(rel, body string) {
		t.Helper()
		file := filepath.Join(root, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(file), 0o755))
		require.NoError(t, os.WriteFile(file, []byte(body), 0o644))
	}
	write("kapi.yaml", `version: v1
name: ctx-suggest-e2e
defaults:
  source_language: en
collections:
  - name: Docs
    content:
      - path: "docs/*.md"
`)
	write("docs/guide.md", "# Guide\n\nSend an email to start.\n\nReply to the email.\n\nOr send an e-mail.\n")
	recipe := filepath.Join(root, "kapi.yaml")

	out := kapi(t, "context", "suggest", "-p", recipe)
	assert.Contains(t, out, "Write email, not e-mail or e mail.")
	assert.Contains(t, out, "Decide with kapi context review.")

	again := kapi(t, "context", "suggest", "-p", recipe)
	assert.Contains(t, again, "Unchanged")

	queue := kapi(t, "context", "review", "--peek", "--json", "-p", recipe)
	var digest struct {
		Suggested []struct {
			Groups []struct {
				Items []struct {
					Sentence  string `json:"sentence"`
					Keepable  bool   `json:"keepable"`
					NoticedBy struct {
						Kind string `json:"kind"`
						Name string `json:"name"`
					} `json:"noticed_by"`
				} `json:"items"`
			} `json:"groups"`
		} `json:"suggested"`
	}
	require.NoError(t, json.Unmarshal([]byte(queue), &digest), queue)
	var found int
	for _, theme := range digest.Suggested {
		for _, g := range theme.Groups {
			for _, it := range g.Items {
				if it.Sentence != "Write email, not e-mail or e mail." {
					continue
				}
				found++
				assert.Equal(t, "tool", it.NoticedBy.Kind)
				assert.Equal(t, "kapi", it.NoticedBy.Name)
				assert.True(t, it.Keepable, "a person can keep it")
			}
		}
	}
	assert.Equal(t, 1, found, "the queue lists the suggestion once:\n%s", queue)

	text := kapi(t, "context", "review", "--peek", "-p", recipe)
	assert.Contains(t, text, "noticed by kapi\n")
}
