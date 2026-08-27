package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// An absolute home path resolves on exactly one laptop, and
// scripts/check-abs-paths.sh holds every tracked file to zero of them. This
// dataset is tracked, and the strings in it are written by an agent working in
// a throwaway directory: it reaches kapi by whatever path it found and prints
// the temp root in its own commands.
//
// The repo has been here before: a Go subtest named after an absolute fixture
// path once put 358 home paths into a committed dataset. The guard catches it,
// but three steps from anything that looks like this file, on a build that
// otherwise had nothing to do with paths.
//
// The fixtures below use "me", which is one of the placeholder names the guard
// itself allows for unit-test tables where a plausible absolute path is the
// point. A test for a path scrubber has to contain paths.

func TestScrubPathsRemovesWhatTheGuardLooksFor(t *testing.T) {
	cases := []struct{ name, in string }{
		{"the developer's checkout", "grep -rn version <(/Users/me/src/neokapi/bin/kapi help recipe 2>&1)"},
		{"a scenario's temp workspace", "cd /private/var/folders/vp/xxx/T/skilleval-p04-1955381723 && kgrep -i sales docs/"},
		{"a linux home", "/home/runner/work/neokapi/bin/kapi check --ship"},
		{"a homebrew prefix", "/opt/homebrew/bin/kgrep -r utilize docs/"},
		{"a bare temp path", "wrote /tmp/kapi-bench-cwd/out.json"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := scrubPaths(c.in)
			for _, root := range []string{"/Users/", "/home/", "/private/", "/opt/", "/tmp/", "/var/"} {
				assert.NotContains(t, got, root,
					"%q survived scrubbing as %q", c.in, got)
			}
		})
	}
}

// TestScrubPathsKeepsTheReadablePart: scrubbing is not redaction. A command is
// evidence, and one reduced to "<path>" tells a reader nothing about what the
// agent did.
func TestScrubPathsKeepsTheReadablePart(t *testing.T) {
	got := scrubPaths("/opt/homebrew/bin/kgrep -r utilize docs/")
	assert.Contains(t, got, "-r utilize docs/", "the arguments are the interesting half")

	got = scrubPaths("kapi voice check --profile-file voice.yaml")
	assert.Equal(t, "kapi voice check --profile-file voice.yaml", got,
		"a command with no absolute path must come through untouched")
}

// TestNoAbsolutePathReachesTheReport walks a whole report the way the guard
// walks the file, so a new field carrying agent text cannot slip past by being
// somewhere the scrubber was never wired.
func TestNoAbsolutePathReachesTheReport(t *testing.T) {
	r := &Report{
		Mode:    modeTrigger,
		Surface: surfaceSkill,
		Runner:  Runner{Kapi: "bin/kapi", KapiVersion: scrubPaths("kapi v1 (/Users/me/src/kapi)")},
		Results: []Result{{
			Scenario: scenarios[0],
			Runs: []Run{{
				Triggered:    true,
				KapiCommands: []string{scrubPaths("cd /private/var/folders/x/T/w && kapi version")},
				FinalText:    scrubPaths("I read /Users/me/src/neokapi/SKILL.md first."),
				Gate:         &GateResult{Command: "test -f out.json", Output: scrubPaths("/tmp/w/out.json missing")},
			}},
		}},
	}

	body, err := json.Marshal(r)
	require.NoError(t, err)
	text := string(body)

	for _, root := range []string{"/Users/", "/home/", "/private/var/", "/opt/", "/tmp/"} {
		assert.False(t, strings.Contains(text, root),
			"a serialized report still carries %q, which the tracked-file guard rejects", root)
	}
}
