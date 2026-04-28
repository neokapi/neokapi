// Command registry-update inserts a new plugin version into the
// registry's manifest-plugins.json.
//
// Usage:
//
//	registry-update \
//	  --registry path/to/manifest-plugins.json \
//	  --plugin bowrain \
//	  --version 1.4.0 \
//	  --channel stable \
//	  --released 2026-04-27 \
//	  --min-kapi-version 0.1.0 \
//	  --platform darwin/arm64 \
//	  --url https://github.com/.../kapi-bowrain_1.4.0_darwin_arm64.tar.gz \
//	  --sha256 abcdef...
//
// Repeat the --platform/--url/--sha256 trio for each platform in the
// release.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
)

type platformEntry struct {
	URL            string `json:"url"`
	SHA256         string `json:"sha256"`
	Signature      string `json:"signature,omitempty"`
	CertIdentity   string `json:"cert_identity,omitempty"`
	CertOIDCIssuer string `json:"cert_oidc_issuer,omitempty"`
}

type versionEntry struct {
	Released       string                   `json:"released,omitempty"`
	Channel        string                   `json:"channel,omitempty"`
	MinKapiVersion string                   `json:"min_kapi_version,omitempty"`
	Platforms      map[string]platformEntry `json:"platforms"`
}

type pluginEntry struct {
	Description string                  `json:"description,omitempty"`
	Homepage    string                  `json:"homepage,omitempty"`
	Author      string                  `json:"author,omitempty"`
	License     string                  `json:"license,omitempty"`
	Group       string                  `json:"group,omitempty"`
	Channels    []string                `json:"channels,omitempty"`
	Versions    map[string]versionEntry `json:"versions"`
}

type registryFile struct {
	Schema  string                 `json:"schema,omitempty"`
	Comment string                 `json:"comment,omitempty"`
	Plugins map[string]pluginEntry `json:"plugins"`
}

type stringList []string

func (s *stringList) String() string { return fmt.Sprintf("%v", *s) }
func (s *stringList) Set(v string) error {
	*s = append(*s, v)
	return nil
}

func main() {
	var (
		regFile        string
		plugin         string
		version        string
		channel        string
		released       string
		minKapiVersion string
		platforms      stringList
		urls           stringList
		shas           stringList
		sigs           stringList
		certIdentity   string
		certIssuer     string
	)
	flag.StringVar(&regFile, "registry", "", "path to manifest-plugins.json")
	flag.StringVar(&plugin, "plugin", "", "plugin name (e.g. bowrain)")
	flag.StringVar(&version, "version", "", "plugin version (e.g. 1.4.0)")
	flag.StringVar(&channel, "channel", "stable", "registry channel")
	flag.StringVar(&released, "released", "", "release date (YYYY-MM-DD)")
	flag.StringVar(&minKapiVersion, "min-kapi-version", "", "minimum kapi version")
	flag.Var(&platforms, "platform", "platform key (e.g. darwin/arm64); repeat per artifact")
	flag.Var(&urls, "url", "tarball URL; repeat in same order as --platform")
	flag.Var(&shas, "sha256", "tarball SHA-256; repeat in same order as --platform")
	flag.Var(&sigs, "signature", "Sigstore bundle URL (.sigstore.json); repeat per --platform; use empty string for unsigned")
	flag.StringVar(&certIdentity, "cert-identity", "", "cosign cert identity (applied to every platform)")
	flag.StringVar(&certIssuer, "cert-oidc-issuer", "", "cosign OIDC issuer (applied to every platform)")
	flag.Parse()

	if regFile == "" || plugin == "" || version == "" {
		fmt.Fprintln(os.Stderr, "registry-update: --registry, --plugin, and --version are required")
		os.Exit(2)
	}
	if len(platforms) == 0 {
		fmt.Fprintln(os.Stderr, "registry-update: at least one --platform/--url/--sha256 trio is required")
		os.Exit(2)
	}
	if len(platforms) != len(urls) || len(platforms) != len(shas) {
		fmt.Fprintln(os.Stderr, "registry-update: --platform/--url/--sha256 lists must have the same length")
		os.Exit(2)
	}
	if len(sigs) != 0 && len(sigs) != len(platforms) {
		fmt.Fprintln(os.Stderr, "registry-update: when --signature is set it must repeat per --platform (use empty string to mark a platform unsigned)")
		os.Exit(2)
	}

	plats := make(map[string]platformEntry, len(platforms))
	for i, p := range platforms {
		entry := platformEntry{URL: urls[i], SHA256: shas[i]}
		if len(sigs) == len(platforms) && sigs[i] != "" {
			entry.Signature = sigs[i]
			entry.CertIdentity = certIdentity
			entry.CertOIDCIssuer = certIssuer
		}
		plats[p] = entry
	}

	if err := mergePluginVersion(regFile, plugin, version, channel, released, minKapiVersion, plats); err != nil {
		fmt.Fprintln(os.Stderr, "registry-update:", err)
		os.Exit(1)
	}
	fmt.Printf("registry-update: %s %s registered with %d platform(s)\n", plugin, version, len(plats))
}

func mergePluginVersion(path, plugin, version, channel, released, minKapiVersion string, plats map[string]platformEntry) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	var reg registryFile
	if err := json.Unmarshal(data, &reg); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	if reg.Plugins == nil {
		reg.Plugins = map[string]pluginEntry{}
	}
	entry, ok := reg.Plugins[plugin]
	if !ok {
		return fmt.Errorf("plugin %q is not declared in %s — add a stub entry first", plugin, path)
	}
	if entry.Versions == nil {
		entry.Versions = map[string]versionEntry{}
	}
	entry.Versions[version] = versionEntry{
		Released:       released,
		Channel:        channel,
		MinKapiVersion: minKapiVersion,
		Platforms:      plats,
	}
	reg.Plugins[plugin] = entry

	out, err := json.MarshalIndent(reg, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(out, '\n'), 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	return nil
}

var _ = errors.New
