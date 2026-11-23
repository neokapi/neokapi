package host

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"

	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/host/output"
)

// StateAllow is `kapi trust allow --state`: a person vouches for the state in
// the project's state directory, so kapi uses it as this machine's own. It
// shows what is there and where it appears to come from, and records the
// person's answer, confirmed at a terminal or given with --yes, as a fresh
// mark (project.AdoptState). An agent cannot vouch for state: the MCP server
// refuses, and so does a command whose actor resolves to an agent or runs
// under an agent host's marker. An assistant that runs kapi with neither
// marker is not detected.
func (a *App) StateAllow(cmd Command, recipePath string) (output.TrustAllowOutput, error) {
	out := output.TrustAllowOutput{Path: recipePath, Sites: []output.TrustSite{}}
	if a.agentRunning() {
		return out, errors.New("an agent is running this command, and only a person can vouch for a project's state")
	}
	layout, err := project.LayoutFor(recipePath)
	if err != nil {
		return out, err
	}
	// Adopting does not lift the refusal of state git tracks: that is asked
	// first, and its remedy is to untrack the state.
	if err := project.CheckStateTracked(CmdContext(cmd), layout); err != nil {
		return out, err
	}
	origin, err := project.DescribeState(layout)
	if err != nil {
		return out, err
	}
	out.State = origin.Entries
	if len(origin.Entries) == 0 {
		out.Reason = "the project keeps no state yet, so there is nothing to vouch for"
		return out, nil
	}
	if origin.Verified {
		out.Reason = "kapi on this machine wrote this state, so there is nothing to vouch for"
		return out, nil
	}
	printStatePrompt(cmd.ErrOrStderr(), layout, origin)
	if !a.AssumeYes {
		isTTY := a.isTTY
		if isTTY == nil {
			isTTY = defaultIsStdinTTY
		}
		if !isTTY() {
			return out, errors.New("no terminal is attached, so there is nobody to confirm; pass --yes to vouch for this state")
		}
		ok, err := ConfirmDefaultNo(cmd.InOrStdin(), cmd.ErrOrStderr(), "Use this state as this machine's own? [y/N] ")
		if err != nil {
			return out, err
		}
		if !ok {
			out.Reason = "not confirmed"
			return out, nil
		}
	}
	if err := project.AdoptState(layout); err != nil {
		return out, err
	}
	out.Recorded = true
	return out, nil
}

// printStatePrompt shows the state a person is asked to vouch for.
func printStatePrompt(w io.Writer, layout project.Layout, origin project.StateOrigin) {
	fmt.Fprintf(w, "\n%s holds state that kapi on this machine did not write:\n\n", filepath.Join(layout.Root, project.StateDirName))
	for _, e := range origin.Entries {
		fmt.Fprintf(w, "  %s\n", e)
	}
	if origin.Origin != "" {
		// Written by whoever made the state: quoted, so a control character in
		// it cannot reach the terminal.
		fmt.Fprintf(w, "\nIt appears to come from %q.\n", origin.Origin)
	} else {
		fmt.Fprintln(w, "\nNothing in it says where it comes from.")
	}
	fmt.Fprintln(w, "\nVouch for it only if it is yours: a copy, a restored backup or a checkout moved to another disk. State that came with someone else's repository or archive is theirs; delete it instead.")
	fmt.Fprintln(w)
}
