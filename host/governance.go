package host

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/neokapi/neokapi/core/project"
)

// Governance resolution, host side.
//
// Every surface that has to know what governs a piece of content — the flow
// run's partitioning, `kapi check`, the retrieval answer, the coordinates a
// push carries — resolves through project.ResolveGovernanceFor at the run's
// instant. The instant is fixed once per run so a long convergence cannot
// cross a validity boundary halfway through and govern two files differently
// for no reason a reader could see.

// governanceRun is one run's governance state: the instant its resolutions read
// validity windows at, and the transitions already reported.
type governanceRun struct {
	at time.Time

	mu    sync.Mutex
	noted map[string]bool
}

// ensureGovernance builds the run's governance state once. Converge workers are
// pre-seeded with the parent's, so a fanned-out run keeps one instant and one
// set of reported transitions.
func (a *App) ensureGovernance() *governanceRun {
	a.governanceOnce.Do(func() {
		if a.governance == nil {
			a.governance = &governanceRun{at: time.Now(), noted: map[string]bool{}}
		}
	})
	return a.governance
}

// GovernanceInstant is the wall clock this run resolves validity windows
// against. There is no flag to move it: governance that could be resolved for a
// pretended date would make a check's verdict depend on an argument rather than
// on the content.
func (a *App) GovernanceInstant() time.Time { return a.ensureGovernance().at }

// GovernancePointFor builds the point one file sits at, resolved at the run's
// instant. relPath is project-relative and slash-separated; an empty one falls
// back to the collection name, and an empty collection is the project's default
// point.
func (a *App) GovernancePointFor(collection, relPath string) project.GovernancePoint {
	return project.GovernancePoint{Collection: collection, Path: relPath, At: a.GovernanceInstant()}
}

// ResolveGovernanceAtPoint resolves the governance in force at a point and
// reports any validity transition it applied, once per run.
func (a *App) ResolveGovernanceAtPoint(cmd Command, proj *project.KapiProject, pt project.GovernancePoint) (*project.ResolvedGovernance, error) {
	if proj == nil {
		return &project.ResolvedGovernance{VoiceField: project.DefaultVoiceField}, nil
	}
	rc, err := proj.ResolveGovernanceFor(pt)
	if err != nil {
		return nil, err
	}
	a.NoteGovernance(cmd, rc)
	return rc, nil
}

// NoteGovernance reports a profile that stopped governing because the run's
// instant sits outside its window, and what governs instead — once per run, on
// stderr, so a `--json` surface stays machine-readable. A run that swapped
// voices on a date without saying so would be indistinguishable from a bug.
func (a *App) NoteGovernance(cmd Command, rc *project.ResolvedGovernance) {
	if rc == nil || rc.Fallback == nil || a.Quiet {
		return
	}
	msg := rc.Fallback.String()
	g := a.ensureGovernance()
	g.mu.Lock()
	seen := g.noted[msg]
	g.noted[msg] = true
	g.mu.Unlock()
	if seen {
		return
	}
	var w io.Writer = os.Stderr
	if cmd != nil {
		w = cmd.ErrOrStderr()
	}
	fmt.Fprintln(w, "governance: "+msg)
}

// GovernanceNote returns the transition text for a surface that carries notes in
// its result rather than printing them (the retrieval answer), and "" when the
// resolution applied every declared binding. It marks the note reported, so a
// run that both prints and carries notes says it once.
func (a *App) GovernanceNote(rc *project.ResolvedGovernance) string {
	if rc == nil || rc.Fallback == nil {
		return ""
	}
	msg := rc.Fallback.String()
	g := a.ensureGovernance()
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.noted[msg] {
		return ""
	}
	g.noted[msg] = true
	return msg
}
