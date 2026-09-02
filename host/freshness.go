package host

import (
	"path/filepath"
	"strings"
	"sync"

	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/ref"
	"github.com/neokapi/neokapi/core/ref/refcache"
)

// The freshness half of the retrieval surface (AD-037; the surface's freshness
// contract).
//
// A retrieval answer is a snapshot of a graph that other processes move. An
// assistant holds one for the length of a task — sometimes an hour — and the
// wording it settles on is only as good as the context it read at the start. So
// every answer says whether the governance under it has moved since this
// process last looked.
//
// It reports; it never resolves. What a moved context means for work already
// done is a judgement, and nothing here is in a position to make it.
//
// The comparison costs one small file read, and no network call at all. That is
// deliberate: a retrieval is a read path an agent hits repeatedly inside a
// single thought, and a design that phoned the venue each time would trade a
// note nobody waits for against latency on every question asked. Refreshing
// what the cache holds belongs to the transport — a push, a pull, a `kapi up`
// — at the cadence it already runs at; this side only notices that it moved.

// governanceWatch remembers the governance identities this process last read,
// so a later read can say what moved underneath it.
//
// The baseline is per process and starts empty. A first retrieval therefore
// reports nothing — correctly: nothing has moved since a read that had not
// happened yet, and a note on the first answer would be a claim about a
// comparison never made.
type governanceWatch struct {
	mu       sync.Mutex
	baseline ref.Ref
	seen     bool
}

// observe folds a freshly-read ref into the watch and returns the governance
// components that moved since the last read.
//
// The baseline advances whether or not anything moved, so a change is reported
// once — to the answer that first spans it — rather than on every answer from
// then on. A note repeated forever is a note an agent learns to skip.
func (w *governanceWatch) observe(current ref.Ref) []ref.Component {
	w.mu.Lock()
	defer w.mu.Unlock()

	previous, seen := w.baseline, w.seen
	w.baseline, w.seen = current, true
	if !seen {
		return nil
	}
	return ref.Compare(previous, current).Moved()
}

// governanceNotes returns the staleness notes a retrieval answer carries: at
// most one, naming the governance components that moved since this process last
// read the graph.
//
// Silent for a project with no freshness cache — an unconnected project, or one
// that has never reached its venue. There is nothing to be behind.
func (a *App) governanceNotes(cmd Command) []string {
	layout, ok := a.projectLayout(cmd)
	if !ok {
		return nil
	}
	// The stream is not named: the retrieval surface reads the graph, not the
	// transport's configuration, and the cache answers for the only stream it
	// holds when there is only one.
	current := refcache.LoadObserved(layout).ObservedRef("")
	if !hasGovernance(current) {
		// A position and no identities is a project that has moved content and
		// never observed governance. Taking that as a baseline would be taking
		// silence for a fact.
		return nil
	}

	moved := a.freshnessWatch().observe(current)
	if len(moved) == 0 {
		return nil
	}

	return []string{listComponents(moved) +
		" moved since this was last read here. Re-read the project's context before continuing"}
}

// listComponents renders the moved components as a readable list. The note is
// prose an assistant acts on, and "context and terms and decisions" is the
// spelling that makes it read like a machine wrote it.
func listComponents(components []ref.Component) string {
	names := make([]string, 0, len(components))
	for _, c := range components {
		names = append(names, string(c))
	}
	switch len(names) {
	case 0:
		return ""
	case 1:
		return names[0]
	default:
		return strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1]
	}
}

// hasGovernance reports whether a ref carries any governance identity at all.
func hasGovernance(r ref.Ref) bool {
	for _, c := range ref.GovernanceComponents() {
		if r.Identity(c) != "" {
			return true
		}
	}
	return false
}

// freshnessWatch returns the process's watch, creating it on first use.
func (a *App) freshnessWatch() *governanceWatch {
	a.freshnessOnce.Do(func() { a.freshness = &governanceWatch{} })
	return a.freshness
}

// projectLayout resolves the active project's on-disk layout, and reports
// whether there is a project at all. Best-effort throughout: a retrieval answer
// must not fail because the freshness cache could not be located.
func (a *App) projectLayout(cmd Command) (project.Layout, bool) {
	path, err := ResolveProjectPath(cmd)
	if err != nil || path == "" {
		return project.Layout{}, false
	}
	return project.LayoutAt(filepath.Dir(path)), true
}
