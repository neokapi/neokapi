package mdx

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A surfaced segment is written back verbatim, so whatever protectCodeSpans
// returns must concatenate to exactly what it was given. Everything else it
// does is only allowed on top of that.
func TestProtectCodeSpansIsByteFaithful(t *testing.T) {
	inputs := []string{
		"plain prose with no code at all",
		"`docker compose up` + `make dev-server`",
		"Set `BOWRAIN_JWT_SECRET` before starting",
		"a stray ` backtick that never closes",
		"``a span with `inner` backticks``",
		"`` ` ``",
		"trailing code `kapi up`",
		"`kapi up` leading code",
		"",
		"| a | b |",
		"两个 `kapi up` 命令",
	}
	for _, in := range inputs {
		got := model.RunsText(protectCodeSpans(in))
		assert.Equal(t, in, got, "runs must rebuild the segment byte for byte")
	}
}

func TestProtectCodeSpansMarksCommands(t *testing.T) {
	runs := protectCodeSpans("`docker compose up` + `make dev-server`")

	var protected, plain []string
	for _, r := range runs {
		require.NotNil(t, r.Text, "only text runs are produced")
		if r.Text.NoTranslate {
			protected = append(protected, r.Text.Text)
		} else {
			plain = append(plain, r.Text.Text)
		}
	}

	// The fences travel inside the protected run, so a translator cannot
	// separate a command from its delimiters.
	assert.Equal(t, []string{"`docker compose up`", "`make dev-server`"}, protected)
	assert.Equal(t, []string{" + "}, plain)
}

func TestProtectCodeSpansLeavesProseTranslatable(t *testing.T) {
	runs := protectCodeSpans("What runs where")
	require.Len(t, runs, 1)
	assert.False(t, runs[0].Text.NoTranslate, "prose with no code stays translatable")

	// An opener with no partner is prose, not a span that swallows the rest.
	runs = protectCodeSpans("a stray ` backtick")
	require.Len(t, runs, 1)
	assert.False(t, runs[0].Text.NoTranslate)
}
