package host

import (
	"bytes"

	"golang.org/x/sys/unix"
)

// parentProcess reads one process's parent off the kernel's process table.
// macOS keeps no /proc, so the parent pid, the command name and the start time
// all come from the one sysctl that describes a process.
func parentProcess(pid int) (processInfo, error) {
	self, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if err != nil {
		return processInfo{}, err
	}
	ppid := int(self.Eproc.Ppid)
	if ppid <= 0 {
		return processInfo{}, errNoAgentHostProcess
	}
	parent, err := unix.SysctlKinfoProc("kern.proc.pid", ppid)
	if err != nil {
		return processInfo{}, err
	}
	comm := parent.Proc.P_comm
	name := string(bytes.TrimRight(comm[:], "\x00"))
	return processInfo{
		PID:   ppid,
		Name:  name,
		Start: parent.Proc.P_starttime.Sec*1_000_000 + int64(parent.Proc.P_starttime.Usec),
	}, nil
}
