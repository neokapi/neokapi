package klz

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// ManifestVersion is the kapiLocalizationFormat string emitted on
// every Manifest. Consumers MUST reject unknown major versions and
// SHOULD accept unknown minor versions of their major. The format
// version here is independent of core/klf's SchemaVersion — this is
// the archive-level envelope version, RFC 0001 §Versioning.
const ManifestVersion = "1.0"

// PartRole is the manifest-level classification of a part inside a
// .klz archive. Readers SHOULD use this as a fast-path index before
// inflating parts. Matches the role enum in RFC 0001 §.klz manifest
// schema.
type PartRole string

const (
	RoleDocument   PartRole = "document"
	RoleTarget     PartRole = "target"
	RoleSkeleton   PartRole = "skeleton"
	RoleVocabulary PartRole = "vocabulary"
	RoleAsset      PartRole = "asset"
	RoleSignature  PartRole = "signature"
	RoleMeta       PartRole = "meta"
	RoleAnnotation PartRole = "annotation"
)

// Manifest is the in-memory form of a .klz archive's manifest.json.
type Manifest struct {
	KapiLocalizationFormat string             `json:"kapiLocalizationFormat"`
	Created                string             `json:"created,omitempty"`
	Generator              ManifestGenerator  `json:"generator"`
	Project                ManifestProject    `json:"project"`
	Parts                  []ManifestPartInfo `json:"parts"`
}

// ManifestGenerator identifies the extractor that produced the
// archive.
type ManifestGenerator struct {
	ID      string `json:"id"`
	Version string `json:"version"`
}

// ManifestProject identifies the project the archive belongs to.
type ManifestProject struct {
	ID            string   `json:"id"`
	SourceLocale  string   `json:"sourceLocale"`
	TargetLocales []string `json:"targetLocales,omitempty"`
}

// ManifestPartInfo describes one archive entry. Path is POSIX
// relative, NFC-normalized UTF-8, no leading slash, no `..`. SHA256
// is hex-encoded.
type ManifestPartInfo struct {
	Path       string         `json:"path"`
	SHA256     string         `json:"sha256"`
	Size       int64          `json:"size"`
	Role       PartRole       `json:"role"`
	Attributes map[string]any `json:"attributes,omitempty"`
}

// MarshalManifest serializes a Manifest deterministically (2-space
// indent, no HTML escaping, trailing newline). The raw bytes this
// function returns are what the runtime-cache SHA-256 is computed
// over per RFC 0001 §The cache key — so any change here is also a
// cache-key change, by design.
func MarshalManifest(m *Manifest) ([]byte, error) {
	if m == nil {
		return nil, fmt.Errorf("klz: marshal nil manifest")
	}
	if m.KapiLocalizationFormat == "" {
		m.KapiLocalizationFormat = ManifestVersion
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(m); err != nil {
		return nil, fmt.Errorf("klz: encode manifest: %w", err)
	}
	return buf.Bytes(), nil
}

// UnmarshalManifest decodes a manifest.json payload and checks the
// format version.
func UnmarshalManifest(data []byte) (*Manifest, error) {
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("klz: decode manifest: %w", err)
	}
	if err := checkManifestVersion(m.KapiLocalizationFormat); err != nil {
		return nil, err
	}
	return &m, nil
}

func checkManifestVersion(v string) error {
	if v == "" {
		return fmt.Errorf("klz: manifest missing kapiLocalizationFormat")
	}
	wantMajor, _, _ := splitVersion(ManifestVersion)
	major, _, ok := splitVersion(v)
	if !ok {
		return fmt.Errorf("klz: invalid kapiLocalizationFormat %q", v)
	}
	if major != wantMajor {
		return fmt.Errorf("klz: unsupported manifest major version %d (this build speaks %s)", major, ManifestVersion)
	}
	return nil
}

func splitVersion(v string) (major, minor int, ok bool) {
	if v == "" {
		return 0, 0, false
	}
	dot := -1
	for i, r := range v {
		if r == '.' {
			dot = i
			break
		}
	}
	if dot < 0 {
		return 0, 0, false
	}
	majStr, minStr := v[:dot], v[dot+1:]
	for _, r := range majStr {
		if r < '0' || r > '9' {
			return 0, 0, false
		}
		major = major*10 + int(r-'0')
	}
	for _, r := range minStr {
		if r < '0' || r > '9' {
			return 0, 0, false
		}
		minor = minor*10 + int(r-'0')
	}
	return major, minor, true
}

// FindPart returns the manifest entry for a given path, or nil if
// not present.
func (m *Manifest) FindPart(path string) *ManifestPartInfo {
	for i := range m.Parts {
		if m.Parts[i].Path == path {
			return &m.Parts[i]
		}
	}
	return nil
}

// PartsByRole returns the entries whose Role matches r, preserving
// manifest order.
func (m *Manifest) PartsByRole(r PartRole) []ManifestPartInfo {
	var out []ManifestPartInfo
	for _, p := range m.Parts {
		if p.Role == r {
			out = append(out, p)
		}
	}
	return out
}
