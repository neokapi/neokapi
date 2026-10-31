package backend

import (
	"context"
	"errors"

	"github.com/neokapi/neokapi/host"
	"github.com/neokapi/neokapi/host/screens"
)

// The Assistants page: which assistants a project is connected to, and the
// sessions they and the person recorded in it.
//
// An assistant is connected when a HostsOnly dry run of host.ConnectHosts, the
// service `kapi connect` runs, would leave its files as they are and the rules
// files hold kapi's section. Connect runs that service for one assistant.
// Claude Desktop's file is the person's own, so its row goes through
// agentwiring.go. The sessions are the project's feed (contextfeed.go).

// assistantRulesMissing is the reason a row gives when the assistant's own
// files are in place and the project's rules files hold no kapi section.
const assistantRulesMissing = "rules files missing"

// assistantSessionsShown is how many sessions the page lists. The rest are in
// the Sessions view.
const assistantSessionsShown = 8

// connectAllHosts are the assistants the page lists, in the order kapi
// connect lists them.
var connectAllHosts = []host.AgentHost{
	host.AgentHostClaudeCode,
	host.AgentHostCursor,
	host.AgentHostVSCode,
	host.AgentHostCodex,
	host.AgentHostAgents,
}

// AssistantHostView is one assistant on the Assistants page.
type AssistantHostView struct {
	// Host is claude-code, cursor, vscode, codex, agents or claude-desktop,
	// and Label what to call it on screen.
	Host  string `json:"host"`
	Label string `json:"label"`
	// Connected reports that the assistant's files are in place, connecting
	// it again would write none of them, and the project's rules files hold
	// kapi's section.
	Connected bool `json:"connected"`
	// Reason says why an assistant whose own files are in place is not
	// connected: "rules files missing".
	Reason string `json:"reason,omitempty"`
	// Check reports that kapi check runs when the assistant finishes, through
	// the Stop hook of the kapi Claude Code plugin.
	Check bool `json:"check"`
	// Pending are the commands a person runs in the assistant to turn the
	// check on, when the plugin is not enabled yet.
	Pending []string `json:"pending,omitempty"`
	// Files are the assistant's own files, relative to the project. For
	// Claude Desktop it is the absolute path of the person's configuration.
	Files []string `json:"files,omitempty"`
	// Personal reports an assistant configured in a file of the person's
	// own, outside the project, which Connect asks about before it writes.
	Personal bool `json:"personal,omitempty"`
	// Available reports that Connect can run here. Claude Desktop needs its
	// configuration folder on this machine.
	Available bool `json:"available"`
}

// AssistantSession is one session in the project's feed, counted.
type AssistantSession struct {
	// ID is the feed group's id, which the Sessions view opens on.
	ID    string                  `json:"id"`
	Actor screens.ContextActorDTO `json:"actor"`
	// Last is when the session last recorded something, RFC3339 in UTC.
	Last string `json:"last"`
	// Quiet reports a session nothing has been added to for a while.
	Quiet bool `json:"quiet"`
	// Operations is how many operations the session holds.
	Operations int `json:"operations"`
	// Suggestions, Corrections, Kept and Dropped count the session's
	// operations by what they did; Awaiting the suggestions still open.
	Suggestions int `json:"suggestions"`
	Corrections int `json:"corrections"`
	Kept        int `json:"kept"`
	Dropped     int `json:"dropped"`
	Awaiting    int `json:"awaiting"`
	// Files, Checked, Failing and Caught are what the session's check runs
	// found, and Clean and Failed the verdicts they reached, as the feed
	// group carries them.
	Files   int  `json:"files"`
	Checked int  `json:"checked"`
	Failing int  `json:"failing"`
	Caught  int  `json:"caught"`
	Clean   bool `json:"clean"`
	Failed  bool `json:"failed"`
}

// AssistantsView is the Assistants page of one tab.
type AssistantsView struct {
	// Rules is how many rules are in force at the project's own point, the
	// count the Map shows. Nil when it could not be read.
	Rules *int `json:"rules,omitempty"`
	// HostsError and SessionsError say why the assistants or the sessions
	// could not be read; the other part of the page is read all the same.
	HostsError    string `json:"hosts_error,omitempty"`
	SessionsError string `json:"sessions_error,omitempty"`
	// NeedsCheckout reports a tab with no checkout of the project here, which
	// has no files to connect an assistant through.
	NeedsCheckout bool `json:"needs_checkout"`
	// Hosts are the assistants, in the order kapi connect lists them, then
	// Claude Desktop where this platform's configuration folder is known.
	Hosts []AssistantHostView `json:"hosts"`
	// ProjectKey is the project's workspace key, which the Sessions view is
	// narrowed by. Empty for a project the workspace does not know.
	ProjectKey string `json:"project_key,omitempty"`
	// Sessions are the newest of the project's sessions, newest first. The
	// Sessions view lists them all.
	Sessions []AssistantSession `json:"sessions"`
}

