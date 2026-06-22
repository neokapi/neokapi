// Package registry implements the v2 plugin registry index format and
// provides resolve/download/verify primitives for `kapi plugin install`.
//
// The v2 format is described in #438. Each plugin name maps to a list of
// versions; each version maps to per-platform tarball URLs + SHA-256
// hashes (and, on signed builds, cosign cert identities). Channels group
// versions for stable vs. beta tracks.
package registry

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"sort"
	"strings"
)

// IndexV2 is the top-level shape of the registry index.
type IndexV2 struct {
	// Schema is the schema-version sentinel. Currently always "v2" or
	// the integer 2 (we tolerate either when reading).
	Schema any `json:"schema,omitempty"`

	// Plugins maps plugin name → entry.
	Plugins map[string]PluginEntry `json:"plugins"`
}

// PluginEntry describes one plugin in the registry.
type PluginEntry struct {
	Description string                  `json:"description,omitempty"`
	Homepage    string                  `json:"homepage,omitempty"`
	Author      string                  `json:"author,omitempty"`
	License     string                  `json:"license,omitempty"`
	Group       string                  `json:"group,omitempty"`
	Channels    []string                `json:"channels,omitempty"`
	Versions    map[string]VersionEntry `json:"versions"`
	// Deprecated, when set, marks the plugin as retired/deprecated in the
	// registry. It lets the registry refuse new installs and flag the plugin in
	// search without waiting for a kapi release. kapi's compiled-in tombstone
	// list (cli/pluginhost) remains authoritative for load-time enforcement and
	// works offline; this is the faster, reversible, network-side signal.
	Deprecated *Deprecation `json:"deprecated,omitempty"`
}

// Deprecation marks a registry plugin as retired. Mirrors the fields of the
// compiled-in tombstone so the two channels carry the same message.
type Deprecation struct {
	Retired     bool   `json:"retired,omitempty"`     // true → refuse new installs
	Since       string `json:"since,omitempty"`       // kapi version that retired it
	Because     string `json:"because,omitempty"`     // reason fragment
	Replacement string `json:"replacement,omitempty"` // successor plugin name, if any
	Message     string `json:"message,omitempty"`     // free-text guidance
	InfoURL     string `json:"info_url,omitempty"`
}

// VersionEntry describes one published version of a plugin.
type VersionEntry struct {
	Released       string                   `json:"released,omitempty"`
	Channel        string                   `json:"channel,omitempty"`
	MinKapiVersion string                   `json:"min_kapi_version,omitempty"`
	Platforms      map[string]PlatformEntry `json:"platforms"`
}

// PlatformEntry describes one (os/arch) build of one version.
type PlatformEntry struct {
	URL            string `json:"url"`
	SHA256         string `json:"sha256"`
	Signature      string `json:"signature,omitempty"`
	CertIdentity   string `json:"cert_identity,omitempty"`
	CertOIDCIssuer string `json:"cert_oidc_issuer,omitempty"`
}

// PlatformKey returns the canonical platform string for the running
// binary, e.g. "darwin/arm64". The registry index uses this same form.
func PlatformKey() string {
	return runtime.GOOS + "/" + runtime.GOARCH
}

// maxIndexBytes is the maximum size of a registry index download.
// A compromised or redirected registry could otherwise exhaust client memory.
const maxIndexBytes = 32 << 20 // 32 MB

// FetchIndex downloads the registry index from indexURL.
func FetchIndex(ctx context.Context, indexURL string) (*IndexV2, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, indexURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch index %s: %w", indexURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch index %s: HTTP %d", indexURL, resp.StatusCode)
	}
	if resp.ContentLength > maxIndexBytes {
		return nil, fmt.Errorf("fetch index %s: Content-Length %d exceeds limit %d", indexURL, resp.ContentLength, maxIndexBytes)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxIndexBytes+1))
	if err != nil {
		return nil, fmt.Errorf("fetch index %s: read body: %w", indexURL, err)
	}
	if int64(len(data)) > maxIndexBytes {
		return nil, fmt.Errorf("fetch index %s: response exceeds limit %d bytes", indexURL, maxIndexBytes)
	}
	var idx IndexV2
	if err := json.Unmarshal(data, &idx); err != nil {
		return nil, fmt.Errorf("parse index %s: %w", indexURL, err)
	}
	return &idx, nil
}

