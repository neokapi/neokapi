package host

import (
	"context"

	"github.com/neokapi/neokapi/core/change"
)

// The three hooks the change service calls, each built here for the project
// at recipe ("" outside a project). A nil hook is what the service runs
// without: it checks nothing at commit, permits every operation, or records
// nothing. Each is plugged in at this one place.

// changeCommitCheck is the commit check the service runs over the editions a
// change set changes, before anything is written.
func (a *App) changeCommitCheck(_ context.Context, _ string) change.CommitCheck {
	return nil
}

// changePolicy decides which operations an actor may send.
func (a *App) changePolicy(_ string) change.Policy {
	return nil
}

// changeRecorder records an applied change set in the workspace log.
func (a *App) changeRecorder(_ context.Context, _ string) change.Recorder {
	return nil
}
