package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The control arm exists because a scenario note asserted a capability limit
// that was not true: "the agent has no other way to read it" of a .pptx, which
// an unaided agent read with unzip in three calls.
//
// The comparison it feeds is the place where the same mistake would recur, and
// in a more flattering direction. These tests hold it to conservative: no gate
// means no claim, a small effort gap means no claim, and kapi failing means no
// claim regardless of what the control did.

func TestContributionIsConservative(t *testing.T) {
	green := &GateResult{ExitCode: 0}
	red := &GateResult{ExitCode: 1}

	runs := func(spec ...struct {
		msgs int
		gate *GateResult
	},
	) []Run {
		var out []Run
		for _, s := range spec {
			out = append(out, Run{Messages: s.msgs, Gate: s.gate})
		}
		return out
	}
	r := func(msgs int, gate *GateResult) struct {
		msgs int
		gate *GateResult
	} {
		return struct {
			msgs int
			gate *GateResult
		}{msgs, gate}
	}

	cases := []struct {
		name    string
		with    []Run
		unaided []Run
		gated   bool
		want    Contribution
	}{
		{
			name:    "green with kapi, red without",
			with:    runs(r(10, green)),
			unaided: runs(r(30, red)),
			gated:   true,
			want:    ContributionEnabled,
		},
		{
			name:    "green both ways, control did half again as much",
			with:    runs(r(10, green)),
			unaided: runs(r(20, green)),
			gated:   true,
			want:    ContributionEased,
		},
		{
			name:    "green both ways, similar effort",
			with:    runs(r(10, green)),
			unaided: runs(r(11, green)),
			gated:   true,
			want:    ContributionNeither,
		},
		{
			name:    "kapi did not finish either",
			with:    runs(r(10, red)),
			unaided: runs(r(40, red)),
			gated:   true,
			want:    ContributionNeither,
		},
		{
			name:    "no gate and similar effort settles nothing",
			with:    runs(r(10, nil)),
			unaided: runs(r(11, nil)),
			gated:   false,
			want:    ContributionUnknown,
		},
		{
			name:    "no gate but the control worked much harder",
			with:    runs(r(10, nil)),
			unaided: runs(r(25, nil)),
			gated:   false,
			want:    ContributionEased,
		},
		{
			name:    "no control arm",
			with:    runs(r(10, green)),
			unaided: nil,
			gated:   true,
			want:    ContributionUnknown,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, contribution(c.with, c.unaided, c.gated))
		})
	}
}

// TestKapiFailingIsNeverAWin is worth its own test because it is the
// flattering mistake: kapi's gate is red, the control's is red, and an eval
// that scored on effort alone would report "eased" on a task neither arm did.
func TestKapiFailingIsNeverAWin(t *testing.T) {
	red := &GateResult{ExitCode: 1}
	got := contribution(
		[]Run{{Messages: 5, Gate: red}},
		[]Run{{Messages: 60, Gate: red}},
		true,
	)
	assert.Equal(t, ContributionNeither, got,
		"kapi finished nothing; twelve times the effort on the control side does not change that")
}

// TestMedianResistsOneRunawayPass: agent runs vary, and a mean would let a
// single long control pass manufacture an "eased".
func TestMedianResistsOneRunawayPass(t *testing.T) {
	assert.Equal(t, 10, medianMessages([]Run{{Messages: 9}, {Messages: 10}, {Messages: 90}}))
	assert.Equal(t, 0, medianMessages(nil))
	assert.Equal(t, 0, medianMessages([]Run{{Messages: 0}}), "a run that emitted nothing is not a zero-effort run")
}

// TestTheControlCannotReachKapi.
//
// A developer's PATH has kapi in it, from Homebrew or from this checkout. If
// the control arm inherited it, the measurement would be of an agent that HAD
// kapi and merely lacked the skill, which is a different and much weaker
// question.
func TestTheControlCannotReachKapi(t *testing.T) {
	dir := t.TempDir()
	withKapi := filepath.Join(dir, "has-kapi")
	withToolbox := filepath.Join(dir, "has-kgrep")
	clean := filepath.Join(dir, "clean")
	for _, d := range []string{withKapi, withToolbox, clean} {
		require.NoError(t, os.MkdirAll(d, 0o755))
	}
	require.NoError(t, os.WriteFile(filepath.Join(withKapi, "kapi"), []byte("#!/bin/sh\n"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(withToolbox, "kgrep"), []byte("#!/bin/sh\n"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(clean, "ls"), []byte("#!/bin/sh\n"), 0o755))

	in := strings.Join([]string{withKapi, clean, withToolbox}, string(os.PathListSeparator))
	out := stripKapiFromPath(in)

	assert.NotContains(t, out, withKapi, "a directory holding kapi must be removed")
	assert.NotContains(t, out, withToolbox, "so must one holding a toolbox binary; kgrep is kapi too")
	assert.Contains(t, out, clean, "everything else must survive, or the control has no shell at all")
}

// TestADirectoryNamedKapiIsNotEnough: the check is for a kapi BINARY, not for
// a path that happens to contain the word. Stripping this checkout's whole tree
// would leave the control without git, python or anything else it needs.
func TestADirectoryNamedKapiIsNotEnough(t *testing.T) {
	dir := t.TempDir()
	looksLikeIt := filepath.Join(dir, "src", "neokapi", "tools")
	require.NoError(t, os.MkdirAll(looksLikeIt, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(looksLikeIt, "jq"), []byte("#!/bin/sh\n"), 0o755))

	out := stripKapiFromPath(looksLikeIt)
	assert.Contains(t, out, looksLikeIt, "no kapi binary lives here; the name is a coincidence")
}
