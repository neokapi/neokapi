package backend

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/neokapi/neokapi/kapi-desktop/backend/sample"
)

// CreateSampleProject scaffolds a sample project and opens it as a tab.
// name must be "kapimart" or "okapimart".
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
	kapiPath := filepath.Join(targetDir, "project.kapi")

	// Idempotent: if already scaffolded and the recipe still opens, reuse it.
	// A sample scaffolded by an older app version may carry a recipe that no
	// longer parses against the current schema (e.g. legacy top-level languages
	// or list-form `plugins:`); in that case re-scaffold over it so the sample
	// opens cleanly instead of failing with a YAML unmarshal error.
	if _, err := os.Stat(kapiPath); err == nil {
		if tab, err := a.OpenProject(kapiPath); err == nil {
			return tab, nil
		}
		a.logger.Printf("sample %q recipe is stale/unparseable — re-scaffolding", name)
		// Clear the regenerable state dir first: a tm.db / termbase.db left by an
		// older app version carries an incompatible migration history, so re-seeding
		// into it fails ("apply migration N: no such table ..."). Removing .kapi lets
		// Scaffold create fresh DBs; the user's input/ and output/ are preserved.
		if err := os.RemoveAll(filepath.Join(targetDir, ".kapi")); err != nil {
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

// ResetSampleProject refreshes an out-of-date sample to the version embedded in
// this kapi: it closes the project, backs up the existing directory (so nothing
// is lost), re-scaffolds a fresh copy in place, and reopens it — returning the
// new tab. Only valid for projects scaffolded from a sample.
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

	// Close first so file watchers, the block store, and TM/termbase handles
	// release the directory before we move it.
	a.CloseProject(tabID)

	backup := backupSampleDir(dir, m.Revision)
	if err := os.Rename(dir, backup); err != nil {
		return nil, fmt.Errorf("back up sample: %w", err)
	}
	a.logger.Printf("sample %q reset: backed up to %s", m.Sample, backup)

	if err := sample.Scaffold(m.Sample, dir); err != nil {
		return nil, fmt.Errorf("re-scaffold sample: %w", err)
	}
	return a.OpenProject(kapiPath)
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
