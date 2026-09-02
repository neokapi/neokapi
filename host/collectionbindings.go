package host

import (
	"path/filepath"
	"strings"
	"sync"

	"github.com/neokapi/neokapi/core/project"
)

// bindingGroup is one partition of a run's input set: the files governed
// identically, and so sharing one voice and one terms store.
//
// A project flow resolves its governance once and bakes it into the tool chain
// before any content is seen (buildFlowTools → applyBindings), so a run cannot
// switch voice per file. Splitting the input set instead is what makes the
// coordinates real: one run per (binding group, locale pass), each with its own
// bindings and its own tools. A recipe whose collections are all governed the
// same way produces exactly one group, so the common case keeps the single run,
// the single tool chain, and the same event stream.
type bindingGroup struct {
	// Point is the place in the context space this group's files sit at, as one
	// representative of them: every file in the group resolves to the same
	// governance, so re-resolving through any of them yields the group's voice
	// and terms. The zero point is the project's default one — either because
	// nothing claimed these files, or because what did resolves to exactly the
	// defaults.
	Point project.GovernancePoint
	// Inputs are the group's source files, in the order they were given.
	Inputs []string
}

// groupInputsByBinding partitions inputPaths by the governance that applies to
// each path, resolved per file through project.ResolveGovernanceFor — so a
// content item's own `channel:` partitions its files out of the rest of its
// collection, and a profile outside its validity window governs nothing.
// Paths outside the project root, and paths no content pattern claims, sit at
// the project's default point.
//
// Grouping is by what the point *resolves to*, not by the reference that
// selected it: every file resolving to the same profile, voice, terms and
// channel — two collections on one profile's channel, say — stays in one group
// and shares one tool chain. When nothing declares a channel at all, the whole
// input set is one group at the default point, which resolves the project-wide
// bindings exactly as an ungrouped run always did.
//
// The error is an unresolvable channel reference; a recipe that loaded cleanly
// cannot produce one.
func (a *App) groupInputsByBinding(cmd Command, proj *project.KapiProject, projectDir string, inputPaths []string) ([]bindingGroup, error) {
	whole := []bindingGroup{{Inputs: inputPaths}}
	if proj == nil || projectDir == "" || len(inputPaths) == 0 || !hasCollectionContext(proj) {
		return whole, nil
	}

	defaultRC, err := a.ResolveGovernanceAtPoint(cmd, proj, a.GovernancePointFor("", ""))
	if err != nil {
		return nil, err
	}
	defaults := bindingKey(defaultRC)

	var groups []bindingGroup
	index := make(map[string]int, 2)
	for _, in := range inputPaths {
		point := a.GovernancePointFor("", "")
		if rel, ok := projectRelPath(projectDir, in); ok {
			point = a.GovernancePointFor("", rel)
		}
		rc, rerr := a.ResolveGovernanceAtPoint(cmd, proj, point)
		if rerr != nil {
			return nil, rerr
		}
		key := bindingKey(rc)
		if key == defaults {
			// This file is governed exactly as the project is: carry the empty
			// point, so the group resolves through the identical project-wide
			// path.
			point = a.GovernancePointFor("", "")
		}
		i, seen := index[key]
		if !seen {
			i = len(groups)
			index[key] = i
			groups = append(groups, bindingGroup{Point: point})
		}
		groups[i].Inputs = append(groups[i].Inputs, in)
	}
	return groups, nil
}

// hasCollectionContext reports whether anything in the recipe places content
// somewhere in the context space — a collection's channel, or a content item's
// own. Nothing to split on when nothing does, which is the answer that keeps an
// ordinary recipe on its single, ungrouped run.
func hasCollectionContext(proj *project.KapiProject) bool {
	for i := range proj.Collections {
		if proj.Collections[i].Channel != "" {
			return true
		}
		for _, item := range proj.Collections[i].EffectiveItems() {
			if item.Channel != "" {
				return true
			}
		}
	}
	return false
}

