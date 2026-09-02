package backend

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/kapi-desktop/backend/sample"
)

// CreateSampleProject scaffolds a sample project and opens it as a tab.
// name must be one of sample.List() — currently "kapimart".
// If the project already exists on disk, it is opened without re-scaffolding.
func (a *App) CreateSampleProject(name string) (*TabInfo, error) {
	displayName, ok := sample.DisplayName[name]
	if !ok {
		return nil, fmt.Errorf("unknown sample project %q", name)
	}

	home, err := userHomeDir()
	if err != nil {
		return nil, fmt.Errorf("cannot determine home directory: %w", err)
	}

	targetDir := filepath.Join(home, "KapiProjects", displayName)
	kapiPath := filepath.Join(targetDir, "kapi.yaml")

	// Idempotent: if already scaffolded and the recipe still opens, reuse it.
	// A sample scaffolded by an older app version may carry a recipe that no
	// longer parses against the current schema (e.g. legacy top-level languages
	// or list-form `plugins:`); in that case re-scaffold over it so the sample
	// opens cleanly instead of failing with a YAML unmarshal error.
	if _, err := os.Stat(kapiPath); err == nil {
		if tab, err := a.OpenProject(kapiPath); err == nil {
			return tab, nil
		}
		a.logger.Printf("sample %q recipe is stale/unparseable, re-scaffolding", name)
		// Drop the store first: one left by an older app version carries an
		// incompatible migration history, so re-seeding into it fails ("apply
		// migration N: no such table ..."). Deleting it is enough and costs
		// nothing — every subsystem in there is a projection of committed sources.
		// The rest of `.kapi/` stays, so the committed unit record survives; the
		// user's input/ and output/ were never at risk.
		if err := a.resetProjectStore(targetDir); err != nil {
			return nil, fmt.Errorf("reset stale sample state: %w", err)
		}
		if err := sample.Scaffold(name, targetDir); err != nil {
			return nil, fmt.Errorf("re-scaffold stale sample project: %w", err)
		}
		return a.OpenProject(kapiPath)
	}

	if err := sample.Scaffold(name, targetDir); err != nil {
		return nil, fmt.Errorf("scaffold sample project: %w", err)
	}

	return a.OpenProject(kapiPath)
}

// SampleInfo describes whether an open project is a scaffolded sample and
// whether a newer revision of that sample ships with this kapi.
type SampleInfo struct {
	IsSample         bool   `json:"is_sample"`
	Name             string `json:"name,omitempty"`
	DisplayName      string `json:"display_name,omitempty"`
	OnDiskRevision   int    `json:"on_disk_revision"`
	CurrentRevision  int    `json:"current_revision"`
	UpgradeAvailable bool   `json:"upgrade_available"`
}

// GetSampleInfo reports the sample status of an open project by reading its
// .kapi/sample.json marker. Non-sample projects return IsSample=false.
func (a *App) GetSampleInfo(tabID string) SampleInfo {
	op := a.getOpenProject(tabID)
	if op == nil || op.Path == "" {
		return SampleInfo{}
	}
	m, ok := sample.ReadManifest(filepath.Dir(op.Path))
	if !ok {
		return SampleInfo{}
	}
	cur := sample.CurrentRevision(m.Sample)
	return SampleInfo{
		IsSample:         true,
		Name:             m.Sample,
		DisplayName:      sample.DisplayName[m.Sample],
		OnDiskRevision:   m.Revision,
		CurrentRevision:  cur,
		UpgradeAvailable: cur > m.Revision,
	}
}

