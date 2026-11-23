package backend

import "github.com/neokapi/neokapi/host"

// PendingExecTrust is the question a tab's project's commands need answered
// before a run, or nil when they need none. The desktop asks it through a
// dialog before each run, so a script changed after the answer is asked about
// again mid-session rather than run.
func (a *App) PendingExecTrust(tabID string) (*host.ExecTrustRequest, error) {
	path, err := a.checkoutRecipe(tabID)
	if err != nil {
		return nil, err
	}
	return host.PendingExecTrust(path)
}

// ApproveExecTrust records that the person allows a tab's project's commands
// as the dialog showed them (digest). A project that changed since is not
// recorded, and the dialog asks again.
func (a *App) ApproveExecTrust(tabID, digest string) error {
	path, err := a.checkoutRecipe(tabID)
	if err != nil {
		return err
	}
	return a.hostEngine().ApproveExecTrustAt(path, digest)
}
