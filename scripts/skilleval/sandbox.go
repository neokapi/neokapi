package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Confining the driven agent to what the eval needs.
//
// A driven agent runs headless with tools in a scratch workspace. Headless
// means it must not stop to ask, which is what --permission-mode
// bypassPermissions buys; that removes the prompting and nothing else. What the
// process can actually reach is a separate question.
//
// It matters more here than the flag suggests, because these transcripts are
// published: every tool result is recorded whole and served from a public CDN,
// so anything the agent reads it can also publish.
//
// Claude Code answers it. `sandbox.enabled` turns on process-level confinement —
// seatbelt on macOS, bubblewrap on Linux, a separate user session on Windows —
// with filesystem and network rules beside it. Passing it through --settings
// keeps the harness out of the developer's own ~/.claude/settings.json.
//
// Measured before it was wired in, because a schema is not a measurement.
// Asked to `cat ~/.aws/config`, an unconfined agent printed the file. The same
// prompt under these settings was refused by the sandbox, and an ordinary
// command still worked, so the confinement is real and the eval still runs.
//
// What this is not: a containment boundary for hostile code. It is built to
// stop an accident, and a determined exploit in the tree being read is a
// different threat than the one it was designed for. It closes the easy path,
// which is the one that was open.

// sandboxSettings is the policy handed to `claude --settings`.
//
// Deny-read rather than allow-read: an allow-list of every path a node process
// touches is not maintainable, and a profile nobody maintains gets removed the
// first time it breaks a run.
func sandboxSettings(scratch string) (string, error) {
	settings := map[string]any{
		"sandbox": map[string]any{
			"enabled": true,
			"filesystem": map[string]any{
				"denyRead": []string{
					"~/.aws/**", "~/.ssh/**", "~/.gnupg/**", "~/.kube/**",
					"~/.docker/config.json", "~/.config/gh/**", "~/.config/gcloud/**",
					"~/.netrc", "~/.npmrc", "~/.pypirc",
				},
			},
			"network": map[string]any{
				// A scenario's work is local: read the workspace, run kapi,
				// write files. Nothing in it needs the open internet, and a
				// transcript of a run that reached it would publish wherever
				// it went.
				"allowedDomains":  []string{"api.anthropic.com", "console.anthropic.com", "statsig.anthropic.com"},
				"strictAllowlist": true,
			},
		},
	}
	body, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return "", err
	}
	path := filepath.Join(scratch, "eval-settings.json")
	if err := os.WriteFile(path, body, 0o600); err != nil {
		return "", err
	}
	return path, nil
}

// sandboxArgs returns the flags that confine a run, and whether they do.
//
// Returns confined=false rather than failing when the settings cannot be
// written: this harness is driven by a developer on their own machine, and
// refusing to run at all would trade a real capability for a theoretical one.
// The dataset records which was used rather than assuming.
func sandboxArgs(scratch string) (args []string, confined bool) {
	path, err := sandboxSettings(scratch)
	if err != nil {
		return nil, false
	}
	return []string{"--settings", path}, true
}
