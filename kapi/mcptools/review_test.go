package mcptools

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/cli"
	"github.com/neokapi/neokapi/host"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeMCPReviewProject scaffolds a two-unit nb project with a pending review
// queue for the MCP review-tool handlers.
func writeMCPReviewProject(t *testing.T) string {
	t.Helper()
	t.Setenv("KAPI_NO_PROJECT", "1")
	root := t.TempDir()
	recipe := `version: v1
name: rev
defaults:
  source_language: en
  target_languages: [nb]
collections:
  - name: app
    content:
      - path: en.json
        target: "{lang}.json"
ship_gate: { translated: 100, established: 100 }
`
	require.NoError(t, os.WriteFile(filepath.Join(root, "kapi.yaml"), []byte(recipe), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "en.json"),
		[]byte(`{"a":"Apple","b":"Banana"}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "nb.json"),
		[]byte(`{"a":"Eple","b":"Banan"}`), 0o644))
	return root
}

func TestHandleReviewQueue(t *testing.T) {
	root := writeMCPReviewProject(t)
	a := testApp()
	proj := filepath.Join(root, "kapi.yaml")

	_, out, err := handleReviewQueue(t.Context(), a, ReviewQueueInput{Project: proj})
	require.NoError(t, err)
	assert.Equal(t, 2, out.Total)
	require.Len(t, out.Pending, 2)
	assert.Equal(t, "nb", out.Pending[0].Locale)
	assert.Equal(t, "app", out.Pending[0].Collection)

	// Locale filter — no de units exist.
	_, out, err = handleReviewQueue(t.Context(), a, ReviewQueueInput{Project: proj, Locale: "de"})
	require.NoError(t, err)
	assert.Equal(t, 0, out.Total)

	// Collection filter.
	_, out, err = handleReviewQueue(t.Context(), a, ReviewQueueInput{Project: proj, Collection: "app"})
	require.NoError(t, err)
	assert.Equal(t, 2, out.Total)
	_, out, err = handleReviewQueue(t.Context(), a, ReviewQueueInput{Project: proj, Collection: "other"})
	require.NoError(t, err)
	assert.Equal(t, 0, out.Total)
}

func TestHandleReviewQueue_NoProject(t *testing.T) {
	t.Setenv("KAPI_NO_PROJECT", "1")
	a := testApp()
	_, _, err := handleReviewQueue(t.Context(), a, ReviewQueueInput{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no kapi project")
}

func TestHandleReviewUnit(t *testing.T) {
	root := writeMCPReviewProject(t)
	a := testApp()
	proj := filepath.Join(root, "kapi.yaml")

	_, queue, err := handleReviewQueue(t.Context(), a, ReviewQueueInput{Project: proj})
	require.NoError(t, err)
	item := queue.Pending[0]

	_, out, err := handleReviewUnit(t.Context(), a, ReviewUnitInput{
		Project: proj, Locale: item.Locale, File: item.File, Key: item.Key,
	})
	require.NoError(t, err)
	require.NotNil(t, out.Unit)
	assert.Equal(t, "translated", out.Unit.Status)
	assert.NotEmpty(t, out.Unit.Source)
	assert.NotEmpty(t, out.Unit.Target)

	_, _, err = handleReviewUnit(t.Context(), a, ReviewUnitInput{
		Project: proj, Locale: item.Locale, File: item.File, Key: "missing",
	})
	require.Error(t, err)
}

// TestHandleReviewUnit_CarriesTheContext holds the bar for the agent surface:
// an agent asked to judge a translation is handed at least what the model that
// produced it was handed.
func TestHandleReviewUnit_CarriesTheContext(t *testing.T) {
	root := writeMCPReviewProject(t)
	a := testApp()
	proj := filepath.Join(root, "kapi.yaml")

	_, queue, err := handleReviewQueue(t.Context(), a, ReviewQueueInput{Project: proj})
	require.NoError(t, err)
	require.Len(t, queue.Pending, 2)

	_, out, err := handleReviewUnit(t.Context(), a, ReviewUnitInput{
		Project: proj, Locale: "nb", File: "nb.json", Key: "a",
	})
	require.NoError(t, err)
	require.NotNil(t, out.Unit)
	require.NotNil(t, out.Unit.Context, "review_unit answers with the review model")

	rc := out.Unit.Context
	assert.Equal(t, "app", rc.Point.Collection)
	assert.Equal(t, "en.json", rc.Point.Path, "the point is the SOURCE file's coordinate")
	assert.Equal(t, "a", rc.Neighbourhood.Key)
	assert.Equal(t, host.DefaultReviewWindow, rc.Neighbourhood.Window)
	assert.Empty(t, rc.Neighbourhood.Before, "`a` is the first unit in the file")
	require.Len(t, rc.Neighbourhood.After, 1)
	assert.Equal(t, "b", rc.Neighbourhood.After[0].Key)
	require.Len(t, rc.Neighbourhood.After[0].Source, 1)
	require.NotNil(t, rc.Neighbourhood.After[0].Source[0].Text)
	assert.Equal(t, "Banana", rc.Neighbourhood.After[0].Source[0].Text.Text)
}

// TestHandlePreReview_AnnotatesWithoutDeciding: an agent's pre-review records
// a score on the unit and leaves it in the queue for a person.
func TestHandlePreReview_AnnotatesWithoutDeciding(t *testing.T) {
	root := writeMCPReviewProject(t)
	a := testApp()
	proj := filepath.Join(root, "kapi.yaml")

	_, queue, err := handleReviewQueue(t.Context(), a, ReviewQueueInput{Project: proj})
	require.NoError(t, err)
	require.Len(t, queue.Pending, 2)
	first := queue.Pending[0]

	_, out, err := handlePreReview(t.Context(), a, PreReviewInput{
		Project: proj, Locale: first.Locale, File: first.File, Key: first.Key,
		Score: 72, Reasons: []PreReviewReason{{Severity: "minor", Message: "reads stiffly", Suggestion: "Eplet"}},
	}, "agent/claude-code")
	require.NoError(t, err)
	assert.True(t, out.Recorded)
	assert.Equal(t, "agent/claude-code", out.By)

	// The unit stays in the queue for a person, carrying the score.
	_, queue, err = handleReviewQueue(t.Context(), a, ReviewQueueInput{Project: proj})
	require.NoError(t, err)
	require.Equal(t, 2, queue.Total, "a pre-review decides nothing")
	var scored *cli.ReviewQueueItem
	for i := range queue.Pending {
		if queue.Pending[i].Key == first.Key {
			scored = &queue.Pending[i]
		}
	}
	require.NotNil(t, scored)
	require.NotNil(t, scored.AIScore)
	assert.Equal(t, 72, *scored.AIScore)

	_, _, err = handlePreReview(t.Context(), a, PreReviewInput{
		Project: proj, Locale: first.Locale, File: first.File, Key: first.Key, Score: 101,
	}, "agent")
	require.Error(t, err, "a score outside 0-100 is refused")
}

// writeMCPSourceGateProject scaffolds a project whose source gate asks for a
// human, so the queue carries source units beside the nb translations.
func writeMCPSourceGateProject(t *testing.T) string {
	t.Helper()
	t.Setenv("KAPI_NO_PROJECT", "1")
	root := t.TempDir()
	recipe := `version: v1
name: rev-source
defaults:
  source_language: en
  target_languages: [nb]
  source_gate: established
collections:
  - name: app
    content:
      - path: en.json
        target: "{lang}.json"
`
	require.NoError(t, os.WriteFile(filepath.Join(root, "kapi.yaml"), []byte(recipe), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "en.json"),
		[]byte(`{"a":"Apple","b":"Banana"}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "nb.json"),
		[]byte(`{"a":"Eple","b":"Banan"}`), 0o644))
	return root
}

