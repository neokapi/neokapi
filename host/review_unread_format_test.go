package host

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/registry"
)

// The review queue lists the project's translated and source units awaiting a
// person. A collection in a format no installed reader opens is left out of it
// and named, and a queue over nothing it could read never reads as a queue with
// nothing left to review.

// reviewUnreadProject is unreadProject, with its source-only `sourcecode`
// collection, plus a translated `okf_idml` collection whose French target is
// committed. When withApp is set it also holds a JSON collection whose French
// translation awaits review.
func reviewUnreadProject(t *testing.T, withApp bool) string {
	t.Helper()
	root := unreadProject(t, "")
	write := func(rel, content string) {
		t.Helper()
		path := filepath.Join(root, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	}
	recipePath := filepath.Join(root, "kapi.yaml")
	recipe, err := os.ReadFile(recipePath)
	require.NoError(t, err)
	if withApp {
		recipe = append(recipe, []byte(`  - name: app
    content:
      - path: english.json
        target: '{lang}.json'
        target_languages: [fr]
`)...)
		write("english.json", `{"hello":"Hello world"}`)
		write("fr.json", `{"hello":"Bonjour le monde"}`)
	}
	recipe = append(recipe, []byte(`  - name: layout
    content:
      - path: pkg/doc.idml
        target: 'pkg/doc.{lang}.idml'
        target_languages: [fr]
        format:
          name: okf_idml
`)...)
	require.NoError(t, os.WriteFile(recipePath, recipe, 0o644))
	write("pkg/doc.idml", "<doc>Hello</doc>\n")
	write("pkg/doc.fr.idml", "<doc>Bonjour</doc>\n")
	return root
}

// reviewStatus runs `kapi status --review` over the project, as JSON when asJSON
// is set, and returns stdout, stderr and the error.
func reviewStatus(t *testing.T, root string, asJSON bool) (string, string, error) {
	t.Helper()
	cmd := NewEnvCommand(context.Background(), "status")
	AddProjectFlag(cmd)
	AddStatusFlags(cmd)
	require.NoError(t, cmd.Flags().Set("project", filepath.Join(root, "kapi.yaml")))
	require.NoError(t, cmd.Flags().Set("review", "true"))
	if asJSON {
		require.NoError(t, cmd.Flags().Set("json", "true"))
	}
	var stderr bytes.Buffer
	cmd.SetErr(&stderr)
	out, err := captureStdout(t, func() error { return (&App{}).RunStatus(cmd, nil) })
	return out, stderr.String(), err
}

// The queue lists the readable translation and names the two collections it
// could not read: the translated okf_idml one and the source-only sourcecode one.
func TestStatusReviewSkipsContentWithNoReader(t *testing.T) {
	root := reviewUnreadProject(t, true)

	out, stderr, err := reviewStatus(t, root, true)
	require.NoError(t, err, "a missing plugin leaves the rest of the queue listable: %s", stderr)
	var q reviewQueueOutput
	require.NoError(t, json.Unmarshal([]byte(out), &q), out)

	var listed bool
	for _, it := range q.Pending {
		if it.File == "fr.json" {
			listed = true
		}
		assert.NotContains(t, it.File, "doc", "a set-aside file lists no unit")
	}
	assert.True(t, listed, "the readable translation awaits review: %+v", q.Pending)
	requireNoReader(t, q.Warnings, "pkg/doc.idml", "okf_idml", "okapi-bridge")
	requireNoReader(t, q.Warnings, "cask/kapi.rb", "sourcecode", "sourcecode")
	assert.Contains(t, stderr, `no reader for format "okf_idml"`)
}

// A project whose only content has no reader has nothing the queue could read.
// The queue says so instead of reporting that nothing awaits review.
func TestStatusReviewOverOnlyUnreadableContentSaysSo(t *testing.T) {
	root := reviewUnreadProject(t, false)

	out, _, err := reviewStatus(t, root, false)
	require.NoError(t, err)
	assert.NotContains(t, out, "Review queue empty: no unit in any language is waiting")
	assert.Contains(t, out, `no reader for format "okf_idml"`)
	assert.Contains(t, out, "no installed reader")
}

// The desktop's convergence report runs the same checks, coverage and queue,
// and carries the same warnings.
func TestProjectConvergenceSkipsContentWithNoReader(t *testing.T) {
	root := reviewUnreadProject(t, true)

	rep, err := (&App{}).ProjectConvergence(context.Background(), filepath.Join(root, "kapi.yaml"), "en")
	require.NoError(t, err)
	require.NotEmpty(t, rep.Review, "the readable translation awaits review")
	requireNoReader(t, rep.Warnings, "pkg/doc.idml", "okf_idml", "okapi-bridge")
	var measured bool
	for _, lc := range rep.Locales {
		if lc.Locale == "fr" && lc.Total > 0 {
			measured = true
		}
	}
	assert.True(t, measured, "the readable translation is measured: %+v", rep.Locales)
}

// Only a missing reader is skipped. A JSON source that does not parse still
// fails the queue.
func TestStatusReviewStillFailsOnABrokenFileInAKnownFormat(t *testing.T) {
	root := reviewUnreadProject(t, true)
	require.NoError(t, os.WriteFile(filepath.Join(root, "english.json"), []byte(`{"hello": "Hello`), 0o644))

	_, _, err := reviewStatus(t, root, true)
	require.Error(t, err)
	require.NotErrorIs(t, err, registry.ErrUnknownFormat)
}
