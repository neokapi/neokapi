package model_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/neokapi/neokapi/core/model"
)

func TestStructuralPath(t *testing.T) {
	assert.Equal(t, "intro/install/p", model.StructuralPath("intro", "install", "p"))
	assert.Equal(t, "intro/p", model.StructuralPath("intro", "", "p"), "empty segments drop out")
	assert.Equal(t, "", model.StructuralPath())

	// A separator inside a segment would forge a level that is not there.
	assert.Equal(t, "a-b/c", model.StructuralPath("a/b", "c"))

	// Re-flowing a heading across lines must not rename everything beneath it.
	assert.Equal(t, "Getting started", model.StructuralPath("Getting\n  started"))
}

func TestNameBuilder_DisambiguatesOnlyRealRepeats(t *testing.T) {
	var b model.NameBuilder

	assert.Equal(t, "intro/p", b.Name("intro/p"))
	assert.Equal(t, "intro/p#2", b.Name("intro/p"), "a genuine repeat needs an ordinal")
	assert.Equal(t, "intro/p#3", b.Name("intro/p"))
	assert.Equal(t, "guide/p", b.Name("guide/p"), "a different path is untouched")
}

func TestNameBuilder_ZeroValueAndReset(t *testing.T) {
	var b model.NameBuilder
	assert.Equal(t, "a", b.Name("a"), "the zero value is usable")

	b.Reset()
	assert.Equal(t, "a", b.Name("a"), "reset starts a new document")
}

// A block with no structural position still needs a name, or it would collide
// with every other unnamed block in the document.
func TestNameBuilder_EmptyPath(t *testing.T) {
	var b model.NameBuilder
	assert.Equal(t, "block", b.Name(""))
	assert.Equal(t, "block#2", b.Name(""))
}
