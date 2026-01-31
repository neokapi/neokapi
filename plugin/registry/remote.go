package registry

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// RemoteRegistry discovers and downloads plugins from an HTTP-based registry.
type RemoteRegistry struct {
	// BaseURL is the registry endpoint that serves a RegistryIndex JSON.
	BaseURL string

	// DownloadDir is the local directory where downloaded plugins are stored.
	DownloadDir string

	// HTTPClient is the HTTP client used for requests. If nil, http.DefaultClient is used.
	HTTPClient *http.Client
}

// NewRemoteRegistry creates a new RemoteRegistry.
func NewRemoteRegistry(baseURL, downloadDir string) *RemoteRegistry {
	return &RemoteRegistry{
		BaseURL:     baseURL,
		DownloadDir: downloadDir,
	}
}

// httpClient returns the configured or default HTTP client.
func (r *RemoteRegistry) httpClient() *http.Client {
	if r.HTTPClient != nil {
		return r.HTTPClient
	}
	return http.DefaultClient
}

// FetchIndex retrieves the plugin registry index from the remote server.
func (r *RemoteRegistry) FetchIndex() (*RegistryIndex, error) {
	resp, err := r.httpClient().Get(r.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("fetching registry index: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("registry returned status %d", resp.StatusCode)
	}

	var index RegistryIndex
	if err := json.NewDecoder(resp.Body).Decode(&index); err != nil {
		return nil, fmt.Errorf("decoding registry index: %w", err)
	}
	return &index, nil
}

// ListAvailable returns manifests matching the current platform.
func (r *RemoteRegistry) ListAvailable() ([]PluginManifest, error) {
	index, err := r.FetchIndex()
	if err != nil {
		return nil, err
	}

	platform := runtime.GOOS + "/" + runtime.GOARCH
	var matching []PluginManifest
	for _, m := range index.Plugins {
		if m.Platform == platform || m.Platform == "" {
			matching = append(matching, m)
		}
	}
	return matching, nil
}

// FindPlugin searches for a plugin by ref in the registry. If the ref includes
// a version, it finds that exact version; otherwise it finds the latest.
func (r *RemoteRegistry) FindPlugin(ref PluginRef) (*PluginManifest, error) {
	index, err := r.FetchIndex()
	if err != nil {
		return nil, err
	}

	platform := runtime.GOOS + "/" + runtime.GOARCH
	if ref.IsVersioned() {
		return index.FindExactVersion(ref.Name, ref.Version, platform)
	}
	return index.FindLatest(ref.Name, platform)
}

// Download downloads a plugin binary from the given manifest, verifies its
// checksum, and saves it to the versioned download directory. Returns the
// path to the downloaded binary.
func (r *RemoteRegistry) Download(manifest *PluginManifest) (string, error) {
	destDir := VersionedPluginDir(r.DownloadDir, manifest.Name, manifest.Version)
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return "", fmt.Errorf("creating download directory: %w", err)
	}

	// Build the destination path.
	filename := "gokapi-plugin-" + manifest.Name
	if runtime.GOOS == "windows" {
		filename += ".exe"
	}
	destPath := filepath.Join(destDir, filename)

	// Download the binary.
	resp, err := r.httpClient().Get(manifest.DownloadURL)
	if err != nil {
		return "", fmt.Errorf("downloading plugin %s: %w", manifest.Name, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download returned status %d for %s", resp.StatusCode, manifest.Name)
	}

	// Read the entire body for checksum verification.
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading plugin binary: %w", err)
	}

	// Verify checksum if provided.
	if manifest.Checksum != "" {
		hash := sha256.Sum256(data)
		actual := hex.EncodeToString(hash[:])
		if actual != manifest.Checksum {
			return "", fmt.Errorf("checksum mismatch for %s: expected %s, got %s",
				manifest.Name, manifest.Checksum, actual)
		}
	}

	// Write to disk with executable permissions.
	if err := os.WriteFile(destPath, data, 0o755); err != nil {
		return "", fmt.Errorf("writing plugin binary: %w", err)
	}

	return destPath, nil
}

// Install downloads and installs a plugin by name from the registry.
// Returns the local path to the installed plugin binary.
func (r *RemoteRegistry) Install(name string) (string, error) {
	manifest, err := r.FindPlugin(PluginRef{Name: name})
	if err != nil {
		return "", err
	}
	return r.Download(manifest)
}

// InstallResult describes the outcome of a plugin installation.
type InstallResult struct {
	Name        string   `json:"name"`
	Version     string   `json:"version"`
	InstallType string   `json:"install_type"`
	Files       []string `json:"files"`
}

