package conformance

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/neokapi/neokapi/core/plugin/manifest"
)

// ManifestFile is the fixed name of a plugin's manifest, relative to its
// install directory.
const ManifestFile = "manifest.json"

// sha256Hex matches a lowercase-hex SHA-256 digest.
var sha256Hex = regexp.MustCompile(`^[0-9a-f]{64}$`)

func manifestChecks() []registryEntry {
	return []registryEntry{
		{
			ID:       "manifest.present",
			Title:    "manifest.json exists and is readable",
			Mode:     ModeAny,
			Required: true,
			Why:      "discovery is a pure filesystem read; a directory without a readable manifest is not a plugin at all.",
			run:      checkManifestPresent,
		},
		{
			ID:       "manifest.parse",
			Title:    "manifest.json parses and passes structural validation",
			Mode:     ModeAny,
			Required: true,
			Why:      "the host skips a plugin whose manifest fails to parse, so every capability silently disappears.",
			run:      checkManifestParse,
		},
		{
			ID:       "manifest.version",
			Title:    "manifest_version is a version the host supports",
			Mode:     ModeAny,
			Required: true,
			Why:      "an unsupported manifest_version is rejected outright, so the plugin never registers.",
			run:      checkManifestVersion,
		},
		{
			ID:       "manifest.no-unknown-fields",
			Title:    "manifest.json carries no keys outside the published schema",
			Mode:     ModeAny,
			Required: true,
			Why:      "the schema forbids extra properties; a misspelled key is silently ignored instead of taking effect.",
			run:      checkManifestNoUnknownFields,
		},
		{
			ID:       "manifest.name-matches-dir",
			Title:    "the declared plugin name matches the install directory name",
			Mode:     ModeAny,
			Required: true,
			Why:      "discovery verifies the two agree and drops the plugin when they do not.",
			run:      checkManifestNameMatchesDir,
		},
		{
			ID:       "manifest.binary-present",
			Title:    "the declared binary exists inside the plugin directory and is executable",
			Mode:     ModeAny,
			Required: true,
			Why:      "every transport execs this path; a missing or non-executable binary fails at dispatch time.",
			run:      checkManifestBinaryPresent,
		},
		{
			ID:       "manifest.transport-declared",
			Title:    "at least one capability section is declared",
			Mode:     ModeAny,
			Required: true,
			Why:      "a plugin that declares no capabilities contributes nothing and cannot be dispatched to.",
			run:      checkManifestTransportDeclared,
		},
		{
			ID:       "manifest.daemon-block",
			Title:    "the daemon block is present exactly when Mode-C capabilities are declared",
			Mode:     ModeAny,
			Required: true,
			Why:      "the host reads daemon timeouts and the handshake shape from this block before it execs the daemon.",
			run:      checkManifestDaemonBlock,
		},
		{
			ID:       "manifest.schema-files",
			Title:    "every referenced JSON Schema resolves inside the plugin directory and is valid JSON",
			Mode:     ModeAny,
			Required: true,
			Why:      "an unreadable schema degrades recipe validation to a structural-only check, so bad config reaches the plugin.",
			run:      checkManifestSchemaFiles,
		},
		{
			ID:       "manifest.models",
			Title:    "declared model assets pin an immutable URL and a SHA-256 digest",
			Mode:     ModeAny,
			Required: true,
			Why:      "the host refuses to stage a model without a pinned digest; the manifest's signature is the only trust root for these bytes.",
			run:      checkManifestModels,
		},
	}
}

func manifestPath(dir string) string { return filepath.Join(dir, ManifestFile) }

func checkManifestPresent(_ context.Context, r *runner) (Status, string, error) {
	if r.readErr != nil {
		return Fail, fmt.Sprintf("%s: %v", ManifestFile, r.readErr), r.readErr
	}
	return Pass, fmt.Sprintf("%s (%d bytes)", manifestPath(r.suite.Dir), len(r.raw)), nil
}

func checkManifestParse(_ context.Context, r *runner) (Status, string, error) {
	switch {
	case r.readErr != nil:
		return Skip, ManifestFile + " unreadable (see manifest.present)", nil
	case r.decodeErr != nil:
		return Fail, fmt.Sprintf("%s is not well-formed JSON: %v", ManifestFile, r.decodeErr), r.decodeErr
	case r.parseErr != nil:
		// The document decoded but failed the host's validation. The checks
		// below name the specific rule where one applies; this reports the
		// first thing the host itself would reject.
		return Fail, r.parseErr.Error(), r.parseErr
	}
	return Pass, fmt.Sprintf("plugin %q version %q, modes %s",
		r.man.Plugin, r.man.Version, modeList(r.report.Modes)), nil
}

