package commands

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	bproject "github.com/neokapi/neokapi/host/venue/project"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// reportCmd is a bare command wired to two buffers, which is all
// reportConceptPush reads: stdout for the NDJSON document, stderr for the
// terminal rendering.
func reportCmd(stdout, stderr *bytes.Buffer) *cobra.Command {
	cmd := &cobra.Command{Use: "up"}
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	return cmd
}

// TestReportConceptPushSaysWhereToReview covers both renderings of a proposal
// made during `kapi up`'s push phase.
//
// The run told the reader a change-set id and stopped, so finding the surface
// that reviews it meant grepping the frontend's route table. Under --json it
// said nothing at all, which is the reading that matters most: a CI job summary
// is built from the run's structured output, not from its stderr, so a proposal
// that exists only as a stderr sentence is invisible to the surface the
// reviewer is most likely to see.
func TestReportConceptPushSaysWhereToReview(t *testing.T) {
	const (
		id   = "EMezX9AS"
		link = "https://bowrain.cloud/acme/context/changes/EMezX9AS"
	)

	t.Run("text names the change-set and links its review surface", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		require.NoError(t, reportConceptPush(reportCmd(&stdout, &stderr), nil, &PushConceptsResult{
			ConceptsProposed: 57, ChangesetID: id, ChangesetURL: link,
		}, false))

		got := stderr.String()
		assert.Contains(t, got, "change-set "+id)
		assert.Contains(t, got, "Review it at "+link)
		assert.Empty(t, stdout.String(), "the terminal rendering does not write to the document stream")
	})

	// A project whose recipe names no server or no workspace cannot be linked
	// to. The id still travels: it is the handle every other surface takes, and
	// a link assembled from a guessed host would read as a page that has gone
	// missing rather than as a hub the caller has not connected to.
	t.Run("text degrades to the id when no server base is known", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		require.NoError(t, reportConceptPush(reportCmd(&stdout, &stderr), nil, &PushConceptsResult{
			ConceptsProposed: 57, ChangesetID: id,
		}, false))

		got := stderr.String()
		assert.Contains(t, got, "change-set "+id)
		assert.NotContains(t, got, "Review it at")
	})

	t.Run("json emits one changeset record carrying the review link", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		require.NoError(t, reportConceptPush(reportCmd(&stdout, &stderr), nil, &PushConceptsResult{
			ConceptsProposed: 57, ChangesetID: id, ChangesetURL: link,
		}, true))

		rec := decodeOneRecord(t, stdout.String())
		assert.Equal(t, "changeset", rec["type"])
		assert.EqualValues(t, 57, rec["concepts_proposed"])
		assert.Equal(t, id, rec["changeset_id"])
		assert.Equal(t, link, rec["changeset_url"])
		assert.Empty(t, stderr.String(), "the document is the whole output under --json")
	})

	t.Run("json omits the link rather than emitting a broken one", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		require.NoError(t, reportConceptPush(reportCmd(&stdout, &stderr), nil, &PushConceptsResult{
			ConceptsProposed: 57, ChangesetID: id,
		}, true))

		rec := decodeOneRecord(t, stdout.String())
		assert.Equal(t, id, rec["changeset_id"])
		assert.NotContains(t, rec, "changeset_url",
			"an absent review surface is reported by omission, never by an empty or guessed link")
	})

	t.Run("a push that proposed nothing writes no record", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		require.NoError(t, reportConceptPush(reportCmd(&stdout, &stderr), nil, &PushConceptsResult{
			ConceptsApplied: 3,
		}, true))
		assert.Empty(t, stdout.String())
	})

	t.Run("a nil result is silent on both renderings", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		require.NoError(t, reportConceptPush(reportCmd(&stdout, &stderr), nil, nil, true))
		require.NoError(t, reportConceptPush(reportCmd(&stdout, &stderr), nil, nil, false))
		assert.Empty(t, stdout.String())
		assert.Empty(t, stderr.String())
	})
}

// decodeOneRecord asserts the NDJSON document holds exactly one record and
// returns it decoded.
func decodeOneRecord(t *testing.T, doc string) map[string]any {
	t.Helper()
	lines := []string{}
	for l := range strings.SplitSeq(strings.TrimSpace(doc), "\n") {
		if strings.TrimSpace(l) != "" {
			lines = append(lines, l)
		}
	}
	require.Len(t, lines, 1, "expected exactly one NDJSON record, got: %s", doc)

	var rec map[string]any
	require.NoError(t, json.Unmarshal([]byte(lines[0]), &rec))
	return rec
}

// TestChangesetURLDegradesWithoutAServerBase pins what the CLI's link builder
// does when the recipe names no hub to link into.
//
// It must return the empty string rather than a partially-built URL: every
// caller treats "" as "report the id alone", and a URL missing its host or its
// workspace segment resolves to a page that does not exist — which reads to a
// reviewer as a proposal that went nowhere.
func TestChangesetURLDegradesWithoutAServerBase(t *testing.T) {
	tests := []struct {
		name string
		proj *bproject.Project
	}{
		{name: "no recipe", proj: &bproject.Project{}},
		{name: "no server block", proj: &bproject.Project{Recipe: &bproject.Recipe{}}},
		{
			name: "a server with no workspace segment",
			proj: &bproject.Project{Recipe: &bproject.Recipe{
				Server: &bproject.ServerSpec{URL: "https://bowrain.cloud"},
			}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Empty(t, changesetURL(tc.proj, "cs-42"))
		})
	}
}