// InstallPlugin downloads and installs a plugin by ref. For bridge plugins,
// it extracts a .tar.gz archive into the versioned plugin directory. For binary
// plugins, it downloads the executable directly. A version file is written to
// track the installation.
func (r *RemoteRegistry) InstallPlugin(ref PluginRef) (*InstallResult, error) {
	manifest, err := r.FindPlugin(ref)
	if err != nil {
		return nil, err
	}

	installType := manifest.InstallType
	if installType == "" {
		installType = "binary"
	}

	destDir := VersionedPluginDir(r.DownloadDir, manifest.Name, manifest.Version)
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating plugin directory: %w", err)
	}

	var files []string

	switch installType {
	case "bridge":
		files, err = r.installBridge(manifest)
	default:
		var path string
		path, err = r.Download(manifest)
		if err == nil {
			files = []string{path}
		}
	}
	if err != nil {
		return nil, err
	}

	// Write version tracking file.
	vf := &VersionFile{
		Name:        manifest.Name,
		Version:     manifest.Version,
		InstallType: installType,
		InstalledAt: time.Now().UTC().Format(time.RFC3339),
		Checksum:    manifest.Checksum,
	}
	if err := WriteVersionFile(r.DownloadDir, manifest.Name, manifest.Version, vf); err != nil {
		return nil, fmt.Errorf("writing version file: %w", err)
	}

	return &InstallResult{
		Name:        manifest.Name,
		Version:     manifest.Version,
		InstallType: installType,
		Files:       files,
	}, nil
}

// installBridge downloads a .tar.gz archive and extracts its contents into
// the versioned plugin directory.
func (r *RemoteRegistry) installBridge(manifest *PluginManifest) ([]string, error) {
	resp, err := r.httpClient().Get(manifest.DownloadURL)
	if err != nil {
		return nil, fmt.Errorf("downloading bridge %s: %w", manifest.Name, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download returned status %d for %s", resp.StatusCode, manifest.Name)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading bridge archive: %w", err)
	}

	// Verify checksum if provided.
	if manifest.Checksum != "" {
		hash := sha256.Sum256(data)
		actual := hex.EncodeToString(hash[:])
		if actual != manifest.Checksum {
			return nil, fmt.Errorf("checksum mismatch for %s: expected %s, got %s",
				manifest.Name, manifest.Checksum, actual)
		}
	}

	destDir := VersionedPluginDir(r.DownloadDir, manifest.Name, manifest.Version)
	return extractTarGz(bytes.NewReader(data), destDir)
}

// extractTarGz extracts a tar.gz archive into destDir, writing files flat
// (ignoring directory paths in the archive). Returns the list of extracted
// file paths.
func extractTarGz(r io.Reader, destDir string) ([]string, error) {
	gr, err := gzip.NewReader(r)
	if err != nil {
		return nil, fmt.Errorf("opening gzip: %w", err)
	}
	defer gr.Close()

	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating extraction directory: %w", err)
	}

	tr := tar.NewReader(gr)
	var files []string

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("reading tar: %w", err)
		}

		// Skip directories — we extract flat.
		if hdr.Typeflag == tar.TypeDir {
			continue
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}

		// Use only the base name to extract flat into destDir.
		name := filepath.Base(hdr.Name)
		if name == "." || name == ".." {
			continue
		}

		destPath := filepath.Join(destDir, name)
		data, err := io.ReadAll(tr)
		if err != nil {
			return nil, fmt.Errorf("reading tar entry %s: %w", name, err)
		}

		perm := os.FileMode(0o644)
		if hdr.Mode&0o111 != 0 {
			perm = 0o755
		}
		if err := os.WriteFile(destPath, data, perm); err != nil {
			return nil, fmt.Errorf("writing %s: %w", name, err)
		}
		files = append(files, destPath)
	}

	return files, nil
}

// ListInstalled returns version information for all installed plugins,
// flattened from the versioned directory structure.
func (r *RemoteRegistry) ListInstalled() ([]InstalledVersion, error) {
	all, err := ListAllInstalled(r.DownloadDir)
	if err != nil {
		return nil, err
	}
	var result []InstalledVersion
	for _, versions := range all {
		result = append(result, versions...)
	}
	return result, nil
}

// PluginUpdate describes an available update for an installed plugin.
type PluginUpdate struct {
	Name             string `json:"name"`
	InstalledVersion string `json:"installed_version"`
	AvailableVersion string `json:"available_version"`
}