// declaredModes returns the transports a manifest declares, in A, B, C order.
func declaredModes(m *manifest.Manifest) []Mode {
	out := []Mode{}
	if m.IsModeA() {
		out = append(out, ModeA)
	}
	if m.IsModeB() {
		out = append(out, ModeB)
	}
	if m.IsModeC() {
		out = append(out, ModeC)
	}
	return out
}

func modeList(modes []Mode) string {
	if len(modes) == 0 {
		return "(none)"
	}
	parts := make([]string, 0, len(modes))
	for _, m := range modes {
		parts = append(parts, string(m))
	}
	return strings.Join(parts, "+")
}

func checkManifestVersion(_ context.Context, r *runner) (Status, string, error) {
	if st, d, err := r.requireManifest(); st != "" {
		return st, d, err
	}
	got := r.man.ManifestVersion
	if !slices.Contains(manifest.SupportedVersions, got) {
		return Fail, fmt.Sprintf("manifest_version %q is not supported (host accepts %s)",
			got, strings.Join(manifest.SupportedVersions, ", ")), nil
	}
	return Pass, fmt.Sprintf("manifest_version %q (host current %q)", got, manifest.CurrentVersion), nil
}

func checkManifestNoUnknownFields(_ context.Context, r *runner) (Status, string, error) {
	if r.readErr != nil {
		return Skip, ManifestFile + " unreadable (see manifest.present)", nil
	}
	dec := json.NewDecoder(bytes.NewReader(r.raw))
	dec.DisallowUnknownFields()
	var probe manifest.Manifest
	if err := dec.Decode(&probe); err != nil {
		// Only an unknown-field error is this check's business; a malformed
		// document is manifest.parse's failure to report.
		if strings.Contains(err.Error(), "unknown field") {
			return Fail, err.Error(), err
		}
		return Skip, ManifestFile + " did not decode (see manifest.parse)", nil
	}
	return Pass, "every key maps to a documented manifest field", nil
}

func checkManifestNameMatchesDir(_ context.Context, r *runner) (Status, string, error) {
	if st, d, err := r.requireManifest(); st != "" {
		return st, d, err
	}
	base := filepath.Base(r.suite.Dir)
	if r.man.Plugin != base {
		return Fail, fmt.Sprintf("manifest declares plugin %q but the install directory is %q",
			r.man.Plugin, base), nil
	}
	return Pass, fmt.Sprintf("plugin %q matches directory %q", r.man.Plugin, base), nil
}

func checkManifestBinaryPresent(_ context.Context, r *runner) (Status, string, error) {
	if st, d, err := r.requireManifest(); st != "" {
		return st, d, err
	}
	if r.binErr != nil {
		return Fail, r.binErr.Error(), r.binErr
	}
	return Pass, fmt.Sprintf("%s (mode %v)", r.binPath, r.binMode), nil
}

func checkManifestTransportDeclared(_ context.Context, r *runner) (Status, string, error) {
	if st, d, err := r.requireManifest(); st != "" {
		return st, d, err
	}
	c := r.man.Capabilities
	counts := []struct {
		name string
		n    int
	}{
		{"commands", len(c.Commands)},
		{"mcp_tools", len(c.MCPTools)},
		{"formats", len(c.Formats)},
		{"tools", len(c.Tools)},
		{"segmenters", len(c.Segmenters)},
		{"source_connectors", len(c.SourceConnectors)},
		{"schema_extensions", len(c.SchemaExtensions)},
		{"command_contributions", len(c.CommandContributions)},
	}
	var declared []string
	for _, e := range counts {
		if e.n > 0 {
			declared = append(declared, fmt.Sprintf("%s=%d", e.name, e.n))
		}
	}
	if len(declared) == 0 {
		return Fail, "no capability section is populated", nil
	}
	return Pass, strings.Join(declared, ", "), nil
}

func checkManifestDaemonBlock(_ context.Context, r *runner) (Status, string, error) {
	if st, d, err := r.requireManifest(); st != "" {
		return st, d, err
	}
	isC := r.man.IsModeC()
	d := r.man.Daemon
	switch {
	case isC && d == nil:
		return Fail, "Mode-C capabilities are declared but there is no daemon block", nil
	case !isC && d != nil:
		return Fail, "a daemon block is present but no format, flow tool, segmenter, or source connector is declared", nil
	case !isC:
		return Pass, "no Mode-C capabilities and no daemon block, consistent", nil
	}
	if hs := d.Handshake; hs != nil {
		if hs.Type != "" && hs.Type != HandshakeTypeStdio {
			return Fail, fmt.Sprintf("handshake type %q is not supported (only %q)", hs.Type, HandshakeTypeStdio), nil
		}
		if len(hs.Fields) > 0 && !slices.Contains(hs.Fields, "socket") {
			return Fail, fmt.Sprintf("handshake fields %v omit \"socket\", which the host requires", hs.Fields), nil
		}
	}
	return Pass, fmt.Sprintf("daemon block present (idle %ds, startup %ds)",
		d.IdleTimeoutSeconds, d.StartupTimeoutSeconds), nil
}

