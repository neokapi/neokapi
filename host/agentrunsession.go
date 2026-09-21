package host

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
)

// The session an agent run records under when nothing in the environment names
// one.
//
// A session has one job: to group everything one agent run recorded, so a
// person can read it back with `kapi context log --session` and take the whole
// of it out again with `kapi context revert --session`. That asks two things of
// an id. It holds still across the separate kapi processes of one run, and it
// differs between runs.
//
// The obvious handles fail the first test. Every command an agent host runs
// gets a shell of its own, so kapi's pid and its parent's change between two
// calls of one run. The host's own process does not: it started when the run
// started and it is an ancestor of every command the run makes. So the session
// is a digest of that process, found by climbing out of the shells.

// processInfo is one process, as much of it as a session needs.
type processInfo struct {
	// PID is the process.
	PID int
	// Name is the executable's base name, which is what the shell test reads.
	Name string
	// Start is when the process began, in whatever unit the platform reports.
	// It is in the digest so a reused pid cannot merge two runs.
	Start int64
}

// errNoAgentHostProcess reports that the process tree gave no host process:
// either the platform does not let this process read it, or the walk reached
// the top without finding one.
var errNoAgentHostProcess = errors.New("host: no agent host process above this one")

// maxProcessWalk bounds the climb. A process tree is shallow, and a bound is
// what keeps a cycle in a platform's answer from spinning.
const maxProcessWalk = 32

// shellNames are the executables the walk climbs past. An agent host starts one
// of these for each command it runs, so a shell is never the run.
var shellNames = []string{
	"ash", "bash", "busybox", "cmd.exe", "csh", "dash", "fish", "ksh",
	"powershell.exe", "pwsh", "pwsh.exe", "sh", "tcsh", "zsh",
}

// agentRunSession is minted once per kapi process. The answer cannot change
// while the process runs, and reading the process tree is a handful of system
// calls that nothing should repeat per operation.
var agentRunSession = sync.OnceValue(func() string {
	proc, err := agentHostProcess(os.Getpid())
	if err != nil {
		// The documented fallback: a session that covers this one kapi
		// process. It still groups what a single command recorded and still
		// tells two commands apart, which is everything a session can honestly
		// promise where the process tree cannot be read.
		return processSessionID(os.Getpid(), 0)
	}
	return processSessionID(proc.PID, proc.Start)
})

// AgentRunSession is the session id for one agent run on this machine, derived
// from the process that started the shell kapi is running in.
//
// Where the process tree cannot be read, which is every platform without a
// parentProcess implementation and any sandbox that hides other processes, the
// answer covers this kapi process alone. `kapi context log --session` then
// finds what one command recorded rather than what the run did, and
// KAPI_AGENT_SESSION is how a host says otherwise.
func AgentRunSession() string { return agentRunSession() }

// agentHostProcess finds the process that started the shell this command runs
// in, reading the tree through the platform.
func agentHostProcess(pid int) (processInfo, error) {
	return walkToAgentHost(pid, parentProcess)
}

// walkToAgentHost climbs out of the shells: from pid upward, an ancestor whose
// executable is a shell is passed over, and the first that is not is the agent
// host. It answers the same whether the shell forked to run kapi or replaced
// itself with it, because both leave the same process above.
//
// parent reads one process's parent, which is the platform's part of the job
// and the seam a test drives a tree of its own through.
func walkToAgentHost(pid int, parent func(int) (processInfo, error)) (processInfo, error) {
	for range maxProcessWalk {
		above, err := parent(pid)
		if err != nil {
			return processInfo{}, err
		}
		// pid 1 is the machine rather than a run, and so is anything that
		// reports no parent.
		if above.PID <= 1 {
			return processInfo{}, errNoAgentHostProcess
		}
		if !isShellProcess(above.Name) {
			return above, nil
		}
		pid = above.PID
	}
	return processInfo{}, errNoAgentHostProcess
}

// isShellProcess reports whether an executable name is one of the shells. A
// login shell is spelled with a leading dash, which the trim covers.
func isShellProcess(name string) bool {
	base := strings.ToLower(strings.TrimPrefix(filepath.Base(name), "-"))
	return slices.Contains(shellNames, base)
}

// processSessionID renders a process as a session id short enough to type,
// because reverting a session is something a person does by hand.
func processSessionID(pid int, start int64) string {
	sum := sha256.Sum256([]byte(strconv.Itoa(pid) + ":" + strconv.FormatInt(start, 10)))
	return "s" + hex.EncodeToString(sum[:6])
}
