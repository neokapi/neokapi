package host

import "context"

// ConnectAllHosts lists every assistant kapi connect writes files for, in the
// order kapi connect lists them.
func ConnectAllHosts() []AgentHost {
	out := make([]AgentHost, 0, len(connectHostLabels))
	for _, l := range connectHostLabels {
		out = append(out, l.host)
	}
	return out
}

// InPlace reports that every one of the assistant's files is already as kapi
// connect writes it, so connecting it again would write nothing. A hook of the
// kapi Claude Code plugin counts as in place while it waits for a person,
// because enabling the plugin happens outside the project.
func (h ConnectHost) InPlace() bool {
	for _, f := range h.Files {
		if f.Kind == ConnectHook && (f.Action == ConnectKept || f.Action == ConnectPending) {
			continue
		}
		if f.Action != ConnectKept {
			return false
		}
	}
	return true
}

// AssistantConnected reports whether at least one assistant is connected to
// the project at root: its files are in place and the project has its rules
// files. It writes nothing. An answer it cannot read is false.
func (a *App) AssistantConnected(ctx context.Context, recipe, root string) bool {
	if !HasRulesFiles(root) {
		return false
	}
	cmd := NewEnvCommand(ctx, "connect")
	cmd.Flags().String("project", recipe, "")
	res, err := a.ConnectHosts(ctx, cmd, ConnectOptions{Dir: root, Hosts: ConnectAllHosts(), HostsOnly: true})
	if err != nil {
		return false
	}
	for _, h := range res.Hosts {
		if h.InPlace() {
			return true
		}
	}
	return false
}
