// Package reviewdeck holds the rules a review deck and its fix sheet apply:
// which group a translation waiting for review sits in, and the ready-made
// corrections offered for it. Kapi Desktop, the CLI and the Bowrain server
// call these functions, so every surface sorts a queue and offers fixes the
// same way. The package depends on the framework only, so the server links
// it without the rest of the host.
package reviewdeck

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"slices"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
)

// snapshotBlock copies a block deeply enough that what a tool does to the
// copy leaves the original as it was: every edition's runs, the properties
// and the annotation map are the copy's own.
func snapshotBlock(b *model.Block) *model.Block {
	c := b.CopyEditions(copyRuns)
	c.Properties = maps.Clone(b.Properties)
	c.Annotations = maps.Clone(b.Annotations)
	return c
}

// copyRuns copies a run sequence through its canonical JSON, so no run of the
// copy shares a pointer with the original.
func copyRuns(runs []model.Run) []model.Run {
	if runs == nil {
		return nil
	}
	var out []model.Run
	if err := json.Unmarshal(model.CanonicalRunsJSON(runs), &out); err != nil {
		return slices.Clone(runs)
	}
	return out
}

// runTools runs each tool of a chain in turn over one block.
func runTools(ctx context.Context, chain []tool.Tool, b *model.Block) error {
	parts := []*model.Part{{Type: model.PartBlock, Resource: b}}
	for _, t := range chain {
		in := make(chan *model.Part, len(parts))
		for _, p := range parts {
			in <- p
		}
		close(in)
		out := make(chan *model.Part)
		errc := make(chan error, 1)
		go func() {
			defer close(out)
			errc <- t.Process(ctx, in, out)
		}()
		var next []*model.Part
		for p := range out {
			next = append(next, p)
		}
		if err := <-errc; err != nil {
			return err
		}
		parts = next
	}
	return nil
}

// plural renders a count with the noun that agrees with it.
func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}
