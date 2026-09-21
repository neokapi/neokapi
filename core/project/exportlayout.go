package project

import "path/filepath"

// ExportLayout addresses the context files under `.kapi/`: the content-memory
// bundles, the per-profile override directories and the committed decision
// record.
//
// A project's context lives in the user's workspace. These files are what an
// export writes and what an explicit import reads, so they are artifacts on
// either side of the store rather than a place kapi resolves context from. The
// paths therefore sit on a value of their own: a read path holds a Layout, and
// a Layout cannot name them.
//
// Layout keeps the paths a project resolves for configuration and machine
// state, which is everything else under `.kapi/`.
type ExportLayout struct {
	Layout
}

// Export returns the layout's context-file addresses, for the import and
// export commands that read and write them.
func (l Layout) Export() ExportLayout {
	return ExportLayout{Layout: l}
}

// MemoryDir returns the absolute path of the content-memory bundles.
func (e ExportLayout) MemoryDir() string {
	return filepath.Join(e.StateDir, MemoryDirName)
}

// ProfilesDir returns the absolute path of the per-profile override root.
func (e ExportLayout) ProfilesDir() string {
	return filepath.Join(e.StateDir, ProfilesDirName)
}

// ProfileDir returns the absolute path of one profile's override directory.
// The name is the profile's key under `profiles:`.
func (e ExportLayout) ProfileDir(name string) string {
	return filepath.Join(e.ProfilesDir(), name)
}

// UnitStateDir returns the absolute path of the decision record.
func (e ExportLayout) UnitStateDir() string {
	return filepath.Join(e.StateDir, UnitStateDirName)
}
