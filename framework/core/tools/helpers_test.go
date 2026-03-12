package tools_test

import (
	"context"
	"testing"

	"github.com/gokapi/gokapi/core/model"
	"github.com/stretchr/testify/require"
)

// processMultipleParts sends multiple parts through a tool and returns all results.
func processMultipleParts(t *testing.T, tl interface {
	Process(ctx context.Context, in <-chan *model.Part, out chan<- *model.Part) error
}, parts []*model.Part) []*model.Part {
	t.Helper()
	in := make(chan *model.Part, len(parts))
	out := make(chan *model.Part, len(parts))
	for _, p := range parts {
		in <- p
	}
	close(in)
	err := tl.Process(context.Background(), in, out)
	close(out)
	require.NoError(t, err)
	var results []*model.Part
	for p := range out {
		results = append(results, p)
	}
	return results
}
