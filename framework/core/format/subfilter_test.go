package format_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/stretchr/testify/assert"
)

func TestSubfilterMapping(t *testing.T) {
	m := format.SubfilterMapping{
		Pattern: "*.body",
		Format:  "html",
	}
	assert.Equal(t, "*.body", m.Pattern)
	assert.Equal(t, "html", m.Format)
}

func TestSubfilterAwareInterface(t *testing.T) {
	// Verify the interface exists and can be used as a type constraint.
	var _ format.SubfilterAware = (*mockSubfilterAware)(nil)
}

type mockSubfilterAware struct{}

func (m *mockSubfilterAware) SetSubfilterResolver(_ format.SubfilterResolver) {}
