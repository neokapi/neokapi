package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestComputeContentHash(t *testing.T) {
	t.Run("stable for same text", func(t *testing.T) {
		h1 := ComputeContentHash("Hello world")
		h2 := ComputeContentHash("Hello world")
		assert.Equal(t, h1, h2)
	})

	t.Run("trims whitespace", func(t *testing.T) {
		h1 := ComputeContentHash("Hello world")
		h2 := ComputeContentHash("  Hello world  ")
		assert.Equal(t, h1, h2)
	})

	t.Run("different text produces different hash", func(t *testing.T) {
		h1 := ComputeContentHash("Hello")
		h2 := ComputeContentHash("World")
		assert.NotEqual(t, h1, h2)
	})

	t.Run("empty string", func(t *testing.T) {
		h := ComputeContentHash("")
		assert.NotEmpty(t, h)
		assert.Len(t, h, 64) // SHA-256 hex = 64 chars
	})

	t.Run("does not lowercase", func(t *testing.T) {
		// Case is content-significant: the normalization must NOT fold case.
		// If this fails, the on-the-wire hash contract has changed and a
		// hash-version migration is required (see ComputeContentHash doc).
		h1 := ComputeContentHash("Hello world")
		h2 := ComputeContentHash("hello world")
		assert.NotEqual(t, h1, h2, "content hash must be case-sensitive")
	})

	t.Run("does not collapse interior whitespace", func(t *testing.T) {
		h1 := ComputeContentHash("Hello world")
		h2 := ComputeContentHash("Hello  world")
		assert.NotEqual(t, h1, h2, "interior whitespace is content-significant")
	})

	t.Run("golden hash pins the wire contract", func(t *testing.T) {
		// This is the exact SHA-256 of "Hello world" (TrimSpace-only
		// normalization). It is part of the on-the-wire content-hash contract
		// consumed by the sync diff engine. If this changes, every existing
		// project's hashes change too — do NOT update this value without a
		// coordinated hash-version migration.
		const golden = "64ec88ca00b268e5ba1a35678a1b5316d212f4f366b2477232534a8aeca37f3c"
		assert.Equal(t, golden, ComputeContentHash("Hello world"))
		// Trimming must yield the same golden hash.
		assert.Equal(t, golden, ComputeContentHash("  Hello world\n"))
	})
}

func TestComputeContextHash(t *testing.T) {
	t.Run("stable for same context", func(t *testing.T) {
		h1 := ComputeContextHash("title", "heading", map[string]string{"level": "1"})
		h2 := ComputeContextHash("title", "heading", map[string]string{"level": "1"})
		assert.Equal(t, h1, h2)
	})

	t.Run("different name produces different hash", func(t *testing.T) {
		h1 := ComputeContextHash("title", "heading", nil)
		h2 := ComputeContextHash("subtitle", "heading", nil)
		assert.NotEqual(t, h1, h2)
	})

	t.Run("property order does not matter", func(t *testing.T) {
		h1 := ComputeContextHash("", "", map[string]string{"a": "1", "b": "2"})
		h2 := ComputeContextHash("", "", map[string]string{"b": "2", "a": "1"})
		assert.Equal(t, h1, h2)
	})

	t.Run("nil properties", func(t *testing.T) {
		h := ComputeContextHash("name", "type", nil)
		assert.NotEmpty(t, h)
	})
}

func TestComputeIdentity(t *testing.T) {
	b := NewBlock("b1", "Hello world")
	b.Name = "greeting"
	b.Type = "text"

	id := ComputeIdentity(b)
	require.NotNil(t, id)
	assert.NotEmpty(t, id.ContentHash)
	assert.NotEmpty(t, id.ContextHash)

	// Same content, different context.
	b2 := NewBlock("b2", "Hello world")
	b2.Name = "farewell"
	b2.Type = "text"
	id2 := ComputeIdentity(b2)

	assert.Equal(t, id.ContentHash, id2.ContentHash, "same text should have same content hash")
	assert.NotEqual(t, id.ContextHash, id2.ContextHash, "different name should have different context hash")
}
