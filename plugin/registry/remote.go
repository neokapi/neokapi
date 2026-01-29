package registry

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
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

// FindPlugin searches for a specific plugin by name in the registry.
func (r *RemoteRegistry) FindPlugin(name string) (*PluginManifest, error) {
	index, err := r.FetchIndex()
	if err != nil {
		return nil, err
	}

	platform := runtime.GOOS + "/" + runtime.GOARCH
	for _, m := range index.Plugins {
		if m.Name == name && (m.Platform == platform || m.Platform == "") {
			return &m, nil
		}
	}
	return nil, fmt.Errorf("plugin %q not found for platform %s", name, platform)
}

// Download downloads a plugin binary from the given manifest, verifies its
// checksum, and saves it to the download directory. Returns the path to the
// downloaded binary.
func (r *RemoteRegistry) Download(manifest *PluginManifest) (string, error) {
	if err := os.MkdirAll(r.DownloadDir, 0o755); err != nil {
		return "", fmt.Errorf("creating download directory: %w", err)
	}

	// Build the destination path.
	filename := "gokapi-plugin-" + manifest.Name
	if runtime.GOOS == "windows" {
		filename += ".exe"
	}
	destPath := filepath.Join(r.DownloadDir, filename)

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
	manifest, err := r.FindPlugin(name)
	if err != nil {
		return "", err
	}
	return r.Download(manifest)
}