// Resolve returns the (version, platform) entry that satisfies the
// given resolution criteria, or an error describing what failed.
//
//   - name:        plugin name to resolve
//   - constraint:  semver-style constraint (^1.0, >=1.4.0, *, "1.4.0", "")
//   - channel:     filter to versions with this channel ("" = any)
//   - kapiVersion: drop versions whose min_kapi_version is newer than this
//
// Picks the highest version that matches all constraints. Returns the
// version string + the entry for the running platform.
func (idx *IndexV2) Resolve(name, constraint, channel, kapiVersion string) (version string, plat PlatformEntry, err error) {
	entry, ok := idx.Plugins[name]
	if !ok {
		return "", PlatformEntry{}, fmt.Errorf("plugin %q not in registry", name)
	}
	candidates := make([]string, 0, len(entry.Versions))
	for v := range entry.Versions {
		candidates = append(candidates, v)
	}
	// Sort descending by semver-ish.
	sort.Slice(candidates, func(i, j int) bool {
		return CompareSemver(candidates[i], candidates[j]) > 0
	})

	platKey := PlatformKey()
	for _, v := range candidates {
		ve := entry.Versions[v]
		if constraint != "" && !MatchConstraint(constraint, v) {
			continue
		}
		if channel != "" && ve.Channel != "" && ve.Channel != channel {
			continue
		}
		if ve.MinKapiVersion != "" && kapiVersion != "" {
			if CompareSemver(kapiVersion, ve.MinKapiVersion) < 0 {
				continue
			}
		}
		pe, ok := ve.Platforms[platKey]
		if !ok {
			continue
		}
		return v, pe, nil
	}
	return "", PlatformEntry{}, fmt.Errorf("no version of plugin %q matches constraint %q on channel %q for platform %s", name, constraint, channel, platKey)
}

// VerifySHA256 streams data and reports whether the hash matches.
func VerifySHA256(data []byte, expected string) error {
	if expected == "" {
		return errors.New("registry: missing sha256 — refusing to install (use --unsafe to override)")
	}
	sum := sha256.Sum256(data)
	got := hex.EncodeToString(sum[:])
	if !strings.EqualFold(got, expected) {
		return fmt.Errorf("registry: sha256 mismatch (expected %s, got %s)", expected, got)
	}
	return nil
}

// Download fetches the URL and returns its raw body.
func Download(ctx context.Context, url string) ([]byte, error) {
	return DownloadWithProgress(ctx, url, nil)
}

// maxArtifactBytes is the maximum size of a plugin artifact download.
// A compromised or redirected registry could otherwise exhaust client memory.
// SHA256/cosign verification happens after the full read, so the cap is the
// only protection against a memory-exhaustion attack.
const maxArtifactBytes = 512 << 20 // 512 MB

// DownloadWithProgress is Download with an optional progress callback, invoked
// as bytes arrive with (bytesSoFar, totalBytes). total is -1 when the server
// sends no Content-Length. The callback runs on the download goroutine, so it
// must be cheap and non-blocking.
func DownloadWithProgress(ctx context.Context, url string, onProgress func(downloaded, total int64)) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download %s: HTTP %d", url, resp.StatusCode)
	}
	if resp.ContentLength > maxArtifactBytes {
		return nil, fmt.Errorf("download %s: Content-Length %d exceeds limit %d", url, resp.ContentLength, maxArtifactBytes)
	}
	var reader = io.LimitReader(resp.Body, maxArtifactBytes+1)
	if onProgress != nil {
		reader = &progressReader{r: reader, total: resp.ContentLength, onProgress: onProgress}
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("download %s: read body: %w", url, err)
	}
	if int64(len(data)) > maxArtifactBytes {
		return nil, fmt.Errorf("download %s: response exceeds limit %d bytes", url, maxArtifactBytes)
	}
	return data, nil
}

// progressReader reports cumulative bytes read through onProgress.
type progressReader struct {
	r          io.Reader
	total      int64
	downloaded int64
	onProgress func(downloaded, total int64)
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	if n > 0 {
		p.downloaded += int64(n)
		p.onProgress(p.downloaded, p.total)
	}
	return n, err
}
