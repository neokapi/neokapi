package backend

import (
	"fmt"

	"github.com/neokapi/neokapi/cli"
)

// CollectionStatus mirrors cli.CollectionStatus but uses plain
// strings for the Wails boundary — JSON-serialisable, no model.LocaleID
// dependency for the frontend.
type CollectionStatus struct {
	Name            string         `json:"name"`
	Archive         string         `json:"archive"`
	ArchiveExists   bool           `json:"archiveExists"`
	BlockCount      int            `json:"blockCount"`
	Coverage        map[string]int `json:"coverage"`
	TargetLanguages []string       `json:"targetLanguages"`
}

// ProjectStatus is the full per-project bundle returned to the UI.
type ProjectStatus struct {
	ProjectPath string             `json:"projectPath"`
	ProjectName string             `json:"projectName"`
	Collections []CollectionStatus `json:"collections"`
}

// GetProjectStatus reads the project at the given tab's path and
// returns the archive-backed translation state. The UI renders this
// as a coverage panel on the project view.
func (a *App) GetProjectStatus(tabID string) (*ProjectStatus, error) {
	a.mu.RLock()
	op, ok := a.projects[tabID]
	a.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("project tab %q not found", tabID)
	}

	internal, err := cli.CollectProjectStatus(op.Path)
	if err != nil {
		return nil, err
	}

	out := &ProjectStatus{
		ProjectPath: internal.ProjectPath,
		ProjectName: internal.ProjectName,
		Collections: make([]CollectionStatus, 0, len(internal.Collections)),
	}
	for _, cs := range internal.Collections {
		coverage := make(map[string]int, len(cs.Coverage))
		for loc, n := range cs.Coverage {
			coverage[string(loc)] = n
		}
		targets := make([]string, 0, len(cs.TargetLanguages))
		for _, loc := range cs.TargetLanguages {
			targets = append(targets, string(loc))
		}
		out.Collections = append(out.Collections, CollectionStatus{
			Name:            cs.Name,
			Archive:         cs.Archive,
			ArchiveExists:   cs.ArchiveExists,
			BlockCount:      cs.BlockCount,
			Coverage:        coverage,
			TargetLanguages: targets,
		})
	}
	return out, nil
}
