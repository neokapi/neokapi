package event

import (
	"strings"
	"testing"

	platev "github.com/neokapi/neokapi/bowrain/core/event"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/stretchr/testify/assert"
)

func TestPushSummary_UsesStructuredCounts(t *testing.T) {
	cases := []struct {
		name string
		data map[string]string
		want string
	}{
		{
			name: "many files and blocks (dogfood shape)",
			data: map[string]string{"files_count": "474", "blocks_count": "20345"},
			want: "pushed 474 files · 20,345 blocks",
		},
		{
			name: "single file, single block pluralization",
			data: map[string]string{"files_count": "1", "blocks_count": "1"},
			want: "pushed 1 file · 1 block",
		},
		{
			name: "zero blocks still renders",
			data: map[string]string{"files_count": "3", "blocks_count": "0"},
			want: "pushed 3 files · 0 blocks",
		},
		{
			name: "no blocks field falls back to files only",
			data: map[string]string{"files_count": "5"},
			want: "pushed 5 files",
		},
		{
			name: "zero files",
			data: map[string]string{"files_count": "0"},
			want: "pushed content",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, pushSummary(tc.data))
		})
	}
}

// The summary must never enumerate the pushed file paths, even when the (still
// present, for downstream automations) joined "items" list is huge.
func TestPushSummary_NeverEnumeratesPaths(t *testing.T) {
	paths := make([]string, 474)
	for i := range paths {
		paths[i] = "web/docs/react/writing-components-" + strings.Repeat("x", 3) + ".md"
	}
	joined := strings.Join(paths, ",")

	summary := pushSummary(map[string]string{
		"items":        joined,
		"files_count":  "474",
		"blocks_count": "20345",
	})

	assert.Equal(t, "pushed 474 files · 20,345 blocks", summary)
	assert.NotContains(t, summary, "writing-components")
	assert.Less(t, len(summary), 60)
}

// Backward compatibility: an older event that only carries the joined "items"
// list (no structured counts) is summarized by counting entries, not by
// embedding the whole list.
func TestPushSummary_LegacyItemsFallback(t *testing.T) {
	summary := pushSummary(map[string]string{
		"items": "a.md,b.md,c.md",
	})
	assert.Equal(t, "pushed 3 files", summary)
	assert.NotContains(t, summary, "a.md")
}

func TestGroupThousands(t *testing.T) {
	cases := map[int]string{
		0: "0", 5: "5", 42: "42", 999: "999",
		1000: "1,000", 20345: "20,345", 1234567: "1,234,567",
	}
	for in, want := range cases {
		assert.Equal(t, want, groupThousands(in))
	}
}

// mapEventToActivity must produce the compact summary for a push event and not
// stuff the item list into the summary.
func TestMapEventToActivity_PushSummary(t *testing.T) {
	r := &ActivityRecorder{}
	a := r.mapEventToActivity(platev.Event{
		Type:      platev.EventPushCompleted,
		ProjectID: "proj-1",
		Data: map[string]string{
			"workspace_slug": "ws-1",
			"items":          "web/a.md,web/b.md,web/c.md",
			"files_count":    "3",
			"blocks_count":   "128",
		},
	})
	if assert.NotNil(t, a) {
		assert.Equal(t, bstore.ActivityItemPushed, a.Type)
		assert.Equal(t, "pushed 3 files · 128 blocks", a.Summary)
		assert.NotContains(t, a.Summary, "web/a.md")
	}
}
