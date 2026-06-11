package project

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/neokapi/neokapi/core/model"
	"gopkg.in/yaml.v3"
)

// ExtractionsDirName is the subdirectory of CacheDir() that holds per-batch
// extraction state (AD-017). The full path is .kapi/cache/extractions/.
const ExtractionsDirName = "extractions"

// CollectionsDirName is the subdirectory of CacheDir() that holds overlay
// layers per content collection. The full path is .kapi/cache/collections/.
const CollectionsDirName = "collections"

// ExtractionManifestFilename is the manifest filename inside each
// <batch-id>/ directory.
const ExtractionManifestFilename = "manifest.yaml"

// ExtractionManifestKind tags the YAML manifest so tooling can verify the
// document shape at load time.
const ExtractionManifestKind = "kapi-extraction"

// ExtractionSchemaVersion is the current manifest schema version.
const ExtractionSchemaVersion = 1

// ExtractionFormatXLIFF2 is the canonical format tag for XLIFF 2.x
// manifest entries. PO / other formats can be added as the extract
// surface widens.
const ExtractionFormatXLIFF2 = "xliff2"

// ExtractionFormatPO tags PO (gettext) manifest entries.
const ExtractionFormatPO = "po"

// ExtractionManifest records one `kapi extract` invocation so `kapi merge`
// can resolve a returning bilingual file back to its source file,
// skeleton, and staleness fingerprint (AD-017, AD-008).
type ExtractionManifest struct {
	SchemaVersion int                     `yaml:"schemaVersion" json:"schemaVersion"`
	Kind          string                  `yaml:"kind" json:"kind"`
	BatchID       string                  `yaml:"batchId" json:"batchId"`
	Generator     ExtractionGenerator     `yaml:"generator" json:"generator"`
	CreatedAt     string                  `yaml:"createdAt" json:"createdAt"`
	SourceLocale  model.LocaleID          `yaml:"sourceLocale,omitempty" json:"sourceLocale,omitempty"`
	Options       ExtractionOptions       `yaml:"options,omitempty" json:"options,omitzero"`
	Pairs         []ExtractionPair        `yaml:"pairs" json:"pairs"`
	Totals        ExtractionLeverageStats `yaml:"totals,omitempty" json:"totals,omitzero"`
}

// ExtractionGenerator records the kapi version that wrote the manifest,
// so tooling can detect forward-compat issues before parsing.
type ExtractionGenerator struct {
	ID      string `yaml:"id" json:"id"`
	Version string `yaml:"version" json:"version"`
}

// ExtractionOptions capture the CLI flags that shaped a batch so the run
// can be reproduced or explained after the fact.
type ExtractionOptions struct {
	Format       string `yaml:"format,omitempty" json:"format,omitempty"`
	XLIFFVersion string `yaml:"xliffVersion,omitempty" json:"xliffVersion,omitempty"`
	NoTM         bool   `yaml:"noTM,omitempty" json:"noTM,omitempty"`
	Only         string `yaml:"only,omitempty" json:"only,omitempty"`
	Pattern      string `yaml:"pattern,omitempty" json:"pattern,omitempty"`
	Segmentation bool   `yaml:"segmentation,omitempty" json:"segmentation,omitempty"`
}

// ExtractionPair groups one target locale with the set of bilingual
// files emitted for it in this batch.
type ExtractionPair struct {
	TargetLocale model.LocaleID   `yaml:"targetLocale" json:"targetLocale"`
	Output       string           `yaml:"output" json:"output"` // relative to project root
	Files        []ExtractionFile `yaml:"files" json:"files"`
}

// ExtractionFile records a single source → bilingual-output mapping
// inside a pair.
type ExtractionFile struct {
	Source     string                  `yaml:"source" json:"source"`         // path relative to project root
	SourceHash string                  `yaml:"sourceHash" json:"sourceHash"` // sha256:hex at extract time
	Format     string                  `yaml:"format,omitempty" json:"format,omitempty"`
	Blocks     int                     `yaml:"blocks" json:"blocks"`
	Segments   int                     `yaml:"segments" json:"segments"`
	Leverage   ExtractionLeverageStats `yaml:"leverage,omitempty" json:"leverage,omitzero"`
	Skeleton   string                  `yaml:"skeleton,omitempty" json:"skeleton,omitempty"` // filename inside the batch dir
}

