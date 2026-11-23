package host

import (
	"path/filepath"

	"github.com/neokapi/neokapi/core/dataroot"
)

// The data root is resolved in core/dataroot, below the host, so the project
// store and the plugins reach the same directory. These names keep the host's
// callers on it.

// EnvDataDir names the environment variable that overrides the data root
// outright (dataroot.EnvDir).
const EnvDataDir = dataroot.EnvDir

// DataDir returns kapi's per-user data root (dataroot.Dir).
func DataDir() string { return dataroot.Dir() }

// RemoveTestDataDir removes the data root this test binary was given, once its
// tests have run (dataroot.RemoveTestDir).
func RemoveTestDataDir() { dataroot.RemoveTestDir() }

// underTest reports whether this process is a test binary.
func underTest() bool { return dataroot.UnderTest() }

// NormalizeCheckoutPath renders a file or directory in the one spelling two
// paths to the same place share (dataroot.NormalizePath).
func NormalizeCheckoutPath(p string) string { return dataroot.NormalizePath(p) }

// WorkspacesDirName holds the workspaces under the data root, one directory
// each, and DefaultWorkspaceName is the one every machine account has without
// asking for it.
//
// A workspace holds the context of every project worked on from this account:
// the project registry, the context graph, and one context store per project
// (core/workspace). It is created on first use, and a recipe carries no binding
// to it, so moving a checkout or cloning it again reaches the same context.
const (
	WorkspacesDirName    = "workspaces"
	DefaultWorkspaceName = "default"
)

// DefaultWorkspaceDir returns this machine account's implicit workspace.
func DefaultWorkspaceDir() string {
	return filepath.Join(DataDir(), WorkspacesDirName, DefaultWorkspaceName)
}
