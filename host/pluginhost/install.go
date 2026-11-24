package pluginhost

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/neokapi/neokapi/core/dataroot"

	"github.com/neokapi/neokapi/core/plugin/manifest"
	"github.com/neokapi/neokapi/host/pluginhost/registry"
)

// ErrEmptyPluginsDir is returned for an install under $KAPI_PLUGINS_DIR_ONLY
// when $KAPI_PLUGINS_DIR names no directory: discovery then reads no root, so
// a plugin installed anywhere would never be discovered.
var ErrEmptyPluginsDir = errors.New("KAPI_PLUGINS_DIR_ONLY confines plugin discovery to $KAPI_PLUGINS_DIR, but KAPI_PLUGINS_DIR is empty, so an installed plugin would never be discovered: set KAPI_PLUGINS_DIR to the directory to install into")

// InstallTarget returns the directory `kapi plugin install` unpacks plugins
// into, for discovery that reads $KAPI_PLUGINS_DIR. See InstallTargetFor.
func InstallTarget() (string, error) {
	return InstallTargetFor(os.Getenv("KAPI_PLUGINS_DIR"))
}

// InstallTargetFor returns the install directory for a front-end whose first
// discovery root is pluginsDir ($KAPI_PLUGINS_DIR, or the CLI's --plugin-dir
// in its place). By default it is $XDG_DATA_HOME/kapi/plugins, whatever
// pluginsDir holds. With $KAPI_PLUGINS_DIR_ONLY set, discovery reads pluginsDir
// alone, so the install goes to its first entry, and a pluginsDir that names
// no directory is ErrEmptyPluginsDir.
func InstallTargetFor(pluginsDir string) (string, error) {
	if !onlyEnvDirFromEnv() {
		return dataInstallTarget()
	}
	paths := splitPathList(pluginsDir)
	if len(paths) == 0 {
		return "", ErrEmptyPluginsDir
	}
	return filepath.Clean(paths[0]), nil
}

// DefaultDiscoverOptions are the discovery roots for a front end with no
// --plugin-dir: $KAPI_PLUGINS_DIR, then the user root InstallTarget writes
// to, then the system roots. The CLI, Kapi Desktop and the media-engine
// probes all install there and search these, so a plugin one of them
// installs is found by the others.
func DefaultDiscoverOptions() DiscoverOptions {
	return DiscoverOptions{EnvPluginsDir: os.Getenv("KAPI_PLUGINS_DIR")}
}

// dataInstallTarget returns $XDG_DATA_HOME/kapi/plugins, the per-user install
// root that discovery scans second. With no XDG_DATA_HOME and no home
// directory there is no per-user place to install into, and it fails rather
// than fall back to a shared directory another user could write to.
func dataInstallTarget() (string, error) {
	xdg := os.Getenv("XDG_DATA_HOME")
	if xdg == "" {
		home, err := dataroot.UserHomeDir()
		if err != nil || home == "" {
			return "", errNoUserDataDir
		}
		xdg = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(xdg, "kapi", "plugins"), nil
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
	// false, install fails, no silent unsigned installs.
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
		t, err := InstallTarget()
		if err != nil {
			return nil, fmt.Errorf("install: %w", err)
		}
		target = t
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
	// Refuse to install a plugin the registry marks retired (the network-side
	// signal; the compiled-in tombstone is the offline authority and is checked
	// by the CLI before we get here).
	if entry, ok := idx.Plugins[opts.PluginName]; ok && entry.Deprecated != nil && entry.Deprecated.Retired {
		d := entry.Deprecated
		msg := fmt.Sprintf("plugin %q was retired", opts.PluginName)
		if d.Since != "" {
			msg += " in kapi " + d.Since
		}
		if d.Because != "" {
			msg += ": " + d.Because
		}
		if d.Message != "" {
			msg += "\n  " + d.Message
		}
		if d.InfoURL != "" {
			msg += "\n  More: " + d.InfoURL
		}
		return nil, errors.New(msg)
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
		logf("Warning: --unsafe skips the SHA-256 and signature checks")
	}

	pluginDir := filepath.Join(target, opts.PluginName)
	if err := os.MkdirAll(target, pluginDirMode); err != nil {
		return nil, fmt.Errorf("install: mkdir %s: %w", target, err)
	}
	// A plugin is an executable: kapi installs one only where no other user
	// can replace it (safety.go).
	if err := checkPath(target); err != nil {
		return nil, fmt.Errorf("install: %w", err)
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
		Archive:     archiveDigest(body),
	}); err != nil {
		// Non-fatal; install succeeded.
		logf("Warning: write installed metadata: " + err.Error())
	}

	// Absorb macOS's one-time first-exec assessment of the new plugin binary
	// here (it stalls the binary's first exec, proportional to size, up to
	// ~1s for the larger plugins) rather than on the plugin's first real
	// dispatch mid-workflow.
	prewarmFirstExec(ctx, &Plugin{
		Dir:        pluginDir,
		Source:     Source{Path: target},
		Manifest:   m,
		BinaryPath: filepath.Join(pluginDir, filepath.FromSlash(m.Binary)),
	}, logf)

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
	// Archive is the sha256 of the archive the plugin was installed from, in
	// hex: the plugin's bytes, the same whenever those bytes are installed
	// again. The parse cache keys a plugin's parses by it (Plugin.Fingerprint).
	Archive string `json:"archive_sha256,omitempty"`
}

