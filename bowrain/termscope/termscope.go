// Package termscope limits the concepts of a workspace's terms store to the
// projects they apply in.
//
// A workspace keeps one terms store, and its projects share it. A concept
// applies in every project of the workspace unless it is limited to some of
// them. The limit is the concept property PropProjects, which lists project
// ids joined with commas. A concept without the property applies in the whole
// workspace, so a concept written before the property existed keeps applying
// everywhere.
//
// The property sits beside terms.Concept.ProjectID, which the SQLite store
// keeps as a column and the PostgreSQL store has no column for. Projects
// reads both, so a concept means the same thing whichever store holds it.
//
// Every surface that applies terms to one project reads the store through
// View or StoreView, so the checks, the translate prompts, the context deck
// and the review plan agree on which concepts govern the project.
package termscope

import (
	"maps"
	"slices"
	"strings"

	"github.com/neokapi/neokapi/terms"
)

// PropProjects is the concept property naming the projects a concept is
// limited to: project ids joined with commas. A concept without it applies
// in the whole workspace.
const PropProjects = "project_id"

// Projects are the ids of the projects a concept is limited to, sorted, nil
// for a concept of the whole workspace.
func Projects(cp terms.Concept) []string {
	var out []string
	if p := strings.TrimSpace(cp.ProjectID); p != "" {
		out = append(out, p)
	}
	for p := range strings.SplitSeq(cp.Properties[PropProjects], ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}

// Workspace reports whether a concept applies in the whole workspace.
func Workspace(cp terms.Concept) bool { return len(Projects(cp)) == 0 }

// AppliesIn reports whether a concept applies in a project: it applies in
// the whole workspace, or it is limited to projects that include this one.
// An empty project id names no project to narrow to, and every concept
// applies.
func AppliesIn(cp terms.Concept, projectID string) bool {
	ps := Projects(cp)
	return projectID == "" || len(ps) == 0 || slices.Contains(ps, projectID)
}

// Limit limits a concept to projects, replacing the limit it carried. No
// project ids widen it to the whole workspace. A concept limited to one
// project names it in terms.Concept.ProjectID too, which a SQLite store
// keeps as a column.
func Limit(cp *terms.Concept, projectIDs ...string) {
	ids := Projects(terms.Concept{Properties: map[string]string{PropProjects: strings.Join(projectIDs, ",")}})
	cp.ProjectID = ""
	if len(ids) == 1 {
		cp.ProjectID = ids[0]
	}
	// The map may be shared with the concept the caller read, so it is
	// copied before it changes.
	cp.Properties = maps.Clone(cp.Properties)
	if len(ids) == 0 {
		delete(cp.Properties, PropProjects)
		return
	}
	if cp.Properties == nil {
		cp.Properties = map[string]string{}
	}
	cp.Properties[PropProjects] = strings.Join(ids, ",")
}

// Widen makes a concept apply in the whole workspace, and reports whether it
// was limited to projects before.
func Widen(cp *terms.Concept) bool {
	if Workspace(*cp) {
		return false
	}
	Limit(cp)
	return true
}

// Filter keeps the concepts that apply in a project.
func Filter(concepts []terms.Concept, projectID string) []terms.Concept {
	if projectID == "" {
		return concepts
	}
	out := make([]terms.Concept, 0, len(concepts))
	for _, cp := range concepts {
		if AppliesIn(cp, projectID) {
			out = append(out, cp)
		}
	}
	return out
}
