package host

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/state"
)

// Reviewing, approving or annotating one unit names that unit's file. When no
// reader for the file's format is installed the request fails, names the plugin
// to install, and records nothing. It never approves a unit it could not read.

// requireInstallHint holds an error to the sentinel and to naming the plugin.
func requireInstallHint(t *testing.T, err error, plugin string) {
	t.Helper()
	require.Error(t, err)
	require.ErrorIs(t, err, registry.ErrUnknownFormat)
	assert.Contains(t, err.Error(), "(kapi plugins install "+plugin+")")
}

// readableReviewKey returns the key of the readable translation awaiting review,
// the positive control every named-unit test approves or reads.
func readableReviewKey(t *testing.T, recipe string) string {
	t.Helper()
	q, err := (&App{}).ReviewQueue(context.Background(), recipe, "en", ReviewQueueOptions{})
	require.NoError(t, err)
	for _, it := range q.Pending {
		if it.File == "fr.json" {
			return it.Key
		}
	}
	t.Fatalf("no readable unit awaits review: %+v", q.Pending)
	return ""
}

func TestApproveReviewUnitWithNoReaderFailsAndRecordsNothing(t *testing.T) {
	root := reviewUnreadProject(t, true)
	recipe := filepath.Join(root, "kapi.yaml")
	ctx := context.Background()

	ok, err := (&App{}).ApproveReviewUnit(ctx, recipe, "en", "fr", "pkg/doc.fr.idml", "hello")
	requireInstallHint(t, err, "okapi-bridge")
	assert.False(t, ok)
	st, serr := (&App{}).OpenProjectState(ctx, root)
	require.NoError(t, serr)
	recorded, aerr := st.All(ctx)
	require.NoError(t, aerr)
	assert.Empty(t, recorded, "nothing was approved")

	// The readable unit beside it approves as usual.
	ok, err = (&App{}).ApproveReviewUnit(ctx, recipe, "en", "fr", "fr.json", readableReviewKey(t, recipe))
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestReviewUnitWithNoReaderNamesThePlugin(t *testing.T) {
	root := reviewUnreadProject(t, true)
	recipe := filepath.Join(root, "kapi.yaml")
	ctx := context.Background()

	_, err := (&App{}).ReviewUnit(ctx, recipe, "en", ReviewUnitRef{File: "pkg/doc.fr.idml", Key: "hello", Locale: "fr"})
	requireInstallHint(t, err, "okapi-bridge")

	info, err := (&App{}).ReviewUnit(ctx, recipe, "en", ReviewUnitRef{File: "fr.json", Key: readableReviewKey(t, recipe), Locale: "fr"})
	require.NoError(t, err)
	assert.NotEmpty(t, info.Target, "the readable unit is read")
}

func TestRecordAIReviewsWithNoReaderNamesThePlugin(t *testing.T) {
	root := reviewUnreadProject(t, true)
	recipe := filepath.Join(root, "kapi.yaml")

	n, err := (&App{}).RecordAIReviews(context.Background(), recipe, "en", "fr", "pkg/doc.fr.idml",
		map[string]state.AIReview{"hello": {Score: 90}})
	requireInstallHint(t, err, "okapi-bridge")
	assert.Zero(t, n)
}

// A source unit is named by its source file. The request says which plugin reads
// it rather than reporting that the unit does not exist.
func TestSourceUnitReviewAndApprovalWithNoReaderNameThePlugin(t *testing.T) {
	root := reviewUnreadProject(t, true)
	recipe := filepath.Join(root, "kapi.yaml")
	ctx := context.Background()

	_, err := (&App{}).ReviewUnit(ctx, recipe, "en", ReviewUnitRef{File: "pkg/doc.idml", Key: "hello", Locale: "en"})
	requireInstallHint(t, err, "okapi-bridge")

	ok, err := (&App{}).ApproveSourceUnit(ctx, recipe, "en", SourceUnitRef{File: "pkg/doc.idml", Key: "hello"})
	requireInstallHint(t, err, "okapi-bridge")
	assert.False(t, ok)
}
