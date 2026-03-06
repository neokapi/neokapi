package bridge

import (
	"os/exec"
	"syscall"
)

// setPdeathsig configures the subprocess to receive SIGKILL when its parent
// process dies. This prevents orphaned JVM subprocesses on Linux.
func setPdeathsig(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Pdeathsig = syscall.SIGKILL
}
