package kaz

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/gokapi/gokapi/core/model"
	"gopkg.in/yaml.v3"
)

// StoreManifest describes the manifest for a store-based KAZ export.
type StoreManifest struct {
	FormatVersion string   `yaml:"format_version"` // "2.0" for store exports
	GokapiVersion string   `yaml:"gokapi_version"`
	ProjectID     string   `yaml:"project_id"`
	ProjectName   string   `yaml:"project_name"`
	SourceLocale  string   `yaml:"source_locale"`
	TargetLocales []string `yaml:"target_locales"`
	BlockCount    int      `yaml:"block_count"`
	VersionLabel  string   `yaml:"version_label,omitempty"`
	CreatedAt     string   `yaml:"created_at"`
}

// ExportBlock is the serialized form of a block in a store KAZ export.
type ExportBlock struct {
	ID          string            `json:"id"`
	Name        string            `json:"name,omitempty"`
	Type        string            `json:"type,omitempty"`
	Source      string            `json:"source"`
	Targets     map[string]string `json:"targets,omitempty"`
	ContentHash string            `json:"content_hash,omitempty"`
	ContextHash string            `json:"context_hash,omitempty"`
	Properties  map[string]string `json:"properties,omitempty"`
	ConnectorID string            `json:"connector_id,omitempty"`
	ExternalID  string            `json:"external_id,omitempty"`
}

// ExportVersion is the serialized form of a version snapshot.
type ExportVersion struct {
	ID          string   `json:"id"`
	Label       string   `json:"label"`
	Description string   `json:"description,omitempty"`
	BlockIDs    []string `json:"block_ids"`
	CreatedAt   string   `json:"created_at"`
}

// ConnectorMeta stores connector configuration in a KAZ export.
type ConnectorMeta struct {
	ID       string            `json:"id"`
	Type     string            `json:"type"`
	Name     string            `json:"name"`
	Category string            `json:"category"`
	Config   map[string]string `json:"config,omitempty"`
}

// StoreExportOptions configures a store-based KAZ export.
type StoreExportOptions struct {
	ProjectID     string
	ProjectName   string
	SourceLocale  string
	TargetLocales []string
	Blocks        []ExportBlock
	Versions      []ExportVersion
	Connectors    []ConnectorMeta
	TMData        []byte // JSON-encoded TM entries
	TermsData     []byte // JSON-encoded term concepts
	VersionLabel  string // Current version label
}

// StorePackage is the in-memory representation of a store-based KAZ package.
type StorePackage struct {
	Manifest   StoreManifest
	Blocks     []ExportBlock
	Versions   []ExportVersion
	Connectors []ConnectorMeta
	TMData     []byte
	TermsData  []byte
}

// ExportStore writes a store-based KAZ package to the writer.
func ExportStore(w io.Writer, opts StoreExportOptions) error {
	zw := zip.NewWriter(w)
	defer zw.Close()

	now := time.Now().UTC().Format(time.RFC3339)

	manifest := StoreManifest{
		FormatVersion: "2.0",
		GokapiVersion: "0.8.0",
		ProjectID:     opts.ProjectID,
		ProjectName:   opts.ProjectName,
		SourceLocale:  opts.SourceLocale,
		TargetLocales: opts.TargetLocales,
		BlockCount:    len(opts.Blocks),
		VersionLabel:  opts.VersionLabel,
		CreatedAt:     now,
	}

	// Write manifest
	manifestData, err := yaml.Marshal(&manifest)
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}
	if err := writeZipEntry(zw, "manifest.yaml", manifestData); err != nil {
		return err
	}

	// Write blocks
	blocksData, err := json.MarshalIndent(opts.Blocks, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal blocks: %w", err)
	}
	if err := writeZipEntry(zw, "blocks.json", blocksData); err != nil {
		return err
	}

	// Write versions
	if len(opts.Versions) > 0 {
		versionsData, err := json.MarshalIndent(opts.Versions, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal versions: %w", err)
		}
		if err := writeZipEntry(zw, "versions.json", versionsData); err != nil {
			return err
		}
	}

	// Write connectors
	if len(opts.Connectors) > 0 {
		connData, err := json.MarshalIndent(opts.Connectors, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal connectors: %w", err)
		}
		if err := writeZipEntry(zw, "connectors.json", connData); err != nil {
			return err
		}
	}

	// Write TM data
	if len(opts.TMData) > 0 {
		if err := writeZipEntry(zw, "tm/entries.json", opts.TMData); err != nil {
			return err
		}
	}

	// Write terms data
	if len(opts.TermsData) > 0 {
		if err := writeZipEntry(zw, "terms/concepts.json", opts.TermsData); err != nil {
			return err
		}
	}

	return nil
}

