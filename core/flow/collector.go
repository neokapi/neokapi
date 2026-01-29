package flow

import (
	"context"

	"github.com/asgeirf/gokapi/core/model"
)

// Collector accumulates results from processed documents.
// Implementations must be safe for concurrent use.
type Collector interface {
	// Collect receives output parts from a single document.
	Collect(ctx context.Context, item *FlowItem, parts []*model.Part) error
	// Result returns the aggregated result after all documents complete.
	Result() (CollectorResult, error)
}

// CollectorResult holds the output of a Collector.
type CollectorResult struct {
	Name string
	Data interface{}
}
