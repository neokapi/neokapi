package backend

import (
	"bytes"
	"context"
	"fmt"
	"time"

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

// ExtractResult summarises one invocation of RunExtract for the UI.
// The log is the captured stdout/stderr of `kapi extract` so the
// user sees which plugins ran and how many blocks each produced.
type ExtractResult struct {
	Log string `json:"log"`
}

// RunExtract runs `kapi extract -p <project>` for the given tab and
// returns the captured progress output. The UI's Re-extract button
// fires this, then refreshes the status panel.
//
// Tool time is bounded at 5 minutes per extractor subprocess — the
// default on `kapi extract --timeout`, enough for reasonable
// projects, prevents a runaway from hanging the desktop app.
func (a *App) RunExtract(tabID string) (*ExtractResult, error) {
	a.mu.RLock()
	op, ok := a.projects[tabID]
	a.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("project tab %q not found", tabID)
	}

	var buf bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// runExtract lives in the cli package but isn't exported
	// verbatim; piggyback on the thin wrapper that matches
	// `kapi extract`'s UX. We call it indirectly by invoking the
	// binary via os/exec would require PATH assumptions we can't
	// make inside a desktop app. Instead, reuse the in-process
	// planner + runner from the cli package so everything lives
	// inside one Go binary.
	if err := cli.RunExtractInProcess(ctx, &buf, op.Path); err != nil {
		return &ExtractResult{Log: buf.String()}, err
	}
	return &ExtractResult{Log: buf.String()}, nil
}
