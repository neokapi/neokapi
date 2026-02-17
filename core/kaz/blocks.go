package kaz

import (
	"encoding/json"
	"fmt"
	"io"
)

// BlockIndex stores all block and structural data for one item.
// It enables O(1) random access to blocks and reconstruction without the original source.
type BlockIndex struct {
	Version        string      `json:"kat_version"`
	SourceLocale   string      `json:"source_locale"`
	OriginalFormat string      `json:"original_format"`
	OriginalItem   string      `json:"original_item"`
	Blocks         []Block     `json:"blocks"`
	DataParts      []DataPart  `json:"data_parts"`
	DocumentOrder  []string    `json:"document_order"` // "type:id" references
	Layers         []LayerInfo `json:"layers"`
}

// Block is a translatable content unit within a BlockIndex.
type Block struct {
	ID           string            `json:"id"`
	Index        int               `json:"index"`
	Translatable bool              `json:"translatable"`
	Source       string            `json:"source"`
	SourceHTML   string            `json:"source_html"`
	Targets      map[string]string `json:"targets"`
	Skeleton     *SkeletonData     `json:"skeleton,omitempty"`
	Properties   map[string]string `json:"properties"`
}

// DataPart is a non-translatable structural part.
type DataPart struct {
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	Skeleton   *SkeletonData     `json:"skeleton,omitempty"`
	Properties map[string]string `json:"properties"`
}

// LayerInfo stores layer metadata for document reconstruction.
type LayerInfo struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Format   string `json:"format"`
	Locale   string `json:"locale"`
	Encoding string `json:"encoding"`
}

// SkeletonData serializes skeleton information for a block or data part.
type SkeletonData struct {
	Strategy string             `json:"strategy"` // "fragment" or "reparse"
	Parts    []SkeletonPartData `json:"parts,omitempty"`
}

// SkeletonPartData is a single part within a skeleton.
type SkeletonPartData struct {
	Type       string `json:"type"` // "text" or "ref"
	Text       string `json:"text,omitempty"`
	ResourceID string `json:"resource_id,omitempty"`
	Property   string `json:"property,omitempty"`
}

// WriteBlockIndex serializes a BlockIndex as JSON to the writer.
func WriteBlockIndex(w io.Writer, index *BlockIndex) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(index); err != nil {
		return fmt.Errorf("write block index: %w", err)
	}
	return nil
}

// ReadBlockIndex deserializes a BlockIndex from JSON.
func ReadBlockIndex(r io.Reader) (*BlockIndex, error) {
	var index BlockIndex
	if err := json.NewDecoder(r).Decode(&index); err != nil {
		return nil, fmt.Errorf("read block index: %w", err)
	}
	return &index, nil
}
