package change_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/model"
)

// memObserver keeps what each read showed it.
type memObserver struct {
	reads []*memObservation
}

type memObservation struct {
	doc    string
	blocks []string
	eds    map[string][]string
	done   int
}

func (o *memObserver) Observe(_ context.Context, doc change.DocInfo) change.Observation {
	r := &memObservation{doc: doc.Doc, eds: map[string][]string{}}
	o.reads = append(o.reads, r)
	return r
}

func (r *memObservation) Saw(b *model.Block, editions []model.EditionKey) {
	key := change.BlockKey(b)
	r.blocks = append(r.blocks, key)
	for _, k := range editions {
		text, _ := k.MarshalText()
		r.eds[key] = append(r.eds[key], string(text))
	}
}

func (r *memObservation) Done(context.Context) { r.done++ }

func TestRead_ShowsEachBlockItReadsToTheObserver(t *testing.T) {
	h := newMemHome(map[string][]memBlock{"a": {
		textBlock("one", "First", "nb", "Første"),
		textBlock("two", "Second"),
	}})
	obs := &memObserver{}
	svc := newMemService(h, change.WithObserver(obs))
	ctx := context.Background()

	cases := []struct {
		name   string
		read   func() error
		blocks []string
	}{
		{name: "a page", blocks: []string{"one", "two"}, read: func() error {
			_, err := svc.Read(ctx, change.ReadRequest{Doc: "a", Editions: []model.EditionKey{{Locale: "de"}}})
			return err
		}},
		{name: "a streamed read", blocks: []string{"one", "two"}, read: func() error {
			_, err := svc.ReadEach(ctx, change.ReadRequest{Doc: "a", Editions: []model.EditionKey{{Locale: "de"}}},
				func(*model.Block, change.BlockRead) error { return nil })
			return err
		}},
		{name: "a read of one block", blocks: []string{"two"}, read: func() error {
			_, err := svc.Read(ctx, change.ReadRequest{Doc: "a", Blocks: []string{"two"}, Editions: []model.EditionKey{{Locale: "de"}}})
			return err
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			obs.reads = nil
			require.NoError(t, tc.read())
			require.Len(t, obs.reads, 1, "one observation per read")
			r := obs.reads[0]
			assert.Equal(t, "a", r.doc)
			assert.Equal(t, tc.blocks, r.blocks, "every block the read shows, and only those")
			assert.Equal(t, 1, r.done, "the observation ends once")
			for _, b := range tc.blocks {
				assert.Contains(t, r.eds[b], "de", "an edition the read asked for is covered, held or not")
			}
			if len(r.eds["one"]) > 0 {
				assert.Contains(t, r.eds["one"], "nb", "an edition the block holds is covered")
			}
		})
	}

	// A read its caller records itself is not shown.
	obs.reads = nil
	_, err := svc.Read(change.Unobserved(ctx), change.ReadRequest{Doc: "a"})
	require.NoError(t, err)
	assert.Empty(t, obs.reads)
}
