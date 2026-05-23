//go:build acceptance

package xcstrings_test

import (
	"bytes"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
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

// runValidator runs an external validator and asserts it exits cleanly,
// surfacing combined output on failure.
func runValidator(t *testing.T, label, path string, args []string) {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), path, args...)
	out, err := cmd.CombinedOutput()
	assert.NoErrorf(t, err, "%s must accept kapi output: %s", label, out)
}

// isLikelyOffline heuristically detects npm/npx network failures so the schema
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
