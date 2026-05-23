//go:build acceptance

package arb_test

import (
	"bytes"
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
