package comment_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/comment"
)

func TestMarkersLineText(t *testing.T) {
	m := comment.Markers{Line: []string{"//", "#!"}, Block: []comment.BlockMarker{{Open: "/*", Close: "*/"}}}
	for _, tc := range []struct {
		line  string
		n     int
		text  string
		whole bool
	}{
		{"// okapi-skip: reason", 21, " okapi-skip: reason", true},
		{"/// <reference types=\"node\" />", 30, "/ <reference types=\"node\" />", true},
		{"#!/usr/bin/env node", 19, "/usr/bin/env node", true},
		{"/* okapi-skip: x */ const a = 1;", 19, " okapi-skip: x ", true},
		{"/** Parses. */", 14, "* Parses. ", true},
		{"/**", 0, "", false},
		{"/* opens here and closes on a later line", 0, "", false},
		{"* inside a delimited comment */", 0, "", false},
		{"const a = 1;", 0, "", false},
	} {
		n, text, whole := m.LineText([]byte(tc.line))
		assert.Equal(t, tc.whole, whole, tc.line)
		assert.Equal(t, tc.n, n, tc.line)
		assert.Equal(t, tc.text, text, tc.line)
	}

	t.Run("no markers read no line", func(t *testing.T) {
		_, _, whole := comment.Markers{}.LineText([]byte("// a comment"))
		assert.False(t, whole)
	})
}

func TestMarkersLineTextNested(t *testing.T) {
	nested := comment.Markers{Block: []comment.BlockMarker{{Open: "/*", Close: "*/", Nested: true}}}
	n, text, whole := nested.LineText([]byte("/* a /* b */ c */ code"))
	require.True(t, whole)
	assert.Equal(t, len("/* a /* b */ c */"), n)
	assert.Equal(t, " a /* b */ c ", text)

	_, _, whole = nested.LineText([]byte("/* a /* b */ still open"))
	assert.False(t, whole, "a nested comment closes only once the comment opened inside it has closed")

	flat := comment.Markers{Block: []comment.BlockMarker{{Open: "/*", Close: "*/"}}}
	n, _, whole = flat.LineText([]byte("/* a /* b */ c */"))
	require.True(t, whole)
	assert.Equal(t, len("/* a /* b */"), n, "a comment that does not nest closes at its first close")
}

func TestMarkersLineTextSplice(t *testing.T) {
	spliced := comment.Markers{Line: []string{"//"}, Splice: `\`}
	_, _, whole := spliced.LineText([]byte(`// runs on \`))
	assert.False(t, whole, "a splice carries the comment onto the next line")
	_, _, whole = spliced.LineText([]byte("// runs on \\ \t"))
	assert.False(t, whole, "spaces after the splice leave it a splice")
	n, text, whole := spliced.LineText([]byte(`// C:\path\ ends here`))
	assert.True(t, whole, "a backslash inside the line splices nothing")
	assert.Equal(t, 21, n)
	assert.Equal(t, ` C:\path\ ends here`, text)
	_, _, whole = comment.Markers{Line: []string{"//"}}.LineText([]byte(`// runs on \`))
	assert.True(t, whole, "a language without splices ends the comment with its line")
}
