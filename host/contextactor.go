package host

import (
	"fmt"
	"os"
	"strings"

	"github.com/neokapi/neokapi/core/contextop"
)

// Who a command-line context operation is recorded as.
//
// The MCP tools state the actor themselves (host/mcp_grow.go): that server
// process is serving an agent, it knows the client's name from initialize, and
// it mints one session for its own run. The command line has no such handle.
// `kapi context observe` is the same command whether a person typed it or a
// coding agent ran it from its shell, and an operation that states no kind is
// recorded as a person, which is the reading that carries a person's rights.
//
// So the environment answers. A caller states the kind outright with
// KAPI_ACTOR, and otherwise kapi looks for the marker variable an agent host
// exports to every command it runs.
//
// THE SAFE DIRECTION. Reading a person as an agent costs a confirmation step:
// the entry is born a candidate, which is where it was heading anyway. Reading
// an agent as a person puts an unreviewed rule on the record with a person's
// standing behind it, out of reach of `kapi context log --actor agent` and of
// `kapi context revert --session`. A marker therefore means agent. A person
// typing in an agent host's shell says so with KAPI_ACTOR=person, and that
// override is written into the operation's note, because an entry claiming a
// person's rights under a host the environment names is one a reviewer should
// see for what it is.
const (
	// EnvActor names the actor kind outright: "agent" or "person".
	EnvActor = "KAPI_ACTOR"
	// EnvAgentName is the agent's own name, for a host kapi does not recognise.
	// It fills contextop.Actor.Name, which `kapi context log --actor` matches.
	EnvAgentName = "KAPI_AGENT_NAME"
	// EnvAgentSession groups everything one agent run records, so a person can
	// read it back and revert it together.
	EnvAgentSession = "KAPI_AGENT_SESSION"
)

// AgentHostMarker is one coding-agent host kapi recognises from the
// environment.
//
// The table is data, and a row joins it after somebody has seen the variable in
// that host's own shell rather than in its documentation. A row that names a
// host wrongly attributes a person's work to an agent, so a guess costs more
// here than a missing row does: an unrecognised host is covered by KAPI_ACTOR.
type AgentHostMarker struct {
	// Name is what an operation records as the agent's name.
	Name string
	// Marker is the variable the host exports to every command it runs. Its
	// presence is what says an agent is driving this one.
	Marker string
	// Session is a variable the host exports that holds still across one run of
	// it and differs between runs. Empty where a host exports none, and the
	// session is then derived from the host's own process.
	Session string
}

// agentHostMarkers is the table itself.
var agentHostMarkers = []AgentHostMarker{
	{Name: "claude-code", Marker: "CLAUDECODE", Session: "CLAUDE_CODE_SESSION_ID"},
	{Name: "codex", Marker: "CODEX_SESSION_ID", Session: "CODEX_SESSION_ID"},
}

// AgentHostMarkers returns every host kapi recognises from the environment.
func AgentHostMarkers() []AgentHostMarker {
	out := make([]AgentHostMarker, len(agentHostMarkers))
	copy(out, agentHostMarkers)
	return out
}

// ResolvedActor is who a command-line call is recorded as, and what the answer
// rested on.
type ResolvedActor struct {
	// Actor is the answer, ready to ride on a request.
	Actor contextop.Actor
	// Host is the agent host the environment named, empty when none did.
	Host string
	// PersonOverride reports that KAPI_ACTOR asked for a person under a host
	// the environment names.
	PersonOverride bool
}

// NoteWith folds the resolution into the note an operation carries. A person
// working in an agent host's shell leaves that on the record; everything else
// keeps the note the caller wrote.
func (r ResolvedActor) NoteWith(note string) string {
	if !r.PersonOverride {
		return note
	}
	said := "recorded as a person in a " + r.Host + " shell"
	if note == "" {
		return said
	}
	return note + " (" + said + ")"
}

// ResolveCommandActor answers who a command-line context operation belongs to,
// reading the environment through getenv.
//
// An unreadable KAPI_ACTOR is an error rather than a fallback: a typo that
// silently recorded an agent's work as a person's would be the very confusion
// the variable exists to settle.
func ResolveCommandActor(getenv func(string) string) (ResolvedActor, error) {
	agentHost := detectAgentHost(getenv)
	switch strings.ToLower(strings.TrimSpace(getenv(EnvActor))) {
	case string(contextop.ActorPerson):
		return ResolvedActor{
			Actor:          contextop.Actor{Kind: contextop.ActorPerson},
			Host:           agentHost.Name,
			PersonOverride: agentHost.Name != "",
		}, nil
	case string(contextop.ActorAgent):
		return ResolvedActor{Actor: agentActor(getenv, agentHost), Host: agentHost.Name}, nil
	case "":
		if agentHost.Name == "" {
			return ResolvedActor{Actor: contextop.Actor{Kind: contextop.ActorPerson}}, nil
		}
		return ResolvedActor{Actor: agentActor(getenv, agentHost), Host: agentHost.Name}, nil
	default:
		return ResolvedActor{}, fmt.Errorf("%s takes %q or %q, not %q",
			EnvActor, contextop.ActorAgent, contextop.ActorPerson, getenv(EnvActor))
	}
}

// detectAgentHost finds the first row of the table whose marker the
// environment sets. An empty value counts as unset, which is how a harness
// clears a marker it inherited.
func detectAgentHost(getenv func(string) string) AgentHostMarker {
	for _, row := range agentHostMarkers {
		if strings.TrimSpace(getenv(row.Marker)) != "" {
			return row
		}
	}
	return AgentHostMarker{}
}

// agentActor builds the agent's identity: the name it goes by, and the session
// that groups one run of it.
func agentActor(getenv func(string) string, agentHost AgentHostMarker) contextop.Actor {
	name := strings.TrimSpace(getenv(EnvAgentName))
	if name == "" {
		name = agentHost.Name
	}
	return contextop.Actor{
		Kind:    contextop.ActorAgent,
		Name:    name,
		Session: agentSession(getenv, agentHost),
	}
}

// agentSession answers with the session one agent run records under, in three
// steps.
//
//	$KAPI_AGENT_SESSION       the run's own id, used as given
//	the host's session variable   the id that host calls this run
//	the host's process        a digest of the process that started the shell
//
// The last step is what a host with no session variable of its own falls back
// to, and AgentRunSession explains how it holds still across the separate kapi
// processes of one run.
func agentSession(getenv func(string) string, agentHost AgentHostMarker) string {
	if session := strings.TrimSpace(getenv(EnvAgentSession)); session != "" {
		return session
	}
	if agentHost.Session != "" {
		if session := strings.TrimSpace(getenv(agentHost.Session)); session != "" {
			return session
		}
	}
	return AgentRunSession()
}

// commandActor answers who a request that states no actor belongs to.
//
// Inside a `go test` binary the environment is not consulted unless KAPI_ACTOR
// names the kind, for the reason host.DataDir gives about the data root: a
// suite run from a coding agent's shell inherits that host's marker, and every
// operation a test recorded would read as the agent rather than as the person
// the test is written about. ResolveCommandActor takes its environment as an
// argument, so the table is tested directly.
func (a *App) commandActor() (ResolvedActor, error) {
	if underTest() && os.Getenv(EnvActor) == "" {
		return ResolvedActor{Actor: contextop.Actor{Kind: contextop.ActorPerson}}, nil
	}
	return ResolveCommandActor(os.Getenv)
}
