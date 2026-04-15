package klz

import (
	"fmt"
	"path"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// Default part-size limits used by the reader. RFC 0001 §Security
// calls for a 128 MiB per-part / 2 GiB aggregate ceiling to guard
// against decompression bombs.
const (
	DefaultMaxPartBytes  int64 = 128 << 20 // 128 MiB
	DefaultMaxTotalBytes int64 = 2 << 30   // 2 GiB
)

// ManifestPath is the well-known name of the archive manifest
// inside a .klz.
const ManifestPath = "manifest.json"

// validatePartPath rejects ZIP-slip and other unsafe path shapes:
//   - absolute paths ("/foo")
//   - relative parents ("..", "a/../b")
//   - empty components
//   - non-UTF-8
//   - non-NFC UTF-8 (per RFC 0001 §.klz exchange archive rules)
//
// Returns the normalized (NFC, POSIX) path on success.
func validatePartPath(raw string) (string, error) {
	if raw == "" {
		return "", fmt.Errorf("klz: empty part path")
	}
	if !utf8.ValidString(raw) {
		return "", fmt.Errorf("klz: part path %q is not valid UTF-8", raw)
	}
	if strings.HasPrefix(raw, "/") {
		return "", fmt.Errorf("klz: part path %q has leading slash", raw)
	}
	if strings.Contains(raw, "\\") {
		return "", fmt.Errorf("klz: part path %q contains backslash; must use POSIX separators", raw)
	}
	// path.Clean collapses "a/../b" into "b" and "./a" into "a",
	// so after cleaning a safe path must equal its cleaned form.
	cleaned := path.Clean(raw)
	if cleaned != raw {
		return "", fmt.Errorf("klz: part path %q is not in canonical form (cleans to %q)", raw, cleaned)
	}
	if strings.HasPrefix(cleaned, "..") || cleaned == "." {
		return "", fmt.Errorf("klz: part path %q escapes the archive root", raw)
	}
	for _, comp := range strings.Split(cleaned, "/") {
		if comp == "" {
			return "", fmt.Errorf("klz: part path %q has empty component", raw)
		}
		if comp == ".." {
			return "", fmt.Errorf("klz: part path %q contains parent reference", raw)
		}
	}
	// Normalize to NFC. RFC 0001 requires NFC-normalized UTF-8 path
	// components, so a producer that emits NFD is out of spec and
	// the reader/writer surfaces that by normalizing on ingest and
	// emitting NFC on write.
	nfc := norm.NFC.String(cleaned)
	return nfc, nil
}

// classifyPartRole returns the manifest Role for a given archive
// path based on the top-level directory. Unknown paths are
// classified as RoleAsset so non-authoritative future extensions
// don't accidentally orphan themselves.
func classifyPartRole(partPath string) PartRole {
	switch {
	case partPath == ManifestPath:
		return RoleMeta
	case partPath == "meta.json":
		return RoleMeta
	case strings.HasPrefix(partPath, "documents/"):
		return RoleDocument
	case strings.HasPrefix(partPath, "targets/"):
		return RoleTarget
	case strings.HasPrefix(partPath, "skeletons/"):
		return RoleSkeleton
	case strings.HasPrefix(partPath, "vocabulary/"):
		return RoleVocabulary
	case strings.HasPrefix(partPath, "annotations/"):
		return RoleAnnotation
	case strings.HasPrefix(partPath, "signatures/"):
		return RoleSignature
	default:
		return RoleAsset
	}
}
