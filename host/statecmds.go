package host

import (
	"errors"
	"fmt"
	"io"
	"strings"

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
// State recorded for another project is refused (project.PlanRelink);
// sameProject states that state whose record cannot say is this project's.
func (a *App) StateRelink(cmd Command, recipePath, from string, sameProject bool) (output.StateTransferOutput, error) {
	layout, err := project.LayoutFor(recipePath)
	if err != nil {
		return output.StateTransferOutput{Action: "relink"}, err
	}
	plan, err := project.PlanRelink(layout, from, project.RelinkOptions{SameProject: sameProject})
	if err != nil {
		return output.StateTransferOutput{Action: "relink"}, err
	}
	return a.stateTransfer(cmd, layout, "relink", plan, false)
}

// StatePath is `kapi state path`: the directory that holds the checkout's own
// state, and the parse cache directory ParseCacheDir resolves, which
// $KAPI_PARSE_CACHE_DIR can place outside it.
func (a *App) StatePath(recipePath string) (output.StatePathOutput, error) {
	layout, err := project.LayoutFor(recipePath)
	if err != nil {
		return output.StatePathOutput{}, err
	}
	parseCache, err := ParseCacheDir(layout)
	if err != nil {
		return output.StatePathOutput{}, err
	}
	return output.StatePathOutput{Project: layout.Root, Path: layout.LocalStateDir(), ParseCache: parseCache}, nil
}

// stateTransfer shows a transfer, confirms it with the person, and applies it.
func (a *App) stateTransfer(cmd Command, layout project.Layout, action string, plan project.StateTransfer, removeSource bool) (output.StateTransferOutput, error) {
	out := transferOutput(layout, action, plan)
	if a.agentRunning() {
		return out, errors.New("an agent is running this command, and only a person can bring state into a checkout")
	}
	if len(plan.Files) == 0 {
		out.Reason = "there is no state to take"
		if len(plan.Withheld) > 0 {
			printStateTransfer(cmd.ErrOrStderr(), out)
			out.Reason = strings.Join(plan.WithheldWhy, "; ")
		}
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
	return applyTransfer(layout, plan, removeSource, out)
}

// applyTransfer applies plan and reports it in out. An import that took every
// file and could not remove some copies it was asked to remove is reported as
// taken, with the copies that remain.
func applyTransfer(layout project.Layout, plan project.StateTransfer, removeSource bool, out output.StateTransferOutput) (output.StateTransferOutput, error) {
	err := project.ApplyTransfer(layout, plan, removeSource)
	if kept, ok := errors.AsType[*project.SourcesKeptError](err); ok {
		out.Taken = true
		out.Remaining = kept.Files
		return out, nil
	}
	if err != nil {
		return out, err
	}
	out.Taken = true
	out.SourceRemoved = removeSource
	return out, nil
}

// transferOutput is the plan as a surface shows it.
func transferOutput(layout project.Layout, action string, plan project.StateTransfer) output.StateTransferOutput {
	out := output.StateTransferOutput{Action: action, From: plan.From, To: layout.LocalStateDir(), Servers: plan.Servers,
		Project: plan.Project, Unverified: plan.Unverified, RemovesSource: plan.RemovesSource}
	for _, f := range plan.Files {
		out.Files = append(out.Files, output.StateFileOutput{Source: f.Source, Target: f.Target, Size: f.Size})
	}
	for _, f := range plan.Withheld {
		out.Withheld = append(out.Withheld, output.StateFileOutput{Source: f.Source, Target: f.Target, Size: f.Size})
	}
	out.WithheldWhy = plan.WithheldWhy
	if plan.SyncProject != nil {
		out.SyncServer, out.SyncProject = plan.SyncProject.Server, plan.SyncProject.Project
		if ws := plan.SyncProject.Workspace; ws != "" {
			out.SyncProject = ws + "/" + out.SyncProject
		}
	}
	return out
}

// printStateTransfer shows what a transfer would take.
func printStateTransfer(w io.Writer, o output.StateTransferOutput) {
	if len(o.Files) > 0 {
		fmt.Fprintf(w, "kapi would take these files from %s into %s:\n", o.From, o.To)
	}
	for _, f := range o.Files {
		fmt.Fprintf(w, "  %s  (%d bytes)\n", f.Source, f.Size)
	}
	if o.Unverified {
		fmt.Fprintf(w, "kapi's record cannot say this state belongs to this project; it was recorded for %s.\n", o.Project)
	}
	for _, f := range o.Withheld {
		fmt.Fprintf(w, "  stays: %s  (%d bytes)\n", f.Source, f.Size)
	}
	for _, why := range o.WithheldWhy {
		fmt.Fprintf(w, "Why they stay: %s.\n", why)
	}
	if o.SyncProject != "" {
		fmt.Fprintf(w, "The sync state is bound to %s project %s.\n", o.SyncServer, o.SyncProject)
	}
	for _, s := range o.Servers {
		fmt.Fprintf(w, "The sync state among them was issued by %q, and kapi sends its claim token to that server only.\n", s)
	}
	if o.RemovesSource {
		fmt.Fprintf(w, "Once every file is in place and read back, kapi removes %s.\n", o.From)
	}
	fmt.Fprintln(w, "Take it only if this state is yours: a vault holds withheld originals, and sync state can hold a token for a server project.")
}
