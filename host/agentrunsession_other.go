//go:build !darwin && !linux

package host

// parentProcess has no reading of the process tree on this platform, so an
// agent run here falls back to the session AgentRunSession documents: one kapi
// process. Windows is the platform this covers, and a host there names its run
// with KAPI_AGENT_SESSION.
func parentProcess(int) (processInfo, error) {
	return processInfo{}, errNoAgentHostProcess
}