// ImportStore reads a store-based KAZ package.
func ImportStore(r io.ReaderAt, size int64) (*StorePackage, error) {
	zr, err := zip.NewReader(r, size)
	if err != nil {
		return nil, fmt.Errorf("open zip: %w", err)
	}

	pkg := &StorePackage{}
	var manifestData []byte

	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			return nil, fmt.Errorf("open entry %q: %w", f.Name, err)
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return nil, fmt.Errorf("read entry %q: %w", f.Name, err)
		}

		switch f.Name {
		case "manifest.yaml":
			manifestData = data
		case "blocks.json":
			if err := json.Unmarshal(data, &pkg.Blocks); err != nil {
				return nil, fmt.Errorf("parse blocks: %w", err)
			}
		case "versions.json":
			if err := json.Unmarshal(data, &pkg.Versions); err != nil {
				return nil, fmt.Errorf("parse versions: %w", err)
			}
		case "connectors.json":
			if err := json.Unmarshal(data, &pkg.Connectors); err != nil {
				return nil, fmt.Errorf("parse connectors: %w", err)
			}
		case "tm/entries.json":
			pkg.TMData = data
		case "terms/concepts.json":
			pkg.TermsData = data
		}
	}

	if manifestData == nil {
		return nil, fmt.Errorf("manifest.yaml not found in package")
	}
	if err := yaml.Unmarshal(manifestData, &pkg.Manifest); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}
	if pkg.Manifest.FormatVersion != "2.0" {
		return nil, fmt.Errorf("unsupported KAZ format version %q (expected 2.0)", pkg.Manifest.FormatVersion)
	}

	return pkg, nil
}

// ImportStoreFromBytes is a convenience wrapper that accepts a byte slice.
func ImportStoreFromBytes(data []byte) (*StorePackage, error) {
	return ImportStore(bytes.NewReader(data), int64(len(data)))
}

// BlockToExport converts a model.Block into an ExportBlock.
func BlockToExport(b *model.Block) ExportBlock {
	eb := ExportBlock{
		ID:     b.ID,
		Name:   b.Name,
		Type:   b.Type,
		Source: b.SourceText(),
	}

	// Targets
	targets := make(map[string]string)
	for locale := range b.Targets {
		text := b.TargetText(locale)
		if text != "" {
			targets[string(locale)] = text
		}
	}
	if len(targets) > 0 {
		eb.Targets = targets
	}

	// Identity
	if b.Identity != nil {
		eb.ContentHash = b.Identity.ContentHash
		eb.ContextHash = b.Identity.ContextHash
	}

	// Properties
	if len(b.Properties) > 0 {
		eb.Properties = b.Properties
	}

	// ContentRef
	if b.ContentRef != nil {
		eb.ConnectorID = b.ContentRef.ConnectorID
		eb.ExternalID = b.ContentRef.ExternalID
	}

	return eb
}

// ExportToBlock converts an ExportBlock back to a model.Block.
func ExportToBlock(eb ExportBlock) *model.Block {
	b := model.NewBlock(eb.ID, eb.Source)
	b.Name = eb.Name
	b.Type = eb.Type
	b.Properties = eb.Properties

	// Targets
	for locale, text := range eb.Targets {
		b.SetTargetText(model.LocaleID(locale), text)
	}

	// Identity
	if eb.ContentHash != "" || eb.ContextHash != "" {
		b.Identity = &model.BlockIdentity{
			ContentHash: eb.ContentHash,
			ContextHash: eb.ContextHash,
		}
	}

	// ContentRef
	if eb.ConnectorID != "" {
		b.ContentRef = &model.ContentRef{
			ConnectorID: eb.ConnectorID,
			ExternalID:  eb.ExternalID,
		}
	}

	return b
}

func writeZipEntry(zw *zip.Writer, name string, data []byte) error {
	w, err := zw.Create(name)
	if err != nil {
		return fmt.Errorf("create %s: %w", name, err)
	}
	if _, err := w.Write(data); err != nil {
		return fmt.Errorf("write %s: %w", name, err)
	}
	return nil
}
