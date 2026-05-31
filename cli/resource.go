package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// ResourceInfo describes a named resource (termbase or TM) in KAPI_HOME.
type ResourceInfo struct {
	Name     string    `json:"name"`
	Path     string    `json:"path"`
	Size     int64     `json:"size"`
	Modified time.Time `json:"modified"`
}

// AddResourceFlags adds the --name, --local, and --file flags to a command.
// These are mutually exclusive; default behavior (no flag) is the same as --local.
func AddResourceFlags(cmd *cobra.Command) {
	cmd.Flags().String("name", "", "named resource in KAPI_HOME (e.g. --name project-terms)")
	cmd.Flags().Bool("local", false, "use resource in current directory")
	cmd.Flags().String("file", "", "explicit path to resource file")
}

// ResolveResourcePath resolves a resource file path from the --name, --local, and
// --file flags. The kind parameter is the subdirectory name ("termbases" or "tm")
// and defaultFilename is the default filename for --local mode (e.g. "termbase.db").
//
// Resolution order:
//   - --name <n>    → ~/.config/kapi/<kind>/<n>.db
//   - --local       → ./<defaultFilename>
//   - --file <path> → <path>
//   - (no flag)     → ./<defaultFilename>  (same as --local)
//
// Parent directories are created on demand.
func ResolveResourcePath(cmd *cobra.Command, kind, defaultFilename string) (string, error) {
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

	kapiHome, err := kapiHomeDir()
	if err != nil {
		return "", err
	}

	dir := filepath.Join(kapiHome, kind)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("create directory %s: %w", dir, err)
	}

	return filepath.Join(dir, name+".db"), nil
}

// ListNamedResources lists all .db files in ~/.config/kapi/<kind>/.
func ListNamedResources(kind string) ([]ResourceInfo, error) {
	kapiHome, err := kapiHomeDir()
	if err != nil {
		return nil, err
	}

	dir := filepath.Join(kapiHome, kind)
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

// kapiHomeDir returns the KAPI_HOME directory (~/.config/kapi/).
// Uses KAPI_CONFIG_DIR env var if set, otherwise os.UserConfigDir()/kapi.
func kapiHomeDir() (string, error) {
	if dir := os.Getenv("KAPI_CONFIG_DIR"); dir != "" {
		return dir, nil
	}
	configDir, err := os.UserConfigDir()
	if err != nil {
		configDir = filepath.Join(os.Getenv("HOME"), ".config")
	}
	return filepath.Join(configDir, "kapi"), nil
}
