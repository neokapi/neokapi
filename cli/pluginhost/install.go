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
	// IndexURL is the registry index URL (typically https://neokapi.github.io/registry/manifest-plugins.json).
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

	// Unsafe skips both SHA-256 and Sigstore/cosign signature
	// verification. Without Unsafe, SHA-256 is mandatory and signature
	// verification is mandatory whenever the registry entry provides
	// signature fields. If signature fields are absent and Unsafe is
	// false, install fails — no silent unsigned installs.
	Unsafe bool

	// LogF receives progress messages. Optional.
	LogF func(msg string)

	// ProgressF receives download progress as (bytesSoFar, totalBytes); total
	// is -1 when the server sends no Content-Length. Optional.
	ProgressF func(downloaded, total int64)
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
	body, err := registry.DownloadWithProgress(ctx, plat.URL, opts.ProgressF)
	if err != nil {
		return nil, err
	}
	logf("✓ Download complete")

	if !opts.Unsafe {
		if err := registry.VerifySHA256(body, plat.SHA256); err != nil {
			return nil, err
		}
		logf("✓ SHA-256 verified")

		// Cosign keyless signature verification ties the tarball to
		// the GitHub Actions workflow that produced it. We require it
		// unless --unsafe is set; the registry entry MUST carry the
		// signature URL + cert identity + OIDC issuer.
		if plat.Signature == "" || plat.CertIdentity == "" || plat.CertOIDCIssuer == "" {
			return nil, fmt.Errorf("install: registry entry for %s %s on %s is missing signature/cert_identity/cert_oidc_issuer (use --unsafe to install unsigned)", opts.PluginName, version, registry.PlatformKey())
		}
		logf(fmt.Sprintf("Verifying cosign signature against %s (issuer: %s)...", plat.CertIdentity, plat.CertOIDCIssuer))
		if err := registry.VerifyBundle(ctx, plat.Signature, plat.SHA256, plat.CertIdentity, plat.CertOIDCIssuer, registry.CosignVerifyOptions{}); err != nil {
			return nil, fmt.Errorf("install: %w", err)
		}
		logf("✓ Signature verified")
	} else {
		logf("Warning: --unsafe — skipping SHA-256 and signature checks")
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
	if err := writeInstalledMetadata(pluginDir, InstalledMetadata{
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

// InstalledMetadata records install-time choices for `kapi plugin update`.
// Persisted at <pluginDir>/installed.json by InstallFromRegistry.
type InstalledMetadata struct {
	Channel     string `json:"channel,omitempty"`
	Constraint  string `json:"constraint,omitempty"`
	IndexURL    string `json:"index_url,omitempty"`
	Version     string `json:"version,omitempty"`
	InstalledAt string `json:"installed_at,omitempty"`
}

func writeInstalledMetadata(pluginDir string, meta InstalledMetadata) error {
	path := filepath.Join(pluginDir, "installed.json")
	data, err := jsonMarshalIndent(meta)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// ReadInstalledMetadata returns the installed.json bookkeeping for
// pluginDir. Returns os.ErrNotExist when the file is missing — that's
// the expected case for plugins installed before installed.json
// became part of the install layout (tests, dev plugins, etc.).
func ReadInstalledMetadata(pluginDir string) (*InstalledMetadata, error) {
	path := filepath.Join(pluginDir, "installed.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m InstalledMetadata
	if err := jsonUnmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return &m, nil
}

// extractPluginArchive extracts a tarball or zip into the plugin's own dir
// (target/<pluginName>). It accepts both archive layouts we publish: the #438
// convention where every entry is prefixed with "<pluginName>/", and a flat
// archive whose entries are relative to the plugin dir (optionally with a
// leading "./", as `tar -C <dir> .` emits). The kapi-sat and kapi-pdfium
// release tarballs are flat, so requiring the prefix would make them
// uninstallable via `kapi plugins install` (the Homebrew path extracts
// differently); pluginEntryDest collapses both layouts onto pluginRoot.
func extractPluginArchive(body []byte, sourceURL, target, pluginName string) error {
	if strings.HasSuffix(sourceURL, ".zip") {
		return extractZip(body, target, pluginName)
	}
	return extractTarGz(body, target, pluginName)
}

// pluginEntryDest maps an archive entry name onto its on-disk destination
// inside pluginRoot (target/<pluginName>). It strips an optional leading "./"
// and an optional "<pluginName>/" prefix so the prefixed (#438) and flat
// layouts resolve to the same place under pluginRoot.
//
// ok is false for the plugin-dir root entry itself ("./", ".", "<pluginName>"),
// which needs no file written. It returns an error for any entry that would
// escape pluginRoot via "../" or an absolute path. Note a flat entry like
// "otherplugin/x" is a nested subdir of THIS plugin's dir, not the sibling
// plugin — it stays safely within pluginRoot.
func pluginEntryDest(pluginRoot, pluginName, name string) (dest string, ok bool, err error) {
	rel := strings.TrimPrefix(filepath.ToSlash(name), "./")
	if rel == pluginName || rel == pluginName+"/" {
		return "", false, nil
	}
	rel = strings.TrimPrefix(rel, pluginName+"/")
	dest, err = safeJoin(pluginRoot, rel) // rejects "../" escapes and absolute names
	if err != nil {
		return "", false, err
	}
	if dest == filepath.Clean(pluginRoot) {
		return "", false, nil // "" / "." — the plugin dir root
	}
	return dest, true, nil
}

func extractTarGz(body []byte, target, pluginName string) error {
	gz, err := gzip.NewReader(bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("gunzip: %w", err)
	}
	defer gz.Close()
	// pluginRoot is the per-plugin extraction boundary (target/<plugin>).
	// Symlink targets must resolve inside it — not merely inside the
	// shared install root, which would allow cross-plugin escapes.
	pluginRoot := filepath.Join(target, pluginName)
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("tar read: %w", err)
		}
		// Resolve the entry under the plugin's own dir, accepting both the
		// prefixed and flat layouts and rejecting "../"/absolute escapes.
		clean, ok, err := pluginEntryDest(pluginRoot, pluginName, hdr.Name)
		if err != nil {
			return err
		}
		if !ok {
			continue // the plugin-dir root entry — nothing to write
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
			// O_NOFOLLOW: refuse to write through a symlink that a prior
			// (malicious) entry may have planted at this path. The flag is
			// platform-specific (oNoFollow); on js/wasm it is 0 — the wasm
			// build has no filesystem symlink surface to harden against.
			f, err := os.OpenFile(clean, os.O_CREATE|os.O_WRONLY|os.O_TRUNC|oNoFollow, os.FileMode(hdr.Mode)&0o777)
			if err != nil {
				return fmt.Errorf("tarball entry %q: %w", hdr.Name, err)
			}
			if _, err := io.Copy(f, tr); err != nil {
				f.Close()
				return err
			}
			f.Close()
		case tar.TypeSymlink:
			// Resolve the link target relative to the entry's own
			// directory and ensure the result stays inside the plugin
			// dir. A filepath.Rel-based containment check (not a string
			// prefix against the install root) rejects sibling dirs
			// (e.g. plugins-evil), cross-plugin ../otherplugin/...
			// targets, and absolute escapes.
			linkDir := filepath.Dir(clean)
			resolved := filepath.Join(linkDir, hdr.Linkname)
			if !withinDir(pluginRoot, resolved) {
				return fmt.Errorf("tarball symlink %q points outside plugin dir %q", hdr.Name, pluginRoot)
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
	pluginRoot := filepath.Join(target, pluginName)
	for _, f := range r.File {
		clean, ok, err := pluginEntryDest(pluginRoot, pluginName, f.Name)
		if err != nil {
			return err
		}
		if !ok {
			continue // the plugin-dir root entry — nothing to write
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
	if !relWithin(rel) {
		return "", fmt.Errorf("archive entry %q escapes target dir", elem)
	}
	return clean, nil
}

// withinDir reports whether the already-resolved absolute path candidate
// is dir itself or lies inside it. Unlike safeJoin (which joins a
// relative archive name onto a root), candidate is a fully-resolved path,
// so an absolute symlink target like /etc/passwd is correctly rejected
// rather than re-rooted under dir.
func withinDir(dir, candidate string) bool {
	rel, err := filepath.Rel(filepath.Clean(dir), filepath.Clean(candidate))
	if err != nil {
		return false
	}
	return relWithin(rel)
}

// relWithin reports whether a filepath.Rel result stays within its base
// (i.e. does not start with ".." and is not absolute).
func relWithin(rel string) bool {
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

// Uninstall is owned by the plugin system: see (*Host).Remove, which deletes a
// plugin from the exact directory it was discovered in. Front-ends pass only a
// name, so install, discovery, and removal can never disagree on the location.

// DefaultIndexURL is the registry index this binary defaults to.
// Override via $KAPI_REGISTRY_URL.
func DefaultIndexURL() string {
	if v := os.Getenv("KAPI_REGISTRY_URL"); v != "" {
		return v
	}
	return "https://neokapi.github.io/registry/manifest-plugins.json"
}
