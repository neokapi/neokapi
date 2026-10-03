package change_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/model"
)

// histories answers a history from a function, recording what it was asked.
type histories struct {
	asked []historyAsk
	fn    func(b *model.Block, k model.EditionKey, limit int) []change.HistoryEntry
}

type historyAsk struct {
	doc   string
	block string
	key   model.EditionKey
	limit int
}

func (h *histories) EditionHistory(_ context.Context, doc change.DocInfo, b *model.Block, k model.EditionKey, limit int) ([]change.HistoryEntry, error) {
	h.asked = append(h.asked, historyAsk{doc: doc.Doc, block: change.BlockKey(b), key: k, limit: limit})
	if h.fn == nil {
		return nil, nil
	}
	return h.fn(b, k, limit), nil
}

func TestService_History(t *testing.T) {
	at := time.Date(2026, 10, 3, 9, 0, 0, 0, time.UTC)
	entry := func(after string) change.HistoryEntry {
		return change.HistoryEntry{Record: "op-" + after, Before: "absent", After: after,
			Actor: &change.Actor{Kind: change.ActorPerson, Name: "ada"}, Origin: "desktop", At: at}
	}
	tests := []struct {
		name     string
		req      func(b change.BlockRead) change.HistoryRequest
		hook     bool
		wantRef  func(b change.BlockRead) change.Ref
		wantRev  func(b change.BlockRead) string
		wantKey  model.EditionKey
		wantN    int
		wantCode change.Code
	}{
		{
			name:    "the document's own edition",
			req:     func(b change.BlockRead) change.HistoryRequest { return change.HistoryRequest{Ref: b.Ref} },
			hook:    true,
			wantRef: func(b change.BlockRead) change.Ref { return b.Ref },
			wantRev: func(b change.BlockRead) string { return b.Rev },
			wantN:   2,
		},
		{
			name: "a derived edition",
			req: func(b change.BlockRead) change.HistoryRequest {
				return change.HistoryRequest{Ref: atEdition(b.Ref, "nb")}
			},
			hook:    true,
			wantRef: func(b change.BlockRead) change.Ref { return atEdition(b.Ref, "nb") },
			wantRev: func(b change.BlockRead) string { return b.Editions["nb"].Rev },
			wantKey: model.EditionKey{Locale: "nb"},
			wantN:   2,
		},
		{
			name:    "the limit bounds what the host is asked for and what is listed",
			req:     func(b change.BlockRead) change.HistoryRequest { return change.HistoryRequest{Ref: b.Ref, Limit: 1} },
			hook:    true,
			wantRef: func(b change.BlockRead) change.Ref { return b.Ref },
			wantRev: func(b change.BlockRead) string { return b.Rev },
			wantN:   1,
		},
		{
			name:    "without a host the history lists nothing",
			req:     func(b change.BlockRead) change.HistoryRequest { return change.HistoryRequest{Ref: b.Ref} },
			wantRef: func(b change.BlockRead) change.Ref { return b.Ref },
			wantRev: func(b change.BlockRead) string { return b.Rev },
			wantN:   0,
		},
		{
			name: "a block the document does not hold is not found",
			req: func(b change.BlockRead) change.HistoryRequest {
				r := b.Ref
				r.Block = "missing"
				return change.HistoryRequest{Ref: r}
			},
			hook:     true,
			wantCode: change.CodeNotFound,
		},
		{
			name: "a reference with no block is invalid",
			req: func(b change.BlockRead) change.HistoryRequest {
				return change.HistoryRequest{Ref: change.Ref{Doc: b.Ref.Doc}}
			},
			hook:     true,
			wantCode: change.CodeInvalid,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newMemHome(map[string][]memBlock{"a": {textBlock("one", "First", "nb", "Første"), textBlock("two", "Second")}})
			hook := &histories{fn: func(_ *model.Block, _ model.EditionKey, limit int) []change.HistoryEntry {
				// The host answers more than it was asked for; the service holds the limit.
				return []change.HistoryEntry{entry("r:2222222222222222"), entry("r:1111111111111111")}
			}}
			var opts []change.Option
			if tc.hook {
				opts = append(opts, change.WithHistories(hook))
			}
			svc := newMemService(h, opts...)
			b := readBlock(t, svc, "a", "one")

			got, err := svc.History(context.Background(), tc.req(b))
			if tc.wantCode != "" {
				var ce *change.Error
				require.ErrorAs(t, err, &ce)
				assert.Equal(t, tc.wantCode, ce.Code)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantRef(b), got.Ref)
			assert.Equal(t, tc.wantRev(b), got.Rev, "the history names the revision the edition holds now")
			assert.Len(t, got.Entries, tc.wantN)
			assert.NotNil(t, got.Entries, "an empty history is a list, never null")
			if tc.hook {
				require.Len(t, hook.asked, 1)
				assert.Equal(t, "one", hook.asked[0].block)
				assert.Equal(t, tc.wantKey, hook.asked[0].key)
			}
		})
	}
}
