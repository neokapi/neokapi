package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStatusLabel_ApprovedIsTheDisplayOfEstablished(t *testing.T) {
	assert.Equal(t, "approved", StatusLabel("established"))
	assert.Equal(t, "approved", TargetStatusEstablished.Label())
	assert.Equal(t, "approved", SourceStatusEstablished.Label())
	for _, s := range []string{"draft", "translated", "written", "", "withheld"} {
		assert.Equal(t, s, StatusLabel(s), "every other rung reads as its own name")
	}
	// The stored value is unchanged: the label is display only.
	assert.Equal(t, "established", string(TargetStatusEstablished))
}