// bindingKey is the identity of resolved governance: two files with equal keys
// are governed identically and belong in one group.
//
// The key is what the point resolves to — the profile, the voice, the terms and
// the channel — not the reference that selected them. Channel counts because it
// selects an override inside the profile, so one voice on two channels is two
// voices in practice. The profile counts because a profile is answered by its
// own directory: two profiles binding nothing in the recipe still resolve
// different `.kapi/profiles/<name>/terms.json`. Two collections on one profile's
// channel share a profile name, so nothing splits for nothing.
func bindingKey(rc *project.ResolvedGovernance) string {
	var b strings.Builder
	b.WriteString(rc.Profile)
	b.WriteByte(0)
	b.WriteString(rc.Channel)
	b.WriteByte(0)
	if rc.Voice != nil {
		b.WriteString(rc.Voice.ProfileFile)
		b.WriteByte(0)
		b.WriteString(rc.Voice.Profile)
		b.WriteByte(0)
		b.WriteString(rc.Voice.Pack)
	}
	b.WriteByte(0)
	b.WriteString(rc.TermStore)
	return b.String()
}

// localeBindings resolves the governance in force at a point for one target
// locale, and remembers each answer for the length of a run.
//
// A run that fans out over locales resolves at (point, locale) rather than once
// for the whole run, because a term rule pairs a source term with the wording
// approved for THIS target (terms.Concept.PreferredTerm). Resolved before the
// fan-out, with no locale to ask about, a set holds the do-not-translate
// concepts alone, and every producer in the run is handed that. The staleness
// gate resolves per locale (host/verify_staleness.go), so resolving the same
// way here is also what lets the gate recompute a context fingerprint over the
// rules the producer had.
//
// The cache is keyed on the point and the locale, so a run with many groups and
// many locales pays one resolution per distinct pair. at is safe for concurrent
// use: a convergence pass fans its locales out over workers, and each worker
// asks for its own.
type localeBindings struct {
	app         *App
	cmd         Command
	proj        *project.KapiProject
	projectPath string

	mu    sync.Mutex
	cache map[string]*ProjectBindings
}

// newLocaleBindings builds the resolver a run keeps for its whole fan-out.
func (a *App) newLocaleBindings(cmd Command, proj *project.KapiProject, projectPath string) *localeBindings {
	return &localeBindings{
		app:         a,
		cmd:         cmd,
		proj:        proj,
		projectPath: projectPath,
		cache:       map[string]*ProjectBindings{},
	}
}

// at returns the bindings governing point for targetLang, resolving them on
// first ask. A nil result is an answer and is cached with the rest: the project
// carries no bindings at all.
func (l *localeBindings) at(point project.GovernancePoint, targetLang string) (*ProjectBindings, error) {
	key := strings.Join([]string{point.Profile, point.Collection, point.Path, targetLang}, "\x00")
	l.mu.Lock()
	defer l.mu.Unlock()
	if b, ok := l.cache[key]; ok {
		return b, nil
	}
	b, err := l.app.resolveBindingsFor(l.cmd, l.proj, l.projectPath, point, targetLang)
	if err != nil {
		return nil, err
	}
	l.cache[key] = b
	return b, nil
}

// projectRelPath returns path relative to the project root, slash-separated,
// and whether it is inside the root at all. Relative inputs are resolved
// against the working directory first.
func projectRelPath(root, path string) (string, bool) {
	abs := path
	if !filepath.IsAbs(abs) {
		if r, err := filepath.Abs(abs); err == nil {
			abs = r
		}
	}
	rel, err := filepath.Rel(root, abs)
	if err != nil {
		return "", false
	}
	relSlash := filepath.ToSlash(rel)
	if relSlash == ".." || strings.HasPrefix(relSlash, "../") {
		return "", false
	}
	return relSlash, true
}
