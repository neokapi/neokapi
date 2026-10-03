package host

import (
	"context"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/history"
	"github.com/neokapi/neokapi/core/model"
)

// EditionRevisions reads doc of the project at recipe through the change
// service and returns the revision each of editions holds now in each block,
// keyed as the block history names an edition: the block's key and the
// edition's text. An edition a block does not hold is left out.
//
// It is a read like any other, so it records what it finds changed outside
// kapi (design 5.7), and the change that left each revision it returns is in
// the block history when it returns (history.Store.Wrote). It names no
// edition's basis, which is the larger half of a read's cost.
func (a *App) EditionRevisions(ctx context.Context, recipe, doc string, editions []model.EditionKey) (map[history.EditionRef]string, error) {
	svc, err := a.ChangeService(ctx, ChangeServiceOptions{Project: recipe, revisionsOnly: true})
	if err != nil {
		return nil, err
	}
	out := map[history.EditionRef]string{}
	_, err = svc.ReadEach(ctx, change.ReadRequest{Doc: doc, Editions: editions}, func(b *model.Block, _ change.BlockRead) error {
		for _, k := range editions {
			k = b.EditionKeyOf(k)
			if _, held := b.Edition(k); !held {
				continue
			}
			text, err := k.MarshalText()
			if err != nil || len(text) == 0 {
				continue
			}
			out[history.EditionRef{Block: change.BlockKey(b), Edition: string(text)}] = model.EditionRevision(b, k)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