// AssistantsView reads a tab's Assistants page: each assistant and whether it
// is connected, the rules count, and the newest sessions.
func (a *App) AssistantsView(tabID string) (*AssistantsView, error) {
	op, recipe, root, err := a.checkoutOf(tabID)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), agentWiringTimeout)
	defer cancel()

	out := &AssistantsView{
		Hosts:      []AssistantHostView{},
		Sessions:   []AssistantSession{},
		ProjectKey: string(op.workspaceKey),
	}
	// The assistants and the sessions are read apart, so a failure in one
	// leaves the other on the page with the failure beside it.
	if root == "" {
		out.NeedsCheckout = true
	} else {
		hosts, herr := a.assistantHosts(ctx, recipe, root)
		if herr != nil {
			out.HostsError = herr.Error()
		} else {
			out.Hosts = hosts
			if rules, rerr := a.mapRuleCount(ctx, op); rerr == nil {
				out.Rules = &rules
			}
		}
	}

	// A folder opened without a recipe is not in the workspace yet, so its
	// log holds no session.
	if op.workspaceKey != "" && !op.unsaved.Load() {
		feed, ferr := a.contextFeed(ctx, op.workspaceKey, contextFeedDefaultLimit)
		if ferr != nil {
			out.SessionsError = ferr.Error()
			return out, nil
		}
		groups := feed.Groups
		if len(groups) > assistantSessionsShown {
			groups = groups[:assistantSessionsShown]
		}
		for _, g := range groups {
			out.Sessions = append(out.Sessions, AssistantSession{
				ID:          g.ID,
				Actor:       g.Actor,
				Last:        g.Last,
				Quiet:       g.Quiet,
				Operations:  len(g.Entries),
				Suggestions: g.Recorded,
				Corrections: g.Corrected,
				Kept:        g.Kept,
				Dropped:     g.Dropped,
				Awaiting:    g.Awaiting,
				Files:       g.Files,
				Checked:     g.Checked,
				Failing:     g.Failing,
				Caught:      g.Caught,
				Clean:       g.Clean,
				Failed:      g.Failed,
			})
		}
	}
	return out, nil
}

// assistantHosts reads each assistant's connection from a dry run of kapi
// connect over its own files, which plans no rules files, resolves no context
// and writes nothing, and from whether the project's rules files hold kapi's
// section.
func (a *App) assistantHosts(ctx context.Context, recipe, root string) ([]AssistantHostView, error) {
	res, err := a.hostEngine().ConnectHosts(ctx, connectCommand(ctx, recipe), host.ConnectOptions{
		Dir:       root,
		Hosts:     connectAllHosts,
		HostsOnly: true,
	})
	if err != nil {
		return nil, err
	}
	rules := host.HasRulesFiles(root)
	out := make([]AssistantHostView, 0, len(res.Hosts)+1)
	for _, h := range res.Hosts {
		row := AssistantHostView{Host: string(h.Host), Label: h.Label, Connected: true, Available: true}
		for _, f := range h.Files {
			switch {
			case f.Kind == host.ConnectHook && f.Action == host.ConnectKept:
				row.Check = true
			case f.Kind == host.ConnectHook && f.Action == host.ConnectPending:
				row.Pending = host.ClaudePluginCommands()
			case f.Action != host.ConnectKept:
				row.Connected = false
			}
			if f.Path != "" && f.Kind != host.ConnectHook {
				row.Files = append(row.Files, f.Path)
			}
		}
		if row.Connected && !rules {
			row.Connected, row.Reason = false, assistantRulesMissing
		}
		out = append(out, row)
	}
	if st, ok := claudeDesktopStatus(recipe); ok {
		out = append(out, AssistantHostView{
			Host:      st.Host,
			Label:     st.Label,
			Connected: st.Wired,
			Files:     st.Files,
			Personal:  true,
			Available: st.Detected,
		})
	}
	return out, nil
}

// ConnectAssistant connects one assistant to a tab's project as `kapi connect
// <host>` does: the rules files, the assistant's MCP entry and skill, and the
// recipe first when the folder was opened without one. It reports every file
// as created, updated or kept, so a second call reports nothing written.
// Claude Desktop connects through ConnectAgentHost.
func (a *App) ConnectAssistant(tabID, hostName string) (*host.ConnectResult, error) {
	op, _, root, err := a.checkoutOf(tabID)
	if err != nil {
		return nil, err
	}
	if root == "" {
		return nil, errors.New("this tab holds the project's context and no checkout of it; open the project's folder to connect an assistant")
	}
	h, err := host.ParseConnectHost(hostName)
	if err != nil {
		return nil, err
	}
	// The desktop writes the recipe of a folder opened without one, so the
	// tab takes it up; connecting then finds it in place.
	draft := op.unsaved.Load()
	if err := a.ensureRecipe(op); err != nil {
		return nil, err
	}
	a.mu.RLock()
	recipe := op.Path
	a.mu.RUnlock()

	ctx, cancel := context.WithTimeout(context.Background(), agentWiringTimeout)
	defer cancel()
	res, err := a.hostEngine().ConnectHosts(ctx, connectCommand(ctx, recipe), host.ConnectOptions{
		Dir:   root,
		Hosts: []host.AgentHost{h},
	})
	if err != nil {
		return nil, err
	}
	if draft {
		for i := range res.Files {
			if res.Files[i].Kind == host.ConnectRecipe {
				res.Files[i].Action = host.ConnectCreated
			}
		}
	}
	return res, nil
}

// connectCommand is the command ConnectHosts resolves the project from: the
// tab's recipe, as `kapi connect -p <recipe>` names it.
func connectCommand(ctx context.Context, recipe string) host.Command {
	cmd := host.NewEnvCommand(ctx, "connect")
	cmd.Flags().String("project", recipe, "")
	return cmd
}
