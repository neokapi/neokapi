package host

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"

	"github.com/neokapi/neokapi/core/partjoin"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/host/output"
)

// A surface that asks through its own dialog (the desktop) shows a checkout's
// one-time state lines and the transfer each can lead to, and takes a transfer
// only as the dialog showed it. The terminal gives the same lines through
// project.NoticeState and the same transfers through `kapi state import` and
// `kapi state relink`.

// CheckoutStateNotice is one line a surface owes the person about a checkout's
// state, with the transfer it offers.
type CheckoutStateNotice struct {
	// Name is one of project.NoticeEarlierAtPath, NoticeWorkInTree and
	// NoticeStateInTree.
	Name string `json:"name"`
	// Message is the line the terminal prints.
	Message string `json:"message"`
	// Transfer is what `kapi state relink` (for an earlier record at the
	// location) or `kapi state import` (for state in the checkout) would take,
	// or nil for a line that offers none. Its Action names which.
	Transfer *output.StateTransferOutput `json:"transfer,omitempty"`
	// Digest names the transfer as shown; ApplyCheckoutStateTransfer takes it
	// only while the plan still digests to it.
	Digest string `json:"digest,omitempty"`
}

// PendingCheckoutState lists the one-time lines about the state of the checkout
// at recipePath that no person has been shown, each with the transfer it
// offers. It writes nothing. A transfer that cannot be planned (a link among
// the files) is left off its line; the terminal command says why.
func PendingCheckoutState(recipePath string) ([]CheckoutStateNotice, error) {
	layout, err := project.LayoutFor(recipePath)
	if err != nil {
		return nil, err
	}
	var out []CheckoutStateNotice
	for _, n := range project.PendingStateNotices(layout) {
		item := CheckoutStateNotice{Name: n.Name, Message: n.Message}
		if action, plan, ok := planFor(layout, n.Name); ok && len(plan.Files) > 0 {
			t := transferOutput(layout, action, plan)
			item.Transfer = &t
			item.Digest = planDigest(plan)
		}
		out = append(out, item)
	}
	return out, nil
}

// RecordCheckoutStateShown records that a person was shown the lines named
// about the checkout at recipePath, so no surface shows them again.
func RecordCheckoutStateShown(recipePath string, names []string) error {
	layout, err := project.LayoutFor(recipePath)
	if err != nil {
		return err
	}
	var errs []error
	for _, name := range names {
		switch name {
		case project.NoticeEarlierAtPath, project.NoticeWorkInTree, project.NoticeStateInTree:
			errs = append(errs, project.RecordStateNoticeGiven(layout, name))
		default:
			errs = append(errs, fmt.Errorf("no state notice is named %q", name))
		}
	}
	return errors.Join(errs...)
}

// ApplyCheckoutStateTransfer takes the transfer the line name offers, for a
// person who confirmed it in a dialog that showed the plan digest. A plan that
// changed since it was shown is refused, and the dialog shows it again. An
// agent cannot take state into a checkout, as at the terminal. The copies in
// the checkout stay unless removeSource is set, which only an import takes.
func (a *App) ApplyCheckoutStateTransfer(recipePath, name, digest string, removeSource bool) (output.StateTransferOutput, error) {
	layout, err := project.LayoutFor(recipePath)
	if err != nil {
		return output.StateTransferOutput{}, err
	}
	action, plan, ok := planFor(layout, name)
	if !ok {
		return output.StateTransferOutput{}, fmt.Errorf("the state notice %q offers no transfer", name)
	}
	out := transferOutput(layout, action, plan)
	if a.agentRunning() {
		return out, errors.New("an agent is running this command, and only a person can bring state into a checkout")
	}
	if planDigest(plan) != digest {
		return out, errors.New("the state changed after it was shown; look at it again before taking it")
	}
	if action != "import" {
		removeSource = false
	}
	if err := project.ApplyTransfer(layout, plan, removeSource); err != nil {
		return out, err
	}
	out.Taken = true
	out.SourceRemoved = removeSource
	return out, nil
}

// planFor plans the transfer the line name offers: a relink from the
// checkout's own location for an earlier record there, an import for state in
// the checkout. ok is false for a line that offers none or a plan that fails.
func planFor(layout project.Layout, name string) (action string, plan project.StateTransfer, ok bool) {
	var err error
	switch name {
	case project.NoticeEarlierAtPath:
		action = "relink"
		plan, err = project.PlanRelink(layout, layout.Root, project.RelinkOptions{})
	case project.NoticeStateInTree:
		action = "import"
		plan, err = project.PlanImport(layout)
	default:
		return "", project.StateTransfer{}, false
	}
	return action, plan, err == nil
}

// transferOutput is the plan as a surface shows it.
func transferOutput(layout project.Layout, action string, plan project.StateTransfer) output.StateTransferOutput {
	out := output.StateTransferOutput{Action: action, From: plan.From, To: layout.LocalStateDir(), Servers: plan.Servers}
	for _, f := range plan.Files {
		out.Files = append(out.Files, output.StateFileOutput{Source: f.Source, Target: f.Target, Size: f.Size})
	}
	return out
}

// planDigest names a plan by where it takes from, each file with its content
// digest, and the servers its sync state was issued by.
func planDigest(plan project.StateTransfer) string {
	parts := []string{"kapi-state-transfer/v1", plan.From}
	for _, f := range plan.Files {
		parts = append(parts, f.Source, f.Target, strconv.FormatInt(f.Size, 10), f.Digest)
	}
	parts = append(parts, plan.Servers...)
	sum := sha256.Sum256([]byte(partjoin.NUL(parts...)))
	return hex.EncodeToString(sum[:])
}
