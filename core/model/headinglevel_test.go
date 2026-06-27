package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBlockHeadingLevel(t *testing.T) {
	t.Run("from structure annotation", func(t *testing.T) {
		b := NewBlock("h", "Title")
		b.SetSemanticRole(RoleHeading, 3)
		assert.Equal(t, 3, b.HeadingLevel())
	})
	t.Run("from level property fallback", func(t *testing.T) {
		b := NewBlock("h", "Title")
		b.Properties = map[string]string{"level": "2"}
		assert.Equal(t, 2, b.HeadingLevel())
	})
	t.Run("structure level wins over property", func(t *testing.T) {
		b := NewBlock("h", "Title")
		b.SetSemanticRole(RoleHeading, 4)
		b.Properties["level"] = "2"
		assert.Equal(t, 4, b.HeadingLevel())
	})
	t.Run("unset is zero", func(t *testing.T) {
		assert.Equal(t, 0, NewBlock("p", "x").HeadingLevel())
	})
	t.Run("non-numeric property is zero", func(t *testing.T) {
		b := NewBlock("h", "x")
		b.Properties = map[string]string{"level": "abc"}
		assert.Equal(t, 0, b.HeadingLevel())
	})
}
