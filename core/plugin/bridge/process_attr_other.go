//go:build !linux

package bridge

import "os/exec"

// setPdeathsig is a no-op on non-Linux platforms. macOS and Windows do not
// support Pdeathsig; orphan prevention relies on the signal handler and
// explicit test cleanup via TestMain.
func setPdeathsig(_ *exec.Cmd) {}
