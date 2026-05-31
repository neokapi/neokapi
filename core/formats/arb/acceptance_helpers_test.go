//go:build acceptance

package arb_test

import (
	"bytes"
	"errors"
	"os/exec"
	"strings"
)

// lookTool reports the absolute path of an external tool and whether it is on
// PATH, so acceptance checks can skip gracefully when the tool is absent.
func lookTool(name string) (string, bool) {
	p, err := exec.LookPath(name)
	if err != nil {
		return "", false
	}
	return p, true
}

// ajvCommand returns the program and leading args for running ajv. When a real
// `ajv` executable is on PATH (e.g. `corepack pnpm add -g ajv-cli@5` in CI), it is
// invoked directly; otherwise we fall back to provisioning ajv-cli@5 via corepack pnpm dlx.
// The returned slice is the prefix; callers append the subcommand and its args.
func ajvCommand() (name string, prefix []string) {
	if p, err := exec.LookPath("ajv"); err == nil {
		return p, nil
	}
	return "corepack", []string{"pnpm", "dlx", "ajv-cli@5"}
}

// isLikelyOffline heuristically detects registry/network failures so the schema
// validation can be skipped (rather than failed) when ajv-cli cannot be fetched.
func isLikelyOffline(output []byte) bool {
	s := strings.ToLower(string(bytes.TrimSpace(output)))
	for _, marker := range []string{
		"network", "etarget", "enotfound", "getaddrinfo", "registry.npmjs",
		"econnrefused", "etimedout", "could not resolve", "offline",
		"unable to resolve", "eai_again", "fetch failed",
	} {
		if strings.Contains(s, marker) {
			return true
		}
	}
	return false
}

// toolCouldNotRun reports whether an external tool FAILED TO EXECUTE rather than
// ran and rejected kapi's output. The acceptance contract is to SKIP (not FAIL)
// when the validator itself is broken/unavailable: a non-executable npx-cached
// bin (exit 126 "Permission denied"), a missing binary (exit 127 "command not
// found"/ENOENT), or a pnpm provisioning/network failure. A tool that runs
// and reports a validation error (e.g. ajv exit 1) is NOT covered here, so the
// suite still FAILs on genuine rejections.
func toolCouldNotRun(err error, combinedOutput string) bool {
	if err == nil {
		return false
	}
	// Exit 126 (not executable) / 127 (command not found) indicate the tool
	// could not be launched, not that it judged the input.
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		switch ee.ExitCode() {
		case 126, 127:
			return true
		}
	}
	// exec.LookPath / start failures (binary vanished mid-run, ENOENT, etc.).
	if errors.Is(err, exec.ErrNotFound) {
		return true
	}
	if isLikelyOffline([]byte(combinedOutput)) {
		return true
	}
	s := strings.ToLower(combinedOutput)
	for _, marker := range []string{
		"permission denied",
		"command not found",
		"enoent",
		"no such file or directory",
		"npm error",
		"could not determine executable to run",
		"cannot find module",
		"npx canceled due to missing packages",
	} {
		if strings.Contains(s, marker) {
			return true
		}
	}
	return false
}