// ExtractionLeverageStats summarize TM pre-fill outcomes.
type ExtractionLeverageStats struct {
	Exact int `yaml:"exact" json:"exact"`
	Fuzzy int `yaml:"fuzzy" json:"fuzzy"`
	New   int `yaml:"new" json:"new"`
}

// Total returns Exact + Fuzzy + New.
func (s ExtractionLeverageStats) Total() int { return s.Exact + s.Fuzzy + s.New }

// Add accumulates other into s.
func (s *ExtractionLeverageStats) Add(other ExtractionLeverageStats) {
	s.Exact += other.Exact
	s.Fuzzy += other.Fuzzy
	s.New += other.New
}

// ExtractionsRoot returns the directory that holds every extraction batch
// for the project (.kapi/cache/extractions/).
func ExtractionsRoot(layout Layout) string {
	return layout.ExtractionsDir()
}

// ExtractionDir returns the per-batch directory for the given id.
func ExtractionDir(layout Layout, batchID string) string {
	return filepath.Join(ExtractionsRoot(layout), batchID)
}

// EnsureExtractionDir creates the batch directory idempotently.
func EnsureExtractionDir(layout Layout, batchID string) (string, error) {
	dir := ExtractionDir(layout, batchID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("project: create extraction dir: %w", err)
	}
	return dir, nil
}

// SkeletonFilename returns the per-source skeleton filename inside an
// extraction directory. Keyed by content hash so multiple extractions of
// the same source share one skeleton.
func SkeletonFilename(sourceHash string) string {
	return "skel-" + sourceHash + ".bin"
}

// SaveExtractionManifest writes m.yaml to the extraction directory.
// The batch dir is created if it does not exist.
func SaveExtractionManifest(layout Layout, m *ExtractionManifest) error {
	if m.SchemaVersion == 0 {
		m.SchemaVersion = ExtractionSchemaVersion
	}
	if m.Kind == "" {
		m.Kind = ExtractionManifestKind
	}
	dir, err := EnsureExtractionDir(layout, m.BatchID)
	if err != nil {
		return err
	}
	if m.CreatedAt == "" {
		m.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(m); err != nil {
		return fmt.Errorf("project: marshal extraction manifest: %w", err)
	}
	_ = enc.Close()

	path := filepath.Join(dir, ExtractionManifestFilename)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, buf.Bytes(), 0o644); err != nil {
		return fmt.Errorf("project: write extraction manifest: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("project: rename extraction manifest: %w", err)
	}
	return nil
}

// LoadExtractionManifest reads a manifest from the given batch directory.
func LoadExtractionManifest(layout Layout, batchID string) (*ExtractionManifest, error) {
	path := filepath.Join(ExtractionDir(layout, batchID), ExtractionManifestFilename)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("project: read extraction manifest: %w", err)
	}
	var m ExtractionManifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("project: parse extraction manifest %s: %w", path, err)
	}
	if m.Kind != "" && m.Kind != ExtractionManifestKind {
		return nil, fmt.Errorf("project: extraction manifest %s: unexpected kind %q", path, m.Kind)
	}
	return &m, nil
}

// ListExtractionManifests returns every extraction batch known to the
// project, loaded and ready to consult. Errors on any one batch abort
// the listing — merge must never silently skip a candidate manifest.
func ListExtractionManifests(layout Layout) ([]*ExtractionManifest, error) {
	root := ExtractionsRoot(layout)
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("project: read extractions dir: %w", err)
	}
	var out []*ExtractionManifest
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		m, err := LoadExtractionManifest(layout, e.Name())
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, nil
}

// HashFile returns the sha256:hex digest of the file at path — used both
// as the skeleton filename key and as the staleness fingerprint stamped
// on emitted bilingual files.
func HashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}

// HashBytes returns the sha256:hex digest of the given bytes.
func HashBytes(b []byte) string {
	h := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(h[:])
}
