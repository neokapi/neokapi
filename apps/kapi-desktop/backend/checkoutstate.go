package backend

import (
	"github.com/neokapi/neokapi/host"
	"github.com/neokapi/neokapi/host/output"
)

// PendingCheckoutState lists the one-time lines about a tab's checkout state
// that no person has been shown, each with the transfer it offers: state an
// earlier kapi left in the checkout, which an import takes, and state recorded
// for another folder at the checkout's location, which a relink takes. The
// terminal prints the same lines. Asking writes nothing.
func (a *App) PendingCheckoutState(tabID string) ([]host.CheckoutStateNotice, error) {
	path, err := a.checkoutRecipe(tabID)
	if err != nil {
		return nil, err
	}
	return host.PendingCheckoutState(path)
}

// MarkCheckoutStateShown records that the dialog showed the person the lines
// named, so neither the app nor the terminal shows them again.
func (a *App) MarkCheckoutStateShown(tabID string, names []string) error {
	path, err := a.checkoutRecipe(tabID)
	if err != nil {
		return err
	}
	return host.RecordCheckoutStateShown(path, names)
}

// ApplyCheckoutStateTransfer takes the transfer a line offers, as the dialog
// showed it (digest), for the person who confirmed it there. A plan that
// changed since is refused, and the dialog shows it again. removeSource
// removes an import's copies from the checkout.
func (a *App) ApplyCheckoutStateTransfer(tabID, name, digest string, removeSource bool) (output.StateTransferOutput, error) {
	path, err := a.checkoutRecipe(tabID)
	if err != nil {
		return output.StateTransferOutput{}, err
	}
	return a.hostEngine().ApplyCheckoutStateTransfer(path, name, digest, removeSource)
}
