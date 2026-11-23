package host

import (
	"errors"
	"fmt"
	"io"

	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/host/output"
)

// StateImport is `kapi state import`: it takes the state an earlier kapi left
// inside the checkout (the redaction vault, the sync state, the saved filters)
// into the checkout's state directory under the data root. It shows what it
// will take, asks at a terminal or takes --yes, and takes exactly what it
// showed. The copies in the checkout stay unless removeSource is set.
func (a *App) StateImport(cmd Command, recipePath string, removeSource bool) (output.StateTransferOutput, error) {
	layout, err := project.LayoutFor(recipePath)
	if err != nil {
		return output.StateTransferOutput{Action: "import"}, err
	}
	plan, err := project.PlanImport(layout)
	if err != nil {
		return output.StateTransferOutput{Action: "import"}, err
	}
	return a.stateTransfer(cmd, layout, "import", plan, removeSource)
}

// StateRelink is `kapi state relink`: it moves the state kapi recorded for the
// checkout's earlier location into the checkout's state directory, for a
// checkout that was moved, restored or cloned again. It shows what it will
// take, asks at a terminal or takes --yes, and takes exactly what it showed.
func (a *App) StateRelink(cmd Command, recipePath, from string) (output.StateTransferOutput, error) {
	layout, err := project.LayoutFor(recipePath)
	if err != nil {
		return output.StateTransferOutput{Action: "relink"}, err
	}
	plan, err := project.PlanRelink(layout, from)
	if err != nil {
		return output.StateTransferOutput{Action: "relink"}, err
	}
	return a.stateTransfer(cmd, layout, "relink", plan, false)
}

// StatePath is `kapi state path`: the directory that holds the checkout's own
// state, for a CI cache to save and restore.
func (a *App) StatePath(recipePath string) (output.StatePathOutput, error) {
	layout, err := project.LayoutFor(recipePath)
	if err != nil {
		return output.StatePathOutput{}, err
	}
	return output.StatePathOutput{Project: layout.Root, Path: layout.LocalStateDir()}, nil
}

// stateTransfer shows a transfer, confirms it with the person, and applies it.
func (a *App) stateTransfer(cmd Command, layout project.Layout, action string, plan project.StateTransfer, removeSource bool) (output.StateTransferOutput, error) {
	out := output.StateTransferOutput{Action: action, From: plan.From, To: layout.LocalStateDir(), Servers: plan.Servers}
	for _, f := range plan.Files {
		out.Files = append(out.Files, output.StateFileOutput{Source: f.Source, Target: f.Target, Size: f.Size})
	}
	if a.agentRunning() {
		return out, errors.New("an agent is running this command, and only a person can bring state into a checkout")
	}
	if len(plan.Files) == 0 {
		out.Reason = "there is no state to take"
		return out, nil
	}
	printStateTransfer(cmd.ErrOrStderr(), out)
	ok, err := a.confirmAtTerminal(cmd, "Take this state into this checkout? [y/N] ",
		"no terminal is attached, so there is nobody to confirm; pass --yes to take this state")
	if err != nil {
		return out, err
	}
	if !ok {
		out.Reason = "not confirmed"
		return out, nil
	}
	if err := project.ApplyTransfer(layout, plan, removeSource); err != nil {
		return out, err
	}
	out.Taken = true
	out.SourceRemoved = removeSource
	return out, nil
}

// printStateTransfer shows what a transfer would take.
func printStateTransfer(w io.Writer, o output.StateTransferOutput) {
	fmt.Fprintf(w, "kapi would take these files from %s into %s:\n", o.From, o.To)
	for _, f := range o.Files {
		fmt.Fprintf(w, "  %s  (%d bytes)\n", f.Source, f.Size)
	}
	for _, s := range o.Servers {
		fmt.Fprintf(w, "The sync state among them was issued by %q, and kapi sends its claim token to that server only.\n", s)
	}
	fmt.Fprintln(w, "Take it only if this state is yours: a vault holds withheld originals, and sync state can hold a token for a server project.")
}
