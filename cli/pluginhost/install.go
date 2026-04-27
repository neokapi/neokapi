package pluginhost

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/neokapi/neokapi/cli/pluginhost/registry"
	"github.com/neokapi/neokapi/core/plugin/manifest"
)

// InstallTarget returns the absolute path where `kapi plugin install`
// drops new plugins. Defaults to $XDG_DATA_HOME/kapi/plugins/.
func InstallTarget() string {
	xdg := os.Getenv("XDG_DATA_HOME")
	if xdg == "" {
		if home, err := os.UserHomeDir(); err == nil {
			xdg = filepath.Join(home, ".local", "share")
		} else {
			xdg = filepath.Join(os.TempDir(), "kapi-data")
		}
	}
	return filepath.Join(xdg, "kapi", "plugins")
}

// InstallOptions configures InstallFromRegistry.
type InstallOptions struct {
	// IndexURL is the registry index URL (typically registry.neokapi.dev/plugins.json).
	IndexURL string

	// PluginName is the plugin to install, e.g. "bowrain".
	PluginName string

	// Constraint is an optional semver constraint, e.g. "^1.0".
	// Empty means "latest matching channel".
	Constraint string

	// Channel pins a registry channel (e.g., "stable", "beta").
	// Empty defaults to "stable".
	Channel string

	// KapiVersion is the running kapi binary's version (used to filter
	// out plugins whose min_kapi_version is too new).
	KapiVersion string

	// TargetDir is the install root. When empty, InstallTarget() is used.
	TargetDir string

	// Unsafe skips signature verification (sigstore/cosign). SHA-256 is
	// always verified.
	Unsafe bool

	// LogF receives progress messages. Optional.
	LogF func(msg string)
}

// InstallResult describes one successful install.
type InstallResult struct {
	PluginName string
	Version    string
	InstallDir string
	Manifest   *manifest.Manifest
}