// schemaRef is one manifest-declared JSON Schema path plus where it came from.
type schemaRef struct {
	from string
	path string
}

func schemaRefs(m *manifest.Manifest) []schemaRef {
	var out []schemaRef
	for _, e := range m.Capabilities.SchemaExtensions {
		if e.JSONSchema != "" {
			out = append(out, schemaRef{fmt.Sprintf("schema_extensions[%s]", e.Name), e.JSONSchema})
		}
	}
	for _, f := range m.Capabilities.Formats {
		if f.Schema != "" {
			out = append(out, schemaRef{fmt.Sprintf("formats[%s]", f.Name), f.Schema})
		}
	}
	for _, t := range m.Capabilities.Tools {
		if t.Schema != "" {
			out = append(out, schemaRef{fmt.Sprintf("tools[%s]", t.Name), t.Schema})
		}
	}
	for _, s := range m.Capabilities.Segmenters {
		if s.Schema != "" {
			out = append(out, schemaRef{fmt.Sprintf("segmenters[%s]", s.Name), s.Schema})
		}
	}
	return out
}

func checkManifestSchemaFiles(_ context.Context, r *runner) (Status, string, error) {
	if st, d, err := r.requireManifest(); st != "" {
		return st, d, err
	}
	refs := schemaRefs(r.man)
	if len(refs) == 0 {
		return Skip, "no capability references a JSON Schema", nil
	}
	var problems []string
	for _, ref := range refs {
		if filepath.IsAbs(ref.path) {
			problems = append(problems, fmt.Sprintf("%s: %q is absolute", ref.from, ref.path))
			continue
		}
		full := filepath.Join(r.suite.Dir, filepath.FromSlash(ref.path))
		if !withinDir(r.suite.Dir, full) {
			problems = append(problems, fmt.Sprintf("%s: %q escapes the plugin directory", ref.from, ref.path))
			continue
		}
		data, err := os.ReadFile(full)
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s: %v", ref.from, err))
			continue
		}
		var doc any
		if err := json.Unmarshal(data, &doc); err != nil {
			problems = append(problems, fmt.Sprintf("%s (%s): invalid JSON: %v", ref.from, ref.path, err))
		}
	}
	if len(problems) > 0 {
		return Fail, strings.Join(problems, "; "), nil
	}
	return Pass, fmt.Sprintf("%d schema file(s) resolve and parse", len(refs)), nil
}

func checkManifestModels(_ context.Context, r *runner) (Status, string, error) {
	if st, d, err := r.requireManifest(); st != "" {
		return st, d, err
	}
	if len(r.man.Models) == 0 {
		return Skip, "no model assets declared", nil
	}
	var problems []string
	defaults := 0
	files := 0
	for _, a := range r.man.Models {
		if a.Default {
			defaults++
		}
		for _, f := range a.Files {
			files++
			where := fmt.Sprintf("models[%s].files[%s]", a.ID, f.Path)
			if a.Bundled {
				full := filepath.Join(r.suite.Dir, filepath.FromSlash(f.Path))
				if !withinDir(r.suite.Dir, full) {
					problems = append(problems, where+": escapes the plugin directory")
					continue
				}
				if _, err := os.Stat(full); err != nil {
					problems = append(problems, fmt.Sprintf("%s: bundled file missing: %v", where, err))
				}
				continue
			}
			if !sha256Hex.MatchString(f.SHA256) {
				problems = append(problems, where+": sha256 must be a 64-character lowercase hex digest")
			}
			if !strings.HasPrefix(f.URL, "https://") {
				problems = append(problems, where+": url must be an https URL")
			}
		}
	}
	if defaults > 1 {
		problems = append(problems, fmt.Sprintf("%d model assets are marked default; at most one may be", defaults))
	}
	if len(problems) > 0 {
		return Fail, strings.Join(problems, "; "), nil
	}
	return Pass, fmt.Sprintf("%d model asset(s), %d file(s), all pinned", len(r.man.Models), files), nil
}

// withinDir reports whether path is dir itself or lies beneath it, after
// symlink-free lexical cleaning. It is the guard that keeps a manifest from
// pointing the host at a file outside the signed plugin directory.
func withinDir(dir, path string) bool {
	rel, err := filepath.Rel(dir, path)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