// archiveDigest is the sha256 of an installed archive's bytes, in hex.
func archiveDigest(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
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
// pluginDir. Returns os.ErrNotExist when the file is missing, that's
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

// pluginEntryDest maps an archive entry name onto its destination inside
// pluginRoot (target/<pluginName>). It strips a leading "./" and one wrapping
// top-level directory, so every layout resolves to the same place:
//
//   - "<pluginName>/"      the #438 prefixed convention; and
//   - "kapi-<pluginName>/" the staged-binary-dir wrapper the ffmpeg/whisper
//     (av/asr) package scripts produce (they tar the staged "kapi-<plugin>"
//     directory by name rather than its contents).
//
// ok is false for a wrapper-dir root entry itself, which needs no file written.
// An entry that would escape pluginRoot via "../" or an absolute path is an
// error. A flat entry like "otherplugin/x" lands in a subdir of THIS plugin's
// dir, within pluginRoot.
//
// isDir must say whether the archive entry is a directory. Only a directory can
// be a wrapper, because the plugin binary is itself named "kapi-<plugin>"; a
// name test alone would discard that binary from a "<plugin>/"-wrapped or flat
// archive.
func pluginEntryDest(pluginRoot, pluginName, name string, isDir bool) (dest string, ok bool, err error) {
	rel := strings.TrimPrefix(filepath.ToSlash(name), "./")
	for _, wrap := range []string{pluginName, "kapi-" + pluginName} {
		if isDir && (rel == wrap || rel == wrap+"/") {
			return "", false, nil // the wrapper dir entry itself
		}
		rel = strings.TrimPrefix(rel, wrap+"/")
	}
	dest, err = safeJoin(pluginRoot, rel) // rejects "../" escapes and absolute names
	if err != nil {
		return "", false, err
	}
	if dest == filepath.Clean(pluginRoot) {
		return "", false, nil // "" / ".", the plugin dir root
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
	// Symlink targets must resolve inside it, not merely inside the shared
	// install root, which would allow cross-plugin escapes.
	pluginRoot := filepath.Join(target, pluginName)
	// Every entry is written through an os.Root opened on the plugin's own
	// directory, so a chain of links an earlier entry planted (a -> ., then
	// a/x -> ..) cannot lead a later write out of it: the root refuses any
	// name that resolves outside, whatever the links on the way.
	if err := os.MkdirAll(pluginRoot, 0o755); err != nil {
		return err
	}
	root, err := os.OpenRoot(pluginRoot)
	if err != nil {
		return err
	}
	defer root.Close()
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
		clean, ok, err := pluginEntryDest(pluginRoot, pluginName, hdr.Name, hdr.Typeflag == tar.TypeDir)
		if err != nil {
			return err
		}
		if !ok {
			continue // the plugin-dir root entry, with nothing to write
		}
		rel, rerr := filepath.Rel(pluginRoot, clean)
		if rerr != nil || !filepath.IsLocal(rel) {
			return fmt.Errorf("tarball entry %q lies outside plugin dir %q", hdr.Name, pluginRoot)
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := root.MkdirAll(rel, 0o755); err != nil {
				return fmt.Errorf("tarball entry %q: %w", hdr.Name, err)
			}
		case tar.TypeReg:
			if dir := filepath.Dir(rel); dir != "." {
				if err := root.MkdirAll(dir, 0o755); err != nil {
					return fmt.Errorf("tarball entry %q: %w", hdr.Name, err)
				}
			}
			// O_NOFOLLOW: refuse to write through a symlink that a prior
			// (malicious) entry may have planted at this path. The flag is
			// platform-specific (oNoFollow); on js/wasm it is 0, as the
			// wasm build has no filesystem symlink surface to harden against.
			f, err := root.OpenFile(rel, os.O_CREATE|os.O_WRONLY|os.O_TRUNC|oNoFollow, os.FileMode(hdr.Mode)&installFileMask)
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
			_ = root.Remove(rel)
			if err := root.Symlink(hdr.Linkname, rel); err != nil {
				return fmt.Errorf("tarball entry %q: %w", hdr.Name, err)
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
	// As for a tarball, every entry is written through an os.Root on the
	// plugin's own directory, so no name can resolve outside it.
	if err := os.MkdirAll(pluginRoot, 0o755); err != nil {
		return err
	}
	root, err := os.OpenRoot(pluginRoot)
	if err != nil {
		return err
	}
	defer root.Close()
	for _, f := range r.File {
		clean, ok, err := pluginEntryDest(pluginRoot, pluginName, f.Name, f.FileInfo().IsDir())
		if err != nil {
			return err
		}
		if !ok {
			continue // the plugin-dir root entry, nothing to write
		}
		rel, rerr := filepath.Rel(pluginRoot, clean)
		if rerr != nil || !filepath.IsLocal(rel) {
			return fmt.Errorf("zip entry %q lies outside plugin dir %q", f.Name, pluginRoot)
		}
		if f.FileInfo().IsDir() {
			if err := root.MkdirAll(rel, 0o755); err != nil {
				return err
			}
			continue
		}
		if dir := filepath.Dir(rel); dir != "." {
			if err := root.MkdirAll(dir, 0o755); err != nil {
				return err
			}
		}
		if err := writeZipEntry(root, rel, f); err != nil {
			return fmt.Errorf("zip entry %q: %w", f.Name, err)
		}
	}
	return nil
}

// writeZipEntry copies one zip entry to rel under root, with the install mode
// cap applied (no setuid, setgid or sticky bit, nothing writable by others).
func writeZipEntry(root *os.Root, rel string, f *zip.File) error {
	src, err := f.Open()
	if err != nil {
		return err
	}
	defer src.Close()
	dst, err := root.OpenFile(rel, os.O_CREATE|os.O_WRONLY|os.O_TRUNC|oNoFollow, f.Mode()&installFileMask)
	if err != nil {
		return err
	}
	if _, err := io.Copy(dst, src); err != nil {
		dst.Close()
		return err
	}
	return dst.Close()
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

// prewarmFirstExec execs a freshly installed plugin binary once. On macOS, the
// one-time Gatekeeper assessment of the new inode (XProtect scan and online
// notarization lookup) then happens at install time instead of at the
// plugin's first dispatch. `<binary> version` is the standard probe every
// plugin answers (the same one `kapi plugins doctor` uses). The assessment
// completes at exec time, so even a plugin that mishandles the verb is warmed.
// Best-effort and darwin-only; no other platform stalls on first exec.
//
// It runs the binary only after CheckLaunch passes, as every launch does.
func prewarmFirstExec(ctx context.Context, p *Plugin, logf func(string)) {
	if runtime.GOOS != "darwin" {
		return
	}
	binPath := p.BinaryPath
	if _, err := os.Stat(binPath); err != nil {
		return
	}
	if err := CheckLaunch(p); err != nil {
		logf("Warning: " + err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binPath, "version")
	// Setting Env matters even for a throwaway `version` call: leaving it nil
	// inherits the parent environment wholesale, which is the leak this
	// package otherwise avoids.
	cmd.Env = pluginEnviron()
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	_ = cmd.Run()
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