// review_queue lists one queue across the languages: an agent sees the source
// units it is being asked to look at, marked, with the per-language counts
// beside them.
func TestHandleReviewQueue_ListsSourceUnitsAndFiltersByLanguage(t *testing.T) {
	root := writeMCPSourceGateProject(t)
	a := testApp()
	proj := filepath.Join(root, "kapi.yaml")

	_, out, err := handleReviewQueue(t.Context(), a, ReviewQueueInput{Project: proj})
	require.NoError(t, err)
	assert.Equal(t, 4, out.Total)
	assert.Equal(t, []cli.ReviewLanguage{
		{Language: "en", Pending: 2, Source: true},
		{Language: "nb", Pending: 2},
	}, out.Languages)
	assert.True(t, out.Pending[0].IsSource, "the source rows lead the queue")
	assert.Equal(t, "en", out.Pending[0].Language)
	assert.Equal(t, "en.json", out.Pending[0].File)

	_, out, err = handleReviewQueue(t.Context(), a, ReviewQueueInput{Project: proj, Language: "en"})
	require.NoError(t, err)
	assert.Equal(t, 2, out.Total)
	for _, it := range out.Pending {
		assert.True(t, it.IsSource)
	}
	assert.Len(t, out.Languages, 2, "the summary still offers every language")

	_, out, err = handleReviewQueue(t.Context(), a, ReviewQueueInput{Project: proj, Language: "nb"})
	require.NoError(t, err)
	assert.Equal(t, 2, out.Total)
	for _, it := range out.Pending {
		assert.False(t, it.IsSource)
	}
}

