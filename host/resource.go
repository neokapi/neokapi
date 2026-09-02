package host

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ResourceInfo describes a named resource (terms or content memory) in KAPI_HOME.
type ResourceInfo struct {
	Name     string    `json:"name"`
	Path     string    `json:"path"`
	Size     int64     `json:"size"`
	Modified time.Time `json:"modified"`
}

// AddResourceFlags adds the --name, --local, and --file flags to a command.
// These are mutually exclusive; default behavior (no flag) is the same as --local.
func AddResourceFlags(cmd Command) {
	cmd.Flags().String("name", "", "named resource in KAPI_HOME (e.g. --name project-terms)")
	cmd.Flags().Bool("local", false, "use resource in current directory")
	cmd.Flags().String("file", "", "explicit path to resource file")
}

// resolveResourcePath resolves a resource file path from the --name, --local, and
// --file flags. The kind parameter is the subdirectory name ("terms", "memory"
// or "voice") and defaultFilename is the default filename for --local mode
// (e.g. "terms.db"). The same subdirectory names are what core/flow resolves a
// `memory:` or `terms:` reference against, so the two paths agree.
//
// Resolution order:
//   - --name <n>    → ~/.config/kapi/<kind>/<n>.db
//   - --local       → ./<defaultFilename>
//   - --file <path> → <path>
//   - (no flag)     → ./<defaultFilename>  (same as --local)
//
// Parent directories are created on demand.
func resolveResourcePath(cmd Command, kind, defaultFilename string) (string, error) {
	name, _ := cmd.Flags().GetString("name")
	local, _ := cmd.Flags().GetBool("local")
	file, _ := cmd.Flags().GetString("file")

	// Check mutual exclusivity.
	flagCount := 0
	if name != "" {
		flagCount++
	}
	if local {
		flagCount++
	}
	if file != "" {
		flagCount++
	}
	if flagCount > 1 {
		return "", errors.New("--name, --local, and --file are mutually exclusive")
	}

	switch {
	case name != "":
		return resolveNamedResource(kind, name)
	case file != "":
		dir := filepath.Dir(file)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return "", fmt.Errorf("create directory %s: %w", dir, err)
		}
		return file, nil
	default:
		// --local or no flag: use current directory.
		return defaultFilename, nil
	}
}

// resolveNamedResource returns the path to a named resource in KAPI_HOME.
// Creates the parent directory on demand.
func resolveNamedResource(kind, name string) (string, error) {
	if name == "" {
		return "", errors.New("resource name is required")
	}
	if strings.ContainsAny(name, "/\\") {
		return "", fmt.Errorf("resource name must not contain path separators: %q", name)
	}

	dir := filepath.Join(ConfigDir(), kind)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("create directory %s: %w", dir, err)
	}

	return filepath.Join(dir, name+".db"), nil
}

// ListNamedResources lists all .db files in <ConfigDir>/<kind>/. ConfigDir is
// platform-dependent (os.UserConfigDir), so this is ~/.config/kapi/<kind>/ on
// Linux but ~/Library/Application Support/kapi/<kind>/ on macOS. Unlike the
// app-config file there is no legacy fallback location here — a named store is
// only ever resolved under ConfigDir.
func ListNamedResources(kind string) ([]ResourceInfo, error) {
	dir := filepath.Join(ConfigDir(), kind)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read directory %s: %w", dir, err)
	}

	var resources []ResourceInfo
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".db") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".db")
		resources = append(resources, ResourceInfo{
			Name:     name,
			Path:     filepath.Join(dir, e.Name()),
			Size:     info.Size(),
			Modified: info.ModTime(),
		})
	}
	return resources, nil
}

// ConfigDir returns the kapi config root. Named resources (terms stores,
// content memories, voice profiles), flows, format presets, and the plugin dir
// all hang off it. It honors the KAPI_CONFIG_DIR env override (the isolation
// contract), else resolves to <os.UserConfigDir()>/kapi (~/.config/kapi on Linux,
// ~/Library/Application Support/kapi on macOS). Shared by every surface that
// derives kapi paths (the CLI, the Kapi Desktop backend), so the chain is
// defined once.
func ConfigDir() string {
	if dir := os.Getenv("KAPI_CONFIG_DIR"); dir != "" {
		return dir
	}
	configDir, err := os.UserConfigDir()
	if err != nil {
		configDir = filepath.Join(os.Getenv("HOME"), ".config")
	}
	return filepath.Join(configDir, "kapi")
}
