//go:build unix

package main

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

// Each attempt owns a process group so cancellation also reaches MCP servers
// and shell descendants that still hold the transcript pipe open.
func pairedConfigureProcess(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	command.Cancel = func() error { return pairedStopProcess(command) }
}

func pairedStopProcess(command *exec.Cmd) error {
	if command.Process == nil {
		return nil
	}
	err := syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
	if errors.Is(err, syscall.ESRCH) {
		return os.ErrProcessDone
	}
	return err
}