// review_queue offers the source lane even when the default gate holds no source
// units: an agent reading the summary sees the source language at count 0 beside
// the target work, because the handler passes through the queue's Languages set.
func TestHandleReviewQueue_OffersTheSourceLaneWhenSourceIsClean(t *testing.T) {
	t.Setenv("KAPI_NO_PROJECT", "1")
	root := t.TempDir()
	recipe := `version: v1
name: rev-clean
defaults:
  source_language: en
  target_languages: [nb]
  source_gate: written
collections:
  - name: app
    content:
      - path: en.json
        target: "{lang}.json"
`
	require.NoError(t, os.WriteFile(filepath.Join(root, "kapi.yaml"), []byte(recipe), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "en.json"),
		[]byte(`{"a":"Apple","b":"Banana"}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "nb.json"),
		[]byte(`{"a":"Eple","b":"Banan"}`), 0o644))

	a := testApp()
	_, out, err := handleReviewQueue(t.Context(), a, ReviewQueueInput{Project: filepath.Join(root, "kapi.yaml")})
	require.NoError(t, err)
	assert.Equal(t, []cli.ReviewLanguage{
		{Language: "en", Pending: 0, Source: true},
		{Language: "nb", Pending: 2},
	}, out.Languages, "the source lane is offered at zero beside the queue-driven targets")
	for _, it := range out.Pending {
		assert.False(t, it.IsSource, "a clean source under the default gate queues no source rows")
	}
}

// review_unit answers for a source-language unit: the wording, its rung on the
// authoring ladder, and the point governing it.
func TestHandleReviewUnit_AcceptsASourceLanguageUnit(t *testing.T) {
	root := writeMCPSourceGateProject(t)
	a := testApp()
	proj := filepath.Join(root, "kapi.yaml")

	_, out, err := handleReviewUnit(t.Context(), a, ReviewUnitInput{
		Project: proj, Locale: "en", File: "en.json", Key: "a",
	})
	require.NoError(t, err)
	require.NotNil(t, out.Unit)
	assert.True(t, out.Unit.IsSource)
	assert.Equal(t, "en", out.Unit.Language)
	assert.Equal(t, "Apple", out.Unit.Source)
	assert.Empty(t, out.Unit.Target)
	assert.Equal(t, "written", out.Unit.Status)
	require.NotNil(t, out.Unit.Context)
	assert.Equal(t, "en.json", out.Unit.Context.Point.Path)
	assert.True(t, out.Unit.Context.Point.IsSource)
	assert.Equal(t, "a", out.Unit.Context.Neighbourhood.Key)
}

func TestAgentIdentity_Fallback(t *testing.T) {
	// No session (nil request) → the bare "agent" identity.
	assert.Equal(t, "agent", agentIdentity(nil))
}
