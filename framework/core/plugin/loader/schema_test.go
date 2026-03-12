// Schema tests have moved to core/format/schema/schema_test.go.
// This file verifies that the re-exported type aliases work correctly.
package loader

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSchemaAliases(t *testing.T) {
	// Verify NewSchemaRegistry alias works
	reg := NewSchemaRegistry()
	assert.NotNil(t, reg)
	assert.Equal(t, 0, reg.Count())

	// Verify type aliases are usable
	s := &FilterSchema{
		Title: "Test",
		Properties: map[string]PropertySchema{
			"foo": {Type: "boolean"},
		},
	}
	assert.Equal(t, "Test", s.Title)
}
