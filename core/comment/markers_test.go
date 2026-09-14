package comment_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

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
