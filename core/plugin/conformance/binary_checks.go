package conformance

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/neokapi/neokapi/core/clip"
	"github.com/neokapi/neokapi/core/credentials/providerenv"
)

// unknownVerb is an argument the protocol reserves for nothing. A conformant
// plugin must reject it rather than treat it as a default action, because a
// catch-all dispatcher silently swallows every verb a future protocol revision
// adds.
const unknownVerb = "__kapi_conformance_unknown_verb__"

// unknownCommand is the Mode-A counterpart: a command name no manifest can
// legitimately declare.
const unknownCommand = "__kapi_conformance_unknown_command__"

func binaryChecks() []registryEntry {
	return []registryEntry{
		{
			ID:       "binary.version",
			Title:    "`<binary> version` exits 0 and prints a version",
			Mode:     ModeAny,
			Required: true,
			Why:      "the host runs this verb to confirm an installed binary matches its manifest, and to warm first exec.",
			run:      checkBinaryVersion,
		},
		{
			ID:       "binary.version-matches-manifest",
			Title:    "the version verb reports the manifest's version",
			Mode:     ModeAny,
			Required: false,
			Why:      "a binary and manifest that disagree make `kapi plugins list` and install provenance misleading.",
			run:      checkBinaryVersionMatches,
		},
		{
			ID:       "binary.unknown-verb-rejected",
			Title:    "an unrecognised first argument exits non-zero",
			Mode:     ModeAny,
			Required: true,
			Why:      "a catch-all dispatcher silently absorbs verbs added by later protocol revisions instead of failing visibly.",
			run:      checkBinaryUnknownVerb,
		},
		{
			ID:       "binary.doctor",
			Title:    "`<binary> doctor` exits 0 when selfcheck is declared",
			Mode:     ModeAny,
			Required: true,
			Why:      "`kapi plugins doctor` dispatches this verb; a plugin that declares selfcheck must answer it.",
			run:      checkBinaryDoctor,
		},
	}
}

// execResult is the outcome of one plugin subprocess run.
type execResult struct {
	exitCode int
	stdout   string
	stderr   string
	err      error
}

// runPlugin executes the plugin binary with args, capturing both streams. It
// mirrors the host's Mode-A launch: the protocol's KAPI_PLUGIN_* variables are
// set, stdin is closed, and the exit code is reported rather than swallowed.
func (r *runner) runPlugin(ctx context.Context, args ...string) execResult {
	cmd := exec.CommandContext(ctx, r.binPath, args...)
	cmd.Env = r.pluginEnv()
	cmd.Stdin = nil
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb

	err := cmd.Run()
	res := execResult{stdout: out.String(), stderr: errb.String()}
	switch {
	case err == nil:
		res.exitCode = 0
	default:
		if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
			res.exitCode = exitErr.ExitCode()
		} else {
			res.exitCode = -1
			res.err = err
		}
	}
	return res
}

// pluginEnv is the environment every spawned plugin process sees: the host
// environment less the provider API keys, then the protocol's KAPI_PLUGIN_*
// variables, then the suite's own additions (which therefore win on a duplicate
// key).
//
// The scrub is what the host does before launching a plugin for real
// (host/pluginhost), and this harness exists to predict that launch — a
// conformance run that handed over credentials the host withholds would pass a
// plugin that reads them. Suite.Env is added afterwards on purpose: a caller
// naming a variable is granting it, which is a different thing from a binary
// inheriting one.
//
// Name and version come from the manifest when it decoded, and are empty
// otherwise — a plugin whose manifest did not decode is still worth running the
// binary-level checks against.
func (r *runner) pluginEnv() []string {
	var name, version string
	if r.man != nil {
		name, version = r.man.Plugin, r.man.Version
	}
	env := providerenv.Without(os.Environ())
	env = append(env,
		"KAPI_PLUGIN_DIR="+r.suite.Dir,
		"KAPI_PLUGIN_NAME="+name,
		"KAPI_PLUGIN_VERSION="+version,
	)
	return append(env, r.suite.Env...)
}

// firstLine returns the first non-blank line of s, trimmed.
func firstLine(s string) string {
	for line := range strings.SplitSeq(s, "\n") {
		if t := strings.TrimSpace(line); t != "" {
			return t
		}
	}
	return ""
}

func checkBinaryVersion(ctx context.Context, r *runner) (Status, string, error) {
	if st, d, err := r.requireBinary(); st != "" {
		return st, d, err
	}
	res := r.runPlugin(ctx, "version")
	if res.err != nil {
		return Fail, fmt.Sprintf("could not run `version`: %v", res.err), res.err
	}
	if res.exitCode != 0 {
		return Fail, fmt.Sprintf("`version` exited %d (stderr: %s)",
			res.exitCode, clip.Runes(strings.TrimSpace(res.stderr), 200)), nil
	}
	line := firstLine(res.stdout)
	if line == "" {
		return Fail, "`version` exited 0 but printed nothing on stdout", nil
	}
	return Pass, fmt.Sprintf("`version` → %q", clip.Runes(line, 120)), nil
}