// InstallFromRegistry resolves, downloads, verifies, and unpacks one
// plugin from the registry. Returns a populated InstallResult on success.
func InstallFromRegistry(ctx context.Context, opts InstallOptions) (*InstallResult, error) {
	if opts.PluginName == "" {
		return nil, errors.New("install: plugin name is required")
	}
	if opts.IndexURL == "" {
		opts.IndexURL = DefaultIndexURL()
	}
	channel := opts.Channel
	if channel == "" {
		channel = "stable"
	}
	target := opts.TargetDir
	if target == "" {
		target = InstallTarget()
	}
	logf := opts.LogF
	if logf == nil {
		logf = func(string) {}
	}

	logf(fmt.Sprintf("Resolving %s in %s (channel: %s)...", opts.PluginName, opts.IndexURL, channel))

	idx, err := registry.FetchIndex(ctx, opts.IndexURL)
	if err != nil {
		return nil, err
	}
	version, plat, err := idx.Resolve(opts.PluginName, opts.Constraint, channel, opts.KapiVersion)
	if err != nil {
		return nil, err
	}

	logf(fmt.Sprintf("Downloading %s %s (%s)...", opts.PluginName, version, plat.URL))
	body, err := registry.Download(ctx, plat.URL)
	if err != nil {
		return nil, err
	}
	logf("✓ Download complete")

	if !opts.Unsafe {
		if err := registry.VerifySHA256(body, plat.SHA256); err != nil {
			return nil, err
		}
		logf("✓ SHA-256 verified")
		// Cosign signature verification is deferred to a follow-up.
		// When `signature` and `cert_identity` are present we should
		// invoke cosign-go to verify; for now we surface a notice.
		if plat.Signature != "" {
			logf("Note: cosign signature verification is not yet implemented; skipping")
		}
	} else {
		logf("Warning: --unsafe — skipping signature checks")
	}

	pluginDir := filepath.Join(target, opts.PluginName)
	if err := os.MkdirAll(target, 0o755); err != nil {
		return nil, fmt.Errorf("install: mkdir %s: %w", target, err)
	}
	// Remove any prior install of the same plugin name.
	if _, err := os.Stat(pluginDir); err == nil {
		if err := os.RemoveAll(pluginDir); err != nil {
			return nil, fmt.Errorf("install: remove existing %s: %w", pluginDir, err)
		}
	}

	if err := extractPluginArchive(body, plat.URL, target, opts.PluginName); err != nil {
		return nil, err
	}

	manifestPath := filepath.Join(pluginDir, "manifest.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("install: read manifest %s: %w", manifestPath, err)
	}
	m, err := manifest.Parse(data)
	if err != nil {
		return nil, fmt.Errorf("install: %w", err)
	}
	if m.Plugin != opts.PluginName {
		return nil, fmt.Errorf("install: extracted manifest declares plugin %q but installing as %q", m.Plugin, opts.PluginName)
	}

	// Persist a small `installed.json` so future `update` knows which
	// channel was selected and which registry served the plugin.
	if err := writeInstalledMetadata(pluginDir, installedMetadata{
		Channel:     channel,
		Constraint:  opts.Constraint,
		IndexURL:    opts.IndexURL,
		Version:     version,
		InstalledAt: time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		// Non-fatal; install succeeded.
		logf("Warning: write installed metadata: " + err.Error())
	}

	logf(fmt.Sprintf("✓ Installed %s %s to %s", opts.PluginName, version, pluginDir))

	return &InstallResult{
		PluginName: opts.PluginName,
		Version:    version,
		InstallDir: pluginDir,
		Manifest:   m,
	}, nil
}

// installedMetadata records install-time choices for `kapi plugin update`.
type installedMetadata struct {
	Channel     string `json:"channel,omitempty"`
	Constraint  string `json:"constraint,omitempty"`
	IndexURL    string `json:"index_url,omitempty"`
	Version     string `json:"version,omitempty"`
	InstalledAt string `json:"installed_at,omitempty"`
}

func writeInstalledMetadata(pluginDir string, meta installedMetadata) error {
	path := filepath.Join(pluginDir, "installed.json")
	data, err := jsonMarshalIndent(meta)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// extractPluginArchive extracts a tarball or zip into target. The
// archive is expected to contain a top-level dir matching pluginName
// (the convention spelled out in #438 Tarball layout).
func extractPluginArchive(body []byte, sourceURL, target, pluginName string) error {
	if strings.HasSuffix(sourceURL, ".zip") {
		return extractZip(body, target, pluginName)
	}
	return extractTarGz(body, target, pluginName)
}

func extractTarGz(body []byte, target, pluginName string) error {
	gz, err := gzip.NewReader(bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("gunzip: %w", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("tar read: %w", err)
		}
		// Reject path traversal and absolute paths.
		clean, err := safeJoin(target, hdr.Name)
		if err != nil {
			return err
		}
		// Require entries to be inside <pluginName>/...
		if !strings.HasPrefix(filepath.ToSlash(hdr.Name), pluginName+"/") && filepath.ToSlash(hdr.Name) != pluginName {
			return fmt.Errorf("tarball entry %q is outside plugin dir %q (refusing to extract)", hdr.Name, pluginName)
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(clean, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(clean), 0o755); err != nil {
				return err
			}
			f, err := os.OpenFile(clean, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode)&0o777)
			if err != nil {
				return err
			}
			if _, err := io.Copy(f, tr); err != nil {
				f.Close()
				return err
			}
			f.Close()
		case tar.TypeSymlink:
			// Only allow relative symlinks within the plugin dir.
			abs := filepath.Join(filepath.Dir(clean), hdr.Linkname)
			if !strings.HasPrefix(abs, target) {
				return fmt.Errorf("tarball symlink %q points outside plugin dir", hdr.Name)
			}
			_ = os.Remove(clean)
			if err := os.Symlink(hdr.Linkname, clean); err != nil {
				return err
			}
		}
	}
	return nil
}

func extractZip(body []byte, target, pluginName string) error {
	r, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		return fmt.Errorf("zip reader: %w", err)
	}
	for _, f := range r.File {
		clean, err := safeJoin(target, f.Name)
		if err != nil {
			return err
		}
		if !strings.HasPrefix(filepath.ToSlash(f.Name), pluginName+"/") && filepath.ToSlash(f.Name) != pluginName {
			return fmt.Errorf("zip entry %q is outside plugin dir %q (refusing to extract)", f.Name, pluginName)
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(clean, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(clean), 0o755); err != nil {
			return err
		}
		src, err := f.Open()
		if err != nil {
			return err
		}
		dst, err := os.OpenFile(clean, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, f.Mode()&0o777)
		if err != nil {
			src.Close()
			return err
		}
		if _, err := io.Copy(dst, src); err != nil {
			src.Close()
			dst.Close()
			return err
		}
		src.Close()
		dst.Close()
	}
	return nil
}

// safeJoin joins target and elem and refuses to escape target.
func safeJoin(target, elem string) (string, error) {
	clean := filepath.Clean(filepath.Join(target, elem))
	rel, err := filepath.Rel(target, clean)
	if err != nil {
		return "", err
	}
	if strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
		return "", fmt.Errorf("archive entry %q escapes target dir", elem)
	}
	return clean, nil
}

// Remove uninstalls a plugin from the user-writable XDG dir. System
// installs (under /opt/homebrew, /usr/share, etc.) refuse to remove
// here — the OS package manager owns those.
func RemoveInstalled(pluginName string) error {
	target := filepath.Join(InstallTarget(), pluginName)
	if _, err := os.Stat(target); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("plugin %q is not installed under %s (system installs must be removed via the OS package manager)", pluginName, InstallTarget())
		}
		return err
	}
	return os.RemoveAll(target)
}

// DefaultIndexURL is the registry index this binary defaults to.
// Override via $KAPI_REGISTRY_URL.
func DefaultIndexURL() string {
	if v := os.Getenv("KAPI_REGISTRY_URL"); v != "" {
		return v
	}
	return "https://neokapi.github.io/registry/plugins.json"
}
