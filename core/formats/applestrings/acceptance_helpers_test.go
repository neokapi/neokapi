//go:build acceptance

package applestrings_test

import "os/exec"

// lookTool reports the absolute path of an external tool and whether it is on
// PATH, so acceptance checks can skip gracefully when the tool is absent.
func lookTool(name string) (string, bool) {
	p, err := exec.LookPath(name)
	if err != nil {
		return "", false
	}
	return p, true
}
