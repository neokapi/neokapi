package backend

import (
	"fmt"
)

// CollectionStatus is the JSON-serialisable summary the UI renders
// on the project status panel. Stays lean; richer per-block stats
// come from the blockstore layer when the executor migration lands.
type CollectionStatus struct {
	Name            string         `json:"name"`
	BlockCount      int            `json:"blockCount"`
	Coverage        map[string]int `json:"coverage"`
	TargetLanguages []string       `json:"targetLanguages"`
}

// ProjectStatus bundles the per-collection summaries.
type ProjectStatus struct {
	ProjectPath string             `json:"projectPath"`
	ProjectName string             `json:"projectName"`
	Collections []CollectionStatus `json:"collections"`
}

// GetProjectStatus returns the current per-collection state for a
// project tab. Under the new model, rich coverage/block-count data
// flows from a blockstore.Session against the project's cache/blocks.db —
// that's wired up in a follow-up. For now the status surface is
// intentionally sparse: recipe identity + the list of declared
// collections + their declared target languages. The frontend uses
// it to draw the shell of the status panel; cells that depend on
// blockstore data render as pending until the session integration
// lands.
func (a *App) GetProjectStatus(tabID string) (*ProjectStatus, error) {
	a.mu.RLock()
	op, ok := a.projects[tabID]
	a.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("project tab %q not found", tabID)
	}

	out := &ProjectStatus{
		ProjectPath: op.Path,
	}
	if op.Project != nil {
		out.ProjectName = op.Project.Name
		for _, coll := range op.Project.Content {
			targets := make([]string, 0, len(coll.TargetLanguages))
			for _, loc := range coll.TargetLanguages {
				targets = append(targets, string(loc))
			}
			out.Collections = append(out.Collections, CollectionStatus{
				Name:            collectionLabel(coll.Name),
				TargetLanguages: targets,
			})
		}
	}
	return out, nil
}

// ExtractResult summarises one re-extract request from the UI.
type ExtractResult struct {
	Log string `json:"log"`
}

// RunExtract is a placeholder for the desktop's "Re-extract" button.
// Under the new project model extraction is driven by the per-tool
// flow executor (`kapi run`) and by upstream extractors such as
// `kapi-react extract`. A future change re-wires this to run the
// project's declared flow against a session on the project's
// blockstore. For now we return a friendly no-op so existing frontend
// wiring still compiles.
func (a *App) RunExtract(tabID string) (*ExtractResult, error) {
	a.mu.RLock()
	_, ok := a.projects[tabID]
	a.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("project tab %q not found", tabID)
	}
	return &ExtractResult{Log: "Re-extract: run the project's extract flow from your tool chain " +
		"(e.g. `vp kapi-react extract` in the package directory).\n"}, nil
}

func collectionLabel(name string) string {
	if name == "" {
		return "(unnamed)"
	}
	return name
}
