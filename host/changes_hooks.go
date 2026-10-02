package host

import (
	"context"

	"github.com/neokapi/neokapi/core/change"
)

// The three hooks the change service calls, each built here and plugged in at
// this one place. A nil hook is what the service runs without: it checks
// nothing at commit, permits every operation, or records nothing. An error
// stops the service from being built, so a surface never edits without a
// hook it was meant to have.

// changeCommitCheck is the commit check the service runs over the editions a
// change set changes, before anything is written. cmd names the service's
// project with -p, as every check surface resolves one; a service outside a
// project gets a command that names none.
func (a *App) changeCommitCheck(_ Command) (change.CommitCheck, error) {
	return nil, nil
}

// changePolicy decides which operations an actor may send, for the project
// at recipe ("" outside a project).
func (a *App) changePolicy(_ string) (change.Policy, error) {
	return nil, nil
}

// changeRecorder records an applied change set for the project whose root
// directory is root ("" outside a project).
func (a *App) changeRecorder(_ context.Context, _ string) (change.Recorder, error) {
	return nil, nil
}