// CheckUpdates compares installed plugins against the registry and returns
// any that have newer versions available.
func (r *RemoteRegistry) CheckUpdates() ([]PluginUpdate, error) {
	all, err := ListAllInstalled(r.DownloadDir)
	if err != nil {
		return nil, err
	}
	if len(all) == 0 {
		return nil, nil
	}

	index, err := r.FetchIndex()
	if err != nil {
		return nil, err
	}

	platform := runtime.GOOS + "/" + runtime.GOARCH
	var updates []PluginUpdate

	for name, versions := range all {
		// Find latest installed version.
		latestInstalled := versions[0].Version
		for _, v := range versions[1:] {
			if CompareSemver(v.Version, latestInstalled) > 0 {
				latestInstalled = v.Version
			}
		}

		// Find latest available version.
		latestManifest, err := index.FindLatest(name, platform)
		if err != nil {
			continue
		}

		if CompareSemver(latestManifest.Version, latestInstalled) > 0 {
			updates = append(updates, PluginUpdate{
				Name:             name,
				InstalledVersion: latestInstalled,
				AvailableVersion: latestManifest.Version,
			})
		}
	}
	return updates, nil
}

// SearchPlugins searches the registry for plugins whose name or description
// contains the given query string (case-insensitive).
func (r *RemoteRegistry) SearchPlugins(query string) ([]PluginManifest, error) {
	available, err := r.ListAvailable()
	if err != nil {
		return nil, err
	}

	q := strings.ToLower(query)
	var results []PluginManifest
	for _, m := range available {
		if strings.Contains(strings.ToLower(m.Name), q) ||
			strings.Contains(strings.ToLower(m.Description), q) {
			results = append(results, m)
		}
	}
	return results, nil
}

// SearchOptions configures advanced plugin search with AND-logic filtering.
type SearchOptions struct {
	// Query is an optional text search against name, description, and capability names.
	Query string

	// Type filters by capability type (e.g., "format", "tool").
	Type string

	// MimeType filters by MIME type across all capabilities.
	MimeType string

	// Extension filters by file extension across all capabilities (e.g., ".docx").
	Extension string
}

// SearchPluginsAdvanced searches the registry using structured filters.
// All non-empty fields are combined with AND logic. Plugins without capabilities
// fall back to matching via PluginType when filtering by Type.
func (r *RemoteRegistry) SearchPluginsAdvanced(opts SearchOptions) ([]PluginManifest, error) {
	available, err := r.ListAvailable()
	if err != nil {
		return nil, err
	}

	var results []PluginManifest
	for _, m := range available {
		if !matchesSearchOptions(m, opts) {
			continue
		}
		results = append(results, m)
	}
	return results, nil
}

// matchesSearchOptions checks whether a manifest matches all non-empty search criteria.
func matchesSearchOptions(m PluginManifest, opts SearchOptions) bool {
	if opts.Query != "" {
		q := strings.ToLower(opts.Query)
		if !matchesTextQuery(m, q) {
			return false
		}
	}

	if opts.Type != "" {
		if !matchesType(m, strings.ToLower(opts.Type)) {
			return false
		}
	}

	if opts.MimeType != "" {
		if !m.HasMimeType(opts.MimeType) {
			return false
		}
	}

	if opts.Extension != "" {
		if !matchesExtension(m, strings.ToLower(opts.Extension)) {
			return false
		}
	}

	return true
}

// matchesTextQuery checks name, description, and capability names/display names.
func matchesTextQuery(m PluginManifest, q string) bool {
	if strings.Contains(strings.ToLower(m.Name), q) ||
		strings.Contains(strings.ToLower(m.Description), q) {
		return true
	}
	for _, cap := range m.Capabilities {
		if strings.Contains(strings.ToLower(cap.Name), q) ||
			strings.Contains(strings.ToLower(cap.DisplayName), q) {
			return true
		}
	}
	return false
}

// matchesType checks capability types, with fallback to PluginType for legacy manifests.
func matchesType(m PluginManifest, capType string) bool {
	if m.HasCapabilityType(capType) {
		return true
	}
	// Legacy fallback: match PluginType if no capabilities are declared.
	if len(m.Capabilities) == 0 && strings.Contains(strings.ToLower(m.PluginType), capType) {
		return true
	}
	return false
}

// matchesExtension checks whether any capability handles the given extension.
func matchesExtension(m PluginManifest, ext string) bool {
	for _, cap := range m.Capabilities {
		for _, ce := range cap.Extensions {
			if strings.ToLower(ce) == ext {
				return true
			}
		}
	}
	return false
}

// RemovePlugin removes an installed plugin. If version is empty, all versions
// are removed. If version is specified, only that version is removed.
func (r *RemoteRegistry) RemovePlugin(ref PluginRef) error {
	if ref.IsVersioned() {
		dir := VersionedPluginDir(r.DownloadDir, ref.Name, ref.Version)
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			return fmt.Errorf("plugin %s is not installed", ref)
		}
		return os.RemoveAll(dir)
	}
	// Remove all versions.
	dir := filepath.Join(r.DownloadDir, ref.Name)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return fmt.Errorf("plugin %q is not installed", ref.Name)
	}
	return os.RemoveAll(dir)
}