// ResetSampleProject refreshes an out-of-date sample to the version embedded
// in this kapi: it quiesces the tab's handles, backs up the existing directory
// (so nothing is lost), re-scaffolds a fresh copy in place, and reloads the
// SAME tab — same tab ID, recipe re-read, content memory/terms/block-store handles and
// the file watcher reopened. Keeping the tab alive throughout means every
// surface polling it (the home hero's GetConvergence / GetConvergePlan) never
// dangles on a closed tab or a renamed path. Only valid for projects
// scaffolded from a sample.
func (a *App) ResetSampleProject(tabID string) (*TabInfo, error) {
	op := a.getOpenProject(tabID)
	if op == nil || op.Path == "" {
		return nil, fmt.Errorf("tab %q not found", tabID)
	}
	dir := filepath.Dir(op.Path)
	kapiPath := op.Path
	m, ok := sample.ReadManifest(dir)
	if !ok {
		return nil, errors.New("not a sample project")
	}

	// Quiesce first so the file watcher, the block store, and content memory/terms
	// handles release the directory before we move it — but keep the tab
	// entry, so it can be reloaded in place after the re-scaffold.
	a.releaseProjectResources(op)

	backup := backupSampleDir(dir, m.Revision)
	if err := os.Rename(dir, backup); err != nil {
		// Nothing moved — rewire the tab onto the untouched directory.
		a.autoOpenProjectResources(op)
		a.startWatcher(op)
		return nil, fmt.Errorf("back up sample: %w", err)
	}
	a.logger.Printf("sample %q reset: backed up to %s", m.Sample, backup)

	if err := sample.Scaffold(m.Sample, dir); err != nil {
		return nil, fmt.Errorf("re-scaffold sample: %w", err)
	}

	// Reload the open tab in place: re-read the recipe and reopen the
	// project-scoped resources against the fresh scaffold.
	proj, err := project.Load(kapiPath)
	if err != nil {
		return nil, fmt.Errorf("reload reset sample: %w", err)
	}
	a.mu.Lock()
	op.Project = proj
	a.mu.Unlock()
	op.missingWarned.Store(false)
	a.autoOpenProjectResources(op)
	a.startWatcher(op)
	a.emitEvent("project:extracted", map[string]any{"tabID": tabID})
	return &TabInfo{ID: op.ID, Name: projectDisplayName(proj, kapiPath), Path: kapiPath}, nil
}

// AcknowledgeSampleRevision marks the on-disk sample as up to date with the
// embedded revision without re-scaffolding ("keep current"), so the desktop
// stops offering the upgrade for this copy.
func (a *App) AcknowledgeSampleRevision(tabID string) error {
	op := a.getOpenProject(tabID)
	if op == nil || op.Path == "" {
		return fmt.Errorf("tab %q not found", tabID)
	}
	dir := filepath.Dir(op.Path)
	m, ok := sample.ReadManifest(dir)
	if !ok {
		return errors.New("not a sample project")
	}
	return sample.SetManifestRevision(dir, sample.CurrentRevision(m.Sample))
}

// resetProjectStore deletes a project's local store, first releasing the handle
// this process holds for it — a file removed under an open pool is still open,
// and the next writer would carry on into an unlinked inode.
//
// Deleting the whole store is the documented trade of merging the four files:
// content memory, terms, block cache and working set go together. It is
// affordable because every one of them is a projection rebuilt from committed
// sources — except staged decisions, which are decisions nobody has committed
// on a sample being re-scaffolded anyway.
func (a *App) resetProjectStore(root string) error {
	if err := a.hostEngine().CloseProjectDB(root); err != nil {
		return err
	}
	layout := project.LayoutAt(root)
	// The WAL and shared-memory sidecars go too: left behind beside a deleted
	// database they are stale journal for a file that no longer exists.
	for _, suffix := range []string{"", "-wal", "-shm"} {
		if err := os.Remove(layout.StorePath() + suffix); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

// backupSampleDir returns a non-existing sibling backup path for dir, e.g.
// "KapiMart (backup r1)", appending a counter if that already exists.
func backupSampleDir(dir string, rev int) string {
	base := fmt.Sprintf("%s (backup r%d)", dir, rev)
	candidate := base
	for i := 2; ; i++ {
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate
		}
		candidate = fmt.Sprintf("%s (%d)", base, i)
	}
}