func checkBinaryVersionMatches(ctx context.Context, r *runner) (Status, string, error) {
	if st, d, err := r.requireBinary(); st != "" {
		return st, d, err
	}
	res := r.runPlugin(ctx, "version")
	if res.exitCode != 0 {
		return Skip, "`version` did not succeed — see binary.version", nil
	}
	line := firstLine(res.stdout)
	want := r.man.Version
	// Accept a bare version or any line containing it ("myplugin 1.4.0",
	// "myplugin version 1.4.0"), since printing the name alongside is common
	// and harmless.
	if line == want || strings.Contains(line, want) {
		return Pass, fmt.Sprintf("`version` output %q contains manifest version %q", clip.Runes(line, 80), want), nil
	}
	return Fail, fmt.Sprintf("`version` printed %q but the manifest declares %q", clip.Runes(line, 80), want), nil
}

func checkBinaryUnknownVerb(ctx context.Context, r *runner) (Status, string, error) {
	if st, d, err := r.requireBinary(); st != "" {
		return st, d, err
	}
	res := r.runPlugin(ctx, unknownVerb)
	if res.err != nil {
		return Fail, fmt.Sprintf("could not run the plugin: %v", res.err), res.err
	}
	if res.exitCode == 0 {
		return Fail, fmt.Sprintf("`%s` exited 0; an unrecognised verb must be rejected", unknownVerb), nil
	}
	return Pass, fmt.Sprintf("`%s` exited %d", unknownVerb, res.exitCode), nil
}

func checkBinaryDoctor(ctx context.Context, r *runner) (Status, string, error) {
	if st, d, err := r.requireBinary(); st != "" {
		return st, d, err
	}
	if !r.man.Capabilities.SelfCheck {
		return Skip, "manifest does not declare selfcheck", nil
	}
	res := r.runPlugin(ctx, "doctor")
	if res.err != nil {
		return Fail, fmt.Sprintf("could not run `doctor`: %v", res.err), res.err
	}
	if res.exitCode != 0 {
		return Fail, fmt.Sprintf("`doctor` exited %d (stderr: %s)",
			res.exitCode, clip.Runes(strings.TrimSpace(res.stderr), 200)), nil
	}
	return Pass, fmt.Sprintf("`doctor` exited 0 (%s)",
		clip.Runes(firstLine(res.stdout+res.stderr), 120)), nil
}

func modeAChecks() []registryEntry {
	return []registryEntry{
		{
			ID:       "modeA.unknown-command-rejected",
			Title:    "`<binary> command <unknown>` exits non-zero",
			Mode:     ModeA,
			Required: true,
			Why:      "the host builds its dispatch table from the manifest; a plugin that answers undeclared commands hides typos and future collisions.",
			run:      checkModeAUnknownCommand,
		},
		{
			ID:       "modeA.command-probe",
			Title:    "the probed command runs with the expected exit code and output",
			Mode:     ModeA,
			Required: true,
			Why:      "Mode A propagates the plugin's exit code to the kapi caller; a command that cannot be dispatched breaks the verb.",
			run:      checkModeACommandProbe,
		},
	}
}

func (r *runner) requireModeA() (Status, string, error) {
	if st, d, err := r.requireBinary(); st != "" {
		return st, d, err
	}
	if !r.man.IsModeA() {
		return Skip, "manifest declares no commands", nil
	}
	return "", "", nil
}

func checkModeAUnknownCommand(ctx context.Context, r *runner) (Status, string, error) {
	if st, d, err := r.requireModeA(); st != "" {
		return st, d, err
	}
	res := r.runPlugin(ctx, "command", unknownCommand)
	if res.err != nil {
		return Fail, fmt.Sprintf("could not run the plugin: %v", res.err), res.err
	}
	if res.exitCode == 0 {
		return Fail, fmt.Sprintf("`command %s` exited 0; an undeclared command must be rejected", unknownCommand), nil
	}
	return Pass, fmt.Sprintf("`command %s` exited %d", unknownCommand, res.exitCode), nil
}

func checkModeACommandProbe(ctx context.Context, r *runner) (Status, string, error) {
	if st, d, err := r.requireModeA(); st != "" {
		return st, d, err
	}
	probe := r.suite.CommandProbe
	if probe == nil {
		return Skip, "no Suite.CommandProbe configured — the suite never invokes a declared command on its own", nil
	}
	if !declaresCommand(r, probe.Command) {
		return Fail, fmt.Sprintf("probe names command %q, which the manifest does not declare", probe.Command), nil
	}
	args := append([]string{"command", probe.Command}, probe.Args...)
	res := r.runPlugin(ctx, args...)
	if res.err != nil {
		return Fail, fmt.Sprintf("could not run `command %s`: %v", probe.Command, res.err), res.err
	}
	if res.exitCode != probe.WantExit {
		return Fail, fmt.Sprintf("`command %s` exited %d, want %d (stderr: %s)",
			probe.Command, res.exitCode, probe.WantExit,
			clip.Runes(strings.TrimSpace(res.stderr), 200)), nil
	}
	if probe.WantStdout != "" && !strings.Contains(res.stdout, probe.WantStdout) {
		return Fail, fmt.Sprintf("`command %s` stdout does not contain %q (got %q)",
			probe.Command, probe.WantStdout, clip.Runes(res.stdout, 200)), nil
	}
	return Pass, fmt.Sprintf("`command %s` exited %d as expected", probe.Command, res.exitCode), nil
}

func declaresCommand(r *runner, name string) bool {
	for _, c := range r.man.Capabilities.Commands {
		if c.Name == name {
			return true
		}
	}
	return false
}
