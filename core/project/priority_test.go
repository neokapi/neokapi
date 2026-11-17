package project

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPriorityOfFallsBackThroughDefaults(t *testing.T) {
	p := &KapiProject{Defaults: Defaults{Priority: PriorityLow}}
	assert.Equal(t, PriorityHigh, p.PriorityOf(&Collection{Priority: PriorityHigh}))
	assert.Equal(t, PriorityLow, p.PriorityOf(&Collection{}))
	assert.Equal(t, PriorityNormal, (&KapiProject{}).PriorityOf(&Collection{}))
}

func TestValidateRejectsAnUnknownPriority(t *testing.T) {
	p := sourceOnlyProject(Collection{Name: "ui", Priority: "urgent", Content: []ContentItem{{Path: "*.json"}}})
	err := p.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "priority")
}
