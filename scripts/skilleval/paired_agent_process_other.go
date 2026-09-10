//go:build !unix

package main

import "os/exec"

// Pipe closure bounds cancellation on hosts without Unix process groups.
func pairedConfigureProcess(command *exec.Cmd) {
	command.Cancel = func() error { return pairedStopProcess(command) }
}

func pairedStopProcess(command *exec.Cmd) error {
	if command.Process == nil {
		return nil
	}
	return command.Process.Kill()
}
