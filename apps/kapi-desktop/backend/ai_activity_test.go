package backend

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	aiprovider "github.com/neokapi/neokapi/providers/ai"
)

func TestAIActivityLog_NewestFirstAndBounded(t *testing.T) {
	log := newAIActivityLog()
	for i := range aiActivityCap + 20 {
		log.add(
			withAIScope(context.Background(), AIActivityScope{Surface: "review", Key: fmt.Sprint(i)}),
			aiprovider.Exchange{Provider: "mock"},
		)
	}

	entries, dropped := log.recent(0)
	assert.Len(t, entries, aiActivityCap, "the log is a window, not unbounded growth")
	assert.Equal(t, 20, dropped, "evictions are counted so the UI can say the list is partial")
	assert.Equal(t, "519", entries[0].Scope.Key, "newest first")
	assert.Equal(t, "20", entries[len(entries)-1].Scope.Key, "the oldest 20 were dropped")

	top, _ := log.recent(3)
	assert.Len(t, top, 3)
	assert.Equal(t, "519", top[0].Scope.Key)
}

// An action collects its own calls by marking the log before it starts. Without
// the mark it would show whatever a concurrent run happened to add.
func TestAIActivityLog_SinceReturnsOnlyLaterEntries(t *testing.T) {
	log := newAIActivityLog()
	ctx := context.Background()
	log.add(ctx, aiprovider.Exchange{Provider: "before"})

	mark := log.lastID()
	log.add(ctx, aiprovider.Exchange{Provider: "during-1"})
	log.add(ctx, aiprovider.Exchange{Provider: "during-2"})

	got := log.since(mark)
	require.Len(t, got, 2)
	assert.Equal(t, "during-1", got[0].Provider, "oldest first, in call order")
	assert.Equal(t, "during-2", got[1].Provider)
}

func TestAIActivityLog_ClearEmptiesIt(t *testing.T) {
	log := newAIActivityLog()
	log.add(context.Background(), aiprovider.Exchange{Provider: "mock"})
	log.clear()

	entries, dropped := log.recent(0)
	assert.Empty(t, entries)
	assert.Zero(t, dropped)
}

// The end-to-end claim the Review page's disclosure rests on: an AI action
// returns the calls it made, with the messages that were actually sent.
func TestReviewAIAction_ReturnsItsOwnExchanges(t *testing.T) {
	mock := aiprovider.NewMockProvider()
	mock.ChatFunc = func(_ context.Context, _ []aiprovider.Message) (*aiprovider.ChatResponse, error) {
		return &aiprovider.ChatResponse{Content: "Salut", Model: "mock-model"}, nil
	}
	// The mock's own Translate returns a canned string without ever building a
	// prompt. Delegate to StandardTranslate, the way every real provider does,
	// so this exercises the path production takes rather than one that skips
	// both the prompt and the recording.
	mock.TranslateFunc = func(ctx context.Context, req aiprovider.TranslateRequest) (*aiprovider.TranslateResponse, error) {
		return aiprovider.StandardTranslate(ctx, "mock", mock.Chat, req, 0.9)
	}
	app := newAIReviewApp(t, mock)
	tab, _ := newReviewProject(t, app)

	res, err := app.ReviewAIAction(tab.ID, "fr-FR", filepath.Join("locales", "fr-FR.json"),
		"greeting", ReviewAIRetranslate, "make it shorter")
	require.NoError(t, err)

	require.NotEmpty(t, res.Exchanges, "the action reports what it sent")
	ex := res.Exchanges[0]
	assert.Equal(t, "review", ex.Scope.Surface)
	assert.Equal(t, ReviewAIRetranslate, ex.Scope.Action)
	assert.Equal(t, "fr-FR", ex.Scope.Locale)
	assert.Equal(t, "greeting", ex.Scope.Key)
	require.NotEmpty(t, ex.Exchange.Messages, "the messages sent on the wire are carried, not summarised")

	// The reviewer's instruction is the thing they most need to see reflected.
	var sent strings.Builder
	for _, m := range ex.Exchange.Messages {
		sent.WriteString(m.Text())
	}
	assert.Contains(t, sent.String(), "make it shorter")

	// And the same call is in the session log, attributed the same way.
	activity := app.GetAIActivity(0)
	require.NotEmpty(t, activity.Entries)
	assert.Equal(t, "review", activity.Entries[0].Scope.Surface)
	assert.Equal(t, aiActivityCap, activity.Cap)
}

func TestGetAIActivity_EmptyLogReturnsAList(t *testing.T) {
	app := newAIReviewApp(t, aiprovider.NewMockProvider())
	got := app.GetAIActivity(0)
	assert.NotNil(t, got.Entries, "an empty log is [] to the frontend, never null")
	assert.Empty(t, got.Entries)
}
